package cache

import (
	"hash/maphash"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
	"weak"
)

type LockFreeCache[K comparable, V any] struct {
	entries        []atomic.Pointer[cacheEntry[V]]
	pool           sync.Pool
	seed           maphash.Seed
	size           int
	hashProbeDepth int
	initialized    atomic.Bool
	rng            atomic.Pointer[rand.PCG]
}

type cacheEntry[V any] struct {
	keyHash  uint64
	valueRef weak.Pointer[V]
}

func NewLockFreeCache[K comparable, V any](size int) *LockFreeCache[K, V] {
	if size <= 0 {
		return &LockFreeCache[K, V]{}
	}

	lockFreeCache := &LockFreeCache[K, V]{
		entries: make([]atomic.Pointer[cacheEntry[V]], size),
		pool: sync.Pool{
			New: func() any {
				return any(&cacheEntry[V]{})
			},
		},
		seed:           maphash.MakeSeed(),
		size:           size,
		hashProbeDepth: min(size%4, 1),
	}

	seed := uint64(time.Now().UnixNano())

	lockFreeCache.rng.Store(rand.NewPCG(seed, uint64(uintptr(unsafe.Pointer(lockFreeCache)))^seed))
	lockFreeCache.initialized.Store(true)

	return lockFreeCache
}

func (c *LockFreeCache[K, V]) Put(key K, value *V) {
	if !c.initialized.Load() {
		return
	}

	keyHash := maphash.Comparable(c.seed, key)

	// Get cache entry from pool.
	newEntry, _ := c.pool.Get().(*cacheEntry[V])
	newEntry.keyHash = keyHash
	newEntry.valueRef = weak.Make(value)

	// Try to replace existing entry up to hash probe depth.
	for i := range c.hashProbeDepth {
		// Use key hash probing.
		index := int((keyHash + uint64(i)) % uint64(c.size))

		entry := c.entries[index].Load()
		if entry != nil && entry.keyHash == keyHash {
			// Found same key hash.
			if c.entries[index].CompareAndSwap(entry, newEntry) {
				// Same key was swapped, exit.
				return
			}
		}
	}

	// Try to reclaim empty cache slot.
	for i := range c.size {
		// Use key hash probing.
		index := int((keyHash + uint64(i)) % uint64(c.size))

		entry := c.entries[index].Load()
		if entry == nil || entry.keyHash == keyHash ||
			entry.keyHash == 0 || entry.valueRef.Value() == nil {
			// Empty slot was found.
			if c.entries[i].CompareAndSwap(entry, newEntry) {
				// Empty slot was claimed, exit.
				return
			}
		}
	}

	// Overwrite random cache slot.
	for {
		randomIndex := int(c.rng.Load().Uint64() % uint64(c.size))

		if c.entries[randomIndex].CompareAndSwap(c.entries[randomIndex].Load(), newEntry) {
			return
		}
	}
}

func (c *LockFreeCache[K, V]) Get(key K) (V, bool) {
	if !c.initialized.Load() {
		// LockFreeCache was not initialized.
		return *new(V), false
	}

	keyHash := maphash.Comparable(c.seed, key)

	for i := range c.size {
		// Use key hash probing.
		index := int((keyHash + uint64(i)) % uint64(c.size))

		entry := c.entries[index].Load()
		if entry == nil || entry.keyHash == 0 {
			continue
		}

		if entry.valueRef.Value() == nil {
			c.invalidate(entry, index)
			continue
		}

		// Found entry, return value if still valid.
		if entry.keyHash == keyHash {
			if value := entry.valueRef.Value(); value != nil {
				return *value, true
			}

			c.invalidate(entry, index)

			break
		}
	}

	return *new(V), false
}

func (c *LockFreeCache[K, V]) invalidate(entry *cacheEntry[V], index int) {
	entry.keyHash = 0

	// Invalidate cache entry if underlying value was cleaned up by garbage collector.
	if c.entries[index].CompareAndSwap(entry, entry) {
		// Add invalidated cache entry back to the pool.
		c.pool.Put(any(entry))
	}
}

func (c *LockFreeCache[K, V]) Len() int {
	count := 0

	for i := range c.size {
		entry := c.entries[i].Load()
		if entry != nil && entry.valueRef.Value() != nil {
			count++
		}
	}

	return count
}

func (c *LockFreeCache[K, V]) Cap() int {
	return c.size
}
