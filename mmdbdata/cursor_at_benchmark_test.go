package mmdbdata

import (
	"runtime"
	"testing"
)

// Construction is included only for the new_decoder cases. The cursor cases
// retain one decoder and warm its cache before timing reads at different offsets.
func BenchmarkCursorAtRecordReads(b *testing.B) {
	benchmarkCursorAtRecordReads(b, false)
}

func BenchmarkCursorAtRecordReadsParallel(b *testing.B) {
	benchmarkCursorAtRecordReads(b, true)
}

func benchmarkCursorAtRecordReads(b *testing.B, parallel bool) {
	for _, mode := range []struct {
		name   string
		cached bool
		fresh  bool
	}{
		{name: "cursor_uncached"},
		{name: "cursor_cached", cached: true},
		{name: "new_decoder_uncached", fresh: true},
		{name: "new_decoder_cached", cached: true, fresh: true},
	} {
		b.Run(mode.name, func(b *testing.B) {
			buffer := []byte{
				0x45,
				'h',
				'e',
				'l',
				'l',
				'o',
				0x45,
				'w',
				'o',
				'r',
				'l',
				'd',
				0x20,
				0,
				0x20,
				6,
			}
			offsets := []uint{0, 6, 12, 14}
			var options []DecoderOption
			if mode.cached {
				options = append(options, WithStringCache())
			}
			d := NewDecoder(buffer, 0, options...)
			read := func(offset uint) (string, error) {
				if mode.fresh {
					return NewDecoder(buffer, offset, options...).ReadString()
				}
				value, _, err := d.CursorAt(offset).ReadString()
				return value, err
			}
			if !mode.fresh {
				for range 3 {
					for _, offset := range offsets {
						if _, err := read(offset); err != nil {
							b.Fatal(err)
						}
					}
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			if parallel {
				b.RunParallel(func(pb *testing.PB) {
					var value string
					i := 0
					for pb.Next() {
						var err error
						value, err = read(offsets[i%len(offsets)])
						if err != nil {
							b.Error(err)
							return
						}
						i++
					}
					runtime.KeepAlive(value)
				})
			} else {
				var value string
				i := 0
				for b.Loop() {
					var err error
					value, err = read(offsets[i%len(offsets)])
					if err != nil {
						b.Fatal(err)
					}
					i++
				}
				runtime.KeepAlive(value)
			}
		})
	}
}
