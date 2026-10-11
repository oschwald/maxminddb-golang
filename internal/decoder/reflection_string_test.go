package decoder

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type fastFieldString string

type fastStringRecord struct {
	String fastFieldString `maxminddb:"s"`
	Number uint16          `maxminddb:"n"`
}

func stringFieldMap(value []byte) []byte {
	data := append([]byte{0xe2, 0x41, 's'}, value...)
	return append(data, 0x41, 'n', 0xa1, 7)
}

func TestStringFieldEncodings(t *testing.T) {
	for _, encoding := range []struct {
		name    string
		pointer []byte
		target  int
	}{
		{name: "inline"},
		{name: "pointer1", pointer: []byte{0x21, 0}, target: 256},
		{name: "pointer2", pointer: []byte{0x29, 0, 0}, target: pointerBase2 + 1<<16},
		{name: "pointer3", pointer: []byte{0x31, 0, 0, 0}, target: pointerBase3 + 1<<24},
		{name: "pointer4", pointer: []byte{0x38, 0, 0, 0, 8}, target: 8},
		{name: "pointer4 ignored bits", pointer: []byte{0x3f, 0, 0, 0, 8}, target: 8},
	} {
		for _, size := range []int{0, 1, 2, 28, 29, 285, 65821} {
			t.Run(fmt.Sprintf("%s/size%d", encoding.name, size), func(t *testing.T) {
				leaf := stringLeaf(size)
				for i := len(leaf) - size; i < len(leaf); i++ {
					leaf[i] = 'a'
				}
				data := stringFieldMap(leaf)
				root := uint(0)
				if encoding.pointer != nil {
					root = uint(encoding.target + len(leaf))
					record := stringFieldMap(encoding.pointer)
					data = make([]byte, int(root)+len(record))
					copy(data[encoding.target:], leaf)
					copy(data[root:], record)
				}
				want := fastStringRecord{String: fastFieldString(leaf[len(leaf)-size:]), Number: 7}
				for _, newDecoder := range []func([]byte) ReflectionDecoder{New, NewWithoutStringCache} {
					d := newDecoder(data)
					for range 4 {
						var got fastStringRecord
						require.NoError(t, d.Decode(root, &got))
						require.Equal(t, want, got)
					}
				}
			})
		}
	}
}

// Compare the field strategy with general value decoding, including error paths
// and budget changes. Neither decoder may charge payload on a failed fast path.
func TestStringFieldMatchesGeneralDecoder(t *testing.T) {
	fastFields := makeStructFields(reflect.TypeFor[fastStringRecord]())
	plainFields := makeStructFields(reflect.TypeFor[fastStringRecord]())
	require.Equal(t, dispatchString, fastFields.namedFields["s"].dispatch)
	plainFields.namedFields["s"].dispatch = dispatchPlain

	for _, test := range []struct {
		name string
		data []byte
		root uint
	}{
		{name: "inline", data: stringFieldMap(encodedString("abc"))},
		{
			name: "pointer",
			data: append(encodedString("abc"), stringFieldMap(ptr(0))...),
			root: 4,
		},
		{
			name: "long pointer",
			data: append(stringLeaf(29), stringFieldMap(ptr(0))...),
			root: 31,
		},
		{name: "wrong kind", data: stringFieldMap([]byte{0xa1, 7})},
		{
			name: "wrong target kind",
			data: append([]byte{0xa1, 7}, stringFieldMap(ptr(0))...),
			root: 2,
		},
		{name: "truncated pointer", data: []byte{0xe2, 0x41, 's', 0x20}},
		{name: "truncated string", data: []byte{0xe2, 0x41, 's', 0x43, 'a'}},
		{name: "missing target", data: stringFieldMap(ptr(2047))},
		{
			name: "pointer to pointer",
			data: append(ptr(0), stringFieldMap(ptr(0))...),
			root: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, newDecoder := range []func([]byte) ReflectionDecoder{New, NewWithoutStringCache} {
				base := newDecoder(test.data)
				for _, depth := range []int{1, maximumDataStructureDepth - 1, maximumDataStructureDepth, maximumDataStructureDepth + 1} {
					for _, remaining := range []uint32{0, 2, 3, 28, 29, 30} {
						for range 4 {
							fast := newBudgetedDecoder(&base)
							fast.payloadRemaining = remaining
							plain := fast
							got := fastStringRecord{String: "unchanged"}
							want := got
							gotNext, gotErr := fast.decodeStructWithFields(
								2,
								test.root+1,
								addressableValue{Value: reflect.ValueOf(&got).Elem()},
								depth,
								fastFields,
							)
							wantNext, wantErr := plain.decodeStructWithFields(
								2,
								test.root+1,
								addressableValue{Value: reflect.ValueOf(&want).Elem()},
								depth,
								plainFields,
							)
							require.Equal(t, wantErr, gotErr)
							require.Equal(t, wantNext, gotNext)
							require.Equal(t, want, got)
							require.Equal(t, plain.payloadRemaining, fast.payloadRemaining)
							require.Equal(t, plain.budgetRemaining, fast.budgetRemaining)
						}
					}
				}
			}
		})
	}
}

func BenchmarkStringFields(b *testing.B) {
	for _, test := range []struct {
		name string
		data []byte
		root uint
	}{
		{name: "inline", data: stringFieldMap(encodedString("abc"))},
		{name: "pointer", data: append(encodedString("abc"), stringFieldMap(ptr(0))...), root: 4},
		{name: "long pointer", data: append(stringLeaf(29), stringFieldMap(ptr(0))...), root: 31},
	} {
		b.Run(test.name, func(b *testing.B) {
			d := New(test.data)
			var result fastStringRecord
			b.ReportAllocs()
			for b.Loop() {
				if err := d.Decode(test.root, &result); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
