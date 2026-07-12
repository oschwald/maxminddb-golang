package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyDataSectionRejectsInvalidUTF8(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "string", data: []byte{0x42, 0xff, 0xff}},
		{name: "map key", data: []byte{0xe1, 0x41, 0xff, 0xe0}},
		{name: "array element", data: []byte{0x01, 0x04, 0x42, 0xff, 0xff}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New(tt.data)
			err := d.VerifyDataSection(map[uint]bool{0: true})
			require.ErrorContains(t, err, "invalid UTF-8")
		})
	}
}
