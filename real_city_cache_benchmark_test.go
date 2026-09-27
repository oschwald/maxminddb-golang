package maxminddb

import (
	"fmt"
	"math/rand/v2"
	"net/netip"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
)

// BenchmarkRealCityCache uses the local GeoIP2-City database. Set
// MAXMIND_REAL_CITY_DB to its path; this benchmark is skipped otherwise.
func BenchmarkRealCityCache(b *testing.B) {
	benchmarkRealCityCache(b, nil)
}

func benchmarkRealCityCache(b *testing.B, metrics func() (uint64, uint64)) {
	path := os.Getenv("MAXMIND_REAL_CITY_DB")
	if path == "" {
		b.Skip("set MAXMIND_REAL_CITY_DB")
	}
	probe, err := Open(path, DisableStringCache())
	if err != nil {
		b.Fatal(err)
	}
	addresses := make([]netip.Addr, 0, 65536)
	rng := rand.New(rand.NewPCG(1, 2))
	for attempts := 0; len(addresses) < cap(addresses) && attempts < 1<<20; attempts++ {
		n := rng.Uint32()
		addr := netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
		var city benchmarkCity
		if err := probe.Lookup(addr).Decode(&city); err != nil {
			b.Fatal(err)
		}
		if city.City.Names.English != "" {
			addresses = append(addresses, addr)
		}
	}
	if len(addresses) != cap(addresses) {
		b.Fatalf("database supplied only %d City-bearing addresses in 1,048,576 random probes; use a full City database", len(addresses))
	}
	if err := probe.Close(); err != nil {
		b.Fatal(err)
	}
	rng.Shuffle(len(addresses), func(i, j int) { addresses[i], addresses[j] = addresses[j], addresses[i] })
	for _, shape := range []string{"city", "iso"} {
		for _, traffic := range []string{"random", "recurring1024"} {
			for _, goroutines := range []int{1, 12} {
				cacheOptions := []bool{true, false}
				if metrics != nil {
					cacheOptions = cacheOptions[:1]
				}
				for _, enabled := range cacheOptions {
					name := fmt.Sprintf("%s/%s/g%d/cache%t", shape, traffic, goroutines, enabled)
					b.Run(name, func(b *testing.B) {
						options := []ReaderOption{}
						if !enabled {
							options = append(options, DisableStringCache())
						}
						db, err := Open(path, options...)
						if err != nil {
							b.Fatal(err)
						}
						defer db.Close()
						previous := runtime.GOMAXPROCS(goroutines)
						defer runtime.GOMAXPROCS(previous)
						count := len(addresses)
						if traffic == "recurring1024" {
							count = 1024
						}
						decode := func(addr netip.Addr, city *benchmarkCity, iso *struct {
							Country struct {
								ISOCode string `maxminddb:"iso_code"`
							} `maxminddb:"country"`
						}) error {
							if shape == "city" {
								return db.Lookup(addr).Decode(city)
							}
							return db.Lookup(addr).Decode(iso)
						}
						var city benchmarkCity
						var iso struct {
							Country struct {
								ISOCode string `maxminddb:"iso_code"`
							} `maxminddb:"country"`
						}
						for pass := 0; pass < 2; pass++ {
							for i := 0; i < count; i++ {
								if err := decode(addresses[i], &city, &iso); err != nil {
									b.Fatal(err)
								}
							}
						}
						var starts atomic.Uint64
						var hitsBefore, missesBefore uint64
						if metrics == nil {
							b.ReportAllocs()
						} else {
							hitsBefore, missesBefore = metrics()
						}
						b.ResetTimer()
						b.RunParallel(func(pb *testing.PB) {
							var localCity benchmarkCity
							var localISO struct {
								Country struct {
									ISOCode string `maxminddb:"iso_code"`
								} `maxminddb:"country"`
							}
							i := int(starts.Add(1)*104729) % count
							for pb.Next() {
								if err := decode(addresses[i], &localCity, &localISO); err != nil {
									b.Error(err)
									return
								}
								i++
								if i == count {
									i = 0
								}
							}
						})
						if metrics != nil {
							b.StopTimer()
							hitsAfter, missesAfter := metrics()
							hits := hitsAfter - hitsBefore
							misses := missesAfter - missesBefore
							if hits+misses == 0 {
								b.Fatal("no cache-eligible strings were decoded")
							}
							b.ReportMetric(100*float64(hits)/float64(hits+misses), "hit_pct")
							b.ReportMetric(float64(hits+misses)/float64(b.N), "strings/op")
							b.ReportMetric(float64(misses)/float64(b.N), "misses/op")
							b.ReportMetric(0, "ns/op")
						}
					})
				}
			}
		}
	}
}
