package decoder

import (
	"reflect"
	"testing"
)

func BenchmarkUintptrOffset(b *testing.B) {
	type Offset uintptr

	// Pointer at zero to the string at offset 2.
	data := []byte{0x20, 0x02, 0x43, 'f', 'o', 'o'}
	const largeOffset = 4096
	largeData := make([]byte, largeOffset+4)
	copy(largeData, []byte{0x28, 0x08, 0x00})
	copy(largeData[largeOffset:], data[2:])
	tests := []struct {
		name   string
		data   []byte
		offset uint
		result any
		want   uint64
	}{
		{"plain/inline", data, 2, new(uintptr), 2},
		{"plain/pointer", data, 0, new(uintptr), 2},
		{"named/inline", data, 2, new(Offset), 2},
		{"named/pointer", data, 0, new(Offset), 2},
		{"plain/large-inline", largeData, largeOffset, new(uintptr), largeOffset},
		{"plain/large-pointer", largeData, 0, new(uintptr), largeOffset},
		{"named/large-inline", largeData, largeOffset, new(Offset), largeOffset},
		{"named/large-pointer", largeData, 0, new(Offset), largeOffset},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			d := NewWithoutStringCache(tt.data)
			b.ReportAllocs()
			for b.Loop() {
				if err := d.Decode(tt.offset, tt.result); err != nil {
					b.Fatal(err)
				}
			}
			if got := reflect.ValueOf(tt.result).Elem().Uint(); got != tt.want {
				b.Fatalf("decoded offset %d, want %d", got, tt.want)
			}
		})
	}
}
