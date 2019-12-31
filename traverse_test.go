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
func TestNetworksWithin(t *testing.T) {
	_, network, error := net.ParseCIDR("1.1.1.0/24")

	assert.Nil(t, error)

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
			innerIPs = append(innerIPs, record.IP)
		}

		expectedIPs := []string{
			"1.1.1.1",
			"1.1.1.2",
			"1.1.1.4",
			"1.1.1.8",
			"1.1.1.16",
			"1.1.1.32",
		}

		assert.Equal(t, expectedIPs, innerIPs)
		assert.Nil(t, n.Err())
	}
}

func TestNetworksWithinSlash32(t *testing.T) {
	_, network, error := net.ParseCIDR("1.1.1.32/32")

	assert.Nil(t, error)

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
			innerIPs = append(innerIPs, record.IP)
		}

		expectedIPs := []string([]string{"1.1.1.32"})

		assert.Equal(t, expectedIPs, innerIPs)
		assert.Nil(t, n.Err())
	}
}
