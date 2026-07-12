package maxminddb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
