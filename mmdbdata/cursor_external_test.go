package mmdbdata_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func mapReaderLen(reader mmdbdata.MapReader) uint { return reader.Len() }

func TestMapReaderIsPubliclyNameable(t *testing.T) {
	decoder := mmdbdata.NewDecoder([]byte{0xe0}, 0)
	reader, err := decoder.Cursor().MapReader()
	require.NoError(t, err)

	require.Zero(t, mapReaderLen(reader))
}
