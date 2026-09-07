package mmdbdata

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursorOffsetReturnsControlByte(t *testing.T) {
	for _, test := range []struct {
		name   string
		buffer []byte
		offset uint
		value  string
	}{
		{name: "zero offset", buffer: []byte{0x41, 'x'}, value: "x"},
		{name: "nonzero offset", buffer: []byte{0x40, 0x41, 'x'}, offset: 1, value: "x"},
		{name: "extended size", buffer: append([]byte{0x5d, 0}, strings.Repeat("x", 29)...), value: strings.Repeat("x", 29)},
	} {
		t.Run(test.name, func(t *testing.T) {
			decoder := NewDecoder(test.buffer, test.offset)
			cursor := decoder.Cursor()
			offset, err := cursor.Offset()
			require.NoError(t, err)
			require.Equal(t, test.offset, offset)
			require.Equal(t, offset, decoder.Offset())

			value, _, err := cursor.ReadString()
			require.NoError(t, err)
			require.Equal(t, test.value, value)
			value, err = decoder.ReadString()
			require.NoError(t, err)
			require.Equal(t, test.value, value)
		})
	}
}

func TestCursorOffsetResolvesPointerWidths(t *testing.T) {
	for _, test := range []struct {
		name    string
		pointer []byte
		target  uint
	}{
		{name: "one byte", pointer: []byte{0x20, 2}, target: 2},
		{name: "two bytes", pointer: []byte{0x28, 0, 0}, target: 2048},
		{name: "three bytes", pointer: []byte{0x30, 0, 0, 0}, target: 526336},
		{name: "four bytes", pointer: []byte{0x38, 0, 0, 0, 5}, target: 5},
		{name: "ignored high bits", pointer: []byte{0x3f, 0, 0, 0, 5}, target: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			// An extended string size makes the control byte and payload offsets
			// distinct even after the pointer has been followed.
			value := strings.Repeat("x", 29)
			buffer := make([]byte, test.target+2+uint(len(value)))
			copy(buffer, test.pointer)
			buffer[test.target] = 0x5d
			copy(buffer[test.target+2:], value)
			decoder := NewDecoder(buffer, 0)
			cursor := decoder.Cursor()
			offset, err := cursor.Offset()
			require.NoError(t, err)
			require.Equal(t, test.target, offset)
			require.Equal(t, offset, decoder.Offset())

			decoded, next, err := cursor.ReadString()
			require.NoError(t, err)
			require.Equal(t, value, decoded)
			require.NoError(t, decoder.Advance(next))
			decoded, err = NewDecoder(buffer, offset).ReadString()
			require.NoError(t, err)
			require.Equal(t, value, decoded)
		})
	}
}

func TestCursorOffsetIdentifiesRepeatedValues(t *testing.T) {
	// Two pointers and their direct target must use the same cache key.
	cursor := NewDecoder([]byte{0x41, 'x', 0x20, 0, 0x20, 0}, 0).Cursor()
	for range 3 {
		offset, err := cursor.Offset()
		require.NoError(t, err)
		require.Zero(t, offset)
		value, next, err := cursor.ReadString()
		require.NoError(t, err)
		require.Equal(t, "x", value)
		cursor = next
	}
}

func TestCursorOffsetResolutionErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		buffer    []byte
		offset    uint
		wantError string
	}{
		{name: "empty input"},
		{name: "past input", buffer: []byte{0x40}, offset: 1},
		{name: "maximal offset", buffer: []byte{0x40}, offset: ^uint(0)},
		{name: "truncated extended kind", buffer: []byte{0x40, 0}, offset: 1},
		{name: "truncated size", buffer: []byte{0x40, 0x5d}, offset: 1},
		{name: "truncated pointer", buffer: []byte{0x40, 0x20}, offset: 1},
		{name: "truncated wide pointer", buffer: []byte{0x40, 0x38, 0, 0, 0}, offset: 1},
		{name: "target past input", buffer: []byte{0x40, 0x20, 3}, offset: 1},
		{name: "maximal pointer target", buffer: []byte{0x40, 0x38, 0xff, 0xff, 0xff, 0xff}, offset: 1},
		{name: "truncated target kind", buffer: []byte{0x40, 0x20, 3, 0}, offset: 1},
		{name: "truncated target size", buffer: []byte{0x40, 0x20, 3, 0x5d}, offset: 1},
		{name: "pointer chain", buffer: []byte{0x40, 0x20, 3, 0x20, 5, 0x40}, offset: 1, wantError: "pointer-to-pointer chain detected"},
		{name: "self pointer", buffer: []byte{0x40, 0x20, 1}, offset: 1, wantError: "pointer-to-pointer chain detected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			decoder := NewDecoder(test.buffer, test.offset)
			cursor := decoder.Cursor()
			offset, err := cursor.Offset()
			require.Zero(t, offset)
			var invalid InvalidDatabaseError
			require.ErrorAs(t, err, &invalid)
			require.ErrorContains(t, err, fmt.Sprintf("at offset %d:", test.offset))
			wantError := test.wantError
			if wantError == "" {
				wantError = "unexpected end of database"
			}
			require.ErrorContains(t, err, wantError)

			// Resolution errors follow Kind's error contract and do not change
			// the legacy method's original-offset fallback.
			_, kindErr := cursor.Kind()
			require.Error(t, kindErr)
			require.EqualError(t, err, kindErr.Error())
			require.Equal(t, test.offset, decoder.Offset())
		})
	}
}

func TestCursorOffsetRejectsZeroCursor(t *testing.T) {
	offset, err := (Cursor{}).Offset()
	require.Zero(t, offset)
	require.EqualError(t, err, "invalid zero cursor")
}

func TestCursorOffsetDoesNotValidatePayload(t *testing.T) {
	cursor := NewDecoder([]byte{0x20, 2, 0x44}, 0).Cursor()
	offset, err := cursor.Offset()
	require.NoError(t, err)
	require.Equal(t, uint(2), offset)
	_, _, err = cursor.ReadString()
	require.Error(t, err)
}

func BenchmarkCursorOffset(b *testing.B) {
	for _, test := range []struct {
		name   string
		buffer []byte
	}{
		{name: "direct", buffer: []byte{0x41, 'x'}},
		{name: "pointer", buffer: []byte{0x20, 2, 0x41, 'x'}},
		{name: "wide pointer", buffer: []byte{0x38, 0, 0, 0, 5, 0x41, 'x'}},
	} {
		b.Run(test.name, func(b *testing.B) {
			cursor := NewDecoder(test.buffer, 0).Cursor()
			b.ReportAllocs()
			for b.Loop() {
				_, err := cursor.Offset()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
