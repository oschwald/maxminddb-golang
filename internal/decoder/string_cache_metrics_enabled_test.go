//go:build cachemetrics

package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringCacheBenchmarkStats(t *testing.T) {
	cache := newStringCache()
	value := []byte("cached value")
	hitsBefore, missesBefore := StringCacheBenchmarkStats()
	for range 3 {
		cache.internAt(42, value)
	}
	hitsAfter, missesAfter := StringCacheBenchmarkStats()
	require.Equal(t, uint64(1), hitsAfter-hitsBefore)
	require.Equal(t, uint64(2), missesAfter-missesBefore)
}
