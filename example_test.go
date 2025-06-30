package maxminddb_test

import (
	"fmt"
	"log"
	"net/netip"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

// This example shows how to decode to a struct.
func ExampleReader_Lookup_struct() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close() //nolint:errcheck // error doesn't matter

	addr := netip.MustParseAddr("81.2.69.142")

	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	} // Or any appropriate struct

	err = db.Lookup(addr).Decode(&record)
	if err != nil {
		log.Panic(err)
	}
	fmt.Print(record.Country.ISOCode)
	// Output:
	// GB
}

// This example demonstrates how to decode to an any.
func ExampleReader_Lookup_interface() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close() //nolint:errcheck // error doesn't matter

	addr := netip.MustParseAddr("81.2.69.142")

	var record any
	err = db.Lookup(addr).Decode(&record)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("%v", record)
	//nolint:lll
	// Output:
	// map[city:map[geoname_id:2643743 names:map[de:London en:London es:Londres fr:Londres ja:ロンドン pt-BR:Londres ru:Лондон]] continent:map[code:EU geoname_id:6255148 names:map[de:Europa en:Europe es:Europa fr:Europe ja:ヨーロッパ pt-BR:Europa ru:Европа zh-CN:欧洲]] country:map[geoname_id:2635167 iso_code:GB names:map[de:Vereinigtes Königreich en:United Kingdom es:Reino Unido fr:Royaume-Uni ja:イギリス pt-BR:Reino Unido ru:Великобритания zh-CN:英国]] location:map[accuracy_radius:10 latitude:51.5142 longitude:-0.0931 time_zone:Europe/London] registered_country:map[geoname_id:6252001 iso_code:US names:map[de:USA en:United States es:Estados Unidos fr:États-Unis ja:アメリカ合衆国 pt-BR:Estados Unidos ru:США zh-CN:美国]] subdivisions:[map[geoname_id:6269131 iso_code:ENG names:map[en:England es:Inglaterra fr:Angleterre pt-BR:Inglaterra]]]]
}

// This example demonstrates how to iterate over all networks in the
// database.
func ExampleReader_Networks() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-Connection-Type-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close() //nolint:errcheck // error doesn't matter

	for result := range db.Networks() {
		record := struct {
			ConnectionType string `maxminddb:"connection_type"`
		}{}

		err := result.Decode(&record)
		if err != nil {
			log.Panic(err)
		}
		fmt.Printf("%s: %s\n", result.Prefix(), record.ConnectionType)
	}
	// Output:
	// 1.0.0.0/24: Cable/DSL
	// 1.0.1.0/24: Cellular
	// 1.0.2.0/23: Cable/DSL
	// 1.0.4.0/22: Cable/DSL
	// 1.0.8.0/21: Cable/DSL
	// 1.0.16.0/20: Cable/DSL
	// 1.0.32.0/19: Cable/DSL
	// 1.0.64.0/18: Cable/DSL
	// 1.0.128.0/17: Cable/DSL
	// 2.125.160.216/29: Cable/DSL
	// 67.43.156.0/24: Cellular
	// 80.214.0.0/20: Cellular
	// 96.1.0.0/16: Cable/DSL
	// 96.10.0.0/15: Cable/DSL
	// 96.69.0.0/16: Cable/DSL
	// 96.94.0.0/15: Cable/DSL
	// 108.96.0.0/11: Cellular
	// 149.101.100.0/28: Cellular
	// 175.16.199.0/24: Cable/DSL
	// 187.156.138.0/24: Cable/DSL
	// 201.243.200.0/24: Corporate
	// 207.179.48.0/20: Cellular
	// 216.160.83.56/29: Corporate
	// 2003::/24: Cable/DSL
}

// This example demonstrates how to validate a MaxMind DB file and access metadata.
func ExampleReader_Verify() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close() //nolint:errcheck // error doesn't matter

	// Verify database integrity
	if err := db.Verify(); err != nil {
		log.Printf("Database validation failed: %v", err)
		return
	}

	// Access metadata information
	metadata := db.Metadata
	fmt.Printf("Database type: %s\n", metadata.DatabaseType)
	fmt.Printf("Build time: %s\n", metadata.BuildTime().UTC().Format("2006-01-02 15:04:05"))
	fmt.Printf("IP version: IPv%d\n", metadata.IPVersion)
	fmt.Printf("Languages: %v\n", metadata.Languages)

	if desc, ok := metadata.Description["en"]; ok {
		fmt.Printf("Description: %s\n", desc)
	}

	// Output:
	// Database type: GeoIP2-City
	// Build time: 2022-07-26 14:53:10
	// IP version: IPv6
	// Languages: [en zh]
	// Description: GeoIP2 City Test Database (fake GeoIP2 data, for example purposes only)
}

// This example demonstrates how to iterate over all networks in the
// database which are contained within an arbitrary network.
func ExampleReader_NetworksWithin() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-Connection-Type-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close() //nolint:errcheck // error doesn't matter

	prefix, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		log.Panic(err)
	}

	for result := range db.NetworksWithin(prefix) {
		record := struct {
			ConnectionType string `maxminddb:"connection_type"`
		}{}
		err := result.Decode(&record)
		if err != nil {
			log.Panic(err)
		}
		fmt.Printf("%s: %s\n", result.Prefix(), record.ConnectionType)
	}

	// Output:
	// 1.0.0.0/24: Cable/DSL
	// 1.0.1.0/24: Cellular
	// 1.0.2.0/23: Cable/DSL
	// 1.0.4.0/22: Cable/DSL
	// 1.0.8.0/21: Cable/DSL
	// 1.0.16.0/20: Cable/DSL
	// 1.0.32.0/19: Cable/DSL
	// 1.0.64.0/18: Cable/DSL
	// 1.0.128.0/17: Cable/DSL
}

// CustomCity represents a simplified city record with custom unmarshaling.
// This demonstrates the Unmarshaler interface for custom decoding.
type CustomCity struct {
	Names     map[string]string
	GeoNameID uint
}

// UnmarshalMaxMindDB implements the mmdbdata.Unmarshaler interface.
// This provides custom decoding logic, similar to how json.Unmarshaler works
// with encoding/json, allowing fine-grained control over data processing.
func (c *CustomCity) UnmarshalMaxMindDB(d *mmdbdata.Decoder) error {
	for key, err := range d.ReadMap() {
		if err != nil {
			return err
		}

		switch string(key) {
		case "city":
			// Decode nested city structure
			for cityKey, cityErr := range d.ReadMap() {
				if cityErr != nil {
					return cityErr
				}
				switch string(cityKey) {
				case "names":
					// Decode nested map[string]string for localized names
					names := make(map[string]string)
					for nameKey, nameErr := range d.ReadMap() {
						if nameErr != nil {
							return nameErr
						}
						value, valueErr := d.ReadString()
						if valueErr != nil {
							return valueErr
						}
						names[string(nameKey)] = value
					}
					c.Names = names
				case "geoname_id":
					geoID, err := d.ReadUInt32()
					if err != nil {
						return err
					}
					c.GeoNameID = uint(geoID)
				default:
					if err := d.SkipValue(); err != nil {
						return err
					}
				}
			}
		default:
			// Skip unknown fields to ensure forward compatibility
			if err := d.SkipValue(); err != nil {
				return err
			}
		}
	}
	return nil
}

// This example demonstrates how to use the Unmarshaler interface for custom decoding.
// Types implementing Unmarshaler automatically use custom decoding logic instead of
// reflection, similar to how json.Unmarshaler works with encoding/json.
func ExampleUnmarshaler() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close() //nolint:errcheck // error doesn't matter

	addr := netip.MustParseAddr("81.2.69.142")

	// CustomCity implements Unmarshaler, so it will automatically use
	// the custom UnmarshalMaxMindDB method instead of reflection
	var city CustomCity
	err = db.Lookup(addr).Decode(&city)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("City ID: %d\n", city.GeoNameID)
	fmt.Printf("English name: %s\n", city.Names["en"])
	fmt.Printf("German name: %s\n", city.Names["de"])

	// Output:
	// City ID: 2643743
	// English name: London
	// German name: London
}
