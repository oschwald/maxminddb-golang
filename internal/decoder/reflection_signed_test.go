package decoder

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

func TestUnsignedToSignedBoundaries(t *testing.T) {
	type namedInt int
	type namedInt8 int8
	type namedInt16 int16
	type namedInt32 int32
	type namedInt64 int64

	for _, zero := range []any{
		int(0), int8(0), int16(0), int32(0), int64(0),
		namedInt(0), namedInt8(0), namedInt16(0), namedInt32(0), namedInt64(0),
	} {
		typ := reflect.TypeOf(zero)
		maximum := uint64(1)<<(typ.Bits()-1) - 1
		values := []uint64{
			0,
			maximum,
			maximum + 1,
			math.MaxInt64,
			math.MaxInt64 + 1,
			math.MaxUint64,
		}
		slices.Sort(values)
		values = slices.Compact(values)
		for _, source := range []struct {
			name   string
			header []byte
			bits   int
		}{
			{"uint16", []byte{0xa2}, 16},
			{"uint32", []byte{0xc4}, 32},
			{"uint64", []byte{0x08, 0x02}, 64},
		} {
			for _, value := range values {
				if source.bits < 64 && value >= uint64(1)<<source.bits {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/%d", typ, source.name, value), func(t *testing.T) {
					payload := binary.BigEndian.AppendUint64(nil, value)
					data := append(
						append([]byte(nil), source.header...),
						payload[8-source.bits/8:]...,
					)
					d := New(data)
					result := reflect.New(typ)
					result.Elem().SetInt(7)
					err := d.Decode(0, result.Interface())
					if value > maximum {
						var typeErr mmdberrors.UnmarshalTypeError
						require.ErrorAs(t, err, &typeErr)
						require.Equal(t, typ, typeErr.Type)
						require.Equal(t, fmt.Sprintf("%d (uint64)", value), typeErr.Value)
						require.Equal(t, int64(7), result.Elem().Int())
						return
					}
					require.NoError(t, err)
					require.Equal(t, int64(value), result.Elem().Int())
				})
			}
		}
	}
}

func TestUnsignedToSignedNestedBoundaries(t *testing.T) {
	type record struct {
		N *int64 `maxminddb:"n"`
	}
	maximum := int64(math.MaxInt64)
	for _, value := range []uint64{math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64} {
		for _, tc := range []struct {
			name   string
			prefix []byte
			want   any
			path   string
		}{
			{"go_pointer", nil, &maximum, ""},
			{"database_pointer", []byte{0x20, 0x02}, maximum, ""},
			{"struct_pointer_field", []byte{0xe1, 0x41, 'n'}, record{N: &maximum}, "/n"},
			{"map", []byte{0xe1, 0x41, 'n'}, map[string]int64{"n": maximum}, "/n"},
			{"slice", []byte{0x01, 0x04}, []int64{maximum}, "/0"},
		} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, value), func(t *testing.T) {
				data := append(append([]byte(nil), tc.prefix...), 0x08, 0x02)
				data = binary.BigEndian.AppendUint64(data, value)
				d := New(data)
				result := reflect.New(reflect.TypeOf(tc.want))
				err := d.Decode(0, result.Interface())
				if value > math.MaxInt64 {
					var typeErr mmdberrors.UnmarshalTypeError
					require.ErrorAs(t, err, &typeErr)
					require.Equal(t, reflect.TypeFor[int64](), typeErr.Type)
					require.Equal(t, fmt.Sprintf("%d (uint64)", value), typeErr.Value)
					if tc.path != "" {
						var contextErr mmdberrors.ContextualError
						require.ErrorAs(t, err, &contextErr)
						require.Equal(t, tc.path, contextErr.Path)
					}
					if tc.name == "go_pointer" {
						require.Nil(t, result.Elem().Interface())
					}
					if tc.name == "struct_pointer_field" {
						require.Equal(t, record{}, result.Elem().Interface())
					}
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.want, result.Elem().Interface())
			})
		}
	}
}
