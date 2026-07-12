package decoder

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

const testDataHex = "e142656e43466f6f" // Map with: "en"->"Foo"

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
	for _, size := range []int{1023, 1024} {
		data := make([]byte, 0, 4+size*2)
		data = append(data, 0x1e, 0x04, byte((size-285)>>8), byte(size-285))
		for range size {
			data = append(data, 0x00, 0x07) // false
		}
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
