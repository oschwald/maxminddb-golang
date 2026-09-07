package maxminddb

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var verifiedOffsetsSink map[uint]bool

func amplifyingSearchTree(nodeCount uint) []byte {
	buffer := make([]byte, 0, int(nodeCount*6))
	appendPointer := func(pointer uint) {
		buffer = append(buffer, byte(pointer>>16), byte(pointer>>8), byte(pointer))
	}
	for node := range nodeCount {
		child := node + 1
		if child == nodeCount {
			child += dataSectionSeparatorSize
		}
		appendPointer(child)
		appendPointer(child)
	}
	return buffer
}

func BenchmarkVerifySearchTree(b *testing.B) {
	reader, err := Open(testFile("GeoIP2-Country-Test.mmdb"))
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, reader.Close()) })
	v := verifier{reader: reader}

	b.ReportAllocs()
	for b.Loop() {
		verifiedOffsetsSink, err = v.verifySearchTree()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestVerifySearchTreeBoundsDAGTraversal(t *testing.T) {
	const nodeCount uint = 32

	v := verifier{reader: &Reader{
		buffer:          amplifyingSearchTree(nodeCount),
		dataSectionSize: 1,
		nodeOffsetMult:  6,
		Metadata: Metadata{
			IPVersion:  4,
			NodeCount:  nodeCount,
			RecordSize: 24,
		},
	}}
	offsets, stateCount, err := v.verifySearchTreeWithStateCount()
	require.NoError(t, err)
	assert.Equal(t, map[uint]bool{0: true}, offsets)
	assert.Equal(t, nodeCount, stateCount)
}

func TestVerifySearchTreeRejectsCycleWithinStateBound(t *testing.T) {
	v := verifier{reader: &Reader{
		buffer:          make([]byte, 6),
		dataSectionSize: 1,
		nodeOffsetMult:  6,
		Metadata: Metadata{
			IPVersion:  4,
			NodeCount:  1,
			RecordSize: 24,
		},
	}}
	_, stateCount, err := v.verifySearchTreeWithStateCount()
	require.ErrorContains(t, err, "cycle")
	assert.Equal(t, uint(1), stateCount)
}

func TestVerifySearchTreeRejectsOverlongPathWithinStateBound(t *testing.T) {
	const nodeCount uint = 33

	v := verifier{reader: &Reader{
		buffer:          amplifyingSearchTree(nodeCount),
		dataSectionSize: 1,
		nodeOffsetMult:  6,
		Metadata: Metadata{
			IPVersion:  4,
			NodeCount:  nodeCount,
			RecordSize: 24,
		},
	}}
	_, stateCount, err := v.verifySearchTreeWithStateCount()
	require.ErrorContains(t, err, "bit depth 128")
	assert.Equal(t, uint(32), stateCount)
}

func TestVerifySearchTreeChecksMemoizedSubtreeHeight(t *testing.T) {
	tests := []struct {
		name      string
		ipVersion uint
		nodeCount uint
		wantError bool
	}{
		{name: "IPv4 at depth limit", ipVersion: 4, nodeCount: 32},
		{name: "IPv4 beyond depth limit", ipVersion: 4, nodeCount: 33, wantError: true},
		{name: "IPv6 at depth limit", ipVersion: 6, nodeCount: 128},
		{name: "IPv6 beyond depth limit", ipVersion: 6, nodeCount: 129, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buffer := amplifyingSearchTree(test.nodeCount)
			setChildren := func(node, left, right uint) {
				for i, pointer := range [2]uint{left, right} {
					offset := node*6 + uint(i)*3
					buffer[offset] = byte(pointer >> 16)
					buffer[offset+1] = byte(pointer >> 8)
					buffer[offset+2] = byte(pointer)
				}
			}
			// The left branch first caches the height of the two-node subtree
			// 1 -> 2 -> data. The right branch takes a longer chain back to node
			// 1, which is still within the address depth limit. Its cached height
			// must count toward the full path, even without revisiting node 2.
			setChildren(0, 1, 3)
			dataPointer := test.nodeCount + dataSectionSeparatorSize
			setChildren(2, dataPointer, dataPointer)
			setChildren(test.nodeCount-1, 1, 1)

			v := verifier{reader: &Reader{
				buffer:          buffer,
				dataSectionSize: 1,
				nodeOffsetMult:  6,
				Metadata: Metadata{
					IPVersion:  test.ipVersion,
					NodeCount:  test.nodeCount,
					RecordSize: 24,
				},
			}}
			offsets, stateCount, err := v.verifySearchTreeWithStateCount()
			if test.wantError {
				require.ErrorContains(t, err, "path exceeds 128 bits")
			} else {
				require.NoError(t, err)
				assert.Equal(t, map[uint]bool{0: true}, offsets)
			}
			assert.Equal(t, test.nodeCount, stateCount)
		})
	}
}

func TestVerifyRejectsInvalidPointerInUnknownMetadataField(t *testing.T) {
	data, err := os.ReadFile(testFile("MaxMind-DB-test-ipv4-24.mmdb"))
	require.NoError(t, err)

	markerOffset := bytes.LastIndex(data, metadataStartMarker)
	require.NotEqual(t, -1, markerOffset)
	metadataOffset := markerOffset + len(metadataStartMarker)
	require.Equal(t, byte(0xE9), data[metadataOffset])
	data[metadataOffset] = 0xEA
	data = append(data, 0x47)
	data = append(data, "unknown"...)
	data = append(data, 0x27, 0xFF)

	reader, err := OpenBytes(data)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	require.Error(t, reader.Verify())
}

func TestVerifyOnGoodDatabases(t *testing.T) {
	databases := []string{
		"GeoIP2-Anonymous-IP-Test.mmdb",
		"GeoIP2-City-Test.mmdb",
		"GeoIP2-Connection-Type-Test.mmdb",
		"GeoIP2-Country-Test.mmdb",
		"GeoIP2-Domain-Test.mmdb",
		"GeoIP2-ISP-Test.mmdb",
		"GeoIP2-Precision-Enterprise-Test.mmdb",
		"MaxMind-DB-no-ipv4-search-tree.mmdb",
		"MaxMind-DB-string-value-entries.mmdb",
		"MaxMind-DB-test-decoder.mmdb",
		"MaxMind-DB-test-ipv4-24.mmdb",
		"MaxMind-DB-test-ipv4-28.mmdb",
		"MaxMind-DB-test-ipv4-32.mmdb",
		"MaxMind-DB-test-ipv6-24.mmdb",
		"MaxMind-DB-test-ipv6-28.mmdb",
		"MaxMind-DB-test-ipv6-32.mmdb",
		"MaxMind-DB-test-mixed-24.mmdb",
		"MaxMind-DB-test-mixed-28.mmdb",
		"MaxMind-DB-test-mixed-32.mmdb",
		"MaxMind-DB-test-nested.mmdb",
	}

	for _, database := range databases {
		t.Run(database, func(t *testing.T) {
			reader, err := Open(testFile(database))
			require.NoError(t, err)

			require.NoError(
				t,
				reader.Verify(),
				"Received error (%v) when verifying %v",
				err,
				database,
			)
		})
	}
}

func TestVerifyOnBrokenDatabases(t *testing.T) {
	databases := []string{
		"GeoIP2-City-Test-Broken-Double-Format.mmdb",
		"MaxMind-DB-test-broken-pointers-24.mmdb",
		"MaxMind-DB-test-broken-search-tree-24.mmdb",
	}

	for _, database := range databases {
		reader, err := Open(testFile(database))
		require.NoError(t, err)
		assert.Error(t, reader.Verify(),
			"Did not receive expected error when verifying %v", database,
		)
	}
}

func TestVerifyMetadataRejectsInvalidUTF8(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Metadata)
		errMsg string
	}{
		{
			name: "database type",
			mutate: func(metadata *Metadata) {
				metadata.DatabaseType = "Test\xff"
			},
			errMsg: "database_type contains invalid UTF-8",
		},
		{
			name: "description key",
			mutate: func(metadata *Metadata) {
				metadata.Description = map[string]string{"\xff": "test"}
			},
			errMsg: "description contains invalid UTF-8",
		},
		{
			name: "description value",
			mutate: func(metadata *Metadata) {
				metadata.Description = map[string]string{"en": "test\xff"}
			},
			errMsg: "description contains invalid UTF-8",
		},
		{
			name: "languages",
			mutate: func(metadata *Metadata) {
				metadata.Languages = []string{"en\xff"}
			},
			errMsg: "languages contains invalid UTF-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := Metadata{
				Description:              map[string]string{"en": "test"},
				DatabaseType:             "Test",
				BinaryFormatMajorVersion: 2,
				BinaryFormatMinorVersion: 0,
				IPVersion:                4,
				NodeCount:                1,
				RecordSize:               24,
			}
			tt.mutate(&metadata)

			v := verifier{reader: &Reader{Metadata: metadata}}
			require.ErrorContains(t, v.verifyMetadata(), tt.errMsg)
		})
	}
}

func TestVerifyDataSectionSeparatorOutOfBounds(t *testing.T) {
	v := verifier{reader: &Reader{
		buffer: []byte{0x00},
		Metadata: Metadata{
			NodeCount:  1,
			RecordSize: 32,
		},
	}}

	require.NotPanics(t, func() {
		err := v.verifyDataSectionSeparator()
		assert.Error(t, err)
		assert.ErrorContains(t, err, "unexpected end of database")
	})
}

func TestSearchTreeSizeBytesUsesSafeMultiplicationOrder(t *testing.T) {
	nodeCount := ^uint(0)/16 + 1

	assert.Equal(t, nodeCount*8, searchTreeSizeBytes(nodeCount, 32))
	assert.NotEqual(t, (nodeCount*32)/4, searchTreeSizeBytes(nodeCount, 32))
}
