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
	d      DataDecoder
	offset uint

	hasNextOffset bool
	nextOffset    uint
}

// DecodeBool decodes the value pointed by the decoder as a bool.
//
// Returns an error if the database is malformed or if the pointed value is not a bool.
func (d *Decoder) DecodeBool() (bool, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(TypeBool)
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

// DecodeString decodes the value pointed by the decoder as a string.
//
// Returns an error if the database is malformed or if the pointed value is not a string.
func (d *Decoder) DecodeString() (string, error) {
	val, err := d.decodeBytes(TypeString)
	if err != nil {
		return "", err
	}
	return string(val), err
}

// DecodeBytes decodes the value pointed by the decoder as bytes.
//
// Returns an error if the database is malformed or if the pointed value is not bytes.
func (d *Decoder) DecodeBytes() ([]byte, error) {
	return d.decodeBytes(TypeBytes)
}

// DecodeFloat32 decodes the value pointed by the decoder as a float32.
//
// Returns an error if the database is malformed or if the pointed value is not a float.
func (d *Decoder) DecodeFloat32() (float32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(TypeFloat32)
	if err != nil {
		return 0, err
	}

	if size != 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float32 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.decodeFloat32(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// DecodeFloat64 decodes the value pointed by the decoder as a float64.
//
// Returns an error if the database is malformed or if the pointed value is not a double.
func (d *Decoder) DecodeFloat64() (float64, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(TypeFloat64)
	if err != nil {
		return 0, err
	}

	if size != 8 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float64 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.decodeFloat64(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// DecodeInt32 decodes the value pointed by the decoder as a int32.
//
// Returns an error if the database is malformed or if the pointed value is not an int32.
func (d *Decoder) DecodeInt32() (int32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(TypeInt32)
	if err != nil {
		return 0, err
	}

	if size > 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (int32 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.decodeInt32(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)

	return value, nil
}

// DecodeUInt16 decodes the value pointed by the decoder as a uint16.
//
// Returns an error if the database is malformed or if the pointed value is not an uint16.
func (d *Decoder) DecodeUInt16() (uint16, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(TypeUint16)
	if err != nil {
		return 0, err
	}

	if size > 2 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint16 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.decodeUint16(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// DecodeUInt32 decodes the value pointed by the decoder as a uint32.
//
// Returns an error if the database is malformed or if the pointed value is not an uint32.
func (d *Decoder) DecodeUInt32() (uint32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(TypeUint32)
	if err != nil {
		return 0, err
	}

	if size > 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint32 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.decodeUint32(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// DecodeUInt64 decodes the value pointed by the decoder as a uint64.
//
// Returns an error if the database is malformed or if the pointed value is not an uint64.
func (d *Decoder) DecodeUInt64() (uint64, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(TypeUint64)
	if err != nil {
		return 0, err
	}

	if size > 8 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint64 size of %v)",
			size,
		)
	}

	value, nextOffset, err := d.d.decodeUint64(size, offset)
	if err != nil {
		return 0, err
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// DecodeUInt128 decodes the value pointed by the decoder as a uint128.
//
// Returns an error if the database is malformed or if the pointed value is not an uint128.
func (d *Decoder) DecodeUInt128() (hi, lo uint64, err error) {
	size, offset, err := d.decodeCtrlDataAndFollow(TypeUint128)
	if err != nil {
		return 0, 0, err
	}

	if size > 16 {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint128 size of %v)",
			size,
		)
	}

	if offset+size > uint(len(d.d.buffer)) {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (offset+size %d exceeds buffer length %d)",
			offset+size,
			len(d.d.buffer),
		)
	}

	for _, b := range d.d.buffer[offset : offset+size] {
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

// DecodeMap returns an iterator to decode the map. The first value from the
// iterator is the key. Please note that this byte slice is only valid during
// the iteration. This is done to avoid an unnecessary allocation. You must
// make a copy of it if you are storing it for later use. The second value is
// an error indicating that the database is malformed or that the pointed
// value is not a map.
func (d *Decoder) DecodeMap() iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		size, offset, err := d.decodeCtrlDataAndFollow(TypeMap)
		if err != nil {
			yield(nil, err)
			return
		}

		currentOffset := offset

		for range size {
			key, keyEndOffset, err := d.d.decodeKey(currentOffset)
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
			valueEndOffset, err := d.d.nextValueOffset(keyEndOffset, 1)
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

// DecodeSlice returns an iterator over the values of the slice. The iterator
// returns an error if the database is malformed or if the pointed value is
// not a slice.
func (d *Decoder) DecodeSlice() iter.Seq[error] {
	return func(yield func(error) bool) {
		size, offset, err := d.decodeCtrlDataAndFollow(TypeSlice)
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
					endOffset, err := d.d.nextValueOffset(currentOffset, remaining)
					if err == nil {
						d.reset(endOffset)
					}
				}
				return
			}

			// Advance to next element
			nextOffset, err := d.d.nextValueOffset(currentOffset, 1)
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
	nextOffset, err := d.d.nextValueOffset(d.offset, 1)
	if err != nil {
		return err
	}
	d.reset(nextOffset)
	return nil
}

// PeekType returns the type of the current value without consuming it.
// This allows for look-ahead parsing similar to jsontext.Decoder.PeekKind().
func (d *Decoder) PeekType() (Type, error) {
	typeNum, _, _, err := d.d.decodeCtrlData(d.offset)
	if err != nil {
		return 0, err
	}

	// Follow pointers to get the actual type
	if typeNum == TypePointer {
		// We need to follow the pointer to get the real type
		dataOffset := d.offset
		for {
			var size uint
			typeNum, size, dataOffset, err = d.d.decodeCtrlData(dataOffset)
			if err != nil {
				return 0, err
			}
			if typeNum != TypePointer {
				break
			}
			dataOffset, _, err = d.d.decodePointer(size, dataOffset)
			if err != nil {
				return 0, err
			}
		}
	}

	return typeNum, nil
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

func unexpectedTypeErr(expectedType, actualType Type) error {
	return fmt.Errorf("unexpected type %d, expected %d", actualType, expectedType)
}

func (d *Decoder) decodeCtrlDataAndFollow(expectedType Type) (uint, uint, error) {
	dataOffset := d.offset
	for {
		var typeNum Type
		var size uint
		var err error
		typeNum, size, dataOffset, err = d.d.decodeCtrlData(dataOffset)
		if err != nil {
			return 0, 0, err
		}

		if typeNum == TypePointer {
			var nextOffset uint
			dataOffset, nextOffset, err = d.d.decodePointer(size, dataOffset)
			if err != nil {
				return 0, 0, err
			}
			d.setNextOffset(nextOffset)
			continue
		}

		if typeNum != expectedType {
			return 0, 0, unexpectedTypeErr(expectedType, typeNum)
		}

		return size, dataOffset, nil
	}
}

func (d *Decoder) decodeBytes(typ Type) ([]byte, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(typ)
	if err != nil {
		return nil, err
	}
	if offset+size > uint(len(d.d.buffer)) {
		return nil, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (offset+size %d exceeds buffer length %d)",
			offset+size,
			len(d.d.buffer),
		)
	}
	d.setNextOffset(offset + size)
	return d.d.buffer[offset : offset+size], nil
}
