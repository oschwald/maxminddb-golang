package maxminddb

import (
	"fmt"
	"log"
	"net"
)

type onlyCountry struct {
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

// ExampleStruct shows how to decode to a struct
func ExampleStruct() {
	db, err := Open("test-data/test-data/GeoIP2-City-Test.mmdb")
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
	db.Close()
	// Output:
	// GB
}

// ExampleInterface demonstrates how to decode to an interface{}
func ExampleInterface() {
	db, err := Open("test-data/test-data/GeoIP2-City-Test.mmdb")
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
	// Output:
	// map[city:map[geoname_id:2643743 names:map[de:London en:London es:Londres fr:Londres ja:ロンドン pt-BR:Londres ru:Лондон]] continent:map[code:EU geoname_id:6255148 names:map[de:Europa en:Europe es:Europa fr:Europe ja:ヨーロッパ pt-BR:Europa ru:Европа zh-CN:欧洲]] country:map[geoname_id:2635167 iso_code:GB names:map[de:Vereinigtes Königreich en:United Kingdom es:Reino Unido fr:Royaume-Uni ja:イギリス pt-BR:Reino Unido ru:Великобритания zh-CN:英国]] location:map[latitude:51.5142 longitude:-0.0931 time_zone:Europe/London] registered_country:map[geoname_id:6252001 iso_code:US names:map[de:USA en:United States es:Estados Unidos fr:États-Unis ja:アメリカ合衆国 pt-BR:Estados Unidos ru:США zh-CN:美国]] subdivisions:[map[geoname_id:6269131 iso_code:ENG names:map[en:England es:Inglaterra fr:Angleterre pt-BR:Inglaterra]]]]
}
