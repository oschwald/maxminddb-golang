package decoder

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type cursorMapKey string

func (key *cursorMapKey) UnmarshalMaxMindDBCursor(cursor Cursor) (Cursor, error) {
	value, next, err := cursor.ReadString()
	if err == nil {
		*key = cursorMapKey(strings.ToUpper(value))
	}
	return next, err
}

type legacyMapKey string

func (key *legacyMapKey) UnmarshalMaxMindDB(decoder *Decoder) error {
	value, err := decoder.ReadString()
	if err == nil {
		*key = legacyMapKey("legacy:" + value)
	}
	return err
}

type dualMapKey string

func (key *dualMapKey) UnmarshalMaxMindDBCursor(cursor Cursor) (Cursor, error) {
	value, next, err := cursor.ReadString()
	if err == nil {
		*key = dualMapKey("cursor:" + value)
	}
	return next, err
}

func (key *dualMapKey) UnmarshalMaxMindDB(decoder *Decoder) error {
	value, err := decoder.ReadString()
	if err == nil {
		*key = dualMapKey("legacy:" + value)
	}
	return err
}

type skippingMapKey string

func (*skippingMapKey) UnmarshalMaxMindDBCursor(cursor Cursor) (Cursor, error) {
	return cursor.Skip()
}

var errMapKeyCallback = errors.New("map key callback failed")

type errorMapKey string

func (*errorMapKey) UnmarshalMaxMindDBCursor(Cursor) (Cursor, error) {
	return Cursor{}, errMapKeyCallback
}

func TestMapKeyCustomUnmarshal(t *testing.T) {
	data := []byte{
		0xe1,
		0x42, 'e', 'n',
		0x00, 0x07,
		0x45, 'a', 'f', 't', 'e', 'r',
	}

	t.Run("cursor", func(t *testing.T) {
		result := decodeMapWithTrailingValue[cursorMapKey](t, data)
		require.Equal(t, map[cursorMapKey]bool{"EN": false}, result)
	})

	t.Run("legacy", func(t *testing.T) {
		result := decodeMapWithTrailingValue[legacyMapKey](t, data)
		require.Equal(t, map[legacyMapKey]bool{"legacy:en": false}, result)
	})

	t.Run("cursor takes precedence", func(t *testing.T) {
		result := decodeMapWithTrailingValue[dualMapKey](t, data)
		require.Equal(t, map[dualMapKey]bool{"cursor:en": false}, result)
	})
}

func TestMapKeyCustomUnmarshalValidatesRawKey(t *testing.T) {
	data := []byte{
		0xe1,
		0xa1, 1,
		0x00, 0x07,
	}
	decoder := New(data)
	var result map[skippingMapKey]bool
	require.ErrorContains(t, decoder.Decode(0, &result), "unexpected map key type: Uint16")
}

func TestMapKeyCustomUnmarshalError(t *testing.T) {
	data := []byte{
		0xe1,
		0x42, 'e', 'n',
		0x00, 0x07,
	}
	decoder := New(data)
	var result map[errorMapKey]bool
	err := decoder.Decode(0, &result)
	require.ErrorIs(t, err, errMapKeyCallback)
	require.ErrorContains(t, err, "en")
}

func decodeMapWithTrailingValue[K ~string](t *testing.T, data []byte) map[K]bool {
	t.Helper()
	decoder := New(data)
	result := make(map[K]bool)
	next, err := decoder.decode(0, reflect.ValueOf(&result), 0)
	require.NoError(t, err)
	value, _, err := (Cursor{decoder: &decoder.DataDecoder, offset: next}).ReadString()
	require.NoError(t, err)
	require.Equal(t, "after", value)
	return result
}
