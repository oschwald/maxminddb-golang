package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursorAtConstructionDoesNotAllocate(t *testing.T) {
	data := NewDataDecoder([]byte{0x45, 'h', 'e', 'l', 'l', 'o'})
	d := NewDecoder(data, 0)
	var cursor Cursor
	allocs := testing.AllocsPerRun(100, func() { cursor = d.CursorAt(0) })
	require.Zero(t, allocs)
	require.Nil(t, data.stringCache.table.Load())
	value, _, err := cursor.ReadString()
	require.NoError(t, err)
	require.Equal(t, "hello", value)
	require.NotNil(
		t,
		data.stringCache.table.Load(),
		"the first eligible read initializes the shared table",
	)
}
