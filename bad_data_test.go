package maxminddb

import (
	"math/bits"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBadDataFixtures(t *testing.T) {
	tests := []struct {
		name             string
		openError        string
		verifyError      string
		lookupIP         string
		lookupError      string
		decodeError      string
		iterateCount     int
		iterateError     string
		iterateDecodeErr string
	}{
		{
			name:         "libmaxminddb/libmaxminddb-corrupt-search-tree.mmdb",
			verifyError:  "description - Expected: non-empty map",
			iterateCount: 2,
		},
		{
			name:             "libmaxminddb/libmaxminddb-deep-array-nesting.mmdb",
			verifyError:      "description - Expected: non-empty map",
			lookupIP:         "1.1.1.1",
			decodeError:      "exceeded maximum data structure depth",
			iterateCount:     2,
			iterateDecodeErr: "exceeded maximum data structure depth",
		},
		{
			name:             "libmaxminddb/libmaxminddb-deep-nesting.mmdb",
			verifyError:      "description - Expected: non-empty map",
			lookupIP:         "1.1.1.1",
			decodeError:      "exceeded maximum data structure depth",
			iterateCount:     2,
			iterateDecodeErr: "exceeded maximum data structure depth",
		},
		{
			name:         "libmaxminddb/libmaxminddb-empty-array-last-in-metadata.mmdb",
			verifyError:  "description - Expected: non-empty map",
			iterateCount: 2,
		},
		{
			name:         "libmaxminddb/libmaxminddb-empty-map-last-in-metadata.mmdb",
			verifyError:  "description - Expected: non-empty map",
			iterateCount: 2,
		},
		{
			name:      "libmaxminddb/libmaxminddb-offset-integer-overflow.mmdb",
			openError: "unexpected end of database",
		},
		{
			name:             "libmaxminddb/libmaxminddb-oversized-array.mmdb",
			verifyError:      "description - Expected: non-empty map",
			lookupIP:         "1.1.1.1",
			decodeError:      "unexpected end of database",
			iterateCount:     1,
			iterateDecodeErr: "unexpected end of database",
		},
		{
			name:             "libmaxminddb/libmaxminddb-oversized-map.mmdb",
			verifyError:      "description - Expected: non-empty map",
			lookupIP:         "1.1.1.1",
			decodeError:      "unexpected end of database",
			iterateCount:     1,
			iterateDecodeErr: "unexpected end of database",
		},
		{
			name:         "libmaxminddb/libmaxminddb-uint64-max-epoch.mmdb",
			verifyError:  "description - Expected: non-empty map",
			iterateCount: 2,
		},
		{
			name:      "maxminddb-golang/cyclic-data-structure.mmdb",
			openError: "path /record_size: unexpected end of database",
		},
		{
			name:      "maxminddb-golang/invalid-bytes-length.mmdb",
			openError: "path /description/en: unexpected end of database",
		},
		{
			name:      "maxminddb-golang/invalid-data-record-offset.mmdb",
			openError: "unexpected end of database",
		},
		{
			name:      "maxminddb-golang/invalid-map-key-length.mmdb",
			openError: "path /record_size: unexpected end of database",
		},
		{
			name:      "maxminddb-golang/invalid-string-length.mmdb",
			openError: "path /description: unexpected end of database",
		},
		{
			name:      "maxminddb-golang/metadata-is-an-uint128.mmdb",
			openError: "unexpected end of database",
		},
		{
			name:      "maxminddb-golang/unexpected-bytes.mmdb",
			openError: "cannot unmarshal [] ([]uint8) into type []string",
		},
		{
			name:         "maxminddb-python/bad-unicode-in-map-key.mmdb",
			verifyError:  "search tree is corrupt",
			lookupIP:     "0.0.0.0",
			lookupError:  "search tree is corrupt",
			decodeError:  "search tree is corrupt",
			iterateCount: 1,
			iterateError: "search tree is corrupt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip uint64-max-epoch test on 32-bit architectures: the test fails as
			// BuildEpoch is defined as a uint type (ie. 32 bits), leading to error:
			// cannot unmarshal 18446744073709551615 (uint64) into type uint
			if bits.UintSize == 32 {
				if tt.name == "libmaxminddb/libmaxminddb-uint64-max-epoch.mmdb" {
					t.Skip("Skipping on 32-bit architectures")
				}
			}

			reader, err := Open(badDataFile(tt.name))
			if tt.openError != "" {
				require.ErrorContains(t, err, tt.openError)
				return
			}

			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, reader.Close())
			})

			if tt.verifyError != "" {
				require.ErrorContains(t, reader.Verify(), tt.verifyError)
			}

			if tt.lookupIP == "" {
				return
			}

			result := reader.Lookup(netip.MustParseAddr(tt.lookupIP))
			if tt.lookupError != "" {
				require.ErrorContains(t, result.Err(), tt.lookupError)
			} else {
				require.NoError(t, result.Err())
			}

			var value any
			if tt.decodeError != "" {
				require.ErrorContains(t, result.Decode(&value), tt.decodeError)
			} else {
				require.NoError(t, result.Decode(&value))
			}

			if tt.iterateCount == 0 {
				return
			}

			count := 0
			sawIterError := false
			sawIterDecodeError := false
			for iterResult := range reader.Networks(IncludeNetworksWithoutData()) {
				count++
				if tt.iterateError != "" && iterResult.Err() != nil {
					require.ErrorContains(t, iterResult.Err(), tt.iterateError)
					sawIterError = true
					break
				}

				require.NoError(t, iterResult.Err())

				var iterValue any
				if tt.iterateDecodeErr != "" {
					err = iterResult.Decode(&iterValue)
					if err != nil {
						require.ErrorContains(t, err, tt.iterateDecodeErr)
						sawIterDecodeError = true
						break
					}
					continue
				}

				require.NoError(t, iterResult.Decode(&iterValue))
			}

			require.Equal(t, tt.iterateCount, count)
			if tt.iterateError != "" {
				require.True(t, sawIterError)
			}
			if tt.iterateDecodeErr != "" {
				require.True(t, sawIterDecodeError)
			}
		})
	}
}
