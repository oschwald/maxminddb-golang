package decoder

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

func TestDecodePathScalarDepthLimit(t *testing.T) {
	type namedUint16 uint16
	tests := []struct {
		name string
		data []byte
		want any
	}{
		{name: "bool", data: []byte{0x01, 0x07}, want: true},
		{name: "int32", data: []byte{0x01, 0x01, 7}, want: int32(7)},
		{name: "uint", data: []byte{0xa1, 7}, want: uint(7)},
		{name: "uint16", data: []byte{0xa1, 7}, want: uint16(7)},
		{name: "uint32", data: []byte{0xc1, 7}, want: uint32(7)},
		{name: "uint64", data: []byte{0x01, 0x02, 7}, want: uint64(7)},
		{name: "float32", data: []byte{0x04, 0x08, 0x3f, 0x80, 0, 0}, want: float32(1)},
		{name: "float64", data: []byte{0x68, 0x3f, 0xf0, 0, 0, 0, 0, 0, 0}, want: float64(1)},
		{name: "string", data: []byte{0x43, 'f', 'o', 'o'}, want: "foo"},
		{name: "bytes", data: []byte{0x83, 1, 2, 3}, want: []byte{1, 2, 3}},
		{name: "named uint16", data: []byte{0xa1, 7}, want: namedUint16(7)},
		{name: "any", data: []byte{0xa1, 7}, want: uint64(7)},
	}
	for _, test := range tests {
		for _, depth := range []int{maximumDataStructureDepth, maximumDataStructureDepth + 1} {
			t.Run(fmt.Sprintf("%s/%d", test.name, depth), func(t *testing.T) {
				data := bytes.Repeat(sliceHeader(1), depth)
				data = append(data, test.data...)
				path := make([]any, depth)
				for i := range path {
					path[i] = 0
				}
				resultType := reflect.TypeOf(test.want)
				if test.name == "any" {
					resultType = reflect.TypeFor[any]()
				}
				result := reflect.New(resultType)
				decoder := New(data)
				err := decoder.DecodePath(0, path, result.Interface())
				if depth > maximumDataStructureDepth {
					var invalid mmdberrors.InvalidDatabaseError
					require.ErrorAs(t, err, &invalid)
					require.ErrorContains(t, err, "exceeded maximum data structure depth")
					var contextual mmdberrors.ContextualError
					require.ErrorAs(t, err, &contextual)
					require.Equal(t, uint(depth*2), contextual.Offset)
					require.True(t, result.Elem().IsZero(), "destination must remain unchanged")
					return
				}
				require.NoError(t, err)
				require.Equal(t, test.want, result.Elem().Interface())
			})
		}
	}
}

func BenchmarkDecodePathScalar(b *testing.B) {
	tests := []struct {
		name      string
		data      []byte
		newResult func() any
	}{
		{name: "uint16", data: []byte{0xa1, 7}, newResult: func() any { return new(uint16) }},
		{name: "any", data: []byte{0xa1, 7}, newResult: func() any { return new(any) }},
		{
			name:      "string",
			data:      []byte{0x43, 'f', 'o', 'o'},
			newResult: func() any { return new(string) },
		},
		{name: "bytes", data: []byte{0x83, 1, 2, 3}, newResult: func() any { return new([]byte) }},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			data := append([]byte{0xe1, 0x41, 'k'}, test.data...)
			decoder := NewWithoutStringCache(data)
			result := test.newResult()
			path := []any{"k"}
			b.ReportAllocs()
			for b.Loop() {
				if err := decoder.DecodePath(0, path, result); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
