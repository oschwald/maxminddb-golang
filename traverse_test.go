package maxminddb

import (
	"fmt"
	"net/netip"
	"reflect"
	"runtime"
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

			for result := range reader.Networks() {
				record := struct {
					IP string `maxminddb:"ip"`
				}{}
				err := result.Decode(&record)
				require.NoError(t, err)

				network := result.Prefix()
				assert.Equal(t, record.IP, network.Addr().String(),
					"expected %s got %s", record.IP, network.Addr().String(),
				)
			}
			require.NoError(t, reader.Close())
		}
	}
}

func TestNetworksWithInvalidSearchTree(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-broken-search-tree-24.mmdb"))
	require.NoError(t, err, "unexpected error while opening database: %v", err)

	for result := range reader.Networks() {
		var record any
		err = result.Decode(&record)
		if err != nil {
			break
		}
	}
	require.EqualError(t, err, "invalid search tree at 128.128.128.128/32")

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
	},
	{
		Network:  "::2:0:40/123",
		Database: "ipv6",
		Expected: []string{
			"::2:0:40/124",
			"::2:0:50/125",
			"::2:0:58/127",
		},
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
			"::ffff:1.1.1.1/128",
			"::ffff:1.1.1.2/127",
			"::ffff:1.1.1.4/126",
			"::ffff:1.1.1.8/125",
			"::ffff:1.1.1.16/124",
			"::ffff:1.1.1.32/128",
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
		Options: []NetworksOption{IncludeAliasedNetworks},
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
	},
	{
		Network:  "1.0.0.0/8",
		Database: "mixed",
		Expected: []string{
			"1.0.0.0/16",
			"1.1.0.0/24",
			"1.1.1.0/32",
			"1.1.1.1/32",
			"1.1.1.2/31",
			"1.1.1.4/30",
			"1.1.1.8/29",
			"1.1.1.16/28",
			"1.1.1.32/32",
			"1.1.1.33/32",
			"1.1.1.34/31",
			"1.1.1.36/30",
			"1.1.1.40/29",
			"1.1.1.48/28",
			"1.1.1.64/26",
			"1.1.1.128/25",
			"1.1.2.0/23",
			"1.1.4.0/22",
			"1.1.8.0/21",
			"1.1.16.0/20",
			"1.1.32.0/19",
			"1.1.64.0/18",
			"1.1.128.0/17",
			"1.2.0.0/15",
			"1.4.0.0/14",
			"1.8.0.0/13",
			"1.16.0.0/12",
			"1.32.0.0/11",
			"1.64.0.0/10",
			"1.128.0.0/9",
		},
		Options: []NetworksOption{IncludeNetworksWithoutData},
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
			var opts []string
			for _, o := range v.Options {
				opts = append(opts, runtime.FuncForPC(reflect.ValueOf(o).Pointer()).Name())
			}
			name := fmt.Sprintf(
				"%s-%d: %s, options: %v",
				v.Database,
				recordSize,
				v.Network,
				opts,
			)
			t.Run(name, func(t *testing.T) {
				fileName := testFile(fmt.Sprintf("MaxMind-DB-test-%s-%d.mmdb", v.Database, recordSize))
				reader, err := Open(fileName)
				require.NoError(t, err, "unexpected error while opening database: %v", err)

				// We are purposely not using net.ParseCIDR so that we can pass in
				// values that aren't in canonical form.
				parts := strings.Split(v.Network, "/")
				ip, err := netip.ParseAddr(parts[0])
				require.NoError(t, err)
				prefixLength, err := strconv.Atoi(parts[1])
				require.NoError(t, err)
				network, err := ip.Prefix(prefixLength)
				require.NoError(t, err)

				require.NoError(t, err)
				var innerIPs []string

				for result := range reader.NetworksWithin(network, v.Options...) {
					record := struct {
						IP string `maxminddb:"ip"`
					}{}
					err := result.Decode(&record)
					require.NoError(t, err)
					innerIPs = append(innerIPs, result.Prefix().String())
				}

				assert.Equal(t, v.Expected, innerIPs)

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

		prefix, err := netip.ParsePrefix(v.Network)
		require.NoError(t, err)
		var innerIPs []string

		for result := range reader.NetworksWithin(prefix) {
			record := struct {
				IP string `maxminddb:"ip"`
			}{}
			err := result.Decode(&record)
			require.NoError(t, err)
			innerIPs = append(innerIPs, result.Prefix().String())
		}

		assert.Equal(t, v.Expected, innerIPs)

		require.NoError(t, reader.Close())
	}
}

func BenchmarkNetworks(b *testing.B) {
	db, err := Open(testFile("GeoIP2-Country-Test.mmdb"))
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for r := range db.Networks() {
			var rec struct{}
			err = r.Decode(&rec)
			if err != nil {
				b.Error(err)
			}
		}
	}
	require.NoError(b, db.Close(), "error on close")
}
