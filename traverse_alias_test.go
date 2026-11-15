package maxminddb

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworksSharingIPv6DataRecord(t *testing.T) {
	// Both children of the single 24-bit node point to the same data record.
	// Traversing toward ::/96 stops at this record, not an IPv4 subtree.
	data := []byte{0, 0, 17, 0, 0, 17}
	data = append(data, make([]byte, dataSectionSeparatorSize)...)
	data = append(data, 0x41, 'X')
	data = append(data, metadataStartMarker...)

	encodeString := func(s string) []byte {
		return append([]byte{0x40 | byte(len(s))}, s...)
	}
	fields := []struct {
		key   string
		value []byte
	}{
		{"node_count", []byte{0xc1, 1}},
		{"record_size", []byte{0xa1, 24}},
		{"ip_version", []byte{0xa1, 6}},
		{"binary_format_major_version", []byte{0xa1, 2}},
		{"binary_format_minor_version", []byte{0xa0}},
		{"database_type", encodeString("Test")},
		{
			"description",
			append(append([]byte{0xe1}, encodeString("en")...), encodeString("Test")...),
		},
		{"languages", append([]byte{1, 4}, encodeString("en")...)},
		{"build_epoch", []byte{1, 2, 1}},
	}
	data = append(data, 0xe0|byte(len(fields)))
	for _, field := range fields {
		data = append(data, encodeString(field.key)...)
		data = append(data, field.value...)
	}

	reader, err := OpenBytes(data)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	require.NoError(t, reader.Verify())

	for _, ip := range []string{"::", "8000::"} {
		var value string
		require.NoError(t, reader.Lookup(netip.MustParseAddr(ip)).Decode(&value))
		require.Equal(t, "X", value)
	}

	for _, includeAliases := range []bool{false, true} {
		name := "default"
		var options []NetworksOption
		if includeAliases {
			name = "include aliases"
			options = []NetworksOption{IncludeAliasedNetworks()}
		}
		t.Run(name, func(t *testing.T) {
			for _, test := range []struct {
				name     string
				prefix   string
				expected []string
			}{
				{"Networks", "", []string{"::/1", "8000::/1"}},
				{"NetworksWithin all", "::/0", []string{"::/1", "8000::/1"}},
				{"NetworksWithin shared record", "8000::/1", []string{"8000::/1"}},
				{"NetworksWithin contained prefix", "9000::/16", []string{"8000::/1"}},
			} {
				t.Run(test.name, func(t *testing.T) {
					networks := reader.Networks(options...)
					if test.prefix != "" {
						networks = reader.NetworksWithin(
							netip.MustParsePrefix(test.prefix),
							options...,
						)
					}
					var prefixes []string
					for result := range networks {
						require.NoError(t, result.Err())
						var value string
						require.NoError(t, result.Decode(&value))
						assert.Equal(t, "X", value)
						prefixes = append(prefixes, result.Prefix().String())
					}
					assert.Equal(t, test.expected, prefixes)
				})
			}
		})
	}
}
