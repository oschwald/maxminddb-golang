package decoder

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReflectionCompactStringPointerBudgetAndDepth(t *testing.T) {
	type namedString string
	for _, newDecoder := range []func([]byte) ReflectionDecoder{New, NewWithoutStringCache} {
		base := newDecoder([]byte{0x20, 0x04, 0xa1, 7, 0x43, 'a', 'b', 'c'})
		for _, depth := range []int{0, maximumDataStructureDepth - 1, maximumDataStructureDepth} {
			for _, remaining := range []uint32{2, 3, 4} {
				t.Run(
					fmt.Sprintf(
						"cache=%t/depth=%d/budget=%d",
						base.stringCache != nil,
						depth,
						remaining,
					),
					func(t *testing.T) {
						// Repeat with the same cache to cover admission and cache hits.
						for range 4 {
							d := newBudgetedDecoder(&base)
							d.payloadRemaining = remaining
							result := namedString("unchanged")
							next, err := d.decodeValue(
								0,
								addressableValue{Value: reflect.ValueOf(&result).Elem()},
								depth,
							)
							switch {
							case depth == maximumDataStructureDepth:
								require.ErrorContains(
									t,
									err,
									"exceeded maximum data structure depth",
								)
								require.Equal(t, namedString("unchanged"), result)
								require.Equal(t, remaining, d.payloadRemaining)
							case remaining < 3:
								requireExpansionLimit(t, err)
								require.Equal(t, namedString("unchanged"), result)
							default:
								require.NoError(t, err)
								require.Equal(t, namedString("abc"), result)
								require.Equal(t, remaining-3, d.payloadRemaining)
								require.Equal(t, uint(2), next)
							}
						}
					},
				)
			}
		}
	}
}

func TestReflectionStringPointerFallbacks(t *testing.T) {
	type namedString string
	for _, data := range [][]byte{
		{0x20},                   // Truncated pointer.
		{0x20, 0xff},             // Missing target.
		{0x20, 2, 0x43, 'a'},     // Truncated compact string.
		{0x20, 2, 0x20, 4, 0x40}, // Pointer to pointer.
		{0x20, 2, 0xa1, 7},       // Wrong target kind.
	} {
		d := New(data)
		result := namedString("unchanged")
		require.Error(t, d.Decode(0, &result))
		require.Equal(t, namedString("unchanged"), result)
	}
	for _, target := range [][]byte{
		stringLeaf(29),
		{0x03, 0xfb, 'a', 'b', 'c'}, // Extended kind wraps to string; retain generic handling.
	} {
		d := New(append(ptr(2), target...))
		var got, want namedString
		require.NoError(t, d.Decode(2, &want))
		require.NoError(t, d.Decode(0, &got))
		require.Equal(t, want, got)
	}

	d := New([]byte{0x20, 2, 0x43, 'a', 'b', 'c'})
	var legacy markedString
	require.NoError(t, d.Decode(0, &legacy))
	require.Equal(t, markedString("X:abc"), legacy)
	var cursor cursorMarkedString
	require.NoError(t, d.Decode(0, &cursor))
	require.Equal(t, cursorMarkedString("cursor:abc"), cursor)
}
