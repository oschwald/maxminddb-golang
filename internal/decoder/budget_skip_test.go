package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkBudgetedSkipInlineMap(b *testing.B) {
	// A small inline names map with pointer-backed values, as found in
	// fields omitted from a partial City destination.
	base := NewWithoutStringCache([]byte{
		0xe2, 0x42, 'e', 'n', 0x20, 0xff,
		0x42, 'f', 'r', 0x20, 0xff,
	})
	b.ReportAllocs()
	for b.Loop() {
		d := newBudgetedDecoder(&base)
		var err error
		cursorSizeSink, err = d.nextValueOffsetBudgetedSlow(0, 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestBudgetedSkipHeaders(t *testing.T) {
	for _, tt := range []struct {
		name     string
		data     []byte
		children uint32
	}{
		{"compact string", stringLeaf(28), 0},
		{"extended string", stringLeaf(29), 0},
		{"bytes", bytesLeaf(28), 0},
		{"uint16", []byte{0xa2, 1, 2}, 0},
		{"uint32", []byte{0xc4, 1, 2, 3, 4}, 0},
		{"bool", []byte{1, 7}, 0},
		{"float64", []byte{0x68, 0, 0, 0, 0, 0, 0, 0, 0}, 0},
		{"one-byte pointer", []byte{0x20, 0xff}, 0},
		{"two-byte pointer", []byte{0x28, 0xff, 0xff}, 0},
		{"three-byte pointer", []byte{0x30, 0xff, 0xff, 0xff}, 0},
		{"four-byte pointer", []byte{0x3f, 0xff, 0xff, 0xff, 0xff}, 0},
		{"map", []byte{0xe1, 0x41, 'k', 0xa0}, 2},
		{"extended map", []byte{1, 0, 0x41, 'k', 0xa0}, 2},
		{"slice", []byte{1, 4, 0xa0}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Pointer targets are deliberately invalid: skipping must never
			// follow them or charge their payloads.
			base := NewWithoutStringCache(tt.data)
			d := newBudgetedDecoder(&base)
			before := d.budgetRemaining
			next, err := d.nextValueOffsetBudgetedSlow(0, 1)
			require.NoError(t, err)
			require.Equal(t, uint(len(tt.data)), next)
			require.Equal(t, before-tt.children, d.budgetRemaining)
			require.Equal(t, uint32(decodePayloadBudgetBytes), d.payloadRemaining)
			for end := range len(tt.data) {
				base := NewWithoutStringCache(tt.data[:end])
				d := newBudgetedDecoder(&base)
				_, err := d.nextValueOffsetBudgetedSlow(0, 1)
				require.Error(t, err, "truncated at %d", end)
			}
		})
	}
	for _, data := range [][]byte{
		{0xa3, 0, 0, 0}, {0xc5, 0, 0, 0, 0, 0}, // Invalid integer sizes.
		{0x67, 0, 0, 0, 0, 0, 0, 0}, // Invalid float size.
		{2, 7}, {0, 8}, {0, 9},      // Invalid bool, float32, and kind.
	} {
		base := NewWithoutStringCache(data)
		d := newBudgetedDecoder(&base)
		_, err := d.nextValueOffsetBudgetedSlow(0, 1)
		require.Error(t, err)
	}
}
