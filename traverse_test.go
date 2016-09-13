package maxminddb

import (
	"fmt"

	. "gopkg.in/check.v1"
)

type TraverseSuite struct{}

var _ = Suite(&TraverseSuite{})

func (s *TraverseSuite) TestNetworks(c *C) {
	for _, recordSize := range []uint{24, 28, 32} {
		for _, ipVersion := range []uint{4, 6} {
			fileName := fmt.Sprintf("test-data/test-data/MaxMind-DB-test-ipv%d-%d.mmdb", ipVersion, recordSize)
			reader, err := Open(fileName)
			c.Assert(err, IsNil)

			defer reader.Close()

			n := reader.Networks()
			for n.Next() {
				record := struct {
					IP string `maxminddb:"ip"`
				}{}
				network, err := n.Network(&record)
				c.Assert(err, IsNil)
				c.Check(record.IP, Equals, network.IP.String())
			}
			c.Assert(n.Err(), IsNil)
		}
	}
}

func (s *TraverseSuite) TestNetworksWithInvalidSearchTree(c *C) {
	reader, err := Open("test-data/test-data/MaxMind-DB-test-broken-search-tree-24.mmdb")
	c.Assert(err, IsNil)
	defer reader.Close()

	n := reader.Networks()
	for n.Next() {
		var record interface{}
		_, err := n.Network(&record)
		c.Assert(err, IsNil)
	}

	c.Assert(n.Err(), NotNil)
	c.Check(n.Err().Error(), Equals, "invalid search tree at 128.128.128.128/32")
}
