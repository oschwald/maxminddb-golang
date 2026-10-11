//go:build !cachemetrics

package maxminddb

import "testing"

// BenchmarkRealCityLookup separates traversal from decoding and compares fresh
// results with reused storage. It uses the same traffic as BenchmarkRealCityCache.
func BenchmarkRealCityLookup(b *testing.B) {
	path, addresses := benchmarkRealCityAddresses(b)
	for _, traffic := range []string{"random", "recurring1024"} {
		b.Run(traffic, func(b *testing.B) {
			count := len(addresses)
			if traffic == "recurring1024" {
				count = 1024
			}
			for _, operation := range []string{"lookup", "decode", "lookup-decode"} {
				b.Run(operation, func(b *testing.B) {
					if operation == "lookup" {
						db, err := Open(path)
						if err != nil {
							b.Fatal(err)
						}
						b.Cleanup(func() { _ = db.Close() })
						b.ReportAllocs()
						i := 0
						for b.Loop() {
							if err := db.Lookup(addresses[i]).Err(); err != nil {
								b.Fatal(err)
							}
							i = (i + 1) % count
						}
						return
					}
					for _, storage := range []string{"fresh", "reused"} {
						b.Run(storage, func(b *testing.B) {
							db, err := Open(path)
							if err != nil {
								b.Fatal(err)
							}
							b.Cleanup(func() { _ = db.Close() })
							results := make([]Result, count)
							for i := range results {
								results[i] = db.Lookup(addresses[i])
							}
							var city benchmarkCity
							for range 2 {
								for _, result := range results {
									if err := result.Decode(&city); err != nil {
										b.Fatal(err)
									}
								}
							}
							b.ReportAllocs()
							i := 0
							for b.Loop() {
								result := results[i]
								if operation == "lookup-decode" {
									result = db.Lookup(addresses[i])
								}
								if storage == "fresh" {
									var fresh benchmarkCity
									err = result.Decode(&fresh)
								} else {
									err = result.Decode(&city)
								}
								if err != nil {
									b.Fatal(err)
								}
								i = (i + 1) % count
							}
						})
					}
				})
			}
		})
	}
}
