package maxminddb

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworks(t *testing.T) {
	for _, recordSize := range []uint{24, 28, 32} {
		for _, ipVersion := range []uint{4, 6} {
			fileName := testFile(
				fmt.Sprintf("MaxMind-DB-test-ipv%d-%d.mmdb", ipVersion, recordSize),
			)
			reader, err := Open(fileName)
			require.NoError(t, err, "unexpected error while opening database: %v", err)

			n := reader.Networks()
			for n.Next() {
				record := struct {
					IP string `maxminddb:"ip"`
				}{}
				network, err := n.Network(&record)
				require.NoError(t, err)
				assert.Equal(t, record.IP, network.IP.String(),
					"expected %s got %s", record.IP, network.IP.String(),
				)
			}
			require.NoError(t, n.Err())
			require.NoError(t, reader.Close())
		}
	}
}

func TestNetworksWithInvalidSearchTree(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-broken-search-tree-24.mmdb"))
	require.NoError(t, err, "unexpected error while opening database: %v", err)

	n := reader.Networks()
	for n.Next() {
		var record any
		_, err := n.Network(&record)
		require.NoError(t, err)
	}
	require.EqualError(t, n.Err(), "invalid search tree at 128.128.128.128/32")

	require.NoError(t, reader.Close())
}

type networkTest struct {
	Network  string
	Database string
	Expected []string
	Options  []NetworksOption
}

var tests = []networkTest{
	{
		Network:  "0.0.0.0/0",
		Database: "ipv4",
		Expected: []string{
			"1.1.1.1/32",
			"1.1.1.2/31",
			"1.1.1.4/30",
			"1.1.1.8/29",
			"1.1.1.16/28",
			"1.1.1.32/32",
		},
	},
	{
		// This is intentionally in non-canonical form to test
		// that we handle it correctly.
		Network:  "1.1.1.1/30",
		Database: "ipv4",
		Expected: []string{
			"1.1.1.1/32",
			"1.1.1.2/31",
		},
	},
	{
		Network:  "1.1.1.2/31",
		Database: "ipv4",
		Expected: []string{
			"1.1.1.2/31",
		},
	},
	{
		Network:  "1.1.1.1/32",
		Database: "ipv4",
		Expected: []string{
			"1.1.1.1/32",
		},
	},
	{
		Network:  "1.1.1.2/32",
		Database: "ipv4",
		Expected: []string{
			"1.1.1.2/31",
		},
	},
	{
		Network:  "1.1.1.3/32",
		Database: "ipv4",
		Expected: []string{
			"1.1.1.2/31",
		},
	},
	{
		Network:  "1.1.1.19/32",
		Database: "ipv4",
		Expected: []string{
			"1.1.1.16/28",
		},
	},
	{
		Network:  "255.255.255.0/24",
		Database: "ipv4",
		Expected: []string(nil),
	},
	{
		Network:  "1.1.1.1/32",
		Database: "mixed",
		Expected: []string{
			"1.1.1.1/32",
		},
	},
	{
		Network:  "255.255.255.0/24",
		Database: "mixed",
		Expected: []string(nil),
	},
	{
		Network:  "::1:ffff:ffff/128",
		Database: "ipv6",
		Expected: []string{
			"::1:ffff:ffff/128",
		},
		Options: []NetworksOption{SkipAliasedNetworks},
	},
	{
		Network:  "::/0",
		Database: "ipv6",
		Expected: []string{
			"::1:ffff:ffff/128",
			"::2:0:0/122",
			"::2:0:40/124",
			"::2:0:50/125",
			"::2:0:58/127",
		},
		Options: []NetworksOption{SkipAliasedNetworks},
	},
	{
		Network:  "::2:0:40/123",
		Database: "ipv6",
		Expected: []string{
			"::2:0:40/124",
			"::2:0:50/125",
			"::2:0:58/127",
		},
		Options: []NetworksOption{SkipAliasedNetworks},
	},
	{
		Network:  "0:0:0:0:0:ffff:ffff:ff00/120",
		Database: "ipv6",
		Expected: []string(nil),
	},
	{
		Network:  "0.0.0.0/0",
		Database: "mixed",
		Expected: []string{
			"1.1.1.1/32",
			"1.1.1.2/31",
			"1.1.1.4/30",
			"1.1.1.8/29",
			"1.1.1.16/28",
			"1.1.1.32/32",
		},
	},
	{
		Network:  "0.0.0.0/0",
		Database: "mixed",
		Expected: []string{
			"1.1.1.1/32",
			"1.1.1.2/31",
			"1.1.1.4/30",
			"1.1.1.8/29",
			"1.1.1.16/28",
			"1.1.1.32/32",
		},
		Options: []NetworksOption{SkipAliasedNetworks},
	},
	{
		Network:  "::/0",
		Database: "mixed",
		Expected: []string{
			"::101:101/128",
			"::101:102/127",
			"::101:104/126",
			"::101:108/125",
			"::101:110/124",
			"::101:120/128",
			"::1:ffff:ffff/128",
			"::2:0:0/122",
			"::2:0:40/124",
			"::2:0:50/125",
			"::2:0:58/127",
			"1.1.1.1/32",
			"1.1.1.2/31",
			"1.1.1.4/30",
			"1.1.1.8/29",
			"1.1.1.16/28",
			"1.1.1.32/32",
			"2001:0:101:101::/64",
			"2001:0:101:102::/63",
			"2001:0:101:104::/62",
			"2001:0:101:108::/61",
			"2001:0:101:110::/60",
			"2001:0:101:120::/64",
			"2002:101:101::/48",
			"2002:101:102::/47",
			"2002:101:104::/46",
			"2002:101:108::/45",
			"2002:101:110::/44",
			"2002:101:120::/48",
		},
	},
	{
		Network:  "::/0",
		Database: "mixed",
		Expected: []string{
			"1.1.1.1/32",
			"1.1.1.2/31",
			"1.1.1.4/30",
			"1.1.1.8/29",
			"1.1.1.16/28",
			"1.1.1.32/32",
			"::1:ffff:ffff/128",
			"::2:0:0/122",
			"::2:0:40/124",
			"::2:0:50/125",
			"::2:0:58/127",
		},
		Options: []NetworksOption{SkipAliasedNetworks},
	},
	{
		Network:  "1.1.1.16/28",
		Database: "mixed",
		Expected: []string{
			"1.1.1.16/28",
		},
	},
	{
		Network:  "1.1.1.4/30",
		Database: "ipv4",
		Expected: []string{
			"1.1.1.4/30",
		},
	},
}

func TestNetworksWithin(t *testing.T) {
	for _, v := range tests {
		for _, recordSize := range []uint{24, 28, 32} {
			name := fmt.Sprintf(
				"%s-%d: %s, options: %v",
				v.Database,
				recordSize,
				v.Network,
				len(v.Options) != 0,
			)
			t.Run(name, func(t *testing.T) {
				fileName := testFile(fmt.Sprintf("MaxMind-DB-test-%s-%d.mmdb", v.Database, recordSize))
				reader, err := Open(fileName)
				require.NoError(t, err, "unexpected error while opening database: %v", err)

				// We are purposely not using net.ParseCIDR so that we can pass in
				// values that aren't in canonical form.
				parts := strings.Split(v.Network, "/")
				ip := net.ParseIP(parts[0])
				if v := ip.To4(); v != nil {
					ip = v
				}
				prefixLength, err := strconv.Atoi(parts[1])
				require.NoError(t, err)
				mask := net.CIDRMask(prefixLength, len(ip)*8)
				network := &net.IPNet{
					IP:   ip,
					Mask: mask,
				}

				require.NoError(t, err)
				n := reader.NetworksWithin(network, v.Options...)
				var innerIPs []string

				for n.Next() {
					record := struct {
						IP string `maxminddb:"ip"`
					}{}
					network, err := n.Network(&record)
					require.NoError(t, err)
					innerIPs = append(innerIPs, network.String())
				}

				assert.Equal(t, v.Expected, innerIPs)
				require.NoError(t, n.Err())

				require.NoError(t, reader.Close())
			})
		}
	}
}

var geoipTests = []networkTest{
	{
		Network:  "81.2.69.128/26",
		Database: "GeoIP2-Country-Test.mmdb",
		Expected: []string{
			"81.2.69.142/31",
			"81.2.69.144/28",
			"81.2.69.160/27",
		},
	},
}

func TestGeoIPNetworksWithin(t *testing.T) {
	for _, v := range geoipTests {
		fileName := testFile(v.Database)
		reader, err := Open(fileName)
		require.NoError(t, err, "unexpected error while opening database: %v", err)

		_, network, err := net.ParseCIDR(v.Network)
		require.NoError(t, err)
		n := reader.NetworksWithin(network)
		var innerIPs []string

		for n.Next() {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			network, err := n.Network(&record)
			require.NoError(t, err)
			innerIPs = append(innerIPs, network.String())
		}

		assert.Equal(t, v.Expected, innerIPs)
		require.NoError(t, n.Err())

		require.NoError(t, reader.Close())
	}
}
