//go:build cachemetrics

package maxminddb

import (
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/internal/decoder"
)

// BenchmarkRealCityCacheHitRate reports eligible-string hits and misses for
// the real City workload. Run with -tags cachemetrics; timing is suppressed.
func BenchmarkRealCityCacheHitRate(b *testing.B) {
	benchmarkRealCityCache(b, decoder.StringCacheBenchmarkStats)
}
