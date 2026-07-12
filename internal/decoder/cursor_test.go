package decoder

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursorReadsNestedContainersWithoutRescanning(t *testing.T) {
	data := []byte{
		0xe2, // map size 2
		0x41, 'a',
		0x43, 'o', 'n', 'e',
		0x41, 'b',
		0x02, 0x04, // slice size 2
		0x43, 't', 'w', 'o',
		0x45, 't', 'h', 'r', 'e', 'e',
		0x44, 'd', 'o', 'n', 'e',
	}

	decoder := NewDecoder(NewDataDecoder(data), 0)
	entries, err := decoder.Cursor().Map()
	require.NoError(t, err)
	mapSize := entries.Size()
	require.Equal(t, uint(2), mapSize)

	var next Cursor
	key, valueCursor, ok := entries.Next(next)
	require.True(t, ok)
	require.Equal(t, "a", string(key))
	value, next, err := valueCursor.ReadString()
	require.NoError(t, err)
	require.Equal(t, "one", value)
	key, valueCursor, ok = entries.Next(next)
	require.True(t, ok)
	require.Equal(t, "b", string(key))
	elements, err := valueCursor.Slice()
	require.NoError(t, err)
	sliceSize, err := elements.Size()
	require.NoError(t, err)
	require.Equal(t, uint(2), sliceSize)

	want := []string{"two", "three"}
	var elementNext Cursor
	for {
		index, elementCursor, more := elements.Next(elementNext)
		if !more {
			break
		}
		var value string
		var readErr error
		value, elementNext, readErr = elementCursor.ReadString()
		require.NoError(t, readErr)
		require.Equal(t, want[index], value)
	}
	require.NoError(t, elements.Err())
	sliceEnd, err := elements.End()
	require.NoError(t, err)
	_, _, ok = entries.Next(sliceEnd)
	require.False(t, ok)
	require.NoError(t, entries.Err())
	mapEnd, err := entries.End()
	require.NoError(t, err)
	require.NoError(t, decoder.Advance(mapEnd))

	value, err = decoder.ReadString()
	require.NoError(t, err)
	require.Equal(t, "done", value)
}

func TestCursorPointerContainerReturnsOuterSuccessor(t *testing.T) {
	data := []byte{
		0x20, 0x0a, // one-byte pointer to offset 10
		0x44, 'd', 'o', 'n', 'e',
		0x00, 0x00, 0x00,
		0xe1, // offset 10: map size 1
		0x41, 'a',
		0x43, 'o', 'n', 'e',
	}

	decoder := NewDecoder(NewDataDecoder(data), 0)
	entries, err := decoder.Cursor().Map()
	require.NoError(t, err)
	var next Cursor
	_, valueCursor, ok := entries.Next(next)
	require.True(t, ok)
	value, next, err := valueCursor.ReadString()
	require.NoError(t, err)
	require.Equal(t, "one", value)
	_, _, ok = entries.Next(next)
	require.False(t, ok)
	end, err := entries.End()
	require.NoError(t, err)
	require.NoError(t, decoder.Advance(end))

	value, err = decoder.ReadString()
	require.NoError(t, err)
	require.Equal(t, "done", value)
}

func TestCursorSliceReadsDirectExtendedKind(t *testing.T) {
	decoder := NewDecoder(NewDataDecoder([]byte{
		0x01, 0x04, // one-element slice; Slice is extended kind 4 + 7
		0x00, 0x07, // false
	}), 0)
	values, err := decoder.Cursor().Slice()
	require.NoError(t, err)

	size, err := values.Size()
	require.NoError(t, err)
	require.Equal(t, uint(1), size)

	_, valueCursor, ok := values.Next(Cursor{})
	require.True(t, ok)
	value, next, err := valueCursor.ReadBool()
	require.NoError(t, err)
	require.False(t, value)

	_, _, ok = values.Next(next)
	require.False(t, ok)
	end, err := values.End()
	require.NoError(t, err)
	require.NoError(t, decoder.Advance(end))
}

func TestCursorRejectsUnprovenAdvancement(t *testing.T) {
	data := []byte{
		0xe1,
		0x41, 'a',
		0x43, 'o', 'n', 'e',
	}
	decoder := NewDecoder(NewDataDecoder(data), 0)
	entries, err := decoder.Cursor().Map()
	require.NoError(t, err)
	_, valueCursor, ok := entries.Next(Cursor{})
	require.True(t, ok)

	_, err = entries.End()
	require.EqualError(t, err, "map was not completely consumed")
	entries.Next(decoder.Cursor())
	require.Error(t, entries.Err())
	require.Error(t, decoder.Advance(valueCursor))
}

func TestMapReaderHardensSuccessorPreconditions(t *testing.T) {
	nonEmpty := NewDecoder(NewDataDecoder([]byte{0xe1, 0x41, 'a', 0x41, 'b'}), 0)
	entries, err := nonEmpty.Cursor().MapReader()
	require.NoError(t, err)
	_, err = entries.End(entries.First())
	require.EqualError(t, err, "map successor is not proven to follow a decoded value")

	empty := NewDecoder(NewDataDecoder([]byte{0xe0}), 0)
	emptyEntries, err := empty.Cursor().MapReader()
	require.NoError(t, err)
	end, err := emptyEntries.End(emptyEntries.First())
	require.NoError(t, err)
	require.NoError(t, empty.Advance(end))

	other := NewDecoder(NewDataDecoder([]byte{0xe0}), 0)
	_, err = emptyEntries.End(other.Cursor())
	require.EqualError(t, err, "map successor belongs to another decoder")

	oversized := append([]byte{0xfe, 0xff, 0xff}, bytes.Repeat([]byte{0xff}, 8)...)
	_, err = NewDecoder(NewDataDecoder(oversized), 0).Cursor().MapReader()
	require.ErrorContains(t, err, "unexpected end of database")
}

func TestCursorIterationProtocolViolations(t *testing.T) {
	mapData := []byte{0xe1, 0x41, 'a', 0x41, 'b'}
	decoder := NewDecoder(NewDataDecoder(mapData), 0)
	other := NewDecoder(NewDataDecoder(mapData), 0)

	entries, err := decoder.Cursor().Map()
	require.NoError(t, err)
	entries.Next(other.Cursor())
	require.EqualError(t, entries.Err(), "map has no current value")

	entries, err = decoder.Cursor().Map()
	require.NoError(t, err)
	_, _, ok := entries.Next(Cursor{})
	require.True(t, ok)
	entries.Next(other.Cursor())
	require.EqualError(t, entries.Err(), "cursor is not the successor of the current map value")
	_, err = entries.End()
	require.EqualError(t, err, "cursor is not the successor of the current map value")

	sliceData := []byte{0x01, 0x04, 0x41, 'a'}
	sliceDecoder := NewDecoder(NewDataDecoder(sliceData), 0)
	values, err := sliceDecoder.Cursor().Slice()
	require.NoError(t, err)
	_, _, ok = values.Next(other.Cursor())
	require.False(t, ok)
	require.EqualError(t, values.Err(), "slice has no current value")

	values, err = sliceDecoder.Cursor().Slice()
	require.NoError(t, err)
	_, _, ok = values.Next(Cursor{})
	require.True(t, ok)
	_, err = values.End()
	require.EqualError(t, err, "slice was not completely consumed")

	stringDecoder := NewDecoder(NewDataDecoder([]byte{0x41, 'x'}), 0)
	_, successor, err := stringDecoder.Cursor().ReadString()
	require.NoError(t, err)
	require.Error(t, other.Advance(successor))
}

func TestCursorMalformedAndWrongKinds(t *testing.T) {
	decoder := NewDecoder(NewDataDecoder([]byte{0x44, 'a'}), 0)
	_, _, err := decoder.Cursor().ReadString()
	require.ErrorContains(t, err, "unexpected end of database")

	decoder = NewDecoder(NewDataDecoder([]byte{0x41, 'a'}), 0)
	_, _, err = decoder.Cursor().ReadBool()
	require.ErrorContains(t, err, "unexpected kind")

	decoder = NewDecoder(NewDataDecoder([]byte{0x20, 0x02, 0x20, 0x02}), 0)
	_, err = decoder.Cursor().Kind()
	require.ErrorContains(t, err, "pointer-to-pointer")
}

func TestCursorRejectsImpossibleContainerSizesBeforeExposingSize(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		openAndSize func(Cursor) error
	}{
		{
			name: "small map",
			data: []byte{0xe2},
			openAndSize: func(cursor Cursor) error {
				_, err := cursor.Map()
				return err
			},
		},
		{
			name: "large map",
			data: []byte{0xff, 0xff, 0xff, 0xff},
			openAndSize: func(cursor Cursor) error {
				_, err := cursor.Map()
				return err
			},
		},
		{
			name: "small slice",
			data: []byte{0x02, 0x04},
			openAndSize: func(cursor Cursor) error {
				_, err := cursor.Slice()
				return err
			},
		},
		{
			name: "large slice",
			data: []byte{0x1f, 0x04, 0xff, 0xff, 0xff},
			openAndSize: func(cursor Cursor) error {
				_, err := cursor.Slice()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder(tt.data), 0)
			err := tt.openAndSize(decoder.Cursor())
			require.ErrorContains(t, err, "unexpected end of database")
		})
	}
}

func TestCursorSizePreflightsLargeContainerContents(t *testing.T) {
	mapData := append([]byte{0xfe, 0x00, 0xe3}, bytes.Repeat([]byte{0xff}, 1024)...)
	mapDecoder := NewDecoder(NewDataDecoder(mapData), 0)
	_, err := mapDecoder.Cursor().Map()
	require.Error(t, err)

	sliceData := append([]byte{0x1e, 0x04, 0x02, 0xe3}, bytes.Repeat([]byte{0xff}, 1024)...)
	sliceDecoder := NewDecoder(NewDataDecoder(sliceData), 0)
	values, err := sliceDecoder.Cursor().Slice()
	require.NoError(t, err)
	size, sizeErr := values.Size()
	require.Zero(t, size)
	require.Error(t, sizeErr)
}

func TestCursorReadsUnsignedKinds(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint64
	}{
		{"uint16", []byte{0xa2, 0x01, 0xf4}, 500},
		{"uint32", []byte{0xc2, 0x01, 0xf4}, 500},
		{"uint64", []byte{0x02, 0x02, 0x01, 0xf4}, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder(tt.data), 0)
			got, end, err := decoder.Cursor().ReadUint()
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.NoError(t, decoder.Advance(end))
			require.Equal(t, uint(len(tt.data)), decoder.offset)
		})
	}
}

func TestCursorIntegerVariants(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		wantSigned bool
		wantInt    int64
		wantUint   uint64
	}{
		{
			name:       "signed",
			data:       []byte{0x04, 0x01, 0xff, 0xff, 0xfe, 0x0c},
			wantSigned: true,
			wantInt:    -500,
		},
		{name: "unsigned", data: []byte{0xa2, 0x01, 0xf4}, wantUint: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder(tt.data), 0)
			integer, signed, next, err := decoder.Cursor().ReadInteger()
			require.NoError(t, err)
			if tt.wantSigned {
				require.True(t, signed)
				require.Equal(t, tt.wantInt, int64(integer))
			} else {
				require.False(t, signed)
				require.Equal(t, tt.wantUint, integer)
			}
			require.NoError(t, decoder.Advance(next))
		})
	}
}

func TestCursorStrictScalarReaders(t *testing.T) {
	t.Run("float32", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x04, 0x08, 0x3f, 0x80, 0x00, 0x00}), 0)
		value, next, err := decoder.Cursor().ReadFloat32()
		require.NoError(t, err)
		require.InDelta(t, float32(1), value, 0)
		require.NoError(t, decoder.Advance(next))
	})
	t.Run("float64", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x68, 0x3f, 0xe0, 0, 0, 0, 0, 0, 0}), 0)
		value, next, err := decoder.Cursor().ReadFloat64()
		require.NoError(t, err)
		require.InDelta(t, 0.5, value, 0)
		require.NoError(t, decoder.Advance(next))
	})
	t.Run("int32", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x04, 0x01, 0xff, 0xff, 0xfe, 0x0c}), 0)
		value, next, err := decoder.Cursor().ReadInt32()
		require.NoError(t, err)
		require.Equal(t, int32(-500), value)
		require.NoError(t, decoder.Advance(next))
	})
	t.Run("uint128", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x08, 0x03, 1, 2, 3, 4, 5, 6, 7, 8}), 0)
		hi, lo, next, err := decoder.Cursor().ReadUint128()
		require.NoError(t, err)
		require.Zero(t, hi)
		require.Equal(t, uint64(0x0102030405060708), lo)
		require.NoError(t, decoder.Advance(next))
	})
	t.Run("bytes", func(t *testing.T) {
		data := []byte{0x83, 'a', 'b', 'c'}
		decoder := NewDecoder(NewDataDecoder(data), 0)
		value, next, err := decoder.Cursor().ReadBytes()
		require.NoError(t, err)
		require.Equal(t, []byte("abc"), value)
		require.NoError(t, decoder.Advance(next))

		decoder = NewDecoder(NewDataDecoder([]byte{0x84, 'a'}), 0)
		_, _, err = decoder.Cursor().ReadBytes()
		require.ErrorContains(t, err, "exceeds buffer length")
	})
}
