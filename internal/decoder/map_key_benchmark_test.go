package decoder

import (
	"fmt"
	"testing"
)

// BenchmarkMapKeyEncodings covers common encodings and the general fallback.
// Keep the same cases when comparing revisions so inline-key regressions do
// not get hidden by pointer-heavy real databases.
func BenchmarkMapKeyEncodings(b *testing.B) {
	for _, encoding := range []struct {
		name    string
		pointer []byte
		target  int
	}{
		{"inline", nil, 0},
		{"pointer1", []byte{0x20, 8}, 8},
		{"pointer2", []byte{0x28, 0, 8}, pointerBase2 + 8},
		{"pointer3", []byte{0x30, 0, 0, 8}, pointerBase3 + 8},
		{"pointer4", []byte{0x38, 0, 0, 0, 8}, 8},
	} {
		for _, size := range []int{2, 30, 285} {
			key := stringLeaf(size)
			data := make([]byte, encoding.target+len(key))
			copy(data, encoding.pointer)
			copy(data[encoding.target:], key)
			d := NewDataDecoderWithoutStringCache(data)
			cursor := NewDecoder(d, 0).Cursor()
			b.Run(fmt.Sprintf("%s/size%d", encoding.name, size), func(b *testing.B) {
				b.Run("Cursor", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if _, _, err := cursor.ReadMapKey(); err != nil {
							b.Fatal(err)
						}
					}
				})
				b.Run("Shared", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if _, _, _, err := d.decodeKeyAt(0); err != nil {
							b.Fatal(err)
						}
					}
				})
			})
		}
	}
}
