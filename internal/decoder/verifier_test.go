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
			err:     "pointer at offset 8 does not point to the start of a field (offset 1)",
		},
		{
			// {"a": pointer to 6} at 0, then "AqAr" at 5.
			name:    "forward into payload",
			data:    []byte{0xe1, 0x41, 0x61, 0x20, 0x06, 0x44, 0x41, 0x71, 0x41, 0x72},
			targets: []uint{0, 5},
			err:     "pointer at offset 3 does not point to the start of a field (offset 6)",
		},
	}

	runVerifyDataSectionTests(t, tests)
}
