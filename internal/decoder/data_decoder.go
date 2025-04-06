// Package decoder decodes values in the data section.
package decoder

import (
	"encoding/binary"
	"math"
	"math/big"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

// DataDecoder is a decoder for the MMDB data section.
type DataDecoder struct {
	buffer []byte
}

// Type corresponds to the data types defined in the MaxMind DB format
// specification v2.0, specifically in the "Output Data Section".
type Type int

const (
	// TypeExtended is an "extended" type. This means that the type is encoded in the
	// next byte(s). It should not be used directly.
	TypeExtended Type = iota
	// TypePointer represents a pointer to another location in the data section.
	TypePointer
	// TypeString represents a UTF-8 string.
	TypeString
	// TypeFloat64 represents a 64-bit floating point number (double).
	TypeFloat64
	// TypeBytes represents a slice of bytes.
	TypeBytes
	// TypeUint16 represents a 16-bit unsigned integer.
	TypeUint16
	// TypeUint32 represents a 32-bit unsigned integer.
	TypeUint32
	// TypeMap represents a map data type. The keys must be strings.
	// The values may be any data type.
	TypeMap
	// TypeInt32 represents a 32-bit signed integer.
	TypeInt32
	// TypeUint64 represents a 64-bit unsigned integer.
	TypeUint64
	// TypeUint128 represents a 128-bit unsigned integer.
	TypeUint128
	// TypeSlice represents an array data type.
	TypeSlice
	// TypeContainer represents a data cache container. This is used for
	// internal database optimization and is not directly used.
	// It is included here as a placeholder per the specification.
	TypeContainer
	// TypeMarker represents an end marker for the data section. It is included
	// here as a placeholder per the specification. It is not used directly.
	TypeMarker
	// TypeBool represents a boolean type.
	TypeBool
	// TypeFloat32 represents a 32-bit floating point number (float).
	TypeFloat32
)

const (
	// This is the value used in libmaxminddb.
	maximumDataStructureDepth = 512
)

// NewDataDecoder creates a [DataDecoder].
func NewDataDecoder(buffer []byte) DataDecoder {
	return DataDecoder{buffer: buffer}
}

func (d *DataDecoder) decodeToDeserializer(
	offset uint,
	dser deserializer,
	depth int,
	getNext bool,
) (uint, error) {
	if depth > maximumDataStructureDepth {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"exceeded maximum data structure depth; database is likely corrupt",
		)
	}
	skip, err := dser.ShouldSkip(uintptr(offset))
	if err != nil {
		return 0, err
	}
	if skip {
		if getNext {
			return d.nextValueOffset(offset, 1)
		}
		return 0, nil
	}

	typeNum, size, newOffset, err := d.decodeCtrlData(offset)
	if err != nil {
		return 0, err
	}

	return d.decodeFromTypeToDeserializer(typeNum, size, newOffset, dser, depth+1)
}

func (d *DataDecoder) decodeCtrlData(offset uint) (Type, uint, uint, error) {
	newOffset := offset + 1
	if offset >= uint(len(d.buffer)) {
		return 0, 0, 0, mmdberrors.NewOffsetError()
	}
	ctrlByte := d.buffer[offset]

	typeNum := Type(ctrlByte >> 5)
	if typeNum == TypeExtended {
		if newOffset >= uint(len(d.buffer)) {
			return 0, 0, 0, mmdberrors.NewOffsetError()
		}
		typeNum = Type(d.buffer[newOffset] + 7)
		newOffset++
	}

	var size uint
	size, newOffset, err := d.sizeFromCtrlByte(ctrlByte, newOffset, typeNum)
	return typeNum, size, newOffset, err
}

func (d *DataDecoder) sizeFromCtrlByte(
	ctrlByte byte,
	offset uint,
	typeNum Type,
) (uint, uint, error) {
	size := uint(ctrlByte & 0x1f)
	if typeNum == TypeExtended {
		return size, offset, nil
	}

	var bytesToRead uint
	if size < 29 {
		return size, offset, nil
	}

	bytesToRead = size - 28
	newOffset := offset + bytesToRead
	if newOffset > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}
	if size == 29 {
		return 29 + uint(d.buffer[offset]), offset + 1, nil
	}

	sizeBytes := d.buffer[offset:newOffset]

	switch {
	case size == 30:
		size = 285 + uintFromBytes(0, sizeBytes)
	case size > 30:
		size = uintFromBytes(0, sizeBytes) + 65821
	}
	return size, newOffset, nil
}

func (d *DataDecoder) decodeFromTypeToDeserializer(
	dtype Type,
	size uint,
	offset uint,
	dser deserializer,
	depth int,
) (uint, error) {
	// For these types, size has a special meaning
	switch dtype {
	case TypeBool:
		v, offset := decodeBool(size, offset)
		return offset, dser.Bool(v)
	case TypeMap:
		return d.decodeMapToDeserializer(size, offset, dser, depth)
	case TypePointer:
		pointer, newOffset, err := d.decodePointer(size, offset)
		if err != nil {
			return 0, err
		}
		_, err = d.decodeToDeserializer(pointer, dser, depth, false)
		return newOffset, err
	case TypeSlice:
		return d.decodeSliceToDeserializer(size, offset, dser, depth)
	case TypeBytes:
		v, offset, err := d.decodeBytes(size, offset)
		if err != nil {
			return 0, err
		}
		return offset, dser.Bytes(v)
	case TypeFloat32:
		v, offset, err := d.decodeFloat32(size, offset)
		if err != nil {
			return 0, err
		}
		return offset, dser.Float32(v)
	case TypeFloat64:
		v, offset, err := d.decodeFloat64(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Float64(v)
	case TypeInt32:
		v, offset, err := d.decodeInt32(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Int32(v)
	case TypeString:
		v, offset, err := d.decodeString(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.String(v)
	case TypeUint16:
		v, offset, err := d.decodeUint16(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Uint16(v)
	case TypeUint32:
		v, offset, err := d.decodeUint32(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Uint32(v)
	case TypeUint64:
		v, offset, err := d.decodeUint64(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Uint64(v)
	case TypeUint128:
		v, offset, err := d.decodeUint128(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Uint128(v)
	default:
		return 0, mmdberrors.NewInvalidDatabaseError("unknown type: %d", dtype)
	}
}

func decodeBool(size, offset uint) (bool, uint) {
	return size != 0, offset
}

func (d *DataDecoder) decodeBytes(size, offset uint) ([]byte, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return nil, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bytes := make([]byte, size)
	copy(bytes, d.buffer[offset:newOffset])
	return bytes, newOffset, nil
}

func (d *DataDecoder) decodeFloat64(size, offset uint) (float64, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bits := binary.BigEndian.Uint64(d.buffer[offset:newOffset])
	return math.Float64frombits(bits), newOffset, nil
}

func (d *DataDecoder) decodeFloat32(size, offset uint) (float32, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bits := binary.BigEndian.Uint32(d.buffer[offset:newOffset])
	return math.Float32frombits(bits), newOffset, nil
}

func (d *DataDecoder) decodeInt32(size, offset uint) (int32, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	var val int32
	for _, b := range d.buffer[offset:newOffset] {
		val = (val << 8) | int32(b)
	}
	return val, newOffset, nil
}

func (d *DataDecoder) decodeMapToDeserializer(
	size uint,
	offset uint,
	dser deserializer,
	depth int,
) (uint, error) {
	err := dser.StartMap(size)
	if err != nil {
		return 0, err
	}
	for range size {
		// TODO - implement key/value skipping?
		offset, err = d.decodeToDeserializer(offset, dser, depth, true)
		if err != nil {
			return 0, err
		}

		offset, err = d.decodeToDeserializer(offset, dser, depth, true)
		if err != nil {
			return 0, err
		}
	}
	err = dser.End()
	if err != nil {
		return 0, err
	}
	return offset, nil
}

func (d *DataDecoder) decodePointer(
	size uint,
	offset uint,
) (uint, uint, error) {
	pointerSize := ((size >> 3) & 0x3) + 1
	newOffset := offset + pointerSize
	if newOffset > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}
	pointerBytes := d.buffer[offset:newOffset]
	var prefix uint
	if pointerSize == 4 {
		prefix = 0
	} else {
		prefix = size & 0x7
	}
	unpacked := uintFromBytes(prefix, pointerBytes)

	var pointerValueOffset uint
	switch pointerSize {
	case 1:
		pointerValueOffset = 0
	case 2:
		pointerValueOffset = 2048
	case 3:
		pointerValueOffset = 526336
	case 4:
		pointerValueOffset = 0
	}

	pointer := unpacked + pointerValueOffset

	return pointer, newOffset, nil
}

func (d *DataDecoder) decodeSliceToDeserializer(
	size uint,
	offset uint,
	dser deserializer,
	depth int,
) (uint, error) {
	err := dser.StartSlice(size)
	if err != nil {
		return 0, err
	}
	for range size {
		offset, err = d.decodeToDeserializer(offset, dser, depth, true)
		if err != nil {
			return 0, err
		}
	}
	err = dser.End()
	if err != nil {
		return 0, err
	}
	return offset, nil
}

func (d *DataDecoder) decodeString(size, offset uint) (string, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return "", 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	return string(d.buffer[offset:newOffset]), newOffset, nil
}

func (d *DataDecoder) decodeUint16(size, offset uint) (uint16, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bytes := d.buffer[offset:newOffset]

	var val uint16
	for _, b := range bytes {
		val = (val << 8) | uint16(b)
	}
	return val, newOffset, nil
}

func (d *DataDecoder) decodeUint32(size, offset uint) (uint32, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bytes := d.buffer[offset:newOffset]

	var val uint32
	for _, b := range bytes {
		val = (val << 8) | uint32(b)
	}
	return val, newOffset, nil
}

func (d *DataDecoder) decodeUint64(size, offset uint) (uint64, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bytes := d.buffer[offset:newOffset]

	var val uint64
	for _, b := range bytes {
		val = (val << 8) | uint64(b)
	}
	return val, newOffset, nil
}

func (d *DataDecoder) decodeUint128(size, offset uint) (*big.Int, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return nil, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	val := new(big.Int)
	val.SetBytes(d.buffer[offset:newOffset])

	return val, newOffset, nil
}

func uintFromBytes(prefix uint, uintBytes []byte) uint {
	val := prefix
	for _, b := range uintBytes {
		val = (val << 8) | uint(b)
	}
	return val
}

// decodeKey decodes a map key into []byte slice. We use a []byte so that we
// can take advantage of https://github.com/golang/go/issues/3512 to avoid
// copying the bytes when decoding a struct. Previously, we achieved this by
// using unsafe.
func (d *DataDecoder) decodeKey(offset uint) ([]byte, uint, error) {
	typeNum, size, dataOffset, err := d.decodeCtrlData(offset)
	if err != nil {
		return nil, 0, err
	}
	if typeNum == TypePointer {
		pointer, ptrOffset, err := d.decodePointer(size, dataOffset)
		if err != nil {
			return nil, 0, err
		}
		key, _, err := d.decodeKey(pointer)
		return key, ptrOffset, err
	}
	if typeNum != TypeString {
		return nil, 0, mmdberrors.NewInvalidDatabaseError(
			"unexpected type when decoding string: %v",
			typeNum,
		)
	}
	newOffset := dataOffset + size
	if newOffset > uint(len(d.buffer)) {
		return nil, 0, mmdberrors.NewOffsetError()
	}
	return d.buffer[dataOffset:newOffset], newOffset, nil
}

// This function is used to skip ahead to the next value without decoding
// the one at the offset passed in. The size bits have different meanings for
// different data types.
func (d *DataDecoder) nextValueOffset(offset, numberToSkip uint) (uint, error) {
	if numberToSkip == 0 {
		return offset, nil
	}
	typeNum, size, offset, err := d.decodeCtrlData(offset)
	if err != nil {
		return 0, err
	}
	switch typeNum {
	case TypePointer:
		_, offset, err = d.decodePointer(size, offset)
		if err != nil {
			return 0, err
		}
	case TypeMap:
		numberToSkip += 2 * size
	case TypeSlice:
		numberToSkip += size
	case TypeBool:
	default:
		offset += size
	}
	return d.nextValueOffset(offset, numberToSkip-1)
}
