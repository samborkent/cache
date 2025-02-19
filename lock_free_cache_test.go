package cache_test

import (
	"context"
	cryptorand "crypto/rand"
	mathrand "math/rand/v2"
	"sync"
	"testing"

	"github.com/samborkent/cache"
)

type keyValue struct {
	key   string
	value uint64
}

const N = 100000

func TestLockFreeCache(t *testing.T) {
	testCache := cache.NewLockFreeCache[string, uint64](N / 100)

	// key := cryptorand.Text()
	// val := mathrand.Uint64()

	// testCache.Put(key, &val)

	// value, ok := testCache.Get(key)
	// check.True(t, ok)
	// check.Equal(t, value, val)

	// t.Logf("metrics: %+v", testCache.Metrics())

	keyValues := make(chan keyValue, 1)

	ctx, cancel := context.WithCancel(t.Context())

	// Create key values
	go func() {
		for range N {
			kv := keyValue{
				key:   cryptorand.Text(),
				value: mathrand.Uint64(),
			}

			keyValues <- kv

			if mathrand.IntN(2) == 1 {
				keyValues <- kv
			}
		}

		cancel()
	}()

	var wg sync.WaitGroup

	wg.Add(2)

	// Put key values
	go func() {
		for {
			select {
			case <-ctx.Done():
				wg.Done()
				return
			case keyValue := <-keyValues:
				testCache.Put(keyValue.key, &keyValue.value)

				if mathrand.IntN(2) == 1 {
					keyValues <- keyValue
				}
			}
		}
	}()

	// Get key values
	go func() {
		for {
			select {
			case <-ctx.Done():
				wg.Done()
				return
			case keyValue := <-keyValues:
				value, ok := testCache.Get(keyValue.key)
				if ok && keyValue.value != value {
					t.Errorf("wrong value: got '%d', want '%d'", value, keyValue.value)
				}
			}
		}
	}()

	wg.Wait()

	t.Logf("metrics: %+v", testCache.Metrics())
}
