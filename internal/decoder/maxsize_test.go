package decoder

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

type maxSizeCursorString string

func BenchmarkEmbeddedStructFields(b *testing.B) {
	type Embedded struct {
		Value string `maxminddb:"value,maxsize:32"`
	}
	type record struct {
		*Embedded
	}
	for b.Loop() {
		fields := makeStructFields(reflect.TypeFor[record]())
		if fields.validationErr != nil {
			b.Fatal(fields.validationErr)
		}
	}
}

var maxSizeCursorStringCalls int

func (value *maxSizeCursorString) UnmarshalMaxMindDBCursor(cursor Cursor) (Cursor, error) {
	maxSizeCursorStringCalls++
	decoded, next, err := cursor.ReadString()
	if err == nil {
		*value = maxSizeCursorString(decoded)
	}
	return next, err
}

type maxSizeLegacyString string

var maxSizeLegacyStringCalls int

func (value *maxSizeLegacyString) UnmarshalMaxMindDB(decoder *Decoder) error {
	maxSizeLegacyStringCalls++
	decoded, err := decoder.ReadString()
	if err == nil {
		*value = maxSizeLegacyString(decoded)
	}
	return err
}

type maxSizeCursorBytes []byte

var maxSizeCursorBytesCalls int

func (value *maxSizeCursorBytes) UnmarshalMaxMindDBCursor(cursor Cursor) (Cursor, error) {
	maxSizeCursorBytesCalls++
	decoded, next, err := cursor.ReadBytes()
	if err == nil {
		*value = append((*value)[:0], decoded...)
	}
	return next, err
}

func TestReflectionMaxSize(t *testing.T) {
	type record struct {
		Text   string          `maxminddb:"text,maxsize:3"`
		Bytes  []byte          `maxminddb:"bytes,maxsize:3"`
		Values []uint16        `maxminddb:"values,maxsize:3"`
		Lookup map[string]bool `maxminddb:"lookup,maxsize:2"`
	}

	data := []byte{0xe4}
	data = appendMaxSizeField(data, "text", []byte{0x43, 'a', 'b', 'c'})
	data = appendMaxSizeField(data, "bytes", []byte{0x83, 1, 2, 3})
	data = appendMaxSizeField(data, "values", []byte{0x03, 0x04, 0xa0, 0xa0, 0xa0})
	data = appendMaxSizeField(data, "lookup", []byte{
		0xe2,
		0x41, 'a', 0x00, 0x07,
		0x41, 'b', 0x01, 0x07,
	})

	var got record
	decoder := New(data)
	require.NoError(t, decoder.Decode(0, &got))
	require.Equal(t, record{
		Text:   "abc",
		Bytes:  []byte{1, 2, 3},
		Values: []uint16{0, 0, 0},
		Lookup: map[string]bool{"a": false, "b": true},
	}, got)
}

func TestReflectionMaxSizeMetadataIsCached(t *testing.T) {
	type record struct {
		Plain  string              `maxminddb:"plain,maxsize:17"`
		Custom maxSizeCursorString `maxminddb:"custom,maxsize:23"`
	}

	fields := cachedFieldsForType(reflect.TypeFor[record]())
	require.Same(t, fields, cachedFieldsForType(reflect.TypeFor[record]()))
	require.Equal(t, uint64(17), fields.namedFields["plain"].maxSize)
	require.False(t, fields.namedFields["plain"].maxSizeCustom)
	require.Equal(t, uint64(23), fields.namedFields["custom"].maxSize)
	require.True(t, fields.namedFields["custom"].maxSizeCustom)
	require.Equal(t, supportedMaxSizeKinds, fields.namedFields["custom"].maxSizeKinds)
}

func TestReflectionMaxSizeRejectsBeforeMutation(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		new  func() any
		want any
	}{
		{
			name: "string",
			data: appendMaxSizeField([]byte{0xe1}, "value", []byte{0x43, 'a', 'b', 'c'}),
			new: func() any {
				return &struct {
					Value string `maxminddb:"value,maxsize:2"`
				}{Value: "keep"}
			},
			want: "keep",
		},
		{
			name: "bytes",
			data: appendMaxSizeField([]byte{0xe1}, "value", []byte{0x83, 1, 2, 3}),
			new: func() any {
				return &struct {
					Value []byte `maxminddb:"value,maxsize:2"`
				}{Value: []byte{9}}
			},
			want: []byte{9},
		},
		{
			name: "bytes encoded as array",
			data: appendMaxSizeField(
				[]byte{0xe1},
				"value",
				[]byte{0x03, 0x04, 0xa1, 1, 0xa1, 2, 0xa1, 3},
			),
			new: func() any {
				return &struct {
					Value []byte `maxminddb:"value,maxsize:2"`
				}{Value: []byte{9}}
			},
			want: []byte{9},
		},
		{
			name: "slice",
			data: appendMaxSizeField(
				[]byte{0xe1},
				"value",
				[]byte{0x03, 0x04, 0xa0, 0xa0, 0xa0},
			),
			new: func() any {
				return &struct {
					Value []uint16 `maxminddb:"value,maxsize:2"`
				}{Value: []uint16{9}}
			},
			want: []uint16{9},
		},
		{
			name: "map",
			data: appendMaxSizeField([]byte{0xe1}, "value", []byte{
				0xe2,
				0x41, 'a', 0x00, 0x07,
				0x41, 'b', 0x01, 0x07,
			}),
			new: func() any {
				return &struct {
					Value map[string]bool `maxminddb:"value,maxsize:1"`
				}{Value: map[string]bool{"keep": true}}
			},
			want: map[string]bool{"keep": true},
		},
		{
			name: "pointer",
			data: appendMaxSizeField(
				[]byte{0xe1},
				"value",
				[]byte{0x03, 0x04, 0xa0, 0xa0, 0xa0},
			),
			new: func() any {
				return &struct {
					Value *[]uint16 `maxminddb:"value,maxsize:2"`
				}{}
			},
			want: (*[]uint16)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.new()
			decoder := New(tt.data)
			err := decoder.Decode(0, got)
			require.ErrorContains(t, err, "exceeds maxsize")
			var invalidDatabase mmdberrors.InvalidDatabaseError
			require.ErrorAs(t, err, &invalidDatabase)
			require.Equal(t, tt.want, reflect.ValueOf(got).Elem().Field(0).Interface())
		})
	}
}

func TestReflectionMaxSizeRejectsBeforeInitializingEmbeddedPointer(t *testing.T) {
	type Embedded struct {
		Value string `maxminddb:"value,maxsize:2"`
	}
	type Record struct {
		*Embedded
	}

	exactData := appendMaxSizeField([]byte{0xe1}, "value", []byte{0x42, 'a', 'b'})
	var exact Record
	decoder := New(exactData)
	require.NoError(t, decoder.Decode(0, &exact))
	require.Equal(t, "ab", exact.Value)

	data := appendMaxSizeField([]byte{0xe1}, "value", []byte{0x43, 'a', 'b', 'c'})
	var got Record
	decoder = New(data)
	err := decoder.Decode(0, &got)
	require.ErrorContains(t, err, "exceeds maxsize")
	require.Nil(t, got.Embedded)
}

func TestReflectionMaxSizeSupportsExplicitEmptyFieldName(t *testing.T) {
	type Record struct {
		Value string `maxminddb:"'',maxsize:1"`
	}

	data := []byte{0xe1, 0x40, 0x41, 'x'}
	var got Record
	decoder := New(data)
	require.NoError(t, decoder.Decode(0, &got))
	require.Equal(t, "x", got.Value)
}

func TestReflectionSupportsQuotedCommaFieldName(t *testing.T) {
	type Record struct {
		Value string `maxminddb:"'city,name'"`
	}

	data := appendMaxSizeField([]byte{0xe1}, "city,name", []byte{0x41, 'x'})
	var got Record
	decoder := New(data)
	require.NoError(t, decoder.Decode(0, &got))
	require.Equal(t, "x", got.Value)
}

func TestReflectionMaxSizePrecedesCustomUnmarshalers(t *testing.T) {
	t.Run("cursor", func(t *testing.T) {
		type record struct {
			Value maxSizeCursorString `maxminddb:"value,maxsize:3"`
		}

		maxSizeCursorStringCalls = 0
		var exact record
		require.NoError(t, decodeAt(
			appendMaxSizeField([]byte{0xe1}, "value", []byte{0x43, 'a', 'b', 'c'}),
			0,
			&exact,
		))
		require.Equal(t, maxSizeCursorString("abc"), exact.Value)
		require.Equal(t, 1, maxSizeCursorStringCalls)

		maxSizeCursorStringCalls = 0
		over := record{Value: "keep"}
		err := decodeAt(
			appendMaxSizeField([]byte{0xe1}, "value", []byte{0x44, 'a', 'b', 'c', 'd'}),
			0,
			&over,
		)
		require.ErrorContains(t, err, "exceeds maxsize")
		require.Equal(t, maxSizeCursorString("keep"), over.Value)
		require.Zero(t, maxSizeCursorStringCalls)
	})

	t.Run("legacy", func(t *testing.T) {
		type record struct {
			Value maxSizeLegacyString `maxminddb:"value,maxsize:3"`
		}

		maxSizeLegacyStringCalls = 0
		var exact record
		require.NoError(t, decodeAt(
			appendMaxSizeField([]byte{0xe1}, "value", []byte{0x43, 'a', 'b', 'c'}),
			0,
			&exact,
		))
		require.Equal(t, maxSizeLegacyString("abc"), exact.Value)
		require.Equal(t, 1, maxSizeLegacyStringCalls)

		maxSizeLegacyStringCalls = 0
		over := record{Value: "keep"}
		err := decodeAt(
			appendMaxSizeField([]byte{0xe1}, "value", []byte{0x44, 'a', 'b', 'c', 'd'}),
			0,
			&over,
		)
		require.ErrorContains(t, err, "exceeds maxsize")
		require.Equal(t, maxSizeLegacyString("keep"), over.Value)
		require.Zero(t, maxSizeLegacyStringCalls)
	})

	t.Run("named byte slice reading bytes", func(t *testing.T) {
		type record struct {
			Value maxSizeCursorBytes `maxminddb:"value,maxsize:2"`
		}

		maxSizeCursorBytesCalls = 0
		var exact record
		require.NoError(t, decodeAt(
			appendMaxSizeField([]byte{0xe1}, "value", []byte{0x82, 1, 2}),
			0,
			&exact,
		))
		require.Equal(t, maxSizeCursorBytes{1, 2}, exact.Value)
		require.Equal(t, 1, maxSizeCursorBytesCalls)

		maxSizeCursorBytesCalls = 0
		over := record{Value: maxSizeCursorBytes{9}}
		err := decodeAt(
			appendMaxSizeField([]byte{0xe1}, "value", []byte{0x83, 1, 2, 3}),
			0,
			&over,
		)
		require.ErrorContains(t, err, "exceeds maxsize")
		require.Equal(t, maxSizeCursorBytes{9}, over.Value)
		require.Zero(t, maxSizeCursorBytesCalls)
	})
}

func TestReflectionMaxSizeLeavesKindMismatchToDecoder(t *testing.T) {
	type record struct {
		Value string `maxminddb:"value,maxsize:0"`
	}
	data := appendMaxSizeField([]byte{0xe1}, "value", []byte{0x00, 0x07})

	var got record
	decoder := New(data)
	err := decoder.Decode(0, &got)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "maxsize")
	var typeError mmdberrors.UnmarshalTypeError
	require.ErrorAs(t, err, &typeError)
}

func TestReflectionMaxSizeTagValidation(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		kind    reflect.Type
		wantErr string
	}{
		{
			name:    "unsupported type",
			tag:     "value,maxsize:1",
			kind:    reflect.TypeFor[uint16](),
			wantErr: "only supported",
		},
		{
			name:    "underscore",
			tag:     "value,max_size:1",
			kind:    reflect.TypeFor[string](),
			wantErr: `specify "maxsize"`,
		},
		{
			name:    "equals",
			tag:     "value,maxsize=1",
			kind:    reflect.TypeFor[string](),
			wantErr: "missing value",
		},
		{
			name:    "duplicate",
			tag:     "value,maxsize:1,maxsize:2",
			kind:    reflect.TypeFor[string](),
			wantErr: "duplicate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structType := reflect.StructOf([]reflect.StructField{{
				Name: "Field",
				Type: tt.kind,
				Tag:  reflect.StructTag(`maxminddb:"` + tt.tag + `"`),
			}})
			value := reflect.New(structType).Interface()
			decoder := New([]byte{0xe0})
			err := decoder.Decode(0, value)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestReflectionMaxSizeRejectsStructFields(t *testing.T) {
	type Embedded struct {
		Value string `maxminddb:"value"`
	}
	for _, test := range []struct {
		name      string
		fieldType reflect.Type
		anonymous bool
	}{
		{name: "named struct", fieldType: reflect.TypeFor[Embedded]()},
		{name: "named pointer", fieldType: reflect.TypeFor[*Embedded]()},
		{name: "embedded struct", fieldType: reflect.TypeFor[Embedded](), anonymous: true},
		{name: "embedded pointer", fieldType: reflect.TypeFor[*Embedded](), anonymous: true},
	} {
		for _, tag := range []reflect.StructTag{`maxminddb:",maxsize:0"`, `maxminddb:",maxsize:1"`} {
			t.Run(test.name+"/"+string(tag), func(t *testing.T) {
				recordType := reflect.StructOf([]reflect.StructField{{
					Name:      "Embedded",
					Type:      test.fieldType,
					Anonymous: test.anonymous,
					Tag:       tag,
				}})
				decoder := New([]byte{0xe0})
				// Validation must also survive reuse of the field metadata cache.
				for range 2 {
					result := reflect.New(recordType)
					err := decoder.Decode(0, result.Interface())
					require.ErrorContains(
						t,
						err,
						`invalid maxminddb struct tag on field "Embedded": maxsize is only supported for maps, slices, strings, and bytes`,
					)
					require.True(t, result.Elem().IsZero())
				}
			})
		}
	}
}

func appendMaxSizeField(data []byte, key string, value []byte) []byte {
	data = append(data, byte(0x40+len(key)))
	data = append(data, key...)
	return append(data, value...)
}
