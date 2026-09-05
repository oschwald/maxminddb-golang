package decoder

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

// ptr returns a two-byte pointer control record addressing target, which must
// be below 2048.
func ptr(target int) []byte {
	return []byte{0x20 | byte((target>>8)&0x7), byte(target)}
}

func sliceHeader(n int) []byte {
	switch {
	case n < 29:
		return []byte{byte(n), 0x04}
	case n < 285:
		return []byte{0x1D, 0x04, byte(n - 29)}
	case n < 65821:
		v := n - 285
		return []byte{0x1E, 0x04, byte(v >> 8), byte(v)}
	default:
		v := n - 65821
		return []byte{0x1F, 0x04, byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

func mapHeader(n int) []byte {
	switch {
	case n < 29:
		return []byte{0xE0 | byte(n)}
	case n < 285:
		return []byte{0xE0 | 29, byte(n - 29)}
	case n < 65821:
		v := n - 285
		return []byte{0xE0 | 30, byte(v >> 8), byte(v)}
	default:
		v := n - 65821
		return []byte{0xE0 | 31, byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

func extendedMapHeader(n int) []byte {
	switch {
	case n < 29:
		return []byte{byte(n), 0x00}
	case n < 285:
		return []byte{0x1D, 0x00, byte(n - 29)}
	case n < 65821:
		v := n - 285
		return []byte{0x1E, 0x00, byte(v >> 8), byte(v)}
	default:
		v := n - 65821
		return []byte{0x1F, 0x00, byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

func bytesLeaf(size int) []byte {
	b := make([]byte, 0, size+4)
	switch {
	case size < 29:
		b = append(b, 0x80|byte(size))
	case size < 285:
		b = append(b, 0x80|29, byte(size-29))
	case size < 65821:
		v := size - 285
		b = append(b, 0x80|30, byte(v>>8), byte(v))
	default:
		v := size - 65821
		b = append(b, 0x80|31, byte(v>>16), byte(v>>8), byte(v))
	}
	return append(b, make([]byte, size)...)
}

func stringLeaf(size int) []byte {
	b := bytesLeaf(size)
	b[0] = (b[0] & 0x1f) | 0x40
	return b
}

func encodedString(value string) []byte {
	if len(value) >= 29 {
		panic("test helper only supports compact strings")
	}
	return append([]byte{0x40 | byte(len(value))}, value...)
}

func uint16Array(n int) []byte {
	b := sliceHeader(n)
	for range n {
		b = append(b, 0xA0)
	}
	return b
}

// binaryFanOutDepth returns a tiny encoding that expands to 2**depth leaves.
func binaryFanOutDepth(depth int) []byte {
	buf := make([]byte, 0, depth*6+1)
	for i := range depth {
		target := 6 * (i + 1)
		buf = append(buf, 0x02, 0x04)
		buf = append(buf, ptr(target)...)
		buf = append(buf, ptr(target)...)
	}
	return append(buf, 0xA0)
}

func binaryFanOut() []byte {
	return binaryFanOutDepth(20)
}

const payloadFanOut = 700

func appendFanOut(buf []byte) []byte {
	buf = append(buf, sliceHeader(payloadFanOut)...)
	for range payloadFanOut {
		buf = append(buf, ptr(0)...)
	}
	return buf
}

func decodeAt(buf []byte, offset uint, v any) error {
	d := New(buf)
	return d.Decode(offset, v)
}

func requireExpansionLimit(t *testing.T, err error) {
	t.Helper()
	var invalid mmdberrors.InvalidDatabaseError
	require.ErrorAs(t, err, &invalid)
	assert.Contains(t, err.Error(), "maximum decoded record size")
}

func TestDynamicPointerFanOutRejected(t *testing.T) {
	var out any
	requireExpansionLimit(t, decodeAt(binaryFanOut(), 0, &out))
}

func TestDynamicPayloadFanOutRejected(t *testing.T) {
	const (
		leafSize = 100 << 10
	)

	t.Run("direct", func(t *testing.T) {
		buf := bytesLeaf(leafSize)
		root := len(buf)
		buf = appendFanOut(buf)

		var out any
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
	})

	t.Run("wrapped", func(t *testing.T) {
		buf := append([]byte{0x01, 0x04}, bytesLeaf(leafSize)...)
		root := len(buf)
		buf = appendFanOut(buf)

		var out any
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
	})

	t.Run("string without cache", func(t *testing.T) {
		buf := stringLeaf(leafSize)
		root := len(buf)
		buf = appendFanOut(buf)

		decoder := NewWithoutStringCache(buf)
		var out any
		requireExpansionLimit(t, decoder.Decode(uint(root), &out))
	})
}

func TestRootBudgetErrorIncludesOffset(t *testing.T) {
	const (
		leafSize = 100 << 10
	)
	buf := bytesLeaf(leafSize)
	root := len(buf)
	buf = appendFanOut(buf)

	var out any
	err := decodeAt(buf, uint(root), &out)
	requireExpansionLimit(t, err)
	var contextErr mmdberrors.ContextualError
	require.ErrorAs(t, err, &contextErr)
	require.Equal(t, uint(root), contextErr.Offset)
}

func TestDynamicMapKeyFanOutRejected(t *testing.T) {
	const (
		leafSize = 256 << 10
		entries  = 16
	)
	buf := stringLeaf(leafSize)
	root := len(buf)
	buf = append(buf, mapHeader(entries)...)
	for range entries {
		buf = append(buf, ptr(0)...)
		buf = append(buf, 0xA0)
	}

	var out map[string]any
	requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
}

func pointer4KKeyMap(entries int) ([]byte, uint) {
	buf := stringLeaf(4 << 10)
	root := uint(len(buf))
	buf = append(buf, mapHeader(entries)...)
	for range entries {
		buf = append(buf, ptr(0)...)
		buf = append(buf, 0xA0)
	}
	return buf, root
}

func TestMapKeyPayloadBoundary(t *testing.T) {
	const keySize = 4 << 10

	t.Run("materialized map keys", func(t *testing.T) {
		exactCount := decodePayloadBudgetBytes / keySize
		buf, root := pointer4KKeyMap(exactCount)
		var exact map[string]uint16
		require.NoError(t, decodeAt(buf, root, &exact))

		buf, root = pointer4KKeyMap(exactCount + 1)
		var over map[string]uint16
		requireExpansionLimit(t, decodeAt(buf, root, &over))
	})

	for _, test := range []struct {
		name string
		out  func() any
	}{
		{
			name: "cursor custom map keys",
			out:  func() any { return new(map[cursorMapKey]uint16) },
		},
		{
			name: "legacy custom map keys",
			out:  func() any { return new(map[legacyMapKey]uint16) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			exactCount := decodePayloadBudgetBytes / keySize
			buf, root := pointer4KKeyMap(exactCount)
			require.NoError(t, decodeAt(buf, root, test.out()))

			buf, root = pointer4KKeyMap(exactCount + 1)
			err := decodeAt(buf, root, test.out())
			requireExpansionLimit(t, err)
			var contextErr mmdberrors.ContextualError
			require.ErrorAs(t, err, &contextErr)
			assert.Empty(t, contextErr.Path)
		})
	}
}

func TestInterfaceMapKeyPayloadChargedOnce(t *testing.T) {
	t.Run("pointer-backed keys", func(t *testing.T) {
		const exactCount = decodePayloadBudgetBytes / (4 << 10)
		buf, root := pointer4KKeyMap(exactCount)
		var exact map[any]uint16
		require.NoError(t, decodeAt(buf, root, &exact))

		buf, root = pointer4KKeyMap(exactCount + 1)
		var over map[any]uint16
		requireExpansionLimit(t, decodeAt(buf, root, &over))
	})

	for _, size := range []int{
		decodePayloadBudgetBytes/2 + 1,
		decodePayloadBudgetBytes,
		decodePayloadBudgetBytes + 1,
	} {
		data := append(mapHeader(1), stringLeaf(size)...)
		data = append(data, 0xa1, 7)
		decoder := New(data)
		// Repeat to verify that each decode starts with a fresh payload budget.
		for range 2 {
			var got map[any]uint16
			err := decoder.Decode(0, &got)
			if size > decodePayloadBudgetBytes {
				requireExpansionLimit(t, err)
				continue
			}
			require.NoError(t, err)
			require.Equal(t, map[any]uint16{string(make([]byte, size)): 7}, got)
		}
	}
}

func TestDynamicExpansionBoundary(t *testing.T) {
	units := decodeExpansionBudgetBytes >> decodeBudgetUnitShift

	t.Run("container at limit", func(t *testing.T) {
		var out any
		require.NoError(t, decodeAt(uint16Array(units), 0, &out))
	})

	t.Run("container over limit", func(t *testing.T) {
		var out any
		requireExpansionLimit(t, decodeAt(uint16Array(units+1), 0, &out))
	})

	t.Run("dynamic standalone payload is bounded", func(t *testing.T) {
		const size = decodePayloadBudgetBytes + 1

		var out any
		requireExpansionLimit(t, decodeAt(bytesLeaf(size), 0, &out))

		buf := bytesLeaf(size)
		root := len(buf)
		buf = append(buf, ptr(0)...)
		out = nil
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))

		mapRoot := len(buf)
		buf = append(buf, mapHeader(1)...)
		buf = append(buf, encodedString("value")...)
		buf = append(buf, ptr(0)...)
		out = nil
		decoder := New(buf)
		requireExpansionLimit(t, decoder.DecodePath(
			uint(mapRoot),
			[]any{"value"},
			&out,
		))
	})

	t.Run("dynamic standalone string boundary", func(t *testing.T) {
		var exact any
		require.NoError(t, decodeAt(stringLeaf(decodePayloadBudgetBytes), 0, &exact))
		require.Len(t, exact, decodePayloadBudgetBytes)

		var over any
		requireExpansionLimit(t, decodeAt(
			stringLeaf(decodePayloadBudgetBytes+1),
			0,
			&over,
		))
	})

	t.Run("named empty-interface boundary", func(t *testing.T) {
		type namedAny any

		const size = decodePayloadBudgetBytes + 1

		var scalar namedAny
		require.NoError(t, decodeAt(bytesLeaf(size), 0, &scalar))
		payload, ok := scalar.([]byte)
		require.True(t, ok)
		require.Len(t, payload, size)

		var container namedAny
		requireExpansionLimit(t, decodeAt(binaryFanOut(), 0, &container))
	})

	t.Run("typed standalone payload is not expansion bounded", func(t *testing.T) {
		const size = decodePayloadBudgetBytes + 1

		buf := bytesLeaf(size)
		var out []byte
		require.NoError(t, decodeAt(buf, 0, &out))
		require.Len(t, out, size)

		out = nil
		decoder := New(buf)
		require.NoError(t, decoder.DecodePath(0, nil, &out))
		require.Len(t, out, size)

		root := len(buf)
		buf = append(buf, ptr(0)...)
		out = nil
		require.NoError(t, decodeAt(buf, uint(root), &out))
		require.Len(t, out, size)
	})

	t.Run("typed standalone string is not expansion bounded", func(t *testing.T) {
		const size = decodePayloadBudgetBytes + 1

		var out string
		require.NoError(t, decodeAt(stringLeaf(size), 0, &out))
		require.Len(t, out, size)
	})
}

func TestPayloadExpansionBoundary(t *testing.T) {
	const leafSize = 4 << 10

	for _, test := range []struct {
		name string
		leaf func(int) []byte
		out  func() any
	}{
		{
			name: "bytes",
			leaf: bytesLeaf,
			out:  func() any { return new([][]byte) },
		},
		{
			name: "strings",
			leaf: stringLeaf,
			out:  func() any { return new([]string) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			exactCount := decodePayloadBudgetBytes / leafSize
			makeBuffer := func(count int) ([]byte, uint) {
				buf := test.leaf(leafSize)
				root := uint(len(buf))
				buf = append(buf, sliceHeader(count)...)
				for range count {
					buf = append(buf, ptr(0)...)
				}
				return buf, root
			}

			buf, root := makeBuffer(exactCount)
			exact := test.out()
			require.NoError(t, decodeAt(buf, root, exact))

			buf, root = makeBuffer(exactCount + 1)
			over := test.out()
			requireExpansionLimit(t, decodeAt(buf, root, over))
		})
	}

	t.Run("concrete duplicate field", func(t *testing.T) {
		type record struct {
			Payload string `maxminddb:"payload,maxsize:4096"`
		}

		makeBuffer := func(count int) ([]byte, uint) {
			buf := stringLeaf(leafSize)
			root := uint(len(buf))
			buf = append(buf, mapHeader(count)...)
			for range count {
				buf = append(buf, encodedString("payload")...)
				buf = append(buf, ptr(0)...)
			}
			return buf, root
		}

		exactCount := decodePayloadBudgetBytes / leafSize
		buf, root := makeBuffer(exactCount)
		var exact record
		require.NoError(t, decodeAt(buf, root, &exact))

		buf, root = makeBuffer(exactCount + 1)
		var over record
		requireExpansionLimit(t, decodeAt(buf, root, &over))
	})

	for _, test := range []struct {
		name string
		leaf func(int) []byte
		out  func() any
	}{
		{
			name: "short bytes are charged exactly",
			leaf: bytesLeaf,
			out:  func() any { return new([][]byte) },
		},
		{
			name: "short strings are charged exactly",
			leaf: stringLeaf,
			out:  func() any { return new([]string) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			const leafSize = 64
			elementCount := decodePayloadBudgetBytes / leafSize
			makeBuffer := func(lastSize int) ([]byte, uint) {
				buf := test.leaf(leafSize)
				lastOffset := 0
				if lastSize != leafSize {
					lastOffset = len(buf)
					buf = append(buf, test.leaf(lastSize)...)
				}
				root := uint(len(buf))
				buf = append(buf, sliceHeader(elementCount)...)
				for range elementCount - 1 {
					buf = append(buf, ptr(0)...)
				}
				buf = append(buf, ptr(lastOffset)...)
				return buf, root
			}

			buf, root := makeBuffer(leafSize)
			require.NoError(t, decodeAt(buf, root, test.out()))

			buf, root = makeBuffer(leafSize + 1)
			requireExpansionLimit(t, decodeAt(buf, root, test.out()))
		})
	}
}

func TestConcretePreallocatedContainerIsExpansionBounded(t *testing.T) {
	n := (decodeExpansionBudgetBytes >> decodeBudgetUnitShift) + 1
	out := make([]uint16, n)
	requireExpansionLimit(t, decodeAt(uint16Array(n), 0, &out))
}

func TestConcreteDirectContainerIsExpansionBounded(t *testing.T) {
	n := (decodeExpansionBudgetBytes >> decodeBudgetUnitShift) + 1
	var out []uint16
	requireExpansionLimit(t, decodeAt(uint16Array(n), 0, &out))
	require.Nil(t, out)
}

func TestConcretePayloadCollectionIsExpansionBounded(t *testing.T) {
	t.Run("bytes", func(t *testing.T) {
		const (
			leafSize = 100 << 10
			fanOut   = 22
		)
		buf := bytesLeaf(leafSize)
		root := len(buf)
		buf = append(buf, sliceHeader(fanOut)...)
		for range fanOut {
			buf = append(buf, ptr(0)...)
		}

		var out [][]byte
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
	})

	t.Run("strings", func(t *testing.T) {
		const (
			leafSize = 4 << 10
			fanOut   = 600
		)
		buf := stringLeaf(leafSize)
		root := len(buf)
		buf = append(buf, sliceHeader(fanOut)...)
		for range fanOut {
			buf = append(buf, ptr(0)...)
		}

		var out []string
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
	})

	t.Run("wrapped scalar", func(t *testing.T) {
		type record struct {
			Payload []byte `maxminddb:"payload"`
		}
		const (
			leafSize = 100 << 10
			fanOut   = 22
		)
		buf := mapHeader(1)
		buf = append(buf, encodedString("payload")...)
		buf = append(buf, bytesLeaf(leafSize)...)
		root := len(buf)
		buf = append(buf, sliceHeader(fanOut)...)
		for range fanOut {
			buf = append(buf, ptr(0)...)
		}

		var out []record
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
	})
}

func TestBoundedStringReservesPayloadBeforeMaterialization(t *testing.T) {
	type record struct {
		Padding string `maxminddb:"padding"`
		Limited string `maxminddb:"limited,maxsize:3"`
	}

	data := make([]byte, 1, decodePayloadBudgetBytes+64)
	data[0] = 0xe2
	data = append(data, encodedString("padding")...)
	data = append(data, stringLeaf(decodePayloadBudgetBytes-2)...)
	data = append(data, encodedString("limited")...)
	limitedOffset := uint(len(data))
	data = append(data, encodedString("abc")...)

	decoder := New(data)
	got := record{Limited: "keep"}
	requireExpansionLimit(t, decoder.Decode(0, &got))
	require.Equal(t, "keep", got.Limited)

	// A cold string read records its control-record offset as a cache miss.
	// The aggregate payload check must reject this value before that read.
	primary := limitedOffset & (stringCacheSlots - 1)
	alternate := stringCacheAlternateIndex(limitedOffset, primary)
	require.NotEqual(
		t,
		uint64(limitedOffset)+1,
		decoder.stringCache.recentMisses[alternate].Load(),
	)
}

func TestCitySubdivisionsRejectDeclaredSizeBeforeAllocating(t *testing.T) {
	type subdivision struct {
		GeoNameID uint `maxminddb:"geoname_id"`
	}
	type city struct {
		Subdivisions []subdivision `maxminddb:"subdivisions"`
	}

	n := (decodeExpansionBudgetBytes >> decodeBudgetUnitShift) + 1
	buf := mapHeader(1)
	buf = append(buf, encodedString("subdivisions")...)
	buf = append(buf, sliceHeader(n)...)

	var out city
	requireExpansionLimit(t, decodeAt(buf, 0, &out))
	require.Nil(t, out.Subdivisions)

	var subdivisions []subdivision
	decoder := New(buf)
	requireExpansionLimit(t, decoder.DecodePath(
		0,
		[]any{"subdivisions"},
		&subdivisions,
	))
	require.Nil(t, subdivisions)
}

func TestConcreteMapRejectsDeclaredSizeBeforeAllocating(t *testing.T) {
	n := (decodeExpansionBudgetBytes >> decodeBudgetUnitShift) / 2
	var out map[string]uint16
	requireExpansionLimit(t, decodeAt(mapHeader(n+1), 0, &out))
	require.Nil(t, out)
}

func TestConcreteSequenceChargesRepeatedSharedStructMaps(t *testing.T) {
	type emptyRecord struct{}

	const (
		mapSize = 200
		fanOut  = 300
	)
	buf := mapHeader(mapSize)
	for range mapSize {
		buf = append(buf, 0x40, 0xE0)
	}
	root := len(buf)
	buf = append(buf, sliceHeader(fanOut)...)
	for range fanOut {
		buf = append(buf, ptr(0)...)
	}

	var out []emptyRecord
	requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
}

func TestNestedStructFastPathsChargeMaps(t *testing.T) {
	type child struct{}
	type record struct {
		Child child `maxminddb:"child"`
	}
	type pointerRecord struct {
		Child *child `maxminddb:"child"`
	}

	t.Run("declared direct map", func(t *testing.T) {
		n := (decodeExpansionBudgetBytes >> decodeBudgetUnitShift) / 2
		buf := mapHeader(1)
		buf = append(buf, encodedString("child")...)
		buf = append(buf, mapHeader(n)...)

		var out record
		requireExpansionLimit(t, decodeAt(buf, 0, &out))

		var pointerOut pointerRecord
		requireExpansionLimit(t, decodeAt(buf, 0, &pointerOut))
		require.Nil(t, pointerOut.Child)
	})

	t.Run("repeated shared pointer map", func(t *testing.T) {
		const (
			mapSize = 200
			fanOut  = 300
		)
		buf := mapHeader(mapSize)
		for range mapSize {
			buf = append(buf, 0x40, 0xE0)
		}
		recordOffset := len(buf)
		buf = append(buf, mapHeader(1)...)
		buf = append(buf, encodedString("child")...)
		buf = append(buf, ptr(0)...)
		root := len(buf)
		buf = append(buf, sliceHeader(fanOut)...)
		for range fanOut {
			buf = append(buf, ptr(recordOffset)...)
		}

		var out []record
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))

		var pointerOut []pointerRecord
		requireExpansionLimit(t, decodeAt(buf, uint(root), &pointerOut))
	})

	t.Run("pointer target payload", func(t *testing.T) {
		type payloadChild struct {
			Payload []byte `maxminddb:"payload"`
		}
		type payloadRecord struct {
			Child payloadChild `maxminddb:"child"`
		}
		type pointerPayloadRecord struct {
			Child *payloadChild `maxminddb:"child"`
		}

		leafSize := decodeExpansionBudgetBytes + (1 << decodeBudgetUnitShift)
		buf := mapHeader(1)
		buf = append(buf, encodedString("payload")...)
		buf = append(buf, bytesLeaf(leafSize)...)
		root := len(buf)
		buf = append(buf, mapHeader(1)...)
		buf = append(buf, encodedString("child")...)
		buf = append(buf, ptr(0)...)

		var out payloadRecord
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))

		var pointerOut pointerPayloadRecord
		requireExpansionLimit(t, decodeAt(buf, uint(root), &pointerOut))
		require.Nil(t, pointerOut.Child)
	})
}

func TestNestedAnyLikeDestinationsAreBudgeted(t *testing.T) {
	t.Run("struct field", func(t *testing.T) {
		type record struct {
			Values []any `maxminddb:"values"`
		}

		buf := binaryFanOut()
		root := len(buf)
		buf = append(buf, mapHeader(1)...)
		buf = append(buf, encodedString("values")...)
		buf = append(buf, ptr(0)...)

		var out record
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
	})

	t.Run("slice element", func(t *testing.T) {
		type element struct {
			Value any `maxminddb:"value"`
		}

		buf := binaryFanOut()
		elementOffset := len(buf)
		buf = append(buf, mapHeader(1)...)
		buf = append(buf, encodedString("value")...)
		buf = append(buf, ptr(0)...)
		root := len(buf)
		buf = append(buf, sliceHeader(1)...)
		buf = append(buf, ptr(elementOffset)...)

		var out []element
		requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
	})
}

func TestRecursiveConcreteTypeAcceptsBoundedData(t *testing.T) {
	type node struct {
		Children []*node `maxminddb:"children"`
	}

	var out node
	require.NoError(t, decodeAt([]byte{0xE0}, 0, &out))
}

func TestRecursiveConcretePointerFanOutIsRejected(t *testing.T) {
	type node struct {
		Children []*node `maxminddb:"children"`
	}

	const fanOutDepth = 20
	buf := make([]byte, 1, 1+fanOutDepth*16)
	buf[0] = 0xE0
	previous := 0
	for range fanOutDepth {
		root := len(buf)
		buf = append(buf, mapHeader(1)...)
		buf = append(buf, encodedString("children")...)
		buf = append(buf, sliceHeader(2)...)
		buf = append(buf, ptr(previous)...)
		buf = append(buf, ptr(previous)...)
		previous = root
	}

	var out node
	requireExpansionLimit(t, decodeAt(buf, uint(previous), &out))
}

func TestRecursiveTypeWithDynamicFieldIsBudgeted(t *testing.T) {
	type node struct {
		Children []*node `maxminddb:"children"`
		Value    any     `maxminddb:"value"`
	}

	buf := binaryFanOut()
	root := len(buf)
	buf = append(buf, mapHeader(1)...)
	buf = append(buf, encodedString("value")...)
	buf = append(buf, ptr(0)...)

	var out node
	requireExpansionLimit(t, decodeAt(buf, uint(root), &out))
}

func TestDecodePathBudgetsDynamicResult(t *testing.T) {
	var out any
	decoder := New(binaryFanOut())
	requireExpansionLimit(t, decoder.DecodePath(0, []any{0}, &out))
}

func TestPointerToInterfaceDestinationsAreBudgeted(t *testing.T) {
	t.Run("one pointer", func(t *testing.T) {
		var out *any
		requireExpansionLimit(t, decodeAt(binaryFanOut(), 0, &out))
	})

	t.Run("two pointers", func(t *testing.T) {
		var out **any
		requireExpansionLimit(t, decodeAt(binaryFanOut(), 0, &out))
	})

	t.Run("decode path", func(t *testing.T) {
		var out *any
		decoder := New(binaryFanOut())
		requireExpansionLimit(t, decoder.DecodePath(0, []any{0}, &out))
	})
}

func TestDecodePathBudgetsPointerBackedKeys(t *testing.T) {
	const keySize = 4 << 10
	exactCount := decodePayloadBudgetBytes / keySize

	buf, root := pointer4KKeyMap(exactCount)
	out := uint16(42)
	decoder := New(buf)
	require.NoError(t, decoder.DecodePath(root, []any{"missing"}, &out))
	require.Equal(t, uint16(42), out)

	buf, root = pointer4KKeyMap(exactCount + 1)
	decoder = New(buf)
	requireExpansionLimit(t, decoder.DecodePath(root, []any{"missing"}, &out))
}

func TestDecodePathSharesOperationBudgets(t *testing.T) {
	t.Run("container work across hops", func(t *testing.T) {
		limit := decodeExpansionBudgetBytes >> decodeBudgetUnitShift
		buf := sliceHeader(limit / 2)
		buf = append(buf, sliceHeader(limit/2+1)...)

		var out uint16
		decoder := New(buf)
		requireExpansionLimit(t, decoder.DecodePath(0, []any{0, 0}, &out))
	})

	t.Run("navigation and selected payload", func(t *testing.T) {
		const keySize = 4 << 10
		exactCount := decodePayloadBudgetBytes / keySize
		buf := stringLeaf(keySize)
		root := uint(len(buf))
		buf = append(buf, mapHeader(exactCount+1)...)
		for range exactCount {
			buf = append(buf, ptr(0)...)
			buf = append(buf, 0xA0)
		}
		buf = append(buf, encodedString("target")...)
		buf = append(buf, stringLeaf(1)...)

		var out string
		decoder := New(buf)
		requireExpansionLimit(t, decoder.DecodePath(root, []any{"target"}, &out))
	})

	t.Run("nonempty path budgets typed scalar", func(t *testing.T) {
		size := decodePayloadBudgetBytes + 1
		buf := mapHeader(1)
		buf = append(buf, encodedString("value")...)
		buf = append(buf, bytesLeaf(size)...)

		var out []byte
		decoder := New(buf)
		requireExpansionLimit(t, decoder.DecodePath(0, []any{"value"}, &out))
	})
}

func TestDecodePathBudgetsSkippedInlineContainers(t *testing.T) {
	limit := decodeExpansionBudgetBytes >> decodeBudgetUnitShift

	t.Run("map scan skips slice", func(t *testing.T) {
		makeBuffer := func(size int) []byte {
			buf := mapHeader(2)
			buf = append(buf, encodedString("ignored")...)
			buf = append(buf, sliceHeader(size)...)
			for range size {
				buf = append(buf, 0xA0)
			}
			buf = append(buf, encodedString("target")...)
			return append(buf, 0xA1, 7)
		}

		var exact uint16
		decoder := New(makeBuffer(limit - 4))
		require.NoError(t, decoder.DecodePath(0, []any{"target"}, &exact))
		require.Equal(t, uint16(7), exact)

		over := uint16(42)
		decoder = New(makeBuffer(limit - 3))
		requireExpansionLimit(t, decoder.DecodePath(0, []any{"target"}, &over))
		require.Equal(t, uint16(42), over)
	})

	t.Run("slice scan skips map", func(t *testing.T) {
		makeBuffer := func(size int) []byte {
			buf := sliceHeader(2)
			buf = append(buf, mapHeader(size)...)
			for range size {
				buf = append(buf, 0x40, 0xA0)
			}
			return append(buf, 0xA1, 7)
		}

		var exact uint16
		decoder := New(makeBuffer(limit/2 - 1))
		require.NoError(t, decoder.DecodePath(0, []any{1}, &exact))
		require.Equal(t, uint16(7), exact)

		over := uint16(42)
		decoder = New(makeBuffer(limit / 2))
		requireExpansionLimit(t, decoder.DecodePath(0, []any{1}, &over))
		require.Equal(t, uint16(42), over)
	})
}

func TestTypedStructBudgetSkipsUnknownPointerTargets(t *testing.T) {
	type record struct {
		Value any `maxminddb:"value"`
	}

	buf := binaryFanOut()
	root := len(buf)
	buf = append(buf, mapHeader(2)...)
	buf = append(buf, encodedString("unknown")...)
	buf = append(buf, ptr(0)...)
	buf = append(buf, encodedString("value")...)
	buf = append(buf, 0xA0)

	var out record
	require.NoError(t, decodeAt(buf, uint(root), &out))
	require.Equal(t, uint64(0), out.Value)
}

func TestTypedStructBudgetsIgnoredInlineContainers(t *testing.T) {
	type emptyRecord struct{}

	limit := decodeExpansionBudgetBytes >> decodeBudgetUnitShift
	t.Run("slice", func(t *testing.T) {
		buf := mapHeader(1)
		buf = append(buf, encodedString("unknown")...)
		buf = append(buf, sliceHeader(limit)...)

		var out emptyRecord
		requireExpansionLimit(t, decodeAt(buf, 0, &out))
	})

	t.Run("extended map", func(t *testing.T) {
		makeBuffer := func(size int) []byte {
			buf := mapHeader(1)
			buf = append(buf, encodedString("unknown")...)
			buf = append(buf, extendedMapHeader(size)...)
			for range size {
				buf = append(buf, 0x40, 0xA0)
			}
			return buf
		}

		var exact emptyRecord
		require.NoError(t, decodeAt(makeBuffer(limit/2-1), 0, &exact))

		var over emptyRecord
		requireExpansionLimit(t, decodeAt(makeBuffer(limit/2), 0, &over))
	})
}

func TestTypedStructDoesNotChargeIgnoredPayload(t *testing.T) {
	type record struct {
		Value uint16 `maxminddb:"value"`
	}

	buf := mapHeader(2)
	buf = append(buf, encodedString("unknown")...)
	buf = append(buf, bytesLeaf(decodePayloadBudgetBytes+1)...)
	buf = append(buf, encodedString("value")...)
	buf = append(buf, 0xA0)

	var out record
	require.NoError(t, decodeAt(buf, 0, &out))
}

func TestVerifyDataSectionBudgetsEachRecord(t *testing.T) {
	d := New(binaryFanOut())
	err := d.VerifyDataSection(map[uint]bool{0: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum decoded record size")

	const valuesPerRecord = 20_000
	first := uint16Array(valuesPerRecord)
	secondOffset := uint(len(first))
	data := append([]byte(nil), first...)
	data = append(data, uint16Array(valuesPerRecord)...)
	valid := New(data)
	require.NoError(t, valid.VerifyDataSection(map[uint]bool{
		0:            true,
		secondOffset: true,
	}))
}

func TestDynamicBudgetIsConcurrent(t *testing.T) {
	valid := New(uint16Array(1000))
	malicious := New(binaryFanOut())

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			var out any
			assert.NoError(t, valid.Decode(0, &out))
		})
		wg.Go(func() {
			var out any
			assert.Error(t, malicious.Decode(0, &out))
		})
	}
	wg.Wait()
}

type callerBoundedWalker struct{ leaves int }

func (w *callerBoundedWalker) UnmarshalMaxMindDB(d *Decoder) error {
	kind, err := d.PeekKind()
	if err != nil {
		return err
	}
	if kind != KindSlice {
		w.leaves++
		return d.SkipValue()
	}
	it, _, err := d.ReadSlice()
	if err != nil {
		return err
	}
	for elemErr := range it {
		if elemErr != nil {
			return elemErr
		}
		if err := w.UnmarshalMaxMindDB(d); err != nil {
			return err
		}
	}
	return nil
}

func TestCustomDecoderTraversalIsCallerBounded(t *testing.T) {
	const depth = 16
	var out callerBoundedWalker
	require.NoError(t, decodeAt(binaryFanOutDepth(depth), 0, &out))
	require.Equal(t, 1<<depth, out.leaves)
}
