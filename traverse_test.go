package maxminddb

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworks(t *testing.T) {
	for _, recordSize := range []uint{24, 28, 32} {
		for _, ipVersion := range []uint{4, 6} {
			fileName := testFile(fmt.Sprintf("MaxMind-DB-test-ipv%d-%d.mmdb", ipVersion, recordSize))
			reader, err := Open(fileName)
			require.Nil(t, err, "unexpected error while opening database: %v", err)
			defer reader.Close()

			n := reader.Networks()
			for n.Next() {
				record := struct {
					IP string `maxminddb:"ip"`
				}{}
				network, err := n.Network(&record)
				assert.Nil(t, err)
				assert.Equal(t, record.IP, network.IP.String(),
					"expected %s got %s", record.IP, network.IP.String(),
				)
			}
			assert.Nil(t, n.Err())
		}
	}
}

func TestNetworksWithInvalidSearchTree(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-broken-search-tree-24.mmdb"))
	require.Nil(t, err, "unexpected error while opening database: %v", err)
	defer reader.Close()

	n := reader.Networks()
	for n.Next() {
		var record interface{}
		_, err := n.Network(&record)
		assert.Nil(t, err)
	}
	assert.NotNil(t, n.Err(), "no error received when traversing an broken search tree")
	assert.Equal(t, n.Err().Error(), "invalid search tree at 128.128.128.128/32")
}

func TestNetworksWithinV4SearchInV4Db(t *testing.T) {
	var network = &net.IPNet{IP: make(net.IP, 4), Mask: net.CIDRMask(0, 32)}

	for _, recordSize := range []uint{24, 28, 32} {
		fileName := testFile(fmt.Sprintf("MaxMind-DB-test-ipv4-%d.mmdb", recordSize))
		reader, err := Open(fileName)
		require.Nil(t, err, "unexpected error while opening database: %v", err)
		defer reader.Close()

		n := reader.NetworksWithin(network)
		var innerIPs []string

		for n.Next() {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			network, err := n.Network(&record)
			assert.Nil(t, err)
			assert.Equal(t, record.IP, network.IP.String(),
				"expected %s got %s", record.IP, network.IP.String(),
			)
			innerIPs = append(innerIPs, network.String())
		}

		expectedIPs := []string{
			"1.1.1.1/32",
			"1.1.1.2/31",
			"1.1.1.4/30",
			"1.1.1.8/29",
			"1.1.1.16/28",
			"1.1.1.32/32",
		}

		assert.Equal(t, expectedIPs, innerIPs)
		assert.Nil(t, n.Err())
	}
}

func TestNetworksWithinSlash32V4SearchInV4Db(t *testing.T) {
	_, network, err := net.ParseCIDR("1.1.1.1/32")
	assert.Nil(t, err)

	for _, recordSize := range []uint{24, 28, 32} {
		fileName := testFile(fmt.Sprintf("MaxMind-DB-test-ipv4-%d.mmdb", recordSize))
		reader, err := Open(fileName)
		require.Nil(t, err, "unexpected error while opening database: %v", err)
		defer reader.Close()

		n := reader.NetworksWithin(network)
		var innerIPs []string

		for n.Next() {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			network, err := n.Network(&record)
			assert.Nil(t, err)
			assert.Equal(t, record.IP, network.IP.String(),
				"expected %s got %s", record.IP, network.IP.String(),
			)
			innerIPs = append(innerIPs, network.String())
		}

		expectedIPs := []string{
			"1.1.1.1/32",
		}

		assert.Equal(t, expectedIPs, innerIPs)
		assert.Nil(t, n.Err())
	}
}

func TestNetworksWithinSlash32V4SearchInV6Db(t *testing.T) {
	_, network, err := net.ParseCIDR("1.1.1.1/32")
	assert.Nil(t, err)

	for _, recordSize := range []uint{24, 28, 32} {
		fileName := testFile(fmt.Sprintf("MaxMind-DB-test-mixed-%d.mmdb", recordSize))
		reader, err := Open(fileName)
		require.Nil(t, err, "unexpected error while opening database: %v", err)
		defer reader.Close()

		n := reader.NetworksWithin(network)
		var innerIPs []string

		for n.Next() {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			network, err := n.Network(&record)
			assert.Nil(t, err)
			innerIPs = append(innerIPs, network.String())
		}

		expectedIPs := []string{
			"1.1.1.1/32",
		}

		assert.Equal(t, expectedIPs, innerIPs)
		assert.Nil(t, n.Err())
	}

}
func TestNetworksWithinSlash128V6SearchInV6Db(t *testing.T) {
	_, network, err := net.ParseCIDR("::1:ffff:ffff/128")
	assert.Nil(t, err)

	for _, recordSize := range []uint{24, 28, 32} {
		fileName := testFile(fmt.Sprintf("MaxMind-DB-test-ipv6-%d.mmdb", recordSize))
		reader, err := Open(fileName)
		require.Nil(t, err, "unexpected error while opening database: %v", err)
		defer reader.Close()

		n := reader.NetworksWithin(network)
		var innerIPs []string

		for n.Next() {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			network, err := n.Network(&record)
			assert.Nil(t, err)
			assert.Equal(t, record.IP, network.IP.String(),
				"expected %s got %s", record.IP, network.IP.String(),
			)
			innerIPs = append(innerIPs, network.String())
		}

		expectedIPs := []string{
			"::1:ffff:ffff/128",
		}

		assert.Equal(t, expectedIPs, innerIPs)
		assert.Nil(t, n.Err())
	}
}

func TestNetworksWithinV6SearchInV6Db(t *testing.T) {
	var network = &net.IPNet{IP: make(net.IP, 16), Mask: net.CIDRMask(0, 128)}

	for _, recordSize := range []uint{24, 28, 32} {
		fileName := testFile(fmt.Sprintf("MaxMind-DB-test-ipv6-%d.mmdb", recordSize))
		reader, err := Open(fileName)
		require.Nil(t, err, "unexpected error while opening database: %v", err)
		defer reader.Close()

		n := reader.NetworksWithin(network)
		var innerIPs []string

		for n.Next() {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			network, err := n.Network(&record)
			assert.Nil(t, err)
			assert.Equal(t, record.IP, network.IP.String(),
				"expected %s got %s", record.IP, network.IP.String(),
			)
			innerIPs = append(innerIPs, network.String())
		}

		expectedIPs := []string{
			"::1:ffff:ffff/128",
			"::2:0:0/122",
			"::2:0:40/124",
			"::2:0:50/125",
			"::2:0:58/127",
		}

		assert.Equal(
			t,
			expectedIPs,
			innerIPs,
			fmt.Sprintf("inner IPs for %v", fileName),
		)
		assert.Nil(t, n.Err())
	}
}

func TestNetworksWithinV4SearchInV6Db(t *testing.T) {
	var network = &net.IPNet{IP: make(net.IP, 4), Mask: net.CIDRMask(0, 32)}

	for _, recordSize := range []uint{24, 28, 32} {
		fileName := testFile(fmt.Sprintf("MaxMind-DB-test-mixed-%d.mmdb", recordSize))
		reader, err := Open(fileName)
		require.Nil(t, err, "unexpected error while opening database: %v", err)
		defer reader.Close()

		n := reader.NetworksWithin(network)
		var innerIPs []string

		for n.Next() {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			network, err := n.Network(&record)
			assert.Nil(t, err)
			innerIPs = append(innerIPs, network.String())
		}

		expectedIPs := []string{
			"1.1.1.1/32",
			"1.1.1.2/31",
			"1.1.1.4/30",
			"1.1.1.8/29",
			"1.1.1.16/28",
			"1.1.1.32/32",
		}

		assert.Equal(
			t,
			expectedIPs,
			innerIPs,
			fmt.Sprintf("inner IPs for %v", fileName),
		)
		assert.Nil(t, n.Err())
	}
}

func TestNetworksWithinV6SearchInV4Db(t *testing.T) {
	var network = &net.IPNet{IP: make(net.IP, 16), Mask: net.CIDRMask(0, 128)}

	for _, recordSize := range []uint{24, 28, 32} {
		fileName := testFile(fmt.Sprintf("MaxMind-DB-test-ipv4-%d.mmdb", recordSize))
		reader, err := Open(fileName)
		require.Nil(t, err, "unexpected error while opening database: %v", err)
		defer reader.Close()

		n := reader.NetworksWithin(network)
		var innerIPs []string

		for n.Next() {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			network, err := n.Network(&record)
			assert.Nil(t, err)
			innerIPs = append(innerIPs, network.String())
		}

		expectedIPs := []string{
			"101:101::/32",
			"101:102::/31",
			"101:104::/30",
			"101:108::/29",
			"101:110::/28",
			"101:120::/32",
		}

		assert.Equal(
			t,
			expectedIPs,
			innerIPs,
			fmt.Sprintf("inner IPs for %v", fileName),
		)
		assert.Nil(t, n.Err())
	}
}
