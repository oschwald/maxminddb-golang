package decoder

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

type typedNilUnmarshaler struct {
	called bool
}

func (value *typedNilUnmarshaler) UnmarshalMaxMindDB(*Decoder) error {
	value.called = true
	return nil
}

type typedNilCursorUnmarshaler struct {
	called bool
}

func (value *typedNilCursorUnmarshaler) UnmarshalMaxMindDBCursor(
	cursor Cursor,
) (Cursor, error) {
	value.called = true
	return cursor.Skip()
}

type typedNilMapUnmarshaler map[string]bool

func (value typedNilMapUnmarshaler) UnmarshalMaxMindDB(*Decoder) error {
	value["called"] = true
	return nil
}

type typedNilSliceCursorUnmarshaler []bool

func (value typedNilSliceCursorUnmarshaler) UnmarshalMaxMindDBCursor(
	cursor Cursor,
) (Cursor, error) {
	value[0] = true
	return cursor.Skip()
}

type typedNilFuncUnmarshaler func()

func (value typedNilFuncUnmarshaler) UnmarshalMaxMindDB(*Decoder) error {
	value()
	return nil
}

type typedNilChanCursorUnmarshaler chan bool

func (value typedNilChanCursorUnmarshaler) UnmarshalMaxMindDBCursor(
	cursor Cursor,
) (Cursor, error) {
	value <- true
	return cursor.Skip()
}

func TestCursorRejectsTypedNilCustomUnmarshalers(t *testing.T) {
	decoder := NewDecoder(NewDataDecoder([]byte{0x41, 'x'}), 0)

	legacy := []struct {
		name  string
		value Unmarshaler
	}{
		{name: "pointer", value: (*typedNilUnmarshaler)(nil)},
		{name: "map", value: typedNilMapUnmarshaler(nil)},
		{name: "function", value: typedNilFuncUnmarshaler(nil)},
	}
	for _, tt := range legacy {
		t.Run("legacy "+tt.name, func(t *testing.T) {
			next, err := decoder.Cursor().Unmarshal(tt.value)
			require.EqualError(t, err, "cannot unmarshal into nil")
			require.Zero(t, next)
		})
	}

	cursor := []struct {
		name  string
		value CursorUnmarshaler
	}{
		{name: "pointer", value: (*typedNilCursorUnmarshaler)(nil)},
		{name: "slice", value: typedNilSliceCursorUnmarshaler(nil)},
		{name: "channel", value: typedNilChanCursorUnmarshaler(nil)},
	}
	for _, tt := range cursor {
		t.Run("cursor "+tt.name, func(t *testing.T) {
			next, err := decoder.Cursor().UnmarshalCursor(tt.value)
			require.EqualError(t, err, "cannot unmarshal into nil")
			require.Zero(t, next)
		})
	}
}

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

func TestCursorPointerSliceReturnsOuterSuccessor(t *testing.T) {
	data := []byte{
		0x20, 0x0a, // one-byte pointer to offset 10
		0x44, 'd', 'o', 'n', 'e',
		0x00, 0x00, 0x00,
		0x01, 0x04, // offset 10: one-element slice
		0x00, 0x07, // false
	}

	decoder := NewDecoder(NewDataDecoder(data), 0)
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

	trailing, err := decoder.ReadString()
	require.NoError(t, err)
	require.Equal(t, "done", trailing)
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

func TestCursorReadsSize29Containers(t *testing.T) {
	t.Run("map", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder(size29MapData()), 0)
		entries, err := decoder.Cursor().Map()
		require.NoError(t, err)
		require.Equal(t, uint(29), entries.Size())

		var next Cursor
		for range uint(29) {
			key, valueCursor, ok := entries.Next(next)
			require.True(t, ok)
			require.Empty(t, key)
			value, successor, readErr := valueCursor.ReadBool()
			require.NoError(t, readErr)
			require.False(t, value)
			next = successor
		}
		_, _, ok := entries.Next(next)
		require.False(t, ok)
		require.NoError(t, entries.Err())
		end, err := entries.End()
		require.NoError(t, err)
		requireCursorTrailingString(t, end, "done")
	})

	t.Run("map reader", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder(size29MapData()), 0)
		entries, err := decoder.Cursor().MapReader()
		require.NoError(t, err)
		require.Equal(t, uint(29), entries.Len())

		next := entries.First()
		for range entries.Len() {
			key, valueCursor, keyErr := next.ReadMapKey()
			require.NoError(t, keyErr)
			require.Empty(t, key)
			value, successor, readErr := valueCursor.ReadBool()
			require.NoError(t, readErr)
			require.False(t, value)
			next = successor
		}
		end, err := entries.End(next)
		require.NoError(t, err)
		requireCursorTrailingString(t, end, "done")
	})

	t.Run("slice", func(t *testing.T) {
		data := make([]byte, 0, 3+29*2+5)
		data = append(data, 0x1d, 0x04, 0x00)
		data = append(data, bytes.Repeat([]byte{0x00, 0x07}, 29)...)
		data = append(data, 0x44, 'd', 'o', 'n', 'e')
		decoder := NewDecoder(NewDataDecoder(data), 0)
		values, err := decoder.Cursor().Slice()
		require.NoError(t, err)
		size, err := values.Size()
		require.NoError(t, err)
		require.Equal(t, uint(29), size)

		var next Cursor
		for wantIndex := range uint(29) {
			index, valueCursor, ok := values.Next(next)
			require.True(t, ok)
			require.Equal(t, wantIndex, index)
			value, successor, readErr := valueCursor.ReadBool()
			require.NoError(t, readErr)
			require.False(t, value)
			next = successor
		}
		_, _, ok := values.Next(next)
		require.False(t, ok)
		require.NoError(t, values.Err())
		end, err := values.End()
		require.NoError(t, err)
		requireCursorTrailingString(t, end, "done")
	})
}

func size29MapData() []byte {
	data := []byte{0xfd, 0x00}
	data = append(data, bytes.Repeat([]byte{0x40, 0x00, 0x07}, 29)...)
	return append(data, 0x44, 'd', 'o', 'n', 'e')
}

func requireCursorTrailingString(t *testing.T, cursor Cursor, want string) {
	t.Helper()
	value, _, err := cursor.ReadString()
	require.NoError(t, err)
	require.Equal(t, want, value)
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
	var zero MapReader
	_, err := zero.Size()
	require.ErrorIs(t, err, errInvalidZeroMapReader)
	_, ok := zero.SizeForAllocation()
	require.False(t, ok)

	_, err = zero.End(Cursor{})
	require.ErrorIs(t, err, errInvalidZeroMapReader)

	nonEmpty := NewDecoder(NewDataDecoder([]byte{0xe1, 0x41, 'a', 0x41, 'b'}), 0)
	entries, err := nonEmpty.Cursor().MapReader()
	require.NoError(t, err)
	_, err = entries.End(entries.First())
	require.EqualError(t, err, "map successor is not proven to follow a decoded value")

	empty := NewDecoder(NewDataDecoder([]byte{0xe0}), 0)
	emptyEntries, err := empty.Cursor().MapReader()
	require.NoError(t, err)
	size, err := emptyEntries.Size()
	require.NoError(t, err)
	require.Zero(t, size)
	end, err := emptyEntries.End(emptyEntries.First())
	require.NoError(t, err)
	require.NoError(t, empty.Advance(end))

	other := NewDecoder(NewDataDecoder([]byte{0xe0}), 0)
	_, err = emptyEntries.End(other.Cursor())
	require.EqualError(t, err, "map successor belongs to another decoder")

	oversized := append([]byte{0xfe, 0xff, 0xff}, bytes.Repeat([]byte{0xff}, 8)...)
	_, err = NewDecoder(NewDataDecoder(oversized), 0).Cursor().MapReader()
	require.ErrorContains(t, err, "unexpected end of database")

	malformed := append([]byte{0xfe, 0x00, 0xe3}, bytes.Repeat([]byte{0xff}, 1024)...)
	malformedEntries, err := NewDecoder(NewDataDecoder(malformed), 0).Cursor().MapReader()
	require.NoError(t, err)
	_, ok = malformedEntries.SizeForAllocation()
	require.False(t, ok)
	_, err = malformedEntries.Size()
	require.Error(t, err)
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

func TestCursorKindIsNonConsuming(t *testing.T) {
	t.Run("direct", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x41, 'x'}), 0)
		cursor := decoder.Cursor()
		kind, err := cursor.Kind()
		require.NoError(t, err)
		require.Equal(t, KindString, kind)
		value, _, err := cursor.ReadString()
		require.NoError(t, err)
		require.Equal(t, "x", value)
	})

	t.Run("extended kind", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x00, 0x07}), 0)
		cursor := decoder.Cursor()
		kind, err := cursor.Kind()
		require.NoError(t, err)
		require.Equal(t, KindBool, kind)
		value, _, err := cursor.ReadBool()
		require.NoError(t, err)
		require.False(t, value)
	})

	t.Run("pointer", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{
			0x20, 0x07,
			0x44, 'd', 'o', 'n', 'e',
			0x41, 'x',
		}), 0)
		cursor := decoder.Cursor()
		kind, err := cursor.Kind()
		require.NoError(t, err)
		require.Equal(t, KindString, kind)
		value, next, err := cursor.ReadString()
		require.NoError(t, err)
		require.Equal(t, "x", value)
		require.NoError(t, decoder.Advance(next))
		trailing, err := decoder.ReadString()
		require.NoError(t, err)
		require.Equal(t, "done", trailing)
	})
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

func TestCursorWrongKindDoesNotApplyExpansionBudget(t *testing.T) {
	decoder := NewDecoder(
		NewDataDecoder(stringLeaf(decodeExpansionBudgetBytes+1)),
		0,
	)
	_, _, err := decoder.Cursor().ReadBool()
	require.Error(t, err)
	require.NotErrorIs(t, err, errDecodedRecordTooLarge)
	var unexpectedKind UnexpectedKindError
	require.ErrorAs(t, err, &unexpectedKind)
}

func TestCursorCheckMaxSize(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		kind      Kind
		maximum   uint64
		wantError bool
	}{
		{name: "string exact", data: []byte{0x43, 'a', 'b', 'c'}, kind: KindString, maximum: 3},
		{
			name:      "string over",
			data:      []byte{0x43, 'a', 'b', 'c'},
			kind:      KindString,
			maximum:   2,
			wantError: true,
		},
		{
			name:      "bytes over",
			data:      []byte{0x83, 1, 2, 3},
			kind:      KindBytes,
			maximum:   2,
			wantError: true,
		},
		{
			name:      "slice over",
			data:      []byte{0x03, 0x04, 0xa0, 0xa0, 0xa0},
			kind:      KindSlice,
			maximum:   2,
			wantError: true,
		},
		{
			name:      "map over",
			data:      []byte{0xe1, 0x41, 'a', 0x00, 0x07},
			kind:      KindMap,
			maximum:   0,
			wantError: true,
		},
		{name: "kind mismatch", data: []byte{0x43, 'a', 'b', 'c'}, kind: KindMap, maximum: 0},
		{
			name:      "pointer",
			data:      []byte{0x20, 0x02, 0x43, 'a', 'b', 'c'},
			kind:      KindString,
			maximum:   2,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder(tt.data), 0)
			err := decoder.Cursor().CheckMaxSize(NewKindSet(tt.kind), tt.maximum)
			if tt.wantError {
				require.ErrorContains(t, err, "exceeds maxsize")
				var invalidDatabase mmdberrors.InvalidDatabaseError
				require.ErrorAs(t, err, &invalidDatabase)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCursorCheckMaxSizeCompactAndExtendedContainers(t *testing.T) {
	tests := []struct {
		name string
		kind Kind
		data func(int) []byte
	}{
		{name: "bytes", kind: KindBytes, data: bytesLeaf},
		{name: "map", kind: KindMap, data: mapHeader},
	}

	for _, tt := range tests {
		for _, size := range []int{28, 29} {
			for _, pointerBacked := range []bool{false, true} {
				name := tt.name + " size " + strconv.Itoa(size)
				if pointerBacked {
					name += " pointer"
				}
				t.Run(name, func(t *testing.T) {
					data := tt.data(size)
					if pointerBacked {
						data = append(ptr(2), data...)
					}
					cursor := NewDecoder(NewDataDecoder(data), 0).Cursor()
					expected := NewKindSet(tt.kind)
					require.NoError(t, cursor.CheckMaxSize(expected, uint64(size)))
					requireMaxSizeError(t, cursor.CheckMaxSize(expected, uint64(size-1)))
				})
			}
		}
	}
}

func TestCursorBoundedReads(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x43, 'a', 'b', 'c'}), 0)
		value, _, err := decoder.Cursor().ReadStringMaxSize(3)
		require.NoError(t, err)
		require.Equal(t, "abc", value)

		value, _, err = decoder.Cursor().ReadStringMaxSize(2)
		requireMaxSizeError(t, err)
		require.Empty(t, value)
	})

	t.Run("pointer string", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x20, 0x02, 0x43, 'a', 'b', 'c'}), 0)
		value, _, err := decoder.Cursor().ReadStringMaxSize(3)
		require.NoError(t, err)
		require.Equal(t, "abc", value)

		_, _, err = decoder.Cursor().ReadStringMaxSize(2)
		requireMaxSizeError(t, err)
	})

	t.Run("extended string", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder(stringLeaf(29)), 0)
		value, _, err := decoder.Cursor().ReadStringMaxSize(29)
		require.NoError(t, err)
		require.Len(t, value, 29)

		_, _, err = decoder.Cursor().ReadStringMaxSize(28)
		requireMaxSizeError(t, err)
	})

	t.Run("slice", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x03, 0x04, 0xa0, 0xa0, 0xa0}), 0)
		values, err := decoder.Cursor().SliceMaxSize(3)
		require.NoError(t, err)
		size, err := values.Size()
		require.NoError(t, err)
		require.Equal(t, uint(3), size)

		_, err = decoder.Cursor().SliceMaxSize(2)
		requireMaxSizeError(t, err)
	})

	t.Run("extended slice", func(t *testing.T) {
		data := append(sliceHeader(29), make([]byte, 29)...)
		decoder := NewDecoder(NewDataDecoder(data), 0)
		_, err := decoder.Cursor().SliceMaxSize(29)
		require.NoError(t, err)

		_, err = decoder.Cursor().SliceMaxSize(28)
		requireMaxSizeError(t, err)
	})
}

func requireMaxSizeError(t *testing.T, err error) {
	t.Helper()
	var invalidDatabase mmdberrors.InvalidDatabaseError
	require.ErrorAs(t, err, &invalidDatabase)
	require.ErrorContains(t, err, "exceeds maxsize")
}

func TestCursorMalformedWrongKindScalarsReportDatabaseErrors(t *testing.T) {
	t.Run("cursor value", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0x84, 1}), 0)
		_, _, err := decoder.Cursor().ReadBool()
		requireMalformedDatabaseError(t, err)
	})

	t.Run("map key", func(t *testing.T) {
		decoder := NewDecoder(NewDataDecoder([]byte{0xe1, 0x84, 1}), 0)
		entries, err := decoder.Cursor().Map()
		require.NoError(t, err)

		_, _, ok := entries.Next(Cursor{})
		require.False(t, ok)
		requireMalformedDatabaseError(t, entries.Err())
	})
}

func requireMalformedDatabaseError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)

	var invalidDatabase mmdberrors.InvalidDatabaseError
	require.ErrorAs(t, err, &invalidDatabase)
	var unexpectedKind UnexpectedKindError
	require.NotErrorAs(t, err, &unexpectedKind)
	var unmarshalType mmdberrors.UnmarshalTypeError
	require.NotErrorAs(t, err, &unmarshalType)
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

func TestCursorMapPreflightBoundaries(t *testing.T) {
	for _, size := range []int{511, 512} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder(cursorMapBoundaryData(size)), 0)
			entries, err := decoder.Cursor().Map()
			require.NoError(t, err)
			require.Equal(t, uint(size), entries.Size())

			var successor Cursor
			for range size {
				key, valueCursor, ok := entries.Next(successor)
				require.True(t, ok)
				require.Equal(t, []byte("k"), key)
				_, successor, err = valueCursor.ReadBool()
				require.NoError(t, err)
			}
			_, _, ok := entries.Next(successor)
			require.False(t, ok)
			end, err := entries.End()
			require.NoError(t, err)
			requireCursorTrailingString(t, end, "after")
		})
	}
}

func TestMapReaderSizePreflightBoundaries(t *testing.T) {
	for _, size := range []int{511, 512} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder(cursorMapBoundaryData(size)), 0)
			entries, err := decoder.Cursor().MapReader()
			require.NoError(t, err)

			hint, ok := entries.SizeForAllocation()
			require.Equal(t, size == 511, ok)
			if ok {
				require.Equal(t, uint(size), hint)
			} else {
				require.Zero(t, hint)
			}
			validated, err := entries.Size()
			require.NoError(t, err)
			require.Equal(t, uint(size), validated)

			next := entries.First()
			for range size {
				key, valueCursor, keyErr := next.ReadMapKey()
				require.NoError(t, keyErr)
				require.Equal(t, []byte("k"), key)
				_, next, err = valueCursor.ReadBool()
				require.NoError(t, err)
			}
			end, err := entries.End(next)
			require.NoError(t, err)
			requireCursorTrailingString(t, end, "after")
		})
	}
}

func cursorMapBoundaryData(size int) []byte {
	data := make([]byte, 0, 3+4*size+6)
	data = append(data, 0xfe, byte((size-285)>>8), byte(size-285))
	for range size {
		data = append(data, 0x41, 'k', 0x00, 0x07)
	}
	return append(data, 0x45, 'a', 'f', 't', 'e', 'r')
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

func TestCursorUnionKindErrorsReportEveryAcceptedKind(t *testing.T) {
	tests := []struct {
		name string
		read func(Cursor) error
		want KindSet
	}{
		{
			name: "float",
			read: func(cursor Cursor) error {
				_, _, err := cursor.ReadFloat()
				return err
			},
			want: NewKindSet(KindFloat64, KindFloat32),
		},
		{
			name: "uint",
			read: func(cursor Cursor) error {
				_, _, err := cursor.ReadUint()
				return err
			},
			want: NewKindSet(KindUint16, KindUint32, KindUint64),
		},
		{
			name: "integer",
			read: func(cursor Cursor) error {
				_, _, _, err := cursor.ReadInteger()
				return err
			},
			want: NewKindSet(KindUint16, KindUint32, KindInt32, KindUint64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder([]byte{0x41, 'x'}), 0)
			err := tt.read(decoder.Cursor())
			var mismatch UnexpectedKindError
			require.ErrorAs(t, err, &mismatch)
			require.Equal(t, KindString, mismatch.Actual)
			require.Equal(t, tt.want, mismatch.Expected)
			require.False(t, mismatch.Expected.Contains(KindString))
		})
	}
}

func TestCursorPointerScalarSuccessorsUsePointerEnd(t *testing.T) {
	pointerData := func(target []byte) []byte {
		data := make([]byte, 0, 8+len(target))
		data = append(data,
			0x20, 0,
			0x45, 'a', 'f', 't', 'e', 'r',
		)
		data[1] = byte(len(data))
		return append(data, target...)
	}
	tests := []struct {
		name string
		data []byte
		read func(Cursor) (any, Cursor, error)
		want any
	}{
		{
			name: "bool",
			data: pointerData([]byte{0x00, 0x07}),
			read: func(cursor Cursor) (any, Cursor, error) {
				value, next, err := cursor.ReadBool()
				return value, next, err
			},
			want: false,
		},
		{
			name: "float",
			data: pointerData([]byte{0x04, 0x08, 0x3f, 0x80, 0x00, 0x00}),
			read: func(cursor Cursor) (any, Cursor, error) {
				value, next, err := cursor.ReadFloat()
				return value, next, err
			},
			want: float64(1),
		},
		{
			name: "uint",
			data: pointerData([]byte{0xa1, 42}),
			read: func(cursor Cursor) (any, Cursor, error) {
				value, next, err := cursor.ReadUint()
				return value, next, err
			},
			want: uint64(42),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder(tt.data), 0)
			value, next, err := tt.read(decoder.Cursor())
			require.NoError(t, err)
			require.Equal(t, tt.want, value)
			require.NoError(t, decoder.Advance(next))
			trailing, err := decoder.ReadString()
			require.NoError(t, err)
			require.Equal(t, "after", trailing)
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
