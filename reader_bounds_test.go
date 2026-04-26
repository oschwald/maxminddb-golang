package maxminddb

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadNodeBySizeRejectsShortBuffers(t *testing.T) {
	tests := []struct {
		name       string
		buffer     []byte
		recordSize uint
		bit        uint
	}{
		{name: "24-bit", buffer: []byte{0x01, 0x02}, recordSize: 24},
		{name: "28-bit left", buffer: []byte{0x01, 0x02, 0x03}, recordSize: 28},
		{
			name:       "28-bit right",
			buffer:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
			recordSize: 28,
			bit:        1,
		},
		{name: "32-bit", buffer: []byte{0x01, 0x02, 0x03}, recordSize: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readNodeBySize(tt.buffer, 0, tt.bit, tt.recordSize)
			require.ErrorContains(t, err, "bounds check failed")
		})
	}
}

func TestReadNodePairBySizeRejectsShortBuffers(t *testing.T) {
	tests := []struct {
		name       string
		buffer     []byte
		recordSize uint
	}{
		{name: "24-bit", buffer: []byte{0x01, 0x02, 0x03, 0x04, 0x05}, recordSize: 24},
		{name: "28-bit", buffer: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}, recordSize: 28},
		{name: "32-bit", buffer: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, recordSize: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := readNodePairBySize(tt.buffer, 0, tt.recordSize)
			require.ErrorContains(t, err, "bounds check failed")
		})
	}
}

func TestTraverseTreeRejectsShortBuffers(t *testing.T) {
	tests := []struct {
		name       string
		buffer     []byte
		recordSize uint
	}{
		{name: "24-bit", buffer: []byte{0x01, 0x02}, recordSize: 24},
		{name: "28-bit", buffer: []byte{0x01, 0x02, 0x03}, recordSize: 28},
		{name: "32-bit", buffer: []byte{0x01, 0x02, 0x03}, recordSize: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Reader{
				buffer:   tt.buffer,
				Metadata: Metadata{NodeCount: 1, RecordSize: tt.recordSize},
			}
			ip := netip.MustParseAddr("1.1.1.1")

			var err error
			switch tt.recordSize {
			case 24:
				_, _, err = reader.traverseTree24(ip, 0, 1)
			case 28:
				_, _, err = reader.traverseTree28(ip, 0, 1)
			case 32:
				_, _, err = reader.traverseTree32(ip, 0, 1)
			default:
				t.Fatalf("unexpected record size %d", tt.recordSize)
			}
			require.ErrorContains(t, err, "bounds check failed")
		})
	}
}
