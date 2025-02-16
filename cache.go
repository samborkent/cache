package cache

import (
	"hash/maphash"
	"math/rand/v2"
	"runtime"
	"slices"
	"sync"
	"weak"
)

// Cache is a data structure which keeps weak references to values.
// This means that the cache will be automatically cleaned up by the garbage collector.
// The cache can only grow. However, it will reuse already claimed memory,
// once underlying values are cleaned up.
// You can also specify a max cache size, once this size is reached and a new cache entry is put,
// a random cache entry will be overwritten.
type Cache[K comparable, V any] struct {
	keyHashes   []uint64
	values      []weak.Pointer[V]
	seed        maphash.Seed
	lock        sync.RWMutex
	maxSize     int
	initialized bool
}

func NewCache[K comparable, V any](initialSize, maxSize int) *Cache[K, V] {
	return &Cache[K, V]{
		keyHashes:   make([]uint64, 0, initialSize),
		values:      make([]weak.Pointer[V], 0, initialSize),
		seed:        maphash.MakeSeed(),
		maxSize:     maxSize,
		initialized: true,
	}
}

func (c *Cache[K, V]) Put(key K, value *V) {
	if !c.initialized {
		return
	}

	keyHash := maphash.Comparable(c.seed, key)
	valueRef := weak.Make(value)

	c.lock.Lock()
	defer c.lock.Unlock()

	// Find key hash in cache.
	index := slices.Index(c.keyHashes, keyHash)
	if index == -1 {
		// Key hash does not exist yet.
		// Check if there are any zero values.
		zeroIndex := slices.Index(c.keyHashes, 0)
		if zeroIndex == -1 {
			// No zero value found
			if c.maxSize != 0 && len(c.keyHashes) >= c.maxSize {
				// The cache has reached its maximum size, generate a random index.
				index := rand.IntN(len(c.keyHashes))

				// Overwrite random cache entry.
				c.keyHashes[index] = keyHash
				c.values[index] = valueRef

				runtime.AddCleanup(value, c.invalidate, index)

				return
			}

			// Grow cache and append hash/value at the end.
			c.keyHashes = append(c.keyHashes, keyHash)
			c.values = append(c.values, valueRef)

			runtime.AddCleanup(value, c.invalidate, len(c.keyHashes)-1)

			return
		}

		// A zero value was found, overwrite.
		c.keyHashes[zeroIndex] = keyHash
		c.values[zeroIndex] = valueRef

		runtime.AddCleanup(value, c.invalidate, zeroIndex)

		return
	}

	// Key already exists in cache, overwrite value.
	c.values[index] = valueRef
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	if !c.initialized {
		// Cache was not initialized.
		return *new(V), false
	}

	c.lock.RLock()
	defer c.lock.RUnlock()

	index := slices.Index(c.keyHashes, maphash.Comparable(c.seed, key))
	if index == -1 {
		// Key not found in cache.
		return *new(V), false
	}

	value := c.values[index].Value()
	if value == nil {
		// Zero key hash, so its position in memory can be reused.
		c.lock.Lock()
		c.keyHashes[index] = 0
		c.lock.Unlock()

		// Value pointer was cleaned up by garbage collector.
		return *new(V), false
	}

	return *value, true
}

func (c *Cache[K, V]) Len() int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return len(c.keyHashes)
}

func (c *Cache[K, V]) Cap() int {
	return c.maxSize
}

func (c *Cache[K, V]) invalidate(index int) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Check if the value is indeed nil. If not, then the cache value was already overwritten.
	if c.values[index].Value() == nil {
		c.keyHashes[index] = 0
	}
}
