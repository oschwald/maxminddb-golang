package decoder

import (
	"errors"
	"fmt"
	"iter"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

// Decoder allows decoding of a single value stored at a specific offset
// in the database.
type Decoder struct {
	d             DataDecoder
	offset        uint
	nextOffset    uint
	opts          decoderOptions
	hasNextOffset bool
}

type decoderOptions struct {
	// Reserved for future options
}

// DecoderOption configures a Decoder.
//
//nolint:revive // name follows existing library pattern (ReaderOption, NetworksOption)
type DecoderOption func(*decoderOptions)

// NewDecoder creates a new Decoder with the given DataDecoder, offset, and options.
func NewDecoder(d DataDecoder, offset uint, options ...DecoderOption) *Decoder {
	opts := decoderOptions{}
	for _, option := range options {
		option(&opts)
	}
	return &Decoder{d: d, offset: offset, opts: opts}
}

// ReadBool reads the value pointed by the decoder as a bool.
//
// Returns an error if the database is malformed or if the pointed value is not a bool.
func (d *Decoder) ReadBool() (bool, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindBool)
	if err != nil {
		return false, err
	}

	if size > 1 {
		return false, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (bool size of %v)",
			size,
		)
	}

	var value bool
	value, _ = decodeBool(size, offset)
	d.setNextOffset(offset)
	return value, nil
}

// ReadString reads the value pointed by the decoder as a string.
//
// Returns an error if the database is malformed or if the pointed value is not a string.
func (d *Decoder) ReadString() (string, error) {
	val, err := d.readBytes(KindString)
	if err != nil {
		return "", err
	}
	return string(val), err
}

// ReadBytes reads the value pointed by the decoder as bytes.
//
// Returns an error if the database is malformed or if the pointed value is not bytes.
func (d *Decoder) ReadBytes() ([]byte, error) {
	return d.readBytes(KindBytes)
}

// ReadFloat32 reads the value pointed by the decoder as a float32.
//
// Returns an error if the database is malformed or if the pointed value is not a float.
func (d *Decoder) ReadFloat32() (float32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindFloat32)
	if err != nil {
		return 0, err
	}

	if size != 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float32 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.DecodeFloat32(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadFloat64 reads the value pointed by the decoder as a float64.
//
// Returns an error if the database is malformed or if the pointed value is not a double.
func (d *Decoder) ReadFloat64() (float64, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindFloat64)
	if err != nil {
		return 0, err
	}

	if size != 8 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float64 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.DecodeFloat64(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadInt32 reads the value pointed by the decoder as a int32.
//
// Returns an error if the database is malformed or if the pointed value is not an int32.
func (d *Decoder) ReadInt32() (int32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindInt32)
	if err != nil {
		return 0, err
	}

	if size > 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (int32 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.DecodeInt32(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)

	return value, nil
}

// ReadUInt16 reads the value pointed by the decoder as a uint16.
//
// Returns an error if the database is malformed or if the pointed value is not an uint16.
func (d *Decoder) ReadUInt16() (uint16, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindUint16)
	if err != nil {
		return 0, err
	}

	if size > 2 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint16 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.DecodeUint16(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadUInt32 reads the value pointed by the decoder as a uint32.
//
// Returns an error if the database is malformed or if the pointed value is not an uint32.
func (d *Decoder) ReadUInt32() (uint32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindUint32)
	if err != nil {
		return 0, err
	}

	if size > 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint32 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.DecodeUint32(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadUInt64 reads the value pointed by the decoder as a uint64.
//
// Returns an error if the database is malformed or if the pointed value is not an uint64.
func (d *Decoder) ReadUInt64() (uint64, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindUint64)
	if err != nil {
		return 0, err
	}

	if size > 8 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint64 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.DecodeUint64(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadUInt128 reads the value pointed by the decoder as a uint128.
//
// Returns an error if the database is malformed or if the pointed value is not an uint128.
func (d *Decoder) ReadUInt128() (hi, lo uint64, err error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindUint128)
	if err != nil {
		return 0, 0, err
	}

	if size > 16 {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint128 size of %v)",
			size,
		)
	}

	if offset+size > uint(len(d.d.Buffer())) {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (offset+size %d exceeds buffer length %d)",
			offset+size,
			len(d.d.Buffer()),
		)
	}

	for _, b := range d.d.Buffer()[offset : offset+size] {
		var carry byte
		lo, carry = append64(lo, b)
		hi, _ = append64(hi, carry)
	}

	d.setNextOffset(offset + size)

	return hi, lo, nil
}

func append64(val uint64, b byte) (uint64, byte) {
	return (val << 8) | uint64(b), byte(val >> 56)
}

// ReadMap returns an iterator to read the map. The first value from the
// iterator is the key. Please note that this byte slice is only valid during
// the iteration. This is done to avoid an unnecessary allocation. You must
// make a copy of it if you are storing it for later use. The second value is
// an error indicating that the database is malformed or that the pointed
// value is not a map.
func (d *Decoder) ReadMap() iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		size, offset, err := d.decodeCtrlDataAndFollow(KindMap)
		if err != nil {
			yield(nil, err)
			return
		}

		currentOffset := offset

		for range size {
			key, keyEndOffset, err := d.d.DecodeKey(currentOffset)
			if err != nil {
				yield(nil, err)
				return
			}

			// Position decoder to read value after yielding key
			d.reset(keyEndOffset)

			ok := yield(key, nil)
			if !ok {
				return
			}

			// Skip the value to get to next key-value pair
			valueEndOffset, err := d.d.NextValueOffset(keyEndOffset, 1)
			if err != nil {
				yield(nil, err)
				return
			}
			currentOffset = valueEndOffset
		}

		// Set the final offset after map iteration
		d.reset(currentOffset)
	}
}

// ReadSlice returns an iterator over the values of the slice. The iterator
// returns an error if the database is malformed or if the pointed value is
// not a slice.
func (d *Decoder) ReadSlice() iter.Seq[error] {
	return func(yield func(error) bool) {
		size, offset, err := d.decodeCtrlDataAndFollow(KindSlice)
		if err != nil {
			yield(err)
			return
		}

		currentOffset := offset

		for i := range size {
			// Position decoder to read current element
			d.reset(currentOffset)

			ok := yield(nil)
			if !ok {
				// Skip the unvisited elements
				remaining := size - i - 1
				if remaining > 0 {
					endOffset, err := d.d.NextValueOffset(currentOffset, remaining)
					if err == nil {
						d.reset(endOffset)
					}
				}
				return
			}

			// Advance to next element
			nextOffset, err := d.d.NextValueOffset(currentOffset, 1)
			if err != nil {
				yield(err)
				return
			}
			currentOffset = nextOffset
		}

		// Set final offset after slice iteration
		d.reset(currentOffset)
	}
}

// SkipValue skips over the current value without decoding it.
// This is useful in custom decoders when encountering unknown fields.
// The decoder will be positioned after the skipped value.
func (d *Decoder) SkipValue() error {
	// We can reuse the existing nextValueOffset logic by jumping to the next value
	nextOffset, err := d.d.NextValueOffset(d.offset, 1)
	if err != nil {
		return err
	}
	d.reset(nextOffset)
	return nil
}

// PeekKind returns the kind of the current value without consuming it.
// This allows for look-ahead parsing similar to jsontext.Decoder.PeekKind().
func (d *Decoder) PeekKind() (Kind, error) {
	kindNum, _, _, err := d.d.DecodeCtrlData(d.offset)
	if err != nil {
		return 0, err
	}

	// Follow pointers to get the actual kind
	if kindNum == KindPointer {
		// We need to follow the pointer to get the real kind
		dataOffset := d.offset
		for {
			var size uint
			kindNum, size, dataOffset, err = d.d.DecodeCtrlData(dataOffset)
			if err != nil {
				return 0, err
			}
			if kindNum != KindPointer {
				break
			}
			dataOffset, _, err = d.d.DecodePointer(size, dataOffset)
			if err != nil {
				return 0, err
			}
		}
	}

	return kindNum, nil
}

func (d *Decoder) reset(offset uint) {
	d.offset = offset
	d.hasNextOffset = false
	d.nextOffset = 0
}

func (d *Decoder) setNextOffset(offset uint) {
	if !d.hasNextOffset {
		d.hasNextOffset = true
		d.nextOffset = offset
	}
}

func (d *Decoder) getNextOffset() (uint, error) {
	if !d.hasNextOffset {
		return 0, errors.New("no next offset available")
	}
	return d.nextOffset, nil
}

func unexpectedKindErr(expectedKind, actualKind Kind) error {
	return fmt.Errorf("unexpected kind %d, expected %d", actualKind, expectedKind)
}

func (d *Decoder) decodeCtrlDataAndFollow(expectedKind Kind) (uint, uint, error) {
	dataOffset := d.offset
	for {
		var kindNum Kind
		var size uint
		var err error
		kindNum, size, dataOffset, err = d.d.DecodeCtrlData(dataOffset)
		if err != nil {
			return 0, 0, err
		}

		if kindNum == KindPointer {
			var nextOffset uint
			dataOffset, nextOffset, err = d.d.DecodePointer(size, dataOffset)
			if err != nil {
				return 0, 0, err
			}
			d.setNextOffset(nextOffset)
			continue
		}

		if kindNum != expectedKind {
			return 0, 0, unexpectedKindErr(expectedKind, kindNum)
		}

		return size, dataOffset, nil
	}
}

func (d *Decoder) readBytes(kind Kind) ([]byte, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(kind)
	if err != nil {
		return nil, err
	}

	if offset+size > uint(len(d.d.Buffer())) {
		return nil, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (offset+size %d exceeds buffer length %d)",
			offset+size,
			len(d.d.Buffer()),
		)
	}
	d.setNextOffset(offset + size)
	return d.d.Buffer()[offset : offset+size], nil
}
