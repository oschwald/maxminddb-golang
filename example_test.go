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

// Example_struct shows how to decode to a struct
func Example_struct() {
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

// Example_interface demonstrates how to decode to an interface{}
func Example_interface() {
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
