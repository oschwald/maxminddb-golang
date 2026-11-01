package decoder

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapKeyCapacity(t *testing.T) {
	for _, encoding := range []struct {
		name    string
		pointer []byte
		target  int
	}{
		{"inline", nil, 0},
		{"pointer1", []byte{0x20, 8}, 8},
		{"pointer2", []byte{0x28, 0, 8}, pointerBase2 + 8},
		{"pointer3", []byte{0x30, 0, 0, 8}, pointerBase3 + 8},
		{"pointer4", []byte{0x3f, 0, 0, 0, 8}, 8},
	} {
		for _, size := range []int{0, 2, 28, 29, 284, 285} {
			t.Run(fmt.Sprintf("%s/size%d", encoding.name, size), func(t *testing.T) {
				encoded := stringLeaf(size)
				data := make([]byte, encoding.target+len(encoded)+2)
				copy(data, encoding.pointer)
				copy(data[encoding.target:], encoded)
				copy(data[len(data)-2:], []byte{0xa1, 7})
				original := bytes.Clone(data)
				d := NewDataDecoderWithoutStringCache(data)
				for _, decode := range []func() ([]byte, uint, uint, error){
					func() ([]byte, uint, uint, error) { return d.decodeKeyAt(0) },
					func() ([]byte, uint, uint, error) {
						return d.decodeKeyAtSlow(0, uint(len(data)))
					},
				} {
					key, _, _, err := decode()
					require.NoError(t, err)
					require.Len(t, key, size)
					require.Equal(t, len(key), cap(key))
					if size > 0 {
						payload := encoding.target + len(encoded) - size
						require.Same(t, &data[payload], &key[0])
					}
					key = append(key, 0xff)
					require.Equal(t, byte(0xff), key[size])
					require.Equal(t, original, data, "append must not modify database bytes")
				}
			})
		}
	}
}
