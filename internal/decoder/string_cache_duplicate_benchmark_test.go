//go:build !cachemetrics

package decoder

import "testing"

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
