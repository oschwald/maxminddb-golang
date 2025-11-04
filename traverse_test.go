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
		Options: []NetworksOption{IncludeAliasedNetworks()},
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
		Options: []NetworksOption{IncludeNetworksWithoutData()},
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
				fileName := testFile(
					fmt.Sprintf("MaxMind-DB-test-%s-%d.mmdb", v.Database, recordSize),
				)
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

	for b.Loop() {
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

func TestSkipEmptyValues(t *testing.T) {
	// Test with database that has many empty values
	reader, err := Open(testFile("GeoIP2-Anonymous-IP-Test.mmdb"))
	require.NoError(t, err)
	defer reader.Close()

	// Count networks without SkipEmptyValues
	var countWithout, emptyCount int
	for result := range reader.Networks() {
		require.NoError(t, result.Err())
		countWithout++

		if result.Found() {
			var data map[string]any
			err := result.Decode(&data)
			require.NoError(t, err)
			if len(data) == 0 {
				emptyCount++
			}
		}
	}

	// Count networks with SkipEmptyValues
	var countWith int
	for result := range reader.Networks(SkipEmptyValues()) {
		require.NoError(t, result.Err())
		countWith++

		if result.Found() {
			var data map[string]any
			err := result.Decode(&data)
			require.NoError(t, err)
			assert.NotEmpty(t, data, "should not see empty maps with SkipEmptyValues")
		}
	}

	// Verify the option works as expected
	assert.Positive(t, emptyCount, "test database should have empty values")
	assert.Equal(
		t,
		countWithout-emptyCount,
		countWith,
		"SkipEmptyValues should skip exactly the empty values",
	)

	t.Logf("Without SkipEmptyValues: %d networks (%d empty)", countWithout, emptyCount)
	t.Logf("With SkipEmptyValues: %d networks (0 empty)", countWith)
}

func TestSkipEmptyValuesWithNetworksWithin(t *testing.T) {
	tests := []struct {
		name       string
		dbFile     string
		prefix     string
		options    []NetworksOption
		validateFn func(t *testing.T, count, emptyCount int)
	}{
		{
			name:    "NetworksWithin without options on DB with empty values",
			dbFile:  "GeoIP2-Anonymous-IP-Test.mmdb",
			prefix:  "0.0.0.0/0",
			options: nil,
			validateFn: func(t *testing.T, count, emptyCount int) {
				assert.Positive(t, count, "should have networks")
				assert.Positive(t, emptyCount, "should find empty maps")
				t.Logf("Found %d networks, %d empty", count, emptyCount)
			},
		},
		{
			name:    "NetworksWithin with SkipEmptyValues",
			dbFile:  "GeoIP2-Anonymous-IP-Test.mmdb",
			prefix:  "0.0.0.0/0",
			options: []NetworksOption{SkipEmptyValues()},
			validateFn: func(t *testing.T, count, emptyCount int) {
				assert.Positive(t, count, "should have networks")
				assert.Equal(t, 0, emptyCount, "should not find empty maps with SkipEmptyValues")
			},
		},
		{
			name:    "NetworksWithin specific subnet without empty values",
			dbFile:  "GeoIP2-Connection-Type-Test.mmdb",
			prefix:  "1.0.0.0/8",
			options: []NetworksOption{SkipEmptyValues()},
			validateFn: func(t *testing.T, count, _ int) {
				assert.Positive(t, count, "should have networks in 1.0.0.0/8")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := Open(testFile(tt.dbFile))
			require.NoError(t, err)
			defer reader.Close()

			prefix, err := netip.ParsePrefix(tt.prefix)
			require.NoError(t, err)

			var count, emptyCount int
			for result := range reader.NetworksWithin(prefix, tt.options...) {
				require.NoError(t, result.Err())
				count++

				if !result.Found() {
					continue
				}

				var data map[string]any
				err := result.Decode(&data)
				require.NoError(t, err)

				if len(data) == 0 {
					emptyCount++
				}
			}

			tt.validateFn(t, count, emptyCount)
		})
	}
}

func TestSkipEmptyValuesWithOtherOptions(t *testing.T) {
	// Test that SkipEmptyValues works correctly with other options
	reader, err := Open(testFile("GeoIP2-Anonymous-IP-Test.mmdb"))
	require.NoError(t, err)
	defer reader.Close()

	// Test with IncludeNetworksWithoutData - should still skip empty maps
	var count int
	for result := range reader.Networks(IncludeNetworksWithoutData(), SkipEmptyValues()) {
		require.NoError(t, result.Err())
		count++

		if result.Found() {
			var data map[string]any
			err := result.Decode(&data)
			require.NoError(t, err)
			assert.NotEmpty(t, data, "should not see empty maps even with other options")
		}
	}

	assert.Positive(t, count, "should have some networks")
}

func BenchmarkSkipEmptyValues(b *testing.B) {
	db, err := Open(testFile("GeoIP2-Anonymous-IP-Test.mmdb"))
	require.NoError(b, err)
	defer db.Close()

	b.Run("without SkipEmptyValues", func(b *testing.B) {
		for range b.N {
			for r := range db.Networks() {
				if r.Err() != nil {
					b.Fatal(r.Err())
				}
			}
		}
	})

	b.Run("with SkipEmptyValues", func(b *testing.B) {
		for range b.N {
			for r := range db.Networks(SkipEmptyValues()) {
				if r.Err() != nil {
					b.Fatal(r.Err())
				}
			}
		}
	})
}
