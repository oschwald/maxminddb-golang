package maxminddb_test

import (
	"fmt"
	"github.com/oschwald/maxminddb-golang"
	"log"
	"net"
)

type onlyCountry struct {
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

// This example shows how to decode to a struct
func ExampleReader_Lookup_struct() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	ip := net.ParseIP("81.2.69.142")

	var record onlyCountry // Or any appropriate struct
	err = db.Lookup(ip, &record)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(record.Country.IsoCode)
	// Output:
	// GB
	db.Close()
}

// This example demonstrates how to decode to an interface{}
func ExampleReader_Lookup_interface() {
	db, err := maxminddb.Open("test-data/test-data/GeoIP2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	ip := net.ParseIP("81.2.69.142")

	var record interface{}
	err = db.Lookup(ip, &record)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%v", record)
	db.Close()
}
