package mmdbdata

import (
	"strconv"
	"testing"
)

var benchmarkDecoder *Decoder

var benchmarkString string

func BenchmarkDecoderConstruction(b *testing.B) {
	benchmarkDecoderConstruction(b)
}

func BenchmarkDecoderConstructionWithStringCache(b *testing.B) {
	benchmarkDecoderConstruction(b, WithStringCache())
}

// Each operation includes construction and the given number of string reads.
func BenchmarkDecoderReuse(b *testing.B) {
	benchmarkDecoderReuse(b)
}

func BenchmarkDecoderReuseWithStringCache(b *testing.B) {
	benchmarkDecoderReuse(b, WithStringCache())
}

// Construction is excluded to show the cost of retaining a standalone cursor.
func BenchmarkStandaloneCursorReuse(b *testing.B) {
	benchmarkStandaloneCursorReuse(b)
}

func BenchmarkStandaloneCursorReuseWithStringCache(b *testing.B) {
	benchmarkStandaloneCursorReuse(b, WithStringCache())
}

func benchmarkDecoderConstruction(b *testing.B, options ...DecoderOption) {
	buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o'}
	b.Run("NoRead", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkDecoder = NewDecoder(buffer, 0, options...)
		}
	})
	b.Run("String", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var err error
			benchmarkString, err = NewDecoder(buffer, 0, options...).ReadString()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Bool", func(b *testing.B) {
		buffer := []byte{0x01, 0x07}
		b.ReportAllocs()
		for b.Loop() {
			if _, err := NewDecoder(buffer, 0, options...).ReadBool(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func benchmarkDecoderReuse(b *testing.B, options ...DecoderOption) {
	buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o'}
	for _, reads := range []int{1, 32, 1024, 16384} {
		b.Run(strconv.Itoa(reads), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				decoder := NewDecoder(buffer, 0, options...)
				for range reads {
					var err error
					benchmarkString, err = decoder.ReadString()
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func benchmarkStandaloneCursorReuse(b *testing.B, options ...DecoderOption) {
	cursor := NewDecoder([]byte{0x45, 'h', 'e', 'l', 'l', 'o'}, 0, options...).Cursor()
	for range 3 {
		if _, _, err := cursor.ReadString(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		var err error
		benchmarkString, _, err = cursor.ReadString()
		if err != nil {
			b.Fatal(err)
		}
	}
}
