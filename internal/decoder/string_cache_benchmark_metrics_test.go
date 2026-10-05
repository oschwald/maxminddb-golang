//go:build !cachemetrics

package decoder

import "testing"

type stringCacheBenchmarkMetrics struct{}

func startStringCacheBenchmarkMetrics() stringCacheBenchmarkMetrics {
	return stringCacheBenchmarkMetrics{}
}

func (stringCacheBenchmarkMetrics) report(*testing.B) {}
