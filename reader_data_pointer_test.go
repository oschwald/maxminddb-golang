package maxminddb

import (
	"bytes"
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookupRejectsDataPointersOutsideDataSection(t *testing.T) {
	buffer, err := os.ReadFile(testFile("MaxMind-DB-test-ipv4-24.mmdb"))
	require.NoError(t, err)

	reader, err := OpenBytes(buffer)
	require.NoError(t, err)

	metadataStart := bytes.LastIndex(buffer, metadataStartMarker)
	require.NotEqual(t, -1, metadataStart)

	searchTreeSize := int(searchTreeSizeBytes(
		reader.Metadata.NodeCount,
		reader.Metadata.RecordSize,
	))
	dataSectionLen := metadataStart - searchTreeSize - dataSectionSeparatorSize
	require.Positive(t, dataSectionLen)

	badPointer := uint(dataSectionLen) + reader.Metadata.NodeCount + dataSectionSeparatorSize
	require.Greater(t, badPointer, reader.Metadata.NodeCount)
	require.NoError(t, reader.Close())

	buffer[0] = byte(badPointer >> 16)
	buffer[1] = byte(badPointer >> 8)
	buffer[2] = byte(badPointer)

	reader, err = OpenBytes(buffer)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, reader.Close())
	})

	result := reader.Lookup(netip.MustParseAddr("0.0.0.1"))
	require.EqualError(t, result.Err(), "the MaxMind DB file's search tree is corrupt")

	var record any
	require.EqualError(t, result.Decode(&record), "the MaxMind DB file's search tree is corrupt")
}
