package mmdbdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDecoderSmallStringAllocations(t *testing.T) {
	buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o'}
	var value string
	allocations := testing.AllocsPerRun(100, func() {
		var err error
		value, err = NewDecoder(buffer, 0).ReadString()
		if err != nil {
			t.Fatal(err)
		}
	})
	require.Equal(t, "hello", value)
	// A small standalone decode needs only the decoder and the copied string.
	require.LessOrEqual(t, allocations, float64(2))
}

func TestNewDecoderStringsOwnTheirStorage(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		for _, cursor := range []bool{false, true} {
			name := "direct"
			if pointer {
				name = "pointer"
			}
			if cursor {
				name += "/cursor"
			} else {
				name += "/decoder"
			}
			t.Run(name, func(t *testing.T) {
				buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o'}
				if pointer {
					buffer = append([]byte{0x20, 2}, buffer...)
				}
				decoder := NewDecoder(buffer, 0)
				values := make([]string, 3)
				for i := range values {
					var err error
					if cursor {
						values[i], _, err = decoder.Cursor().ReadString()
					} else {
						values[i], err = decoder.ReadString()
					}
					require.NoError(t, err)
				}
				clear(buffer)
				require.Equal(t, []string{"hello", "hello", "hello"}, values)
			})
		}
	}
}
