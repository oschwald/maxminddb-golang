package decoder

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeUintptrOffsets(t *testing.T) {
	type Offset uintptr
	type record struct {
		Inline  Offset  `maxminddb:"a"`
		Pointer uintptr `maxminddb:"b"`
		Tail    uint16  `maxminddb:"c"`
	}

	// A pointer at zero resolves to the string at offset 2, not its payload at 3.
	scalar := []byte{0x20, 0x02, 0x43, 'f', 'o', 'o'}
	// {a: "x", b: pointer to a's value, c: uint16(7)}. The tail verifies
	// that extracting offsets still advances past inline and pointer values.
	object := []byte{0xe3, 0x41, 'a', 0x41, 'x', 0x41, 'b', 0x20, 0x03, 0x41, 'c', 0xa1, 7}
	// ["x", pointer to the first element, uint16(7)].
	array := []byte{0x03, 0x04, 0x41, 'x', 0x20, 0x02, 0xa1, 7}
	namedPointer := Offset(2)

	tests := []struct {
		name   string
		data   []byte
		offset uint
		result any
		want   any
	}{
		{"named/inline", scalar, 2, new(Offset), Offset(2)},
		{"named/pointer", scalar, 0, new(Offset), Offset(2)},
		{"plain/inline", scalar, 2, new(uintptr), uintptr(2)},
		{"plain/pointer", scalar, 0, new(uintptr), uintptr(2)},
		{"named/destination-pointer", scalar, 0, new(*Offset), &namedPointer},
		{"struct", object, 0, new(record), record{Inline: 3, Pointer: 3, Tail: 7}},
		{"named/slice", array, 0, new([]Offset), []Offset{2, 2, 6}},
		{"plain/slice", array, 0, new([]uintptr), []uintptr{2, 2, 6}},
		{
			"named/map",
			object,
			0,
			new(map[string]Offset),
			map[string]Offset{"a": 3, "b": 3, "c": 11},
		},
		{
			"plain/map",
			object,
			0,
			new(map[string]uintptr),
			map[string]uintptr{"a": 3, "b": 3, "c": 11},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewWithoutStringCache(tt.data)
			require.NoError(t, d.Decode(tt.offset, tt.result))
			require.Equal(t, tt.want, reflect.ValueOf(tt.result).Elem().Interface())
		})
	}
}
