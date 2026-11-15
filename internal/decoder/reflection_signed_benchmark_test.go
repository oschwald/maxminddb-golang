package decoder

import (
	"math"
	"testing"
)

func BenchmarkUnsignedToSigned(b *testing.B) {
	for _, tc := range []struct {
		name string
		data []byte
		want int64
	}{
		{"uint16", []byte{0xa2, 0x7f, 0xff}, math.MaxInt16},
		{"uint32", []byte{0xc4, 0x7f, 0xff, 0xff, 0xff}, math.MaxInt32},
		{"uint64", []byte{0x08, 0x02, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, math.MaxInt64},
	} {
		b.Run(tc.name, func(b *testing.B) {
			d := NewWithoutStringCache(tc.data)
			var result int64
			b.ReportAllocs()
			for b.Loop() {
				if err := d.Decode(0, &result); err != nil {
					b.Fatal(err)
				}
			}
			if result != tc.want {
				b.Fatalf("got %d, want %d", result, tc.want)
			}
		})
	}
}
