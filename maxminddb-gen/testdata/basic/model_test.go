package fixture

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/internal/decoder"
	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

type reflectionNested struct {
	Name string `maxminddb:"name"`
}

type reflectionByteShapes struct {
	Pointer *[]byte           `maxminddb:"pointer"`
	Slices  [][]byte          `maxminddb:"slices"`
	Lookup  map[string][]byte `maxminddb:"lookup"`
}

type reflectionRecord struct {
	Name        Label             `maxminddb:"name"`
	Count       uint8             `maxminddb:"count"`
	Count32     uint32            `maxminddb:"count32"`
	Count64     uint64            `maxminddb:"count64"`
	Temperature float32           `maxminddb:"temperature"`
	Enabled     *bool             `maxminddb:"enabled"`
	Values      []uint16          `maxminddb:"values"`
	Lookup      map[string]string `maxminddb:"lookup"`
	Names       map[Code]uint8    `maxminddb:"names"`
	Nested      reflectionNested  `maxminddb:"nested"`
	Custom      Custom            `maxminddb:"custom"`
	Bytes       []byte            `maxminddb:"bytes"`
}

type reflectionFloat64Record struct {
	Value float64 `maxminddb:"value"`
}

type reflectionDualInterfaceRecord struct {
	Value dualCustom `maxminddb:"value"`
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
	lookupAlias := map[string]string{"en": "old", "stale": "keep"}
	reused := Record{Enabled: &existing, Lookup: lookupAlias}
	if err := reused.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0)); err != nil {
		t.Fatal(err)
	}
	if reused.Enabled != &existing {
		t.Fatal("existing pointer was not reused")
	}
	if lookupAlias["en"] != "hi" || lookupAlias["stale"] != "keep" {
		t.Fatalf("existing map alias did not observe reuse: %#v", lookupAlias)
	}
	if reused.Lookup["stale"] != "keep" {
		t.Fatalf("absent existing map key was removed: %#v", reused.Lookup)
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

func TestGeneratedNestedWrongKindErrorParity(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value []byte
	}{
		{
			name:  "slice element",
			field: "values",
			value: []byte{0x01, 0x04, 0x41, 'x'},
		},
		{
			name:  "map value",
			field: "lookup",
			value: []byte{0xe1, 0x42, 'e', 'n', 0x00, 0x07},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reflectionErr, generatedErr := decodeReflectionAndGenerated(
				singleFieldRecord(tt.field, tt.value),
			)
			requireDecodeErrorParity(t, reflectionErr, generatedErr)

			summary := summarizeDecodeError(generatedErr)
			if !summary.unmarshalType || summary.unexpectedKind || summary.invalidDatabase {
				t.Fatalf("unexpected generated error categories: %#v: %v", summary, generatedErr)
			}
		})
	}
}

func TestGeneratedBytesReflectionCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		value   []byte
		initial []byte
		want    []byte
	}{
		{
			name:  "bytes",
			value: []byte{0x83, 1, 2, 3},
			want:  []byte{1, 2, 3},
		},
		{
			name:  "integer slice",
			value: []byte{0x02, 0x04, 0xa1, 1, 0xa1, 2},
			want:  []byte{1, 2},
		},
		{
			name:  "empty bytes with nil destination",
			value: []byte{0x80},
			want:  []byte{},
		},
		{
			name:    "empty bytes with allocated destination",
			value:   []byte{0x80},
			initial: []byte{9},
			want:    []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := append(singleFieldRecord("bytes", tt.value), 0x45, 'a', 'f', 't', 'e', 'r')
			reflected := reflectionRecord{Bytes: append([]byte(nil), tt.initial...)}
			reflectionDecoder := decoder.New(data)
			if err := reflectionDecoder.Decode(0, &reflected); err != nil {
				t.Fatalf("reflection decode: %v", err)
			}

			generated := Record{Bytes: append([]byte(nil), tt.initial...)}
			next, err := generated.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
			if err != nil {
				t.Fatalf("generated decode: %v", err)
			}
			if !reflect.DeepEqual(reflected.Bytes, tt.want) ||
				!reflect.DeepEqual(generated.Bytes, tt.want) {
				t.Fatalf(
					"decoded bytes: reflection=%v generated=%v want=%v",
					reflected.Bytes,
					generated.Bytes,
					tt.want,
				)
			}
			assertTrailingString(t, next, "after")
		})
	}

	t.Run("integer overflow", func(t *testing.T) {
		data := singleFieldRecord("bytes", []byte{0x01, 0x04, 0xa2, 0x01, 0x00})
		var reflected reflectionRecord
		reflectionDecoder := decoder.New(data)
		reflectionErr := reflectionDecoder.Decode(0, &reflected)
		var generated Record
		_, generatedErr := generated.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
		var reflectionTypeError mmdberrors.UnmarshalTypeError
		var generatedTypeError mmdberrors.UnmarshalTypeError
		if !errors.As(reflectionErr, &reflectionTypeError) ||
			!errors.As(generatedErr, &generatedTypeError) {
			t.Fatalf(
				"overflow error mismatch: reflection=%v generated=%v",
				reflectionErr,
				generatedErr,
			)
		}
		var mismatch mmdbdata.UnexpectedKindError
		if errors.As(generatedErr, &mismatch) {
			t.Fatalf("generated overflow retained kind mismatch: %v", generatedErr)
		}
		if !bytes.Equal(reflected.Bytes, generated.Bytes) {
			t.Fatalf(
				"overflow destination mismatch: reflection=%v generated=%v",
				reflected.Bytes,
				generated.Bytes,
			)
		}
	})
}

func TestGeneratedNestedBytesReflectionCompatibility(t *testing.T) {
	pointerValues := []struct {
		name  string
		value []byte
	}{
		{name: "bytes", value: []byte{0x82, 1, 2}},
		{name: "integer slice", value: []byte{0x02, 0x04, 0xa1, 1, 0xa1, 2}},
	}
	for _, tt := range pointerValues {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte{0xe3}
			data = append(data, 0x47, 'p', 'o', 'i', 'n', 't', 'e', 'r')
			data = append(data, tt.value...)
			data = append(data,
				0x46, 's', 'l', 'i', 'c', 'e', 's',
				0x02, 0x04,
				0x82, 3, 4,
				0x02, 0x04, 0xa1, 5, 0xa1, 6,
				0x46, 'l', 'o', 'o', 'k', 'u', 'p',
				0xe2,
				0x45, 'b', 'y', 't', 'e', 's', 0x82, 7, 8,
				0x45, 'a', 'r', 'r', 'a', 'y', 0x02, 0x04, 0xa1, 9, 0xa1, 10,
				0x45, 'a', 'f', 't', 'e', 'r',
			)

			var reflected reflectionByteShapes
			reflectionDecoder := decoder.New(data)
			if err := reflectionDecoder.Decode(0, &reflected); err != nil {
				t.Fatalf("reflection decode: %v", err)
			}
			var generated ByteShapes
			next, err := generated.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
			if err != nil {
				t.Fatalf("generated decode: %v", err)
			}
			want := ByteShapes{
				Pointer: byteSlicePointer(1, 2),
				Slices:  [][]byte{{3, 4}, {5, 6}},
				Lookup:  map[string][]byte{"bytes": {7, 8}, "array": {9, 10}},
			}
			if !reflect.DeepEqual(reflected, reflectionByteShapes(want)) {
				t.Fatalf("unexpected reflection value: %#v", reflected)
			}
			if !reflect.DeepEqual(generated, want) {
				t.Fatalf("unexpected generated value: %#v", generated)
			}
			assertTrailingString(t, next, "after")
		})
	}

	overflow := []byte{0x01, 0x04, 0xa2, 0x01, 0x00}
	errorCases := []struct {
		name  string
		key   string
		value []byte
	}{
		{name: "pointer", key: "pointer", value: overflow},
		{name: "slice", key: "slices", value: append([]byte{0x01, 0x04}, overflow...)},
		{
			name:  "map",
			key:   "lookup",
			value: append([]byte{0xe1, 0x41, 'x'}, overflow...),
		},
	}
	for _, tt := range errorCases {
		t.Run(tt.name+" overflow", func(t *testing.T) {
			data := singleFieldRecord(tt.key, tt.value)
			var reflected reflectionByteShapes
			reflectionDecoder := decoder.New(data)
			reflectionErr := reflectionDecoder.Decode(0, &reflected)
			var generated ByteShapes
			_, generatedErr := generated.UnmarshalMaxMindDBCursor(
				mmdbdata.NewDecoder(data, 0).Cursor(),
			)
			var reflectionTypeError mmdberrors.UnmarshalTypeError
			var generatedTypeError mmdberrors.UnmarshalTypeError
			if !errors.As(reflectionErr, &reflectionTypeError) ||
				!errors.As(generatedErr, &generatedTypeError) {
				t.Fatalf(
					"overflow error mismatch: reflection=%v generated=%v",
					reflectionErr,
					generatedErr,
				)
			}
			if !reflect.DeepEqual(reflected, reflectionByteShapes(generated)) {
				t.Fatalf("destination mismatch: reflection=%#v generated=%#v", reflected, generated)
			}
		})
	}
}

func byteSlicePointer(values ...byte) *[]byte {
	return &values
}

func TestGeneratedFloat64Assignments(t *testing.T) {
	float32Value := float32(1.5)
	float32Data := binary.BigEndian.AppendUint32(
		[]byte{0x04, 0x08},
		math.Float32bits(float32Value),
	)
	float64Value := 1.23456789012345
	float64Data := binary.BigEndian.AppendUint64(
		[]byte{0x68},
		math.Float64bits(float64Value),
	)
	tests := []struct {
		name  string
		value []byte
		want  float64
	}{
		{name: "float32", value: float32Data, want: float64(float32Value)},
		{name: "float64", value: float64Data, want: float64Value},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := append(singleFieldRecord("value", tt.value), 0x45, 'a', 'f', 't', 'e', 'r')
			var reflected reflectionFloat64Record
			reflectionDecoder := decoder.New(data)
			if err := reflectionDecoder.Decode(0, &reflected); err != nil {
				t.Fatalf("reflection decode: %v", err)
			}
			var generated Float64Record
			next, err := generated.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
			if err != nil {
				t.Fatalf("generated decode: %v", err)
			}
			if math.Float64bits(reflected.Value) != math.Float64bits(tt.want) ||
				math.Float64bits(generated.Value) != math.Float64bits(tt.want) {
				t.Fatalf(
					"decoded values: reflection=%v generated=%v want=%v",
					reflected.Value,
					generated.Value,
					tt.want,
				)
			}
			assertTrailingString(t, next, "after")
		})
	}
}

func TestGeneratedDualInterfacePrefersCursor(t *testing.T) {
	data := append(
		singleFieldRecord("value", []byte{0x47, 'd', 'e', 'c', 'o', 'd', 'e', 'd'}),
		0x45, 'a', 'f', 't', 'e', 'r',
	)
	var reflected reflectionDualInterfaceRecord
	reflectionDecoder := decoder.New(data)
	if err := reflectionDecoder.Decode(0, &reflected); err != nil {
		t.Fatalf("reflection decode: %v", err)
	}
	var generated DualInterfaceRecord
	next, err := generated.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err != nil {
		t.Fatalf("generated decode: %v", err)
	}
	assertCursorUsed := func(name string, value dualCustom) {
		t.Helper()
		if value.value != "cursor:decoded" || value.cursorCalls != 1 || value.legacyCalls != 0 {
			t.Fatalf("%s callback result: %#v", name, value)
		}
	}
	assertCursorUsed("reflection", reflected.Value)
	assertCursorUsed("generated", generated.Value)
	assertTrailingString(t, next, "after")
}

func TestGeneratedSkipsUnknownFieldsAndReturnsOuterSuccessor(t *testing.T) {
	data := []byte{0xe3}
	appendKey := func(key string) {
		data = append(data, byte(0x40+len(key)))
		data = append(data, key...)
	}

	appendKey("unknown_nested")
	data = append(data,
		0x02, 0x04, // slice with two values
		0xe1, 0x41, 'x', 0x41, 'y', // nested map
		0x01, 0x04, 0xa1, 7, // nested slice
	)
	appendKey("unknown_pointer")
	data = append(data, 0x20, 0)
	pointerPayload := len(data) - 1
	appendKey("name")
	data = append(data, 0x45, 'k', 'n', 'o', 'w', 'n')
	data = append(data, 0x45, 'a', 'f', 't', 'e', 'r')
	if len(data) > 255 {
		t.Fatal("test pointer target does not fit in a one-byte pointer")
	}
	data[pointerPayload] = byte(len(data))
	data = append(data, 0xe1, 0x41, 'x', 0x41, 'y')

	decoder := mmdbdata.NewDecoder(data, 0)
	var record Record
	next, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor())
	if err != nil {
		t.Fatal(err)
	}
	if record.Name != "known" {
		t.Fatalf("known field was not decoded: %#v", record)
	}
	trailing, _, err := next.ReadString()
	if err != nil {
		t.Fatal(err)
	}
	if trailing != "after" {
		t.Fatalf("outer successor decoded %q, want %q", trailing, "after")
	}
}

func TestGeneratedWideIntegersAndNamedMapKeysReturnOuterSuccessor(t *testing.T) {
	data := []byte{0xe3}
	appendKey := func(key string) {
		data = append(data, byte(0x40+len(key)))
		data = append(data, key...)
	}

	appendKey("count32")
	data = binary.BigEndian.AppendUint32(append(data, 0xc4), 0x01020304)
	appendKey("count64")
	data = binary.BigEndian.AppendUint64(append(data, 0x08, 0x02), 0x0102030405060708)
	appendKey("names")
	data = append(data, 0xe1, 0x42, 'e', 'n', 0xa1, 7)
	data = append(data, 0x45, 'a', 'f', 't', 'e', 'r')

	decoder := mmdbdata.NewDecoder(data, 0)
	var record Record
	next, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor())
	if err != nil {
		t.Fatal(err)
	}
	if record.Count32 != 0x01020304 {
		t.Fatalf("decoded uint32 %#x, want %#x", record.Count32, uint32(0x01020304))
	}
	if record.Count64 != 0x0102030405060708 {
		t.Fatalf("decoded uint64 %#x, want %#x", record.Count64, uint64(0x0102030405060708))
	}
	if !reflect.DeepEqual(record.Names, map[Code]uint8{Code("en"): 7}) {
		t.Fatalf("decoded named-key map %#v", record.Names)
	}
	assertTrailingString(t, next, "after")
}

func TestGeneratedPointerSuccessors(t *testing.T) {
	t.Run("known integer field", func(t *testing.T) {
		data := []byte{
			0xe1,
			0x45, 'c', 'o', 'u', 'n', 't',
			0x20, 0,
			0x45, 'a', 'f', 't', 'e', 'r',
		}
		data[8] = byte(len(data))
		data = append(data, 0xa1, 42)

		decoder := mmdbdata.NewDecoder(data, 0)
		var record Record
		next, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor())
		if err != nil {
			t.Fatal(err)
		}
		if record.Count != 42 {
			t.Fatalf("decoded count %d, want 42", record.Count)
		}
		assertTrailingString(t, next, "after")
	})

	t.Run("top-level map", func(t *testing.T) {
		data := []byte{
			0x20, 0,
			0x45, 'a', 'f', 't', 'e', 'r',
		}
		data[1] = byte(len(data))
		data = append(data,
			0xe1,
			0x44, 'n', 'a', 'm', 'e',
			0x45, 'k', 'n', 'o', 'w', 'n',
		)

		decoder := mmdbdata.NewDecoder(data, 0)
		var record Record
		next, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor())
		if err != nil {
			t.Fatal(err)
		}
		if record.Name != "known" {
			t.Fatalf("decoded name %q, want %q", record.Name, "known")
		}
		assertTrailingString(t, next, "after")
	})

	t.Run("four-byte pointer with high address bits", func(t *testing.T) {
		data := []byte{
			0xe1,
			0x44, 'n', 'a', 'm', 'e',
			0x3f, 0x00, 0x00, 0x00, 0x01,
		}
		var reflected reflectionRecord
		var generated Record
		reflectionErr, generatedErr := decodeReflectionAndGeneratedInto(
			data,
			&reflected,
			&generated,
		)
		if reflectionErr != nil || generatedErr != nil {
			t.Fatalf("unexpected errors: reflection=%v generated=%v", reflectionErr, generatedErr)
		}
		if reflected.Name != "name" || generated.Name != "name" {
			t.Fatalf("decoded names: reflection=%q generated=%q", reflected.Name, generated.Name)
		}
	})
}

func TestGeneratedFloat32Boundaries(t *testing.T) {
	encode := func(value float64) []byte {
		data := []byte{
			0xe1,
			0x4b, 't', 'e', 'm', 'p', 'e', 'r', 'a', 't', 'u', 'r', 'e',
			0x68,
		}
		return binary.BigEndian.AppendUint64(data, math.Float64bits(value))
	}

	var maximum Record
	if err := maximum.UnmarshalMaxMindDB(
		mmdbdata.NewDecoder(encode(float64(math.MaxFloat32)), 0),
	); err != nil {
		t.Fatal(err)
	}
	if maximum.Temperature != math.MaxFloat32 {
		t.Fatalf("decoded maximum %v, want %v", maximum.Temperature, float32(math.MaxFloat32))
	}

	overflow := Record{Temperature: 7}
	err := overflow.UnmarshalMaxMindDB(
		mmdbdata.NewDecoder(encode(2*float64(math.MaxFloat32)), 0),
	)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("cannot unmarshal")) {
		t.Fatalf("expected conversion error, got %v", err)
	}
	if overflow.Temperature != 7 {
		t.Fatalf("overflow mutated destination to %v", overflow.Temperature)
	}
}

func TestGeneratedMalformedErrorParity(t *testing.T) {
	tests := []struct {
		name            string
		data            []byte
		generatedOffset uint
	}{
		{
			name:            "scalar",
			data:            singleFieldRecord("name", []byte{0x44, 'x'}),
			generatedOffset: 6,
		},
		{
			name:            "map key",
			data:            singleFieldRecord("lookup", []byte{0xe1, 0x84, 1}),
			generatedOffset: 9,
		},
		{
			name:            "slice element",
			data:            singleFieldRecord("values", []byte{0x01, 0x04, 0xc4, 1}),
			generatedOffset: 10,
		},
		{
			name: "map value",
			data: singleFieldRecord(
				"lookup",
				[]byte{0xe1, 0x42, 'e', 'n', 0x44, 'x'},
			),
			generatedOffset: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reflectionErr, generatedErr := decodeReflectionAndGenerated(tt.data)
			requireDecodeErrorParity(t, reflectionErr, generatedErr)

			summary := summarizeDecodeError(generatedErr)
			if !summary.invalidDatabase || summary.unmarshalType || summary.unexpectedKind {
				t.Fatalf("unexpected generated error categories: %#v: %v", summary, generatedErr)
			}
			var generatedContext mmdberrors.ContextualError
			if !errors.As(generatedErr, &generatedContext) {
				t.Fatalf("generated error lacks offset context: %v", generatedErr)
			}
			if generatedContext.Offset != tt.generatedOffset {
				t.Fatalf(
					"generated error offset %d, want %d: %v",
					generatedContext.Offset,
					tt.generatedOffset,
					generatedErr,
				)
			}
			var reflectionContext mmdberrors.ContextualError
			if !errors.As(reflectionErr, &reflectionContext) {
				t.Fatalf("reflection error lacks offset context: %v", reflectionErr)
			}
		})
	}
}

func FuzzGeneratedReflectionParity(f *testing.F) {
	f.Add(completeRecord())
	f.Add(singleFieldRecord("name", []byte{0x41, 'x'}))
	f.Add(singleFieldRecord("count", []byte{0xa1, 42}))
	f.Add(singleFieldRecord("temperature", []byte{0x04, 0x08, 0x7f, 0xc0, 0, 0}))
	f.Add(singleFieldRecord("lookup", []byte{0xe1, 0x80, 0x30}))
	f.Add(singleFieldRecord("name", []byte{0x44, 'x'}))
	f.Add(singleFieldRecord("lookup", []byte{0xe1, 0x84, 1}))
	f.Add(singleFieldRecord("values", []byte{0x01, 0x04, 0xc4, 1}))
	f.Add(singleFieldRecord("lookup", []byte{0xe1, 0x42, 'e', 'n', 0x44, 'x'}))
	f.Add([]byte("\xe1Dname?\x00\x00\x00\x01"))
	f.Add([]byte{0x41, 'x'})
	f.Add([]byte("\xe1Fnested\x80"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var reflected reflectionRecord
		var generated Record
		reflectionErr, generatedErr := decodeReflectionAndGeneratedInto(
			data,
			&reflected,
			&generated,
		)
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

type decodeErrorSummary struct {
	invalidDatabase bool
	unmarshalType   bool
	unexpectedKind  bool
	contextual      bool
}

func summarizeDecodeError(err error) decodeErrorSummary {
	var invalidDatabase mmdberrors.InvalidDatabaseError
	var unmarshalType mmdberrors.UnmarshalTypeError
	var unexpectedKind mmdbdata.UnexpectedKindError
	var contextual mmdberrors.ContextualError
	return decodeErrorSummary{
		invalidDatabase: errors.As(err, &invalidDatabase),
		unmarshalType:   errors.As(err, &unmarshalType),
		unexpectedKind:  errors.As(err, &unexpectedKind),
		contextual:      errors.As(err, &contextual),
	}
}

func requireDecodeErrorParity(t testing.TB, reflectionErr, generatedErr error) {
	t.Helper()
	if reflectionErr == nil || generatedErr == nil {
		t.Fatalf("expected two errors: reflection=%v generated=%v", reflectionErr, generatedErr)
	}
	reflectionSummary := summarizeDecodeError(reflectionErr)
	generatedSummary := summarizeDecodeError(generatedErr)
	if reflectionSummary != generatedSummary {
		t.Fatalf(
			"error category mismatch: reflection=%#v (%v), generated=%#v (%v)",
			reflectionSummary,
			reflectionErr,
			generatedSummary,
			generatedErr,
		)
	}
}

func decodeReflectionAndGenerated(data []byte) (error, error) {
	var reflected reflectionRecord
	var generated Record
	return decodeReflectionAndGeneratedInto(data, &reflected, &generated)
}

func decodeReflectionAndGeneratedInto(
	data []byte,
	reflected *reflectionRecord,
	generated *Record,
) (error, error) {
	reflectionDecoder := decoder.New(data)
	reflectionErr := reflectionDecoder.Decode(0, reflected)
	generatedErr := generated.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
	return reflectionErr, generatedErr
}

func assertTrailingString(t *testing.T, cursor mmdbdata.Cursor, want string) {
	t.Helper()
	got, _, err := cursor.ReadString()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("trailing string %q, want %q", got, want)
	}
}

func equalReflectionRecord(reflected reflectionRecord, generated Record) bool {
	return reflected.Name == generated.Name &&
		reflected.Count == generated.Count &&
		reflected.Count32 == generated.Count32 &&
		reflected.Count64 == generated.Count64 &&
		math.Float32bits(reflected.Temperature) == math.Float32bits(generated.Temperature) &&
		reflect.DeepEqual(reflected.Enabled, generated.Enabled) &&
		reflect.DeepEqual(reflected.Values, generated.Values) &&
		reflect.DeepEqual(reflected.Lookup, generated.Lookup) &&
		reflect.DeepEqual(reflected.Names, generated.Names) &&
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

func TestGeneratedLargeSlicePreflightBoundaries(t *testing.T) {
	for _, size := range []int{1023, 1024} {
		t.Run(fmt.Sprintf("%d", size), func(t *testing.T) {
			data := []byte{0xe1, 0x46, 'v', 'a', 'l', 'u', 'e', 's'}
			data = append(data, 0x1e, 0x04, byte((size-285)>>8), byte(size-285))
			data = append(data, bytes.Repeat([]byte{0xa0}, size)...)
			data = append(data, 0x45, 'a', 'f', 't', 'e', 'r')

			record := Record{Values: make([]uint16, 0, size)}
			decoder := mmdbdata.NewDecoder(data, 0)
			next, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor())
			if err != nil {
				t.Fatal(err)
			}
			if len(record.Values) != size {
				t.Fatalf("decoded %d values, want %d", len(record.Values), size)
			}
			for index, value := range record.Values {
				if value != 0 {
					t.Fatalf("decoded value %d at index %d", value, index)
				}
			}
			assertTrailingString(t, next, "after")
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

func BenchmarkGeneratedRecordReused(b *testing.B) {
	data := completeRecord()
	decoder := mmdbdata.NewDecoder(data, 0)
	var record Record
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
}

func BenchmarkGeneratedBytesReused(b *testing.B) {
	data := singleFieldRecord("bytes", []byte{0x83, 1, 2, 3})
	decoder := mmdbdata.NewDecoder(data, 0)
	var record Record
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
