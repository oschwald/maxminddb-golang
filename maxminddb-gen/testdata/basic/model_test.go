package fixture

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/internal/decoder"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

type reflectionNested struct {
	Name string `maxminddb:"name"`
}

type reflectionRecord struct {
	Name        Label             `maxminddb:"name"`
	Count       uint8             `maxminddb:"count"`
	Temperature float32           `maxminddb:"temperature"`
	Enabled     *bool             `maxminddb:"enabled"`
	Values      []uint16          `maxminddb:"values"`
	Lookup      map[string]string `maxminddb:"lookup"`
	Nested      reflectionNested  `maxminddb:"nested"`
	Custom      Custom            `maxminddb:"custom"`
	Bytes       []byte            `maxminddb:"bytes"`
}

func TestGeneratedRecordHappyPath(t *testing.T) {
	data := completeRecord()
	var record Record
	if err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0)); err != nil {
		t.Fatal(err)
	}
	if record.Name != "test" || record.Count != 42 || record.Temperature != 1.5 {
		t.Fatalf("unexpected scalars: %#v", record)
	}
	if record.Enabled == nil || !*record.Enabled {
		t.Fatalf("unexpected pointer: %#v", record.Enabled)
	}
	if !reflect.DeepEqual(record.Values, []uint16{7, 9}) ||
		!reflect.DeepEqual(record.Lookup, map[string]string{"en": "hi"}) ||
		record.Nested.Name != "inner" || record.Custom != "custom" ||
		!bytes.Equal(record.Bytes, []byte{1, 2, 3}) {
		t.Fatalf("unexpected containers: %#v", record)
	}

	existing := true
	reused := Record{Enabled: &existing}
	if err := reused.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0)); err != nil {
		t.Fatal(err)
	}
	if reused.Enabled != &existing {
		t.Fatal("existing pointer was not reused")
	}
	bytesOffset := bytes.Index(data, []byte{0x83, 1, 2, 3})
	data[bytesOffset+1] = 9
	if !bytes.Equal(record.Bytes, []byte{1, 2, 3}) {
		t.Fatalf("decoded bytes alias input: %v", record.Bytes)
	}
}

func TestGeneratedRecordWrongKinds(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       []byte
		customError bool
	}{
		{name: "string", key: "name", value: []byte{0x00, 0x07}},
		{name: "integer", key: "count", value: []byte{0x41, 'x'}},
		{name: "float", key: "temperature", value: []byte{0x00, 0x07}},
		{name: "pointer", key: "enabled", value: []byte{0x41, 'x'}},
		{name: "slice", key: "values", value: []byte{0xe0}},
		{name: "map", key: "lookup", value: []byte{0x00, 0x04}},
		{name: "nested", key: "nested", value: []byte{0x41, 'x'}},
		{name: "custom", key: "custom", value: []byte{0x00, 0x07}, customError: true},
		{name: "bytes", key: "bytes", value: []byte{0x41, 'x'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var record Record
			err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(singleFieldRecord(tt.key, tt.value), 0))
			if err == nil {
				t.Fatal("expected type error")
			}
			if !bytes.Contains([]byte(err.Error()), []byte("at offset")) {
				t.Fatalf("missing offset context: %v", err)
			}
			if tt.customError {
				if bytes.Contains([]byte(err.Error()), []byte("into type fixture.Custom")) {
					t.Fatalf("custom error was misattributed: %v", err)
				}
			} else if !bytes.Contains([]byte(err.Error()), []byte("cannot unmarshal")) {
				t.Fatalf("missing reflection-compatible type error: %v", err)
			}
		})
	}
}

func FuzzGeneratedReflectionParity(f *testing.F) {
	f.Add(completeRecord())
	f.Add(singleFieldRecord("name", []byte{0x41, 'x'}))
	f.Add(singleFieldRecord("count", []byte{0xa1, 42}))
	f.Add(singleFieldRecord("lookup", []byte{0xe1, 0x80, 0x30}))
	f.Add([]byte{0x41, 'x'})
	f.Fuzz(func(t *testing.T, data []byte) {
		var reflected reflectionRecord
		reflectionDecoder := decoder.New(data)
		reflectionErr := reflectionDecoder.Decode(0, &reflected)

		var generated Record
		generatedErr := generated.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
		if (reflectionErr == nil) != (generatedErr == nil) {
			t.Fatalf("error mismatch: reflection=%v generated=%v", reflectionErr, generatedErr)
		}
		if reflectionErr == nil {
			if !equalReflectionRecord(reflected, generated) {
				t.Fatalf("value mismatch: reflection=%#v generated=%#v", reflected, generated)
			}
			return
		}
	})
}

func equalReflectionRecord(reflected reflectionRecord, generated Record) bool {
	return reflected.Name == generated.Name &&
		reflected.Count == generated.Count &&
		reflected.Temperature == generated.Temperature &&
		reflect.DeepEqual(reflected.Enabled, generated.Enabled) &&
		reflect.DeepEqual(reflected.Values, generated.Values) &&
		reflect.DeepEqual(reflected.Lookup, generated.Lookup) &&
		reflected.Nested.Name == generated.Nested.Name &&
		reflected.Custom == generated.Custom &&
		bytes.Equal(reflected.Bytes, generated.Bytes)
}

func TestMalformedContainerDoesNotMutateCollections(t *testing.T) {
	type sliceState struct {
		values []uint16
		hidden uint16
	}
	tests := []struct {
		name   string
		data   []byte
		decode func([]byte) (any, error)
		want   any
	}{
		{
			name: "nil slice",
			data: malformedRecord("values", []byte{0x1e, 0x04, 0x02, 0xe3}),
			decode: func(data []byte) (any, error) {
				var record Record
				err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
				return record.Values, err
			},
			want: []uint16(nil),
		},
		{
			name: "existing slice",
			data: malformedRecord("values", []byte{0x1e, 0x04, 0x02, 0xe3}),
			decode: func(data []byte) (any, error) {
				record := Record{Values: []uint16{7}}
				err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
				return record.Values, err
			},
			want: []uint16{7},
		},
		{
			name: "existing slice with sufficient capacity",
			data: malformedRecord("values", []byte{0x1e, 0x04, 0x02, 0xe3}),
			decode: func(data []byte) (any, error) {
				backing := make([]uint16, 1024)
				backing[0] = 7
				backing[100] = 9
				record := Record{Values: backing[:1]}
				err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
				return sliceState{values: record.Values, hidden: backing[100]}, err
			},
			want: sliceState{values: []uint16{7}, hidden: 9},
		},
		{
			name: "nil map",
			data: malformedRecord("lookup", []byte{0xfe, 0x00, 0xe3}),
			decode: func(data []byte) (any, error) {
				var record Record
				err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
				return record.Lookup, err
			},
			want: map[string]string(nil),
		},
		{
			name: "existing map",
			data: malformedRecord("lookup", []byte{0xfe, 0x00, 0xe3}),
			decode: func(data []byte) (any, error) {
				record := Record{Lookup: map[string]string{"keep": "value"}}
				err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
				return record.Lookup, err
			},
			want: map[string]string{"keep": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.decode(tt.data)
			if err == nil {
				t.Fatal("expected malformed container error")
			}
			if !reflect.DeepEqual(tt.want, got) {
				t.Fatalf("destination mutated: got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func BenchmarkGeneratedLargeReusedSlice(b *testing.B) {
	for _, size := range []int{1023, 1024} {
		data := []byte{0xe1, 0x46, 'v', 'a', 'l', 'u', 'e', 's'}
		data = append(data, 0x1e, 0x04, byte((size-285)>>8), byte(size-285))
		data = append(data, bytes.Repeat([]byte{0xa0}, size)...)
		b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
			record := Record{Values: make([]uint16, 0, size)}
			decoder := mmdbdata.NewDecoder(data, 0)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func malformedRecord(key string, containerHeader []byte) []byte {
	data := []byte{0xe1, byte(0x40 + len(key))}
	data = append(data, key...)
	data = append(data, containerHeader...)
	return append(data, bytes.Repeat([]byte{0xff}, 1024)...)
}

func singleFieldRecord(key string, value []byte) []byte {
	data := []byte{0xe1, byte(0x40 + len(key))}
	data = append(data, key...)
	return append(data, value...)
}

func completeRecord() []byte {
	data := []byte{0xe9}
	appendField := func(key string, value ...byte) {
		data = append(data, byte(0x40+len(key)))
		data = append(data, key...)
		data = append(data, value...)
	}
	appendField("name", 0x44, 't', 'e', 's', 't')
	appendField("count", 0xa1, 42)
	appendField("temperature", 0x04, 0x08, 0x3f, 0xc0, 0, 0)
	appendField("enabled", 0x01, 0x07)
	appendField("values", 0x02, 0x04, 0xa1, 7, 0xa1, 9)
	appendField("lookup", 0xe1, 0x42, 'e', 'n', 0x42, 'h', 'i')
	appendField("nested", 0xe1, 0x44, 'n', 'a', 'm', 'e', 0x45, 'i', 'n', 'n', 'e', 'r')
	appendField("custom", 0x46, 'c', 'u', 's', 't', 'o', 'm')
	appendField("bytes", 0x83, 1, 2, 3)
	return data
}
