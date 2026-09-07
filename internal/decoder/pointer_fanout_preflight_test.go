package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// buildWideStructuralFanOut builds a data section whose outer value is an array
// large enough to trigger the pre-allocation validation walk
// (containerPreflightValueCount). Its first element points into a deep pointer
// fan-out: 40 nested two-element arrays that all resolve back to a single leaf,
// so an unbounded walk costs 2**40 operations. The remaining elements point at
// the leaf to pad the declared size cheaply. The fan-out is therefore reached
// through the preflight path, not the ordinary value path, which is the case a
// value counter placed only on the value path would miss.
func buildWideStructuralFanOut() (buf []byte, rootOffset uint) {
	ptr := func(target int) []byte {
		return []byte{0x20 | byte((target>>8)&0x7), byte(target)}
	}

	const (
		fanOutDepth = 40
		outerSize   = 1024
	)
	buf = make([]byte, 0, 1+fanOutDepth*6+4+outerSize*2)
	buf = append(buf, 0xA0) // leaf: uint16 with value 0, at offset 0
	prev := 0
	for range fanOutDepth {
		off := len(buf)
		buf = append(buf, 0x02, 0x04) // array, size 2
		buf = append(buf, ptr(prev)...)
		buf = append(buf, ptr(prev)...)
		prev = off
	}
	fanOutTop := prev

	// Outer array of 1024 elements. Header: extended type array (0x04), size
	// code 30 with a two-byte length of 1024-285 = 739 (0x02E3).
	rootOffset = uint(len(buf))
	buf = append(buf, 0x1E, 0x04, 0x02, 0xE3)
	buf = append(buf, ptr(fanOutTop)...)
	for range outerSize - 1 {
		buf = append(buf, ptr(0)...)
	}
	return buf, rootOffset
}

func TestPointerFanOutViaStructuralValidationIsRejected(t *testing.T) {
	buf, root := buildWideStructuralFanOut()
	d := New(buf)

	var result []any
	err := d.Decode(root, &result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "maximum decoded record size")
}
