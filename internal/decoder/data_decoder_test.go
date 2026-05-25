package decoder

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDecodeCtrlData verifies all four size-encoding paths
// (inline, +29, +285, +65821) plus the extended-kind path. Each path
// also has a truncated-buffer case to exercise the bounds check that
// guards reads of the extra size bytes.
func TestDecodeCtrlData(t *testing.T) {
	tests := []struct {
		name       string
		buffer     []byte
		wantKind   Kind
		wantSize   uint
		wantOffset uint
	}{
		{
			name:       "inline size (string size=5)",
			buffer:     []byte{0x45, 'h', 'e', 'l', 'l', 'o'},
			wantKind:   KindString,
			wantSize:   5,
			wantOffset: 1,
		},
		{
			name:       "size 29 marker reads 1 byte (size=29+0=29)",
			buffer:     append([]byte{0x5D, 0x00}, make([]byte, 29)...),
			wantKind:   KindString,
			wantSize:   29,
			wantOffset: 2,
		},
		{
			name:       "size 29 marker reads 1 byte (size=29+255=284)",
			buffer:     append([]byte{0x5D, 0xFF}, make([]byte, 284)...),
			wantKind:   KindString,
			wantSize:   284,
			wantOffset: 2,
		},
		{
			name:       "size 30 marker reads 2 bytes (size=285+258=543)",
			buffer:     append([]byte{0x5E, 0x01, 0x02}, make([]byte, 543)...),
			wantKind:   KindString,
			wantSize:   543,
			wantOffset: 3,
		},
		{
			name:       "size 31 marker reads 3 bytes (size=65821+66051=131872)",
			buffer:     append([]byte{0x5F, 0x01, 0x02, 0x03}, make([]byte, 131872)...),
			wantKind:   KindString,
			wantSize:   131872,
			wantOffset: 4,
		},
		{
			name:       "extended kind (0x04 -> KindSlice, size=5 inline)",
			buffer:     []byte{0x05, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantKind:   KindSlice,
			wantSize:   5,
			wantOffset: 2,
		},
		{
			name:       "extended kind with size 29 marker (KindSlice, size=29+0=29)",
			buffer:     append([]byte{0x1D, 0x04, 0x00}, make([]byte, 29)...),
			wantKind:   KindSlice,
			wantSize:   29,
			wantOffset: 3,
		},
		{
			name:       "extended kind with size 30 marker (KindSlice, size=285+256=541)",
			buffer:     append([]byte{0x1E, 0x04, 0x01, 0x00}, make([]byte, 541)...),
			wantKind:   KindSlice,
			wantSize:   541,
			wantOffset: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDataDecoder(tt.buffer)
			gotKind, gotSize, gotOffset, err := d.decodeCtrlData(0)
			require.NoError(t, err)
			require.Equal(t, tt.wantKind, gotKind, "kind")
			require.Equal(t, tt.wantSize, gotSize, "size")
			require.Equal(t, tt.wantOffset, gotOffset, "newOffset")
		})
	}
}

// TestDecodeCtrlDataTruncated exercises the bounds checks: each case
// has just enough bytes to start decoding but not enough to read the
// trailing size or extended-kind byte(s).
func TestDecodeCtrlDataTruncated(t *testing.T) {
	tests := []struct {
		name   string
		buffer []byte
	}{
		{name: "empty buffer", buffer: []byte{}},
		{name: "size 29 missing extra byte", buffer: []byte{0x5D}},
		{name: "size 30 with only 1 of 2 extra bytes", buffer: []byte{0x5E, 0x00}},
		{name: "size 31 with only 2 of 3 extra bytes", buffer: []byte{0x5F, 0x00, 0x00}},
		{name: "extended kind missing kind byte", buffer: []byte{0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDataDecoder(tt.buffer)
			_, _, _, err := d.decodeCtrlData(0)
			require.Error(t, err)
		})
	}
}

// TestDecodeKeyFastPathBoundary covers the 28/29-character boundary
// between decodeKey's inline fast path (size < 29) and its slow path
// (size >= 29). An off-by-one in the fast-path size check would either
// pull a stale byte from the buffer (slow path size confused with fast
// path data) or read past the end on a truncated input.
func TestDecodeKeyFastPathBoundary(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{name: "size 0 (fast-path min)", size: 0},
		{name: "size 28 (fast-path max)", size: 28},
		{name: "size 29 (slow-path min)", size: 29},
		{name: "size 30 (slow-path)", size: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := strings.Repeat("a", tt.size)
			var buf []byte
			switch {
			case tt.size < 29:
				buf = append(buf, 0x40|byte(tt.size))
			case tt.size == 29:
				buf = append(buf, 0x5D, 0x00) // 29 + 0
			default:
				buf = append(buf, 0x5D, byte(tt.size-29))
			}
			buf = append(buf, key...)

			d := NewDataDecoder(buf)
			got, newOffset, err := d.decodeKey(0)
			require.NoError(t, err)
			require.Equal(t, key, string(got))
			require.Equal(t, uint(len(buf)), newOffset)
		})
	}

	t.Run("size 28 truncated buffer fails", func(t *testing.T) {
		// Fast path: ctrl says size=28, but buffer holds only the ctrl byte.
		d := NewDataDecoder([]byte{0x5C})
		_, _, err := d.decodeKey(0)
		require.Error(t, err)
	})
}

// TestDecodeKeyFollowsPointer verifies that a pointer control byte at the
// key position bypasses the fast path and is followed to the target string.
// A regression that took the fast path on a non-KindString ctrl byte would
// interpret the pointer payload byte as string data.
func TestDecodeKeyFollowsPointer(t *testing.T) {
	// Pointer (ctrl 0x20: high 3 bits = KindPointer, low 5 = pointerSize-1=0
	// and prefix=0) plus payload 0x05 points to offset 5. The pointed-to
	// data is a 3-byte string "key" (ctrl 0x43).
	buf := []byte{
		0x20, 0x05,
		0x00, 0x00, 0x00,
		0x43, 'k', 'e', 'y',
	}
	d := NewDataDecoder(buf)
	got, nextOffset, err := d.decodeKey(0)
	require.NoError(t, err)
	require.Equal(t, "key", string(got))
	require.Equal(t, uint(2), nextOffset,
		"nextOffset should point past the pointer bytes, not past the pointed-to string")
}

// TestDecodePointerKeyFastPointerSizes exercises decodePointerKeyFast for
// pointer sizes 2 and 4. Size 1 is covered by TestDecodeKeyFollowsPointer;
// size 3 requires a >526KB buffer (pointerBase3 offset) and is exercised
// indirectly by the existing real-database lookup tests. A copy-paste error
// in any pointer-size branch — wrong base added, wrong shift, or wrong
// payload-byte count consumed — would change decoded keys or break
// nextOffset.
func TestDecodePointerKeyFastPointerSizes(t *testing.T) {
	t.Run("pointer size 2", func(t *testing.T) {
		// ctrl 0x28: KindPointer, pointerSize-1=1 (so size=2), prefix=0.
		// payload [0x00, 0x05] -> raw=5, plus pointerBase2 (2048) -> 2053.
		// Buffer is 2053 + 4 bytes, with the target "key" placed at 2053.
		buf := make([]byte, 2057)
		buf[0] = 0x28
		buf[1] = 0x00
		buf[2] = 0x05
		buf[2053] = 0x43
		copy(buf[2054:], "key")

		d := NewDataDecoder(buf)
		got, nextOffset, err := d.decodeKey(0)
		require.NoError(t, err)
		require.Equal(t, "key", string(got))
		require.Equal(t, uint(3), nextOffset,
			"nextOffset must point past the 1-byte ctrl + 2-byte payload")
	})

	t.Run("pointer size 4", func(t *testing.T) {
		// ctrl 0x38: KindPointer, pointerSize-1=3 (so size=4), prefix unused.
		// payload [0x00, 0x00, 0x00, 0x09] -> raw=9, no base added -> 9.
		buf := []byte{
			0x38, 0x00, 0x00, 0x00, 0x09,
			0x00, 0x00, 0x00, 0x00,
			0x43, 'k', 'e', 'y',
		}
		d := NewDataDecoder(buf)
		got, nextOffset, err := d.decodeKey(0)
		require.NoError(t, err)
		require.Equal(t, "key", string(got))
		require.Equal(t, uint(5), nextOffset,
			"nextOffset must point past the 1-byte ctrl + 4-byte payload")
	})
}

// TestDecodePointerKeyFastNonStringTarget verifies that a pointer to a
// non-KindString target bails out of the fast path. A regression that
// accepted any pointer-target kind would return bogus key bytes (e.g.
// pointing at a map or uint ctrl byte's payload), silently corrupting
// struct decoding. The slow path also rejects this, but with a specific
// "unexpected type when decoding string" error.
func TestDecodePointerKeyFastNonStringTarget(t *testing.T) {
	// Pointer at offset 0 -> offset 5. At offset 5 we put ctrl 0xC0
	// (KindUint16, size 0). The fast path must not accept this.
	buf := []byte{
		0x20, 0x05,
		0x00, 0x00, 0x00,
		0xC0,
	}
	d := NewDataDecoder(buf)
	_, _, err := d.decodeKey(0)
	require.Error(t, err, "non-string pointer target must be rejected")
	require.Contains(t, err.Error(), "unexpected type when decoding string")
}

// TestDecodePointerKeyFastExtendedSize verifies that a pointer targeting a
// string whose size requires the extended encoding (size byte >= 29) is
// handled correctly via the slow-path fall-through. A regression that
// accepted the raw `pointedSize` byte as the string length would either
// truncate the key (reading only 29 bytes instead of the full string) or
// read past the buffer.
func TestDecodePointerKeyFastExtendedSize(t *testing.T) {
	// 30-byte key triggers the extended size encoding: ctrl 0x5D, extra
	// byte (size - 29) = 0x01, then 30 payload bytes.
	key := strings.Repeat("k", 30)
	// Layout: [pointer ctrl][pointer payload][padding][target ctrl][extra
	// size byte][key bytes...]. Pointer payload 0x05 -> target at offset 5.
	buf := make([]byte, 0, 7+len(key))
	buf = append(buf, 0x20, 0x05, 0x00, 0x00, 0x00, 0x5D, 0x01)
	buf = append(buf, key...)

	d := NewDataDecoder(buf)
	got, nextOffset, err := d.decodeKey(0)
	require.NoError(t, err)
	require.Equal(t, key, string(got))
	require.Equal(t, uint(2), nextOffset,
		"nextOffset must point past the pointer bytes regardless of fast/slow path")
}
