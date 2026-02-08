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
