package decoder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyDataSectionRejectsInvalidUTF8(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "string", data: []byte{0x42, 0xff, 0xff}},
		{name: "map key", data: []byte{0xe1, 0x41, 0xff, 0xe0}},
		{name: "array element", data: []byte{0x01, 0x04, 0x42, 0xff, 0xff}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New(tt.data)
			err := d.VerifyDataSection(map[uint]bool{0: true})
			require.ErrorContains(t, err, "invalid UTF-8")
		})
	}
}

// verifyDataSectionTest is one VerifyDataSection case. An empty err means
// that verification must pass.
type verifyDataSectionTest struct {
	name    string
	data    []byte
	targets []uint
	err     string
}

func runVerifyDataSectionTests(t *testing.T, tests []verifyDataSectionTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offsets := map[uint]bool{}
			for _, target := range tt.targets {
				offsets[target] = true
			}
			d := New(tt.data)
			err := d.VerifyDataSection(offsets)
			if tt.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func TestVerifyDataSectionAcceptsNestedTargets(t *testing.T) {
	// {"a":{"b":"c"}}: the outer map is at 0, key "a" at 1, the inner map at
	// 3, key "b" at 4, and "c" at 6.
	record := []byte{0xe1, 0x41, 0x61, 0xe1, 0x41, 0x62, 0x41, 0x63}
	twoRecords := append(append([]byte(nil), record...), record...)
	// {"a": pointer to 5} at 0, with the pointer at 3, then "x" at 5.
	nestedPointer := []byte{0xe1, 0x41, 0x61, 0x20, 0x05, 0x41, 0x78}
	// "x" at 0, then a pointer to 0 at 2. The Perl writer writes a record
	// like this when a record value repeats a nested value.
	topLevelPointer := []byte{0x41, 0x78, 0x20, 0x00}

	middle := func(record, offset int) string {
		return fmt.Sprintf(
			"search tree points into the middle of a field in the data record at %d (offset %d)",
			record,
			offset,
		)
	}

	tests := []verifyDataSectionTest{
		{name: "nested map", data: record, targets: []uint{0, 3}},
		{name: "nested key", data: record, targets: []uint{0, 4}},
		{name: "nested string", data: record, targets: []uint{0, 3, 6}},
		{name: "nested in second record", data: twoRecords, targets: []uint{0, 8, 11}},
		{name: "nested pointer", data: nestedPointer, targets: []uint{0, 3, 5}},
		{name: "top-level pointer", data: topLevelPointer, targets: []uint{0, 2}},
		{name: "key payload", data: record, targets: []uint{0, 2}, err: middle(0, 2)},
		{name: "nested key payload", data: record, targets: []uint{0, 5}, err: middle(0, 5)},
		{name: "pointer payload", data: nestedPointer, targets: []uint{0, 4, 5}, err: middle(0, 4)},
		{
			name:    "payload in second record",
			data:    twoRecords,
			targets: []uint{0, 3, 8, 13},
			err:     middle(8, 13),
		},
		{name: "lowest bad target", data: record, targets: []uint{0, 2, 5}, err: middle(0, 2)},
		{
			name:    "nested in record that is not a target",
			data:    twoRecords,
			targets: []uint{0, 11},
			err:     "found data (map[a:map[b:c]]) at 8 that the search tree does not point to",
		},
		{
			name:    "past end",
			data:    record,
			targets: []uint{0, 8},
			err:     "found 1 pointers (of 2) in the search tree that we did not see in the data section",
		},
	}

	runVerifyDataSectionTests(t, tests)
}

func TestVerifyDataSectionChecksDataPointerTargets(t *testing.T) {
	tests := []verifyDataSectionTest{
		{
			// "x" at 0, then {"a": pointer to 0} at 2.
			name:    "backward to field",
			data:    []byte{0x41, 0x78, 0xe1, 0x41, 0x61, 0x20, 0x00},
			targets: []uint{0, 2},
		},
		{
			// {"a": pointer to 5} at 0, then "x" at 5.
			name:    "forward to field",
			data:    []byte{0xe1, 0x41, 0x61, 0x20, 0x05, 0x41, 0x78},
			targets: []uint{0, 5},
		},
		{
			// "AqAr" at 0, then {"a": pointer to 1} at 5. Offset 1 is in
			// the payload of "AqAr", but it decodes as "q".
			name:    "backward into payload",
			data:    []byte{0x44, 0x41, 0x71, 0x41, 0x72, 0xe1, 0x41, 0x61, 0x20, 0x01},
			targets: []uint{0, 5},
			err:     "data section pointer at offset 8 does not point to the start of a field (offset 1)",
		},
		{
			// {"a": pointer to 6} at 0, then "AqAr" at 5.
			name:    "forward into payload",
			data:    []byte{0xe1, 0x41, 0x61, 0x20, 0x06, 0x44, 0x41, 0x71, 0x41, 0x72},
			targets: []uint{0, 5},
			err:     "data section pointer at offset 3 does not point to the start of a field (offset 6)",
		},
	}

	runVerifyDataSectionTests(t, tests)
}

func TestVerifyMetadataChecksPointerTargets(t *testing.T) {
	pointerErr := func(offset, target int) string {
		return fmt.Sprintf(
			"metadata pointer at offset %d does not point to the start of a field (offset %d)",
			offset,
			target,
		)
	}
	// In each case, "AqAr" is a string whose payload offset decodes as "q".
	tests := []struct {
		name     string
		metadata []byte
		err      string
	}{
		{
			// {"a": "AqAr", "b": pointer to 3}, with the pointer at 10.
			name:     "backward to field",
			metadata: []byte{0xe2, 0x41, 'a', 0x44, 'A', 'q', 'A', 'r', 0x41, 'b', 0x20, 3},
		},
		{
			name:     "backward into payload",
			metadata: []byte{0xe2, 0x41, 'a', 0x44, 'A', 'q', 'A', 'r', 0x41, 'b', 0x20, 4},
			err:      pointerErr(10, 4),
		},
		{
			// {"a": pointer to 7, "b": "AqAr"}, with the pointer at 3.
			name:     "forward to field",
			metadata: []byte{0xe2, 0x41, 'a', 0x20, 7, 0x41, 'b', 0x44, 'A', 'q', 'A', 'r'},
		},
		{
			name:     "forward into payload",
			metadata: []byte{0xe2, 0x41, 'a', 0x20, 8, 0x41, 'b', 0x44, 'A', 'q', 'A', 'r'},
			err:      pointerErr(3, 8),
		},
		{
			// {"a": pointer to 5}, then "AqAr" at 5 after the map.
			name:     "after map to field",
			metadata: []byte{0xe1, 0x41, 'a', 0x20, 5, 0x44, 'A', 'q', 'A', 'r'},
		},
		{
			name:     "after map into payload",
			metadata: []byte{0xe1, 0x41, 'a', 0x20, 6, 0x44, 'A', 'q', 'A', 'r'},
			err:      pointerErr(3, 6),
		},
		{
			// {"a": "AqAr", "b": pointer to 12}, then {"c": pointer to 4} at 12.
			// The pointer at 15, after the map, points into the map.
			name: "from after map into payload",
			metadata: []byte{
				0xe2, 0x41, 'a', 0x44, 'A', 'q', 'A', 'r', 0x41, 'b', 0x20, 12,
				0xe1, 0x41, 'c', 0x20, 4,
			},
			err: pointerErr(15, 4),
		},
		{
			// {"a": "x"}, then "y", which no pointer reaches.
			name:     "unreferenced value after map",
			metadata: []byte{0xe1, 0x41, 'a', 0x41, 'x', 0x41, 'y'},
		},
		{
			// {"a": "x"}, then a string header with no payload.
			name:     "invalid value after map",
			metadata: []byte{0xe1, 0x41, 'a', 0x41, 'x', 0x42},
			err:      "invalid value after the metadata map (unexpected end of database) at offset of 5",
		},
		{
			name:     "invalid UTF-8 after map",
			metadata: []byte{0xe1, 0x41, 'a', 0x41, 'x', 0x41, 0xff},
			err:      "invalid value after the metadata map (invalid UTF-8 string) at offset of 5",
		},
		{
			name:     "pointer to pointer after map",
			metadata: []byte{0xe1, 0x41, 'a', 0x41, 'x', 0x20, 7, 0x20, 3},
			err:      "invalid pointer to pointer at offset 7",
		},
		{
			name:     "map key that is not a string after map",
			metadata: []byte{0xe1, 0x41, 'a', 0x41, 'x', 0xe1, 0xa1, 5, 0x41, 'y'},
			err:      "unexpected map key type: Uint16",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyMetadata(tt.metadata)
			if tt.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func TestVerifyMetadataSharesOneBudget(t *testing.T) {
	// {"a": "x"}, then a string of almost 1 MiB at 5, then pointers to it.
	// The map, the string, and one pointer fit in the 2 MiB payload budget of
	// the section, so a second pointer must fail. Pointers cannot multiply
	// the work.
	size := 1<<20 - 16
	extra := size - 65821 // A 31 size field adds 65821 to the 3 size bytes.
	header := []byte{
		0xe1,
		0x41,
		'a',
		0x41,
		'x',
		0x5f,
		byte(extra >> 16),
		byte(extra >> 8),
		byte(extra),
	}
	metadata := make([]byte, len(header)+size, len(header)+size+4)
	copy(metadata, header)

	metadata = append(metadata, 0x20, 5)
	require.NoError(t, VerifyMetadata(metadata))

	metadata = append(metadata, 0x20, 5)
	require.ErrorContains(t, VerifyMetadata(metadata), "exceeded maximum decoded record size")
}
