package decoder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtendedKeyBytesFastPath(t *testing.T) {
	for _, size := range []int{29, 30, 100, 284} {
		t.Run(fmt.Sprintf("size%d", size), func(t *testing.T) {
			encoded := stringLeaf(size)
			for i := range size {
				encoded[2+i] = byte('a' + i%26)
			}
			data := append([]byte(nil), encoded...)
			data = append(data, 0xa1, 7)
			key, end := extendedKeyBytes(data, 1, 29)
			// A non-nil key proves that this size does not need the general decoder.
			require.NotNil(t, key)
			require.Equal(t, encoded[2:], key)
			require.Equal(t, uint(len(encoded)), end)
			require.Same(t, &data[2], &key[0])

			key, end = extendedKeyBytes(data[:len(encoded)-1], 1, 29)
			require.Nil(t, key)
			require.Zero(t, end)
		})
	}
}

func TestExtendedKeyBytesFallback(t *testing.T) {
	data := stringLeaf(29)
	for _, size := range []uint{0, 28, 30, 31} {
		key, end := extendedKeyBytes(data, 1, size)
		require.Nil(t, key)
		require.Zero(t, end)
	}
	for _, offset := range []uint{1, 2, ^uint(0)} {
		key, end := extendedKeyBytes(data[:1], offset, 29)
		require.Nil(t, key)
		require.Zero(t, end)
	}
	key, end := extendedKeyBytes(nil, 0, 29)
	require.Nil(t, key)
	require.Zero(t, end)
}
