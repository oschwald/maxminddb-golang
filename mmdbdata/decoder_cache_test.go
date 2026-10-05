package mmdbdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithStringCacheWarmReads(t *testing.T) {
	for _, test := range []struct {
		name    string
		options []DecoderOption
		allocs  float64
	}{
		{name: "default", allocs: 1},
		{name: "cached", options: []DecoderOption{WithStringCache()}},
		{name: "repeated_option", options: []DecoderOption{WithStringCache(), WithStringCache()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, pointer := range []bool{false, true} {
				buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o'}
				if pointer {
					buffer = append([]byte{0x20, 2}, buffer...)
				}
				d := NewDecoder(buffer, 0, test.options...)
				for range 3 {
					value, err := d.ReadString()
					require.NoError(t, err)
					require.Equal(t, "hello", value)
				}
				var value string
				allocs := testing.AllocsPerRun(100, func() {
					var err error
					value, err = d.ReadString()
					if err != nil {
						t.Fatal(err)
					}
				})
				require.LessOrEqual(t, allocs, test.allocs)
				require.Equal(t, "hello", value)
				cursor := d.Cursor()
				allocs = testing.AllocsPerRun(100, func() {
					var err error
					value, _, err = cursor.ReadString()
					if err != nil {
						t.Fatal(err)
					}
				})
				require.LessOrEqual(t, allocs, test.allocs)
				clear(buffer)
				require.Equal(t, "hello", value, "strings must own their storage")
			}
		})
	}
}

func TestWithStringCacheCursorAdvance(t *testing.T) {
	// A string followed by two pointers to the same string.
	buffer := []byte{0x45, 'h', 'e', 'l', 'l', 'o', 0x20, 0, 0x20, 0}
	d := NewDecoder(buffer, 0, WithStringCache())
	for range 2 {
		value, next, err := d.Cursor().ReadString()
		require.NoError(t, err)
		require.Equal(t, "hello", value)
		require.NoError(t, d.Advance(next))
	}
	var value string
	allocs := testing.AllocsPerRun(100, func() {
		var err error
		value, _, err = d.Cursor().ReadString()
		if err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocs, "advancing must retain the shared cache")
	require.Equal(t, "hello", value)
}

func TestWithStringCacheIndependentDecoders(t *testing.T) {
	first := NewDecoder([]byte{0x45, 'h', 'e', 'l', 'l', 'o'}, 0, WithStringCache())
	second := NewDecoder([]byte{0x45, 'w', 'o', 'r', 'l', 'd'}, 0, WithStringCache())
	for range 4 {
		value, err := first.ReadString()
		require.NoError(t, err)
		require.Equal(t, "hello", value)
		value, err = second.ReadString()
		require.NoError(t, err)
		require.Equal(t, "world", value)
	}
}
