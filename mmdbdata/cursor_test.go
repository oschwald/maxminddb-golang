package mmdbdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZeroContainerCursorsAreRejected(t *testing.T) {
	t.Run("map", func(t *testing.T) {
		var entries MapCursor
		require.Zero(t, entries.Size())

		_, _, ok := entries.Next(Cursor{})
		require.False(t, ok)
		require.EqualError(t, entries.Err(), "invalid zero map cursor")

		end, err := entries.End()
		require.EqualError(t, err, "invalid zero map cursor")
		require.Zero(t, end)
	})

	t.Run("slice", func(t *testing.T) {
		var values SliceCursor
		size, ok := values.SizeForCapacity(0)
		require.Zero(t, size)
		require.False(t, ok)

		size, err := values.Size()
		require.EqualError(t, err, "invalid zero slice cursor")
		require.Zero(t, size)

		_, _, ok = values.Next(Cursor{})
		require.False(t, ok)
		require.EqualError(t, values.Err(), "invalid zero slice cursor")

		end, err := values.End()
		require.EqualError(t, err, "invalid zero slice cursor")
		require.Zero(t, end)
	})
}

func TestSliceRejectsMaximalOffset(t *testing.T) {
	decoder := NewDecoder([]byte{0}, ^uint(0))

	var err error
	require.NotPanics(t, func() {
		_, err = decoder.Cursor().Slice()
	})
	require.ErrorContains(t, err, "unexpected end of database")
}
