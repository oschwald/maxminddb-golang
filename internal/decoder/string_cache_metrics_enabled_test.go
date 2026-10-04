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
	admissionsBefore := stringCacheBenchAdmissions.Load()
	for range 3 {
		cache.internAt(42, value)
	}
	for _, ineligible := range [][]byte{nil, {'x'}, make([]byte, 101)} {
		cache.internAt(99, ineligible)
	}
	hitsAfter, missesAfter := StringCacheBenchmarkStats()
	require.Equal(t, uint64(1), hitsAfter-hitsBefore)
	require.Equal(t, uint64(2), missesAfter-missesBefore)
	require.Equal(t, uint64(1), stringCacheBenchAdmissions.Load()-admissionsBefore)
}

func TestStringCacheMetricsWriterContention(t *testing.T) {
	cache := newStringCache()
	const offset = 42
	internTestValue(cache, offset)
	bucket := cache.bucket(offset)
	bucket.control.Store(stringCacheWriting)

	before := startStringCacheBenchmarkMetrics()
	internTestValue(cache, offset)
	after := startStringCacheBenchmarkMetrics()
	require.Equal(t, before.hits, after.hits)
	require.Equal(t, before.misses+1, after.misses)
	require.Equal(t, before.admissions, after.admissions,
		"a contended publisher must not count an admission")

	bucket.control.Store(0)
	internTestValue(cache, offset)
	require.Equal(t, before.admissions+1, stringCacheBenchAdmissions.Load(),
		"retrying after the writer releases the bucket must count one admission")
}

func TestStringCacheMetricsDuplicatePublication(t *testing.T) {
	cache := newStringCache()
	const offset = 42
	for range 2 {
		internTestValue(cache, offset)
	}

	before := startStringCacheBenchmarkMetrics()
	// These callers missed before another caller published the same offset.
	for range 2 {
		cache.miss(cache.bucket(offset), offset, stringCacheTestValue(offset))
	}
	after := startStringCacheBenchmarkMetrics()
	require.Equal(t, before.hits, after.hits)
	require.Equal(t, before.misses+2, after.misses)
	require.Equal(t, before.admissions, after.admissions,
		"finding an entry published by another caller must not count another admission")
}

func TestStringCacheMetricsReplacement(t *testing.T) {
	cache := newStringCache()
	offsets := stringCacheTestOffsets(t, stringCacheBucketSlots+1, false, sharesFirstBucket)
	for _, offset := range offsets[:stringCacheBucketSlots] {
		for range 2 {
			internTestValue(cache, offset)
		}
	}

	before := startStringCacheBenchmarkMetrics()
	offset := offsets[stringCacheBucketSlots]
	// Record, age the full bucket, replace an entry, then hit the replacement.
	for range 4 {
		internTestValue(cache, offset)
	}
	after := startStringCacheBenchmarkMetrics()
	require.Equal(t, before.hits+1, after.hits)
	require.Equal(t, before.misses+3, after.misses)
	require.Equal(t, before.admissions+1, after.admissions,
		"aging a bucket must not count as an admission")
}
