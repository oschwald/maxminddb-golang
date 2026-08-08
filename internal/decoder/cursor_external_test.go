package decoder_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/decoder"
)

type externalCursorValue struct {
	decoded string
	called  bool
}

func (v *externalCursorValue) UnmarshalMaxMindDBCursor(
	cursor decoder.Cursor,
) (decoder.Cursor, error) {
	value, next, err := cursor.ReadString()
	if err != nil {
		return decoder.Cursor{}, err
	}
	v.decoded = "external:" + value
	v.called = true
	return next, nil
}

func TestNestedExternalCursorUnmarshaler(t *testing.T) {
	type outer struct {
		Value externalCursorValue `maxminddb:"value"`
	}
	data := []byte{0xe1, 0x45, 'v', 'a', 'l', 'u', 'e', 0x43, 'F', 'o', 'o'}

	d := decoder.New(data)
	var result outer
	require.NoError(t, d.Decode(0, &result))
	require.Equal(t, "external:Foo", result.Value.decoded)
	require.True(t, result.Value.called)
}
