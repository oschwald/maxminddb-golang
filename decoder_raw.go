package maxminddb

import (
	"fmt"
)

// Decoder allows decoding of a single value stored at a specific offset
// in the database.
type Decoder struct {
	d      decoder
	offset uint

	hasNextOffset bool
	nextOffset    uint
}

// Decoder returns a decoder for a single value stored at offset.
func (r *Reader) Decoder(offset uintptr) *Decoder {
	return &Decoder{
		d:      r.decoder,
		offset: uint(offset),
	}
}

func (d *Decoder) reset(offset uint) {
	d.offset = offset
	d.hasNextOffset = false
	d.nextOffset = 0
}

func (d *Decoder) next(numberToSkip uint) error {
	if numberToSkip > 1 || !d.hasNextOffset {
		offset, err := d.d.nextValueOffset(d.offset, numberToSkip)
		if err != nil {
			return err
		}

		d.reset(offset)
		return nil
	}

	d.reset(d.nextOffset)
	return nil
}

func (d *Decoder) setNextOffset(offset uint) {
	if !d.hasNextOffset {
		d.hasNextOffset = true
		d.nextOffset = offset
	}
}

func (d *Decoder) new(offset uint) *Decoder {
	return &Decoder{
		d:      d.d,
		offset: offset,
	}
}

func unexpectedTypeErr(expectedType, actualType dataType) error {
	return fmt.Errorf("unexpected type %d, expected %d", actualType, expectedType)
}

func (d *Decoder) decodeCtrlDataAndFollow(expectedType dataType) (uint, uint, error) {
	dataOffset := d.offset
	for {
		var typeNum dataType
		var size uint
		var err error
		typeNum, size, dataOffset, err = d.d.decodeCtrlData(dataOffset)
		if err != nil {
			return 0, 0, err
		}

		if typeNum == _Pointer {
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

// DecodeBool decodes the value pointed by the decoder as a bool.
//
// Returns an error if the database is malformed or if the pointed value is not a bool.
func (d *Decoder) DecodeBool() (bool, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(_Bool)
	if err != nil {
		return false, err
	}

	if size > 1 {
		return false, newInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (bool size of %v)",
			size,
		)
	}

	var value bool
	value, _ = d.d.decodeBool(size, offset)
	d.setNextOffset(offset)
	return value, nil
}

func (d *Decoder) decodeBytes(typ dataType) ([]byte, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(typ)
	if err != nil {
		return nil, err
	}
	d.setNextOffset(offset + size)
	return d.d.buffer[offset : offset+size], nil
}

// DecodeString decodes the value pointed by the decoder as a string.
//
// Returns an error if the database is malformed or if the pointed value is not a string.
func (d *Decoder) DecodeString() (string, error) {
	val, err := d.decodeBytes(_String)
	if err != nil {
		return "", err
	}
	return string(val), err
}

// DecodeBytes decodes the value pointed by the decoder as bytes.
//
// Returns an error if the database is malformed or if the pointed value is not bytes.
func (d *Decoder) DecodeBytes() ([]byte, error) {
	return d.decodeBytes(_Bytes)
}

// DecodeFloat32 decodes the value pointed by the decoder as a float32.
//
// Returns an error if the database is malformed or if the pointed value is not a float.
func (d *Decoder) DecodeFloat32() (float32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(_Float32)
	if err != nil {
		return 0, err
	}

	if size != 4 {
		return 0, newInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float32 size of %v)",
			size,
		)
	}

	value, nextOffset := d.d.decodeFloat32(size, offset)
	d.setNextOffset(nextOffset)
	return value, nil
}

// DecodeFloat64 decodes the value pointed by the decoder as a float64.
//
// Returns an error if the database is malformed or if the pointed value is not a double.
func (d *Decoder) DecodeFloat64() (float64, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(_Float64)
	if err != nil {
		return 0, err
	}

	if size != 8 {
		return 0, newInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float64 size of %v)",
			size,
		)
	}

	value, nextOffset := d.d.decodeFloat64(size, offset)
	d.setNextOffset(nextOffset)
	return value, nil
}

// DecodeInt32 decodes the value pointed by the decoder as a int32.
//
// Returns an error if the database is malformed or if the pointed value is not an int32.
func (d *Decoder) DecodeInt32() (int32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(_Int32)
	if err != nil {
		return 0, err
	}

	if size > 4 {
		return 0, newInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (int32 size of %v)",
			size,
		)
	}

	var val int32
	for _, b := range d.d.buffer[offset : offset+size] {
		val = (val << 8) | int32(b)
	}
	d.setNextOffset(offset + size)
	return val, nil
}

// DecodeUInt16 decodes the value pointed by the decoder as a uint16.
//
// Returns an error if the database is malformed or if the pointed value is not an uint16.
func (d *Decoder) DecodeUInt16() (uint16, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(_Uint16)
	if err != nil {
		return 0, err
	}

	if size > 2 {
		return 0, newInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint16 size of %v)",
			size,
		)
	}

	var val uint16
	for _, b := range d.d.buffer[offset : offset+size] {
		val = (val << 8) | uint16(b)
	}
	d.setNextOffset(offset + size)
	return val, nil
}

// DecodeUInt32 decodes the value pointed by the decoder as a uint32.
//
// Returns an error if the database is malformed or if the pointed value is not an uint32.
func (d *Decoder) DecodeUInt32() (uint32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(_Uint32)
	if err != nil {
		return 0, err
	}

	if size > 4 {
		return 0, newInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint32 size of %v)",
			size,
		)
	}

	var val uint32
	for _, b := range d.d.buffer[offset : offset+size] {
		val = (val << 8) | uint32(b)
	}
	d.setNextOffset(offset + size)
	return val, nil
}

// DecodeUInt64 decodes the value pointed by the decoder as a uint64.
//
// Returns an error if the database is malformed or if the pointed value is not an uint64.
func (d *Decoder) DecodeUInt64() (uint64, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(_Uint64)
	if err != nil {
		return 0, err
	}

	if size > 8 {
		return 0, newInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint64 size of %v)",
			size,
		)
	}

	var val uint64
	for _, b := range d.d.buffer[offset : offset+size] {
		val, _ = append64(val, b)
	}
	d.setNextOffset(offset + size)
	return val, nil
}

// DecodeUInt128 decodes the value pointed by the decoder as a uint128.
//
// Returns an error if the database is malformed or if the pointed value is not an uint128.
func (d *Decoder) DecodeUInt128() (hi, lo uint64, err error) {
	size, offset, err := d.decodeCtrlDataAndFollow(_Uint128)
	if err != nil {
		return 0, 0, err
	}

	if size > 16 {
		return 0, 0, newInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint128 size of %v)",
			size,
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

// DecodeMap decodes the value pointed by the decoder as a map.
//
// If the callback returns false, the iteration stops immediately, the remaining
// elements are skipped and DecodeMap returns nil. If any other error is returned,
// the iteration stops immediately and DecodeMap returns that error.
//
// Returns an error if the database is malformed or if the pointed value is not a map.
func (d *Decoder) DecodeMap(cb func(key string, value *Decoder) (bool, error)) error {
	size, offset, err := d.decodeCtrlDataAndFollow(_Map)
	if err != nil {
		return err
	}

	dec := d.new(offset)

	for i := uint(0); i < size; i++ {
		var key string
		key, err = dec.DecodeString()
		if err != nil {
			return err
		}

		err = dec.next(1)
		if err != nil {
			return err
		}

		ok, cbErr := cb(key, dec)

		err = dec.next(1)
		if err != nil {
			return err
		}

		if cbErr != nil {
			return cbErr
		}
		if !ok {
			// Skip the unvisited elements:
			return dec.next((size - i - 1) * 2)
		}
	}

	d.setNextOffset(dec.offset)

	return nil
}

// DecodeSlice decodes the value pointed by the decoder as a slice.
//
// If the callback returns false, the iteration stops immediately, the remaining
// elements are skipped and DecodeSlice returns nil. If an error is returned,
// the iteration stops immediately and DecodeSlice returns that error.
//
// Returns an error if the database is malformed or if the pointed value is not a slice.
func (d *Decoder) DecodeSlice(cb func(value *Decoder) (ok bool, err error)) error {
	size, offset, err := d.decodeCtrlDataAndFollow(_Slice)
	if err != nil {
		return err
	}

	dec := d.new(offset)

	for i := uint(0); i < size; i++ {
		ok, cbErr := cb(dec)

		err := dec.next(1)
		if err != nil {
			return err
		}

		if cbErr != nil {
			return cbErr
		}
		if !ok {
			// Skip the unvisited elements:
			return dec.next((size - i - 1))
		}
	}

	d.setNextOffset(dec.offset)

	return nil
}
