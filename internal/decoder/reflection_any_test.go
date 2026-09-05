package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

func TestDecodeAnyUnsignedScalars(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
		want uint64
	}{
		{"uint16 zero", []byte{0xa0}, 0},
		{"uint16", []byte{0xa2, 0xff, 0xff}, 65535},
		{"uint32 zero", []byte{0xc0}, 0},
		{"uint32", []byte{0xc4, 0xff, 0xff, 0xff, 0xff}, 1<<32 - 1},
		{"uint64 zero", []byte{0, 2}, 0},
		{"uint64", []byte{8, 2, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, ^uint64(0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, pointer := range []bool{false, true} {
				data := tt.data
				if pointer {
					data = append(ptr(2), data...)
				}
				d := New(data)
				var value any = "previous value"
				require.NoError(t, d.Decode(0, &value))
				require.Equal(t, tt.want, value)

				data = append([]byte{0xe1, 0x41, 'v'}, tt.data...)
				d = New(data)
				value = "previous value"
				require.NoError(t, d.DecodePath(0, []any{"v"}, &value))
				require.Equal(t, tt.want, value)
			}
		})
	}
}

func TestDecodeAnyUnsignedRejectsMalformedInput(t *testing.T) {
	for _, data := range [][]byte{
		{0xa3, 0, 0, 0},
		{0xc5, 0, 0, 0, 0, 0},
		{9, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0xa2, 0},
		{0xc4, 0, 0, 0},
		{8, 2, 0},
	} {
		d := New(data)
		var value any = "unchanged"
		err := d.Decode(0, &value)
		var invalid mmdberrors.InvalidDatabaseError
		require.ErrorAs(t, err, &invalid)
		require.Equal(t, "unchanged", value)
	}
}

func TestDecodeAnyUnsignedPreservesExistingPointer(t *testing.T) {
	data := []byte{0xc1, 7}
	d := New(data)
	var number uint32
	var value any = &number
	require.NoError(t, d.Decode(0, &value))
	require.Same(t, &number, value)
	require.Equal(t, uint32(7), number)

	var custom markedUint
	value = &custom
	d = New([]byte{1, 2, 7})
	require.NoError(t, d.Decode(0, &value))
	require.Same(t, &custom, value)
	require.NotEqual(t, markedUint(7), custom, "custom unmarshaler must not be bypassed")
}
