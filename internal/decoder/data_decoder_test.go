package decoder

import (
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
