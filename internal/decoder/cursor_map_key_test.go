package decoder

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursorReadMapKeyPointerEncodings(t *testing.T) {
	for _, pointer := range []struct {
		name   string
		bytes  []byte
		target int
	}{
		{"one byte", []byte{0x20, 8}, 8},
		{"one byte with high bits", []byte{0x27, 0xff}, 2047},
		{"two bytes", []byte{0x28, 0, 8}, pointerBase2 + 8},
		{"three bytes", []byte{0x30, 0, 0, 8}, pointerBase3 + 8},
		{"four bytes", []byte{0x38, 0, 0, 0, 8}, 8},
		{"four bytes with ignored bits", []byte{0x3f, 0, 0, 0, 8}, 8},
	} {
		for _, size := range []int{0, 2, 28, 29, 30, 284, 285} {
			t.Run(fmt.Sprintf("%s/size%d", pointer.name, size), func(t *testing.T) {
				encoded := stringLeaf(size)
				headerSize := len(encoded) - size
				for i := range size {
					encoded[headerSize+i] = byte('a' + i%26)
				}
				data := make([]byte, pointer.target+len(encoded))
				copy(data, pointer.bytes)
				copy(data[len(pointer.bytes):], []byte{0xa1, 7})
				copy(data[pointer.target:], encoded)
				cursor := NewDecoder(NewDataDecoderWithoutStringCache(data), 0).Cursor()

				key, next, err := cursor.ReadMapKey()
				require.NoError(t, err)
				require.Equal(t, encoded[headerSize:], key)
				require.Equal(t, size, cap(key))
				require.Equal(t, uint(len(pointer.bytes)), next.offset)
				require.Zero(t, next.originToken)
				require.Same(t, cursor.decoder, next.decoder)
				if size > 0 {
					require.Same(t, &data[pointer.target+headerSize], &key[0])
				}
				value, _, err := next.ReadUint()
				require.NoError(t, err)
				require.Equal(t, uint64(7), value)
				assertMapKeyMatchesSlow(t, cursor)
			})
		}
	}
}

func TestCursorReadMapKeyBackwardPointer(t *testing.T) {
	data := []byte{0x43, 'k', 'e', 'y', 0x20, 0, 0xa1, 7}
	cursor := NewDecoder(NewDataDecoderWithoutStringCache(data), 4).Cursor()
	key, next, err := cursor.ReadMapKey()
	require.NoError(t, err)
	require.Equal(t, "key", string(key))
	require.Equal(t, uint(6), next.offset)
	value, _, err := next.ReadUint()
	require.NoError(t, err)
	require.Equal(t, uint64(7), value)
	assertMapKeyMatchesSlow(t, cursor)
}

func TestCursorReadMapKeyFallbacks(t *testing.T) {
	for _, size := range []int{0, 28, 29, 284, 285} {
		t.Run(fmt.Sprintf("inline/size%d", size), func(t *testing.T) {
			data := append(stringLeaf(size), 0xa1, 7)
			cursor := NewDecoder(NewDataDecoderWithoutStringCache(data), 0).Cursor()
			key, next, err := cursor.ReadMapKey()
			require.NoError(t, err)
			require.Len(t, key, size)
			require.Equal(t, size, cap(key))
			value, _, err := next.ReadUint()
			require.NoError(t, err)
			require.Equal(t, uint64(7), value)
			assertMapKeyMatchesSlow(t, cursor)
		})
	}

	for _, data := range [][]byte{
		nil,
		{0x20},                            // Missing pointer payload.
		{0x20, 8},                         // Missing target.
		{0x20, 0},                         // Pointer to itself.
		{0x20, 2, 0x20, 4, 0x40},          // Pointer to pointer.
		{0x20, 2, 0xa1, 7},                // Integer key.
		{0x20, 2, 0xe0},                   // Map key.
		{0x20, 2, 0x42, 'a'},              // Truncated compact string.
		{0x20, 2, 0x5d},                   // Missing extended size byte.
		{0x20, 2, 0x5d, 0, 'a'},           // Truncated extended string.
		{0x20, 2, 0x02, 0x07},             // Invalid bool size.
		{0x20, 2, 0x48, 'a', 'b', 'c'},    // Truncated string payload.
		{0x38, 0, 0, 0, 5, 0x20, 7, 0x40}, // Wide pointer to pointer.
	} {
		t.Run(fmt.Sprintf("malformed/%x", data), func(t *testing.T) {
			cursor := NewDecoder(NewDataDecoderWithoutStringCache(data), 0).Cursor()
			_, _, err := cursor.ReadMapKey()
			requireMalformedDatabaseError(t, err)
			assertMapKeyMatchesSlow(t, cursor)
		})
	}
	assertMapKeyMatchesSlow(t, Cursor{})
	invalidOffset := NewDecoder(NewDataDecoderWithoutStringCache([]byte{0x40}), ^uint(0)).Cursor()
	assertMapKeyMatchesSlow(t, invalidOffset)
}

func FuzzCursorReadMapKey(f *testing.F) {
	for _, size := range []int{0, 2, 28, 29, 284, 285} {
		f.Add(append([]byte{0x20, 2}, stringLeaf(size)...), uint64(0))
		f.Add(stringLeaf(size), uint64(0))
	}
	f.Add([]byte{0x27, 0xff}, uint64(0))
	f.Add([]byte{0x20, 2, 0x20, 0}, uint64(0))
	f.Add([]byte{0x40}, ^uint64(0))
	f.Fuzz(func(t *testing.T, data []byte, offset uint64) {
		cursor := NewDecoder(NewDataDecoderWithoutStringCache(data), uint(offset)).Cursor()
		assertMapKeyMatchesSlow(t, cursor)
	})
}

func assertMapKeyMatchesSlow(t *testing.T, cursor Cursor) {
	t.Helper()
	key, next, err := cursor.ReadMapKey()
	wantKey, wantNext, wantErr := readMapKeyReference(cursor)
	if wantErr != nil {
		require.EqualError(t, err, wantErr.Error())
		require.Equal(t, reflect.TypeOf(wantErr), reflect.TypeOf(err))
	} else {
		require.NoError(t, err)
	}
	require.Equal(t, wantKey, key)
	require.Equal(t, wantNext, next)
	if err == nil {
		_, cacheOffset, nextOffset, decodeErr := cursor.decoder.decodeKeyAt(cursor.offset)
		require.NoError(t, decodeErr)
		_, wantCacheOffset, wantNextOffset, slowErr := cursor.decoder.decodeKeyAtSlow(
			cursor.offset, uint(len(cursor.decoder.buffer)),
		)
		require.NoError(t, slowErr)
		require.Equal(t, wantCacheOffset, cacheOffset)
		require.Equal(t, wantNextOffset, nextOffset)
	}
}

func readMapKeyReference(cursor Cursor) ([]byte, Cursor, error) {
	if err := cursor.validate(); err != nil {
		return nil, Cursor{}, err
	}
	key, _, next, err := cursor.decoder.decodeKeyAtSlow(
		cursor.offset, uint(len(cursor.decoder.buffer)),
	)
	if err != nil {
		return nil, Cursor{}, cursor.wrapError(err)
	}
	return key, Cursor{decoder: cursor.decoder, offset: next}, nil
}
