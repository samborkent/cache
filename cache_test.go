package cache_test

import (
	cryptorand "crypto/rand"
	mathrand "math/rand/v2"
	"runtime"
	"testing"

	"github.com/samborkent/cache"
	"github.com/samborkent/check"
)

type Object struct {
	Field1 string
	Field2 int
}

func TestCache(t *testing.T) {
	t.Parallel()

	store := cache.NewCache[string, Object](0, 0)

	object := &Object{
		Field1: cryptorand.Text(),
		Field2: mathrand.Int(),
	}

	key := cryptorand.Text()

	store.Put(key, object)

	value, ok := store.Get(key)
	check.True(t, ok)
	check.Equal(t, value, *object)

	object2 := &Object{
		Field1: cryptorand.Text(),
		Field2: mathrand.Int(),
	}

	key2 := cryptorand.Text()

	store.Put(key2, object2)

	runtime.GC()
	runtime.KeepAlive(object2)

	value, ok = store.Get(key)
	check.True(t, !ok)
	check.AnyZero(t, value)

	value, ok = store.Get(key2)
	check.True(t, ok)
	check.Equal(t, value, *object2)
}
