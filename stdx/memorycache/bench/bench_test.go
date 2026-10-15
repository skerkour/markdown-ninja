package bench

import (
	"fmt"
	"testing"
	"time"

	"markdown.ninja/stdx/memorycache"
)

func BenchmarkCacheSetWithoutTTL(b *testing.B) {
	cache := memorycache.New[string, string]()

	for n := 0; n < b.N; n++ {
		cache.Set(fmt.Sprint(n%1000000), "value", memorycache.NoTTL)
	}
}

func BenchmarkCacheSetWithGlobalTTL(b *testing.B) {
	cache := memorycache.New[string, string](
		memorycache.WithTTL[string, string](50 * time.Millisecond),
	)

	for n := 0; n < b.N; n++ {
		cache.Set(fmt.Sprint(n%1000000), "value", memorycache.DefaultTTL)
	}
}
