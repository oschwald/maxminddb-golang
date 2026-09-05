package mmdbdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecoderZeroValue(t *testing.T) {
	reads := []struct {
		name string
		read func(*Decoder) error
	}{
		{name: "bool", read: func(d *Decoder) error { _, err := d.ReadBool(); return err }},
		{name: "string", read: func(d *Decoder) error { _, err := d.ReadString(); return err }},
		{name: "bytes", read: func(d *Decoder) error { _, err := d.ReadBytes(); return err }},
		{name: "float32", read: func(d *Decoder) error { _, err := d.ReadFloat32(); return err }},
		{name: "float64", read: func(d *Decoder) error { _, err := d.ReadFloat64(); return err }},
		{name: "int32", read: func(d *Decoder) error { _, err := d.ReadInt32(); return err }},
		{name: "uint16", read: func(d *Decoder) error { _, err := d.ReadUint16(); return err }},
		{name: "uint32", read: func(d *Decoder) error { _, err := d.ReadUint32(); return err }},
		{name: "uint64", read: func(d *Decoder) error { _, err := d.ReadUint64(); return err }},
		{
			name: "uint128",
			read: func(d *Decoder) error {
				_, _, err := d.ReadUint128()
				return err
			},
		},
		{
			name: "map",
			read: func(d *Decoder) error {
				_, _, err := d.ReadMap()
				return err
			},
		},
		{
			name: "slice",
			read: func(d *Decoder) error {
				_, _, err := d.ReadSlice()
				return err
			},
		},
		{name: "skip", read: func(d *Decoder) error { return d.SkipValue() }},
		{name: "peek", read: func(d *Decoder) error { _, err := d.PeekKind(); return err }},
	}

	for _, tt := range reads {
		t.Run(tt.name, func(t *testing.T) {
			var decoder Decoder
			require.Error(t, tt.read(&decoder))
		})
	}

	var decoder Decoder
	assert.Zero(t, decoder.Offset())

	var advanceDecoder Decoder
	require.Error(t, advanceDecoder.Advance(Cursor{}))

	var cursorDecoder Decoder
	_, err := cursorDecoder.Cursor().Kind()
	assert.Error(t, err)
}
