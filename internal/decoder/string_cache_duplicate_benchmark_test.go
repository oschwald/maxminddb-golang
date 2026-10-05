//go:build !cachemetrics

package decoder

import (
	"runtime"
	"testing"
)

// These benchmarks force a miss for an already resident entry and include the
// miss-token store in each timed operation. They measure late-duplicate handling,
// not full lookups or how often production readers take this path. They exclude
// cachemetrics because calling miss directly does not produce lookup hit rates.
func BenchmarkStringCacheForcedLateDuplicate(b *testing.B) {
	table, bucket, value := newLateDuplicateCache()
	b.ReportAllocs()
	for b.Loop() {
		table.recentMiss(0).Store(uint64(stringCacheAdmissionValue(0)))
		benchmarkStringCacheSink = table.miss(bucket, 0, value)
	}
}

// All workers force misses through one bucket and one miss-history word.
// This deliberately concentrates contention beyond ordinary warmed lookups.
func BenchmarkStringCacheForcedLateDuplicateParallel(b *testing.B) {
	table, bucket, value := newLateDuplicateCache()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var valueRead string
		for pb.Next() {
			table.recentMiss(0).Store(uint64(stringCacheAdmissionValue(0)))
			valueRead = table.miss(bucket, 0, value)
		}
		runtime.KeepAlive(valueRead)
	})
}
