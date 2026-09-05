package maxminddb

import "testing"

func TestReadNode28(t *testing.T) {
	// Exercise every shared nibble pair, including records ending exactly at
	// the end of the buffer and records at unaligned offsets.
	for prefix := range 4 {
		for shared := range uint(256) {
			for _, payload := range []uint{0, 1, 0x123456, 0xabcdef, 0xffffff} {
				left := (shared>>4)<<24 | payload
				right := (shared&15)<<24 | (payload ^ 0xffffff)
				buffer := make([]byte, prefix+7)
				copy(buffer[prefix:], []byte{
					byte(left >> 16), byte(left >> 8), byte(left), byte(shared),
					byte(right >> 16), byte(right >> 8), byte(right),
				})
				for bit, want := range []uint{left, right} {
					if got := readNode28(buffer, uint(prefix), uint(bit)); got != want {
						t.Fatalf(
							"prefix=%d shared=%02x bit=%d: got %07x, want %07x",
							prefix,
							shared,
							bit,
							got,
							want,
						)
					}
				}
			}
		}
	}
}
