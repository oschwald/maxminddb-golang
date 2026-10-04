//go:build !cachemetrics

package maxminddb

import "testing"

// BenchmarkRealCityCache uses MAXMIND_REAL_CITY_DB or local GeoLite2-City.mmdb.
// It skips if the default file is absent. An invalid explicit path fails.
func BenchmarkRealCityCache(b *testing.B) {
	benchmarkRealCityCache(b, nil)
}
