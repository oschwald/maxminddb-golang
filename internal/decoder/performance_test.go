package decoder

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

const testDataHex = "e142656e43466f6f" // Map with: "en"->"Foo"

var cursorSizeSink uint

var cursorSink Cursor

type benchmarkUnmarshaler struct{}

func (*benchmarkUnmarshaler) UnmarshalMaxMindDB(decoder *Decoder) error {
	_, err := decoder.ReadBool()
	return err
}

type benchmarkCursorUnmarshaler struct{}

func (*benchmarkCursorUnmarshaler) UnmarshalMaxMindDBCursor(
	cursor Cursor,
) (Cursor, error) {
	_, next, err := cursor.ReadBool()
	return next, err
}

type benchmarkLegacyMapKey string

func (key *benchmarkLegacyMapKey) UnmarshalMaxMindDB(decoder *Decoder) error {
	value, err := decoder.ReadString()
	if err == nil {
		*key = benchmarkLegacyMapKey(value)
	}
	return err
}

type benchmarkCursorMapKey string

func (key *benchmarkCursorMapKey) UnmarshalMaxMindDBCursor(cursor Cursor) (Cursor, error) {
	value, next, err := cursor.ReadString()
	if err == nil {
		*key = benchmarkCursorMapKey(value)
	}
	return next, err
}

func BenchmarkCursorCustomUnmarshal(b *testing.B) {
	decoder := NewDecoder(NewDataDecoder([]byte{0x00, 0x07}), 0)
	cursor := decoder.Cursor()

	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		var value benchmarkUnmarshaler
		for b.Loop() {
			var err error
			cursorSink, err = cursor.Unmarshal(&value)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("cursor", func(b *testing.B) {
		b.ReportAllocs()
		var value benchmarkCursorUnmarshaler
		for b.Loop() {
			var err error
			cursorSink, err = cursor.UnmarshalCursor(&value)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkStructDecoding tests the performance of struct decoding
// with the new optimized field access patterns.
func BenchmarkStructDecoding(b *testing.B) {
	// Create test data from field_precedence_test.go
	mmdbHex := testDataHex

	testBytes, err := hex.DecodeString(mmdbHex)
	if err != nil {
		b.Fatalf("Failed to decode hex: %v", err)
	}
	decoder := New(testBytes)

	// Test struct that exercises field access patterns
	type TestStruct struct {
		En string `maxminddb:"en"` // Simple field
	}

	for b.Loop() {
		var result TestStruct
		err := decoder.Decode(0, &result)
		if err != nil {
			b.Fatalf("Decode failed: %v", err)
		}
	}
}

// BenchmarkSimpleDecoding tests basic decoding performance.
func BenchmarkSimpleDecoding(b *testing.B) {
	// Simple test data - same as struct decoding
	mmdbHex := testDataHex

	testBytes, err := hex.DecodeString(mmdbHex)
	if err != nil {
		b.Fatalf("Failed to decode hex: %v", err)
	}
	decoder := New(testBytes)

	type TestStruct struct {
		En string `maxminddb:"en"`
	}

	for b.Loop() {
		var result TestStruct
		err := decoder.Decode(0, &result)
		if err != nil {
			b.Fatalf("Decode failed: %v", err)
		}
	}
}

func BenchmarkScalarDecoding(b *testing.B) {
	tests := []struct {
		name      string
		data      []byte
		newResult func() any
	}{
		{name: "bool", data: []byte{0x01, 0x07}, newResult: func() any { return new(bool) }},
		{
			name:      "string",
			data:      []byte{0x43, 'f', 'o', 'o'},
			newResult: func() any { return new(string) },
		},
		{name: "bytes", data: []byte{0x83, 1, 2, 3}, newResult: func() any { return new([]byte) }},
		{
			name:      "float32",
			data:      []byte{0x04, 0x08, 0, 0, 0, 0},
			newResult: func() any { return new(float32) },
		},
		{
			name:      "float64",
			data:      []byte{0x68, 0, 0, 0, 0, 0, 0, 0, 0},
			newResult: func() any { return new(float64) },
		},
		{
			name:      "int32",
			data:      []byte{0x04, 0x01, 0, 0, 0, 1},
			newResult: func() any { return new(int32) },
		},
		{name: "uint", data: []byte{0xc4, 0, 0, 0, 1}, newResult: func() any { return new(uint) }},
		{name: "uint16", data: []byte{0xa2, 0, 1}, newResult: func() any { return new(uint16) }},
		{
			name:      "uint32",
			data:      []byte{0xc4, 0, 0, 0, 1},
			newResult: func() any { return new(uint32) },
		},
		{
			name:      "uint64",
			data:      []byte{0x08, 0x02, 0, 0, 0, 0, 0, 0, 0, 1},
			newResult: func() any { return new(uint64) },
		},
		{
			name:      "any-string",
			data:      []byte{0x43, 'f', 'o', 'o'},
			newResult: func() any { return new(any) },
		},
		{
			name:      "any-uint32",
			data:      []byte{0xc4, 0, 0, 0, 1},
			newResult: func() any { return new(any) },
		},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			decoder := NewWithoutStringCache(test.data)
			result := test.newResult()
			b.ReportAllocs()
			for b.Loop() {
				if err := decoder.Decode(0, result); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkFieldLookup tests the performance of field lookup with
// the optimized field maps.
func BenchmarkFieldLookup(b *testing.B) {
	// Create a struct with many fields to test map performance
	type LargeStruct struct {
		Field01 string `maxminddb:"f01"`
		Field02 string `maxminddb:"f02"`
		Field03 string `maxminddb:"f03"`
		Field04 string `maxminddb:"f04"`
		Field05 string `maxminddb:"f05"`
		Field06 string `maxminddb:"f06"`
		Field07 string `maxminddb:"f07"`
		Field08 string `maxminddb:"f08"`
		Field09 string `maxminddb:"f09"`
		Field10 string `maxminddb:"f10"`
	}

	// Build the field cache
	var testStruct LargeStruct
	fields := cachedFields(reflect.ValueOf(testStruct))

	fieldNames := []string{"f01", "f02", "f03", "f04", "f05", "f06", "f07", "f08", "f09", "f10"}

	for b.Loop() {
		// Test field lookup performance
		for _, name := range fieldNames {
			_, exists := fields.namedFields[name]
			if !exists {
				b.Fatalf("Field %s not found", name)
			}
		}
	}
}

func BenchmarkLargeSliceDecoding(b *testing.B) {
	for _, size := range []int{64, 256, 1023, 1024} {
		data := benchmarkBoolSliceData(size)
		d := NewWithoutStringCache(data)

		b.Run(fmt.Sprintf("%d/new", size), func(b *testing.B) {
			for b.Loop() {
				var result []bool
				if err := d.Decode(0, &result); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("%d/reused", size), func(b *testing.B) {
			result := make([]bool, 0, size)
			for b.Loop() {
				if err := d.Decode(0, &result); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkBoolSliceData(size int) []byte {
	data := make([]byte, 0, 4+size*2)
	switch {
	case size < 29:
		data = append(data, byte(size), 0x04)
	case size < 285:
		data = append(data, 0x1d, 0x04, byte(size-29))
	default:
		encoded := size - 285
		data = append(data, 0x1e, 0x04, byte(encoded>>8), byte(encoded))
	}
	for range size {
		data = append(data, 0x00, 0x07) // false
	}
	return data
}

func BenchmarkLargeMapDecoding(b *testing.B) {
	for _, size := range []int{511, 512} {
		data := make([]byte, 0, 3+size*6)
		data = append(data, 0xfe, byte((size-285)>>8), byte(size-285))
		for i := range size {
			data = append(data, 0x44)
			data = append(data, fmt.Sprintf("%04x", i)...)
			data = append(data, 0x00, 0x07) // false
		}
		d := NewWithoutStringCache(data)

		b.Run(strconv.Itoa(size), func(b *testing.B) {
			for b.Loop() {
				var result map[string]bool
				if err := d.Decode(0, &result); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMapKeyDecoding(b *testing.B) {
	const size = 64
	data := make([]byte, 0, 2+size*6)
	data = append(data, 0xfd, byte(size-29))
	for i := range size {
		data = append(data, 0x44)
		data = append(data, fmt.Sprintf("%04x", i)...)
		data = append(data, 0x00, 0x07) // false
	}
	d := NewWithoutStringCache(data)

	b.Run("string/reused", func(b *testing.B) {
		result := make(map[string]bool, size)
		b.ReportAllocs()
		for b.Loop() {
			if err := d.Decode(0, &result); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("string-cached/reused", func(b *testing.B) {
		cachedDecoder := New(data)
		result := make(map[string]bool, size)
		b.ReportAllocs()
		for b.Loop() {
			if err := cachedDecoder.Decode(0, &result); err != nil {
				b.Fatal(err)
			}
		}
	})

	type namedString string
	b.Run("named-string/reused", func(b *testing.B) {
		result := make(map[namedString]bool, size)
		b.ReportAllocs()
		for b.Loop() {
			if err := d.Decode(0, &result); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCustomMapKeyDecoding(b *testing.B) {
	for _, size := range []int{1, 64} {
		data := benchmarkBoolMapData(size)
		decoder := NewWithoutStringCache(data)

		b.Run(fmt.Sprintf("legacy/%d", size), func(b *testing.B) {
			result := make(map[benchmarkLegacyMapKey]bool, size)
			b.ReportAllocs()
			for b.Loop() {
				if err := decoder.Decode(0, &result); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("cursor/%d", size), func(b *testing.B) {
			result := make(map[benchmarkCursorMapKey]bool, size)
			b.ReportAllocs()
			for b.Loop() {
				if err := decoder.Decode(0, &result); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkBoolMapData(size int) []byte {
	data := make([]byte, 0, 3+size*7)
	switch {
	case size < 29:
		data = append(data, 0xe0|byte(size))
	case size < 285:
		data = append(data, 0xfd, byte(size-29))
	default:
		encoded := size - 285
		data = append(data, 0xfe, byte(encoded>>8), byte(encoded))
	}
	for i := range size {
		data = append(data, 0x44)
		data = fmt.Appendf(data, "%04x", i)
		data = append(data, 0x00, 0x07) // false
	}
	return data
}

func BenchmarkCursorOpenSmallContainers(b *testing.B) {
	b.Run("map", func(b *testing.B) {
		decoder := NewDecoder(NewDataDecoder([]byte{0xe1, 0x41, 'a', 0x00, 0x07}), 0)
		for b.Loop() {
			entries, err := decoder.Cursor().Map()
			if err != nil {
				b.Fatal(err)
			}
			cursorSizeSink = entries.Size()
		}
	})

	b.Run("slice", func(b *testing.B) {
		// Slice is an extended kind, so its kind byte follows the control byte.
		decoder := NewDecoder(NewDataDecoder([]byte{0x01, 0x04, 0x00, 0x07}), 0)
		for b.Loop() {
			values, err := decoder.Cursor().Slice()
			if err != nil {
				b.Fatal(err)
			}
			cursorSizeSink, err = values.Size()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCursorIterateSmallContainers(b *testing.B) {
	b.Run("map", func(b *testing.B) {
		decoder := NewDecoder(NewDataDecoder([]byte{0xe1, 0x41, 'a', 0x00, 0x07}), 0)
		cursor := decoder.Cursor()
		b.ReportAllocs()
		for b.Loop() {
			entries, err := cursor.Map()
			if err != nil {
				b.Fatal(err)
			}
			key, valueCursor, ok := entries.Next(Cursor{})
			if !ok || len(key) != 1 || key[0] != 'a' {
				b.Fatal("map entry was not returned")
			}
			_, next, err := valueCursor.ReadBool()
			if err != nil {
				b.Fatal(err)
			}
			if _, _, ok := entries.Next(next); ok {
				b.Fatal("map returned too many entries")
			}
			cursorSink, err = entries.End()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("slice", func(b *testing.B) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x01, 0x04, 0x00, 0x07}), 0)
		cursor := decoder.Cursor()
		b.ReportAllocs()
		for b.Loop() {
			values, err := cursor.Slice()
			if err != nil {
				b.Fatal(err)
			}
			index, valueCursor, ok := values.Next(Cursor{})
			if !ok || index != 0 {
				b.Fatal("slice element was not returned")
			}
			_, next, err := valueCursor.ReadBool()
			if err != nil {
				b.Fatal(err)
			}
			if _, _, ok := values.Next(next); ok {
				b.Fatal("slice returned too many elements")
			}
			cursorSink, err = values.End()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkMapReaderSizeForAllocation(b *testing.B) {
	decoder := NewDecoder(NewDataDecoder([]byte{0xe1, 0x40, 0x00, 0x07}), 0)
	entries, err := decoder.Cursor().MapReader()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var ok bool
		cursorSizeSink, ok = entries.SizeForAllocation()
		if !ok {
			b.Fatal("small map did not use the allocation fast path")
		}
	}
}

func BenchmarkMapReaderEnd(b *testing.B) {
	decoder := NewDecoder(NewDataDecoder([]byte{0xe1, 0x40, 0x00, 0x07}), 0)
	entries, err := decoder.Cursor().MapReader()
	if err != nil {
		b.Fatal(err)
	}
	_, valueCursor, err := entries.First().ReadMapKey()
	if err != nil {
		b.Fatal(err)
	}
	_, next, err := valueCursor.ReadBool()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cursorSink, err = entries.End(next)
		if err != nil {
			b.Fatal(err)
		}
	}
}
