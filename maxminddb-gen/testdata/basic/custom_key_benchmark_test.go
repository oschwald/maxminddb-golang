package fixture

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func BenchmarkGeneratedCustomKeyMapReused(b *testing.B) {
	for _, size := range []int{64, 511, 512} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			decoder := mmdbdata.NewDecoder(benchmarkCustomKeyRecordData(size), 0)
			var record BenchmarkCustomKeyRecord
			if _, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor()); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkCustomKeyRecordData(size int) []byte {
	data := make([]byte, 0, 11+7*size)
	data = append(data, 0xe1, 0x46, 'v', 'a', 'l', 'u', 'e', 's')
	switch {
	case size < 29:
		data = append(data, byte(0xe0+size))
	case size < 285:
		data = append(data, 0xfd, byte(size-29))
	default:
		extended := size - 285
		data = append(data, 0xfe, byte(extended>>8), byte(extended))
	}
	for i := range size {
		data = append(data, 0x44)
		data = append(data, fmt.Sprintf("%04x", i)...)
		data = append(data, 0x00, 0x07)
	}
	return data
}
