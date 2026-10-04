//go:build cachemetrics

package decoder

import "testing"

type stringCacheBenchmarkMetrics struct {
	hits       uint64
	misses     uint64
	admissions uint64
}

func startStringCacheBenchmarkMetrics() stringCacheBenchmarkMetrics {
	return stringCacheBenchmarkMetrics{
		hits:       stringCacheBenchHits.Load(),
		misses:     stringCacheBenchMisses.Load(),
		admissions: stringCacheBenchAdmissions.Load(),
	}
}

func (before stringCacheBenchmarkMetrics) report(b *testing.B) {
	b.Helper()
	after := startStringCacheBenchmarkMetrics()
	reads := float64(b.N)
	b.ReportMetric(float64(after.hits-before.hits)/reads, "hits/op")
	b.ReportMetric(float64(after.misses-before.misses)/reads, "misses/op")
	b.ReportMetric(float64(after.admissions-before.admissions)/reads, "admits/op")
	// Counter contention is not part of the production cache.
	b.ReportMetric(0, "ns/op")
}
