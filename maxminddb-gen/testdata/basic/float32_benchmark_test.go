package fixture

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func BenchmarkGeneratedFloat32(b *testing.B) {
	value := binary.BigEndian.AppendUint64([]byte{0x68}, math.Float64bits(1.5))
	data := singleFieldRecord("temperature", value)
	decoder := mmdbdata.NewDecoder(data, 0)
	var record Record
	b.ReportAllocs()
	for b.Loop() {
		if _, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor()); err != nil {
			b.Fatal(err)
		}
	}
	if record.Temperature != 1.5 {
		b.Fatalf("decoded temperature %v, want 1.5", record.Temperature)
	}
}
