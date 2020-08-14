package maxminddb_test

import (
	"fmt"
	"log"
	"net"

	"github.com/oschwald/maxminddb-golang"
)

// This example shows how to decode to a struct
func ExampleReader_Lookup_struct() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ip := net.ParseIP("81.2.69.142")

	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	} // Or any appropriate struct

	err = db.Lookup(ip, &record)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(record.Country.ISOCode)
	// Output:
	// GB
}

// This example demonstrates how to decode to an interface{}
func ExampleReader_Lookup_interface() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ip := net.ParseIP("81.2.69.142")

	var record interface{}
	err = db.Lookup(ip, &record)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%v", record)
}

// This example demonstrates how to iterate over all networks in the
// database
func ExampleReader_Networks() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-Connection-Type-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	record := struct {
		Domain string `maxminddb:"connection_type"`
	}{}

	networks := db.Networks(maxminddb.SkipAliasedNetworks)
	for networks.Next() {
		subnet, err := networks.Network(&record)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %s\n", subnet.String(), record.Domain)
	}
	if networks.Err() != nil {
		log.Fatal(networks.Err())
	}
	// Output:
	// 1.0.0.0/24: Dialup
	// 1.0.1.0/24: Cable/DSL
	// 1.0.2.0/23: Dialup
	// 1.0.4.0/22: Dialup
	// 1.0.8.0/21: Dialup
	// 1.0.16.0/20: Dialup
	// 1.0.32.0/19: Dialup
	// 1.0.64.0/18: Dialup
	// 1.0.128.0/17: Dialup
	// 80.214.0.0/20: Cellular
	// 96.1.0.0/16: Cable/DSL
	// 96.10.0.0/15: Cable/DSL
	// 96.69.0.0/16: Cable/DSL
	// 96.94.0.0/15: Cable/DSL
	// 108.96.0.0/11: Cellular
	// 175.16.199.0/24: Dialup
	// 187.156.138.0/24: Cable/DSL
	// 201.243.200.0/24: Corporate
	// 207.179.48.0/20: Cellular
	// 2003::/24: Cable/DSL
}

// This example demonstrates how to iterate over all networks in the
// database which are contained within an arbitrary network.
func ExampleReader_NetworksWithin() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-Connection-Type-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	record := struct {
		Domain string `maxminddb:"connection_type"`
	}{}

	_, network, err := net.ParseCIDR("1.0.0.0/8")
	if err != nil {
		log.Fatal(err)
	}

	networks := db.NetworksWithin(network, maxminddb.SkipAliasedNetworks)
	for networks.Next() {
		subnet, err := networks.Network(&record)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %s\n", subnet.String(), record.Domain)
	}
	if networks.Err() != nil {
		log.Fatal(networks.Err())
	}

	// Output:
	// 1.0.0.0/24: Dialup
	// 1.0.1.0/24: Cable/DSL
	// 1.0.2.0/23: Dialup
	// 1.0.4.0/22: Dialup
	// 1.0.8.0/21: Dialup
	// 1.0.16.0/20: Dialup
	// 1.0.32.0/19: Dialup
	// 1.0.64.0/18: Dialup
	// 1.0.128.0/17: Dialup
}
