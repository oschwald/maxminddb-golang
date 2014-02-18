package maxminddb

import (
	"errors"
	"fmt"
	. "launchpad.net/gocheck"
	"math/big"
	"net"
	"testing"
)

func TestMaxMindDbReader(t *testing.T) { TestingT(t) }

type MySuite struct{}

var _ = Suite(&MySuite{})

func (s *MySuite) TestReader(c *C) {
	for _, recordSize := range []uint{24, 28, 32} {
		for _, ipVersion := range []uint{4, 6} {
			fileName := fmt.Sprintf("test-data/test-data/MaxMind-DB-test-ipv%d-%d.mmdb", ipVersion, recordSize)
			reader, _ := Open(fileName)

			checkMetadata(c, reader, ipVersion, recordSize)

			if ipVersion == 4 {
				checkIpv4(c, reader)
			} else {
				checkIpv6(c, reader)
			}
		}
	}
}

func (s *MySuite) TestDecoder(c *C) {
	reader, _ := Open("test-data/test-data/MaxMind-DB-test-decoder.mmdb")
	recordInterface, _ := reader.Lookup(net.ParseIP("::1.1.1.0"))
	record := recordInterface.(map[string]interface{})

	c.Assert(record["array"], DeepEquals, []interface{}{uint(1), uint(2), uint(3)})
	c.Assert(record["boolean"], Equals, true)
	c.Assert(record["bytes"], DeepEquals, []byte{0x00, 0x00, 0x00, 0x2a})
	c.Assert(record["double"], Equals, 42.123456)
	c.Assert(record["float"], Equals, float32(1.1))
	c.Assert(record["int32"], Equals, -268435456)
	c.Assert(record["map"], DeepEquals,
		map[string]interface{}{
			"mapX": map[string]interface{}{
				"arrayX":       []interface{}{uint(7), uint(8), uint(9)},
				"utf8_stringX": "hello",
			}})

	c.Assert(record["uint16"], Equals, uint(100))
	c.Assert(record["uint32"], Equals, uint(268435456))
	c.Assert(record["uint64"], Equals, uint(1152921504606846976))
	c.Assert(record["utf8_string"], Equals, "unicode! ☯ - ♫")
	bigInt := new(big.Int)
	bigInt.SetString("1329227995784915872903807060280344576", 10)
	c.Assert(record["uint128"], DeepEquals, bigInt)
	reader.Close()
}

func (s *MySuite) TestIpv6inIpv4(c *C) {
	reader, _ := Open("test-data/test-data/MaxMind-DB-test-ipv4-24.mmdb")
	record, err := reader.Lookup(net.ParseIP("2001::"))
	if record != nil {
		c.Log("nil record from lookup expected")
		c.Fail()
	}
	expected := errors.New("Error looking up 2001::. You attempted to look up an IPv6 address in an IPv4-only database.")
	c.Assert(err, DeepEquals, expected)
	reader.Close()

}

func (s *MySuite) TestBrokenDatabase(c *C) {
	c.Skip("NO TYPE DECODING VALIDATION DONE")
	reader, _ := Open("test-data/test-data/GeoIP2-City-Test-Broken-Double-Format.mmdb")
	// // Should return an error like: "The MaxMind DB file's data "
	// //                              "section contains bad data (unknown data "
	// //                              "type or corrupt data)"
	reader.Lookup(net.ParseIP("2001:220::"))
	reader.Close()

}

func (s *MySuite) TestMissingDatabase(c *C) {
	reader, err := Open("file-does-not-exist.mmdb")
	if reader != nil {
		c.Log("received reader when doing lookups on DB that doesn't exist")
		c.Fail()
	}
	c.Assert(err.Error(), Equals, "open file-does-not-exist.mmdb: no such file or directory")
}

func (s *MySuite) TestNonDatabase(c *C) {
	reader, err := Open("README.md")
	if reader != nil {
		c.Log("received reader when doing lookups on DB that doesn't exist")
		c.Fail()
	}
	c.Assert(err.Error(), Equals, "Error opening database file (README.md). Is this a valid MaxMind DB file?")
}

func checkMetadata(c *C, reader *Reader, ipVersion uint, recordSize uint) {
	metadata := reader.metadata

	c.Assert(metadata["binary_format_major_version"], Equals, uint(2))

	c.Assert(metadata["binary_format_minor_version"], Equals, uint(0))
	c.Assert(metadata["build_epoch"], FitsTypeOf, uint(0))
	c.Assert(metadata["database_type"], Equals, "Test")

	c.Assert(metadata["description"], DeepEquals,
		map[string]interface{}{
			"en": "Test Database",
			"zh": "Test Database Chinese",
		})
	c.Assert(metadata["ip_version"], Equals, ipVersion)
	c.Assert(metadata["languages"], DeepEquals, []interface{}{"en", "zh"})

	if ipVersion == 4 {
		c.Assert(metadata["node_count"], Equals, uint(37))
	} else {
		c.Assert(metadata["node_count"], Equals, uint(188))
	}

	c.Assert(metadata["record_size"], Equals, recordSize)
}

func checkIpv4(c *C, reader *Reader) {

	for i := uint(0); i < 6; i++ {
		address := fmt.Sprintf("1.1.1.%d", uint(1)<<i)
		ip := net.ParseIP(address)

		// XXX - Figure out why ParseIP always returns 16 byte address.
		// Maybe update reader to accommodated
		record, _ := reader.Lookup(ip[12:])
		c.Assert(record, DeepEquals, map[string]interface{}{
			"ip": address})
	}
	pairs := map[string]string{
		"1.1.1.3":  "1.1.1.2",
		"1.1.1.5":  "1.1.1.4",
		"1.1.1.7":  "1.1.1.4",
		"1.1.1.9":  "1.1.1.8",
		"1.1.1.15": "1.1.1.8",
		"1.1.1.17": "1.1.1.16",
		"1.1.1.31": "1.1.1.16",
	}

	for keyAddress, valueAddress := range pairs {
		data := map[string]interface{}{"ip": valueAddress}

		ip := net.ParseIP(keyAddress)

		record, _ := reader.Lookup(ip[12:])
		c.Assert(record, DeepEquals, data)
	}

	for _, address := range []string{"1.1.1.33", "255.254.253.123"} {
		ip := net.ParseIP(address)

		record, _ := reader.Lookup(ip[12:])
		c.Assert(record, IsNil)
	}
}

func checkIpv6(c *C, reader *Reader) {

	subnets := []string{"::1:ffff:ffff", "::2:0:0",
		"::2:0:40", "::2:0:50", "::2:0:58"}

	for _, address := range subnets {
		record, _ := reader.Lookup(net.ParseIP(address))
		c.Assert(record, DeepEquals, map[string]interface{}{"ip": address})
	}

	pairs := map[string]string{
		"::2:0:1":  "::2:0:0",
		"::2:0:33": "::2:0:0",
		"::2:0:39": "::2:0:0",
		"::2:0:41": "::2:0:40",
		"::2:0:49": "::2:0:40",
		"::2:0:52": "::2:0:50",
		"::2:0:57": "::2:0:50",
		"::2:0:59": "::2:0:58",
	}

	for keyAddress, valueAddress := range pairs {
		data := map[string]interface{}{"ip": valueAddress}
		record, _ := reader.Lookup(net.ParseIP(keyAddress))
		c.Assert(record, DeepEquals, data)
	}

	for _, address := range []string{"1.1.1.33", "255.254.253.123", "89fa::"} {
		record, _ := reader.Lookup(net.ParseIP(address))
		c.Assert(record, IsNil)
	}
}