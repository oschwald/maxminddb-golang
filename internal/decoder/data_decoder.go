// Package decoder decodes values in the data section.
package decoder

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

// Kind constants for the different MMDB data kinds.
type Kind int

// MMDB data kind constants.
const (
	// KindExtended indicates an extended kind.
	KindExtended Kind = iota
	// KindPointer is a pointer to another location in the data section.
	KindPointer
	// KindString is a UTF-8 string.
	KindString
	// KindFloat64 is a 64-bit floating point number.
	KindFloat64
	// KindBytes is a byte slice.
	KindBytes
	// KindUint16 is a 16-bit unsigned integer.
	KindUint16
	// KindUint32 is a 32-bit unsigned integer.
	KindUint32
	// KindMap is a map from strings to other data types.
	KindMap
	// KindInt32 is a 32-bit signed integer.
	KindInt32
	// KindUint64 is a 64-bit unsigned integer.
	KindUint64
	// KindUint128 is a 128-bit unsigned integer.
	KindUint128
	// KindSlice is an array of values.
	KindSlice
	// KindContainer is a data cache container.
	KindContainer
	// KindEndMarker marks the end of the data section.
	KindEndMarker
	// KindBool is a boolean value.
	KindBool
	// KindFloat32 is a 32-bit floating point number.
	KindFloat32
)

// String returns a human-readable name for the Kind.
func (k Kind) String() string {
	switch k {
	case KindExtended:
		return "Extended"
	case KindPointer:
		return "Pointer"
	case KindString:
		return "String"
	case KindFloat64:
		return "Float64"
	case KindBytes:
		return "Bytes"
	case KindUint16:
		return "Uint16"
	case KindUint32:
		return "Uint32"
	case KindMap:
		return "Map"
	case KindInt32:
		return "Int32"
	case KindUint64:
		return "Uint64"
	case KindUint128:
		return "Uint128"
	case KindSlice:
		return "Slice"
	case KindContainer:
		return "Container"
	case KindEndMarker:
		return "EndMarker"
	case KindBool:
		return "Bool"
	case KindFloat32:
		return "Float32"
	default:
		return fmt.Sprintf("Unknown(%d)", int(k))
	}
}

// IsContainer returns true if the Kind represents a container type (Map or Slice).
func (k Kind) IsContainer() bool {
	return k == KindMap || k == KindSlice
}

// IsScalar returns true if the Kind represents a scalar value type.
func (k Kind) IsScalar() bool {
	switch k {
	case KindString, KindFloat64, KindBytes, KindUint16, KindUint32,
		KindInt32, KindUint64, KindUint128, KindBool, KindFloat32:
		return true
	default:
		return false
	}
}

// DataDecoder is a decoder for the MMDB data section.
// This is exported so mmdbdata package can use it, but still internal.
type DataDecoder struct {
	stringCache *StringCache
	buffer      []byte
}

const (
	// This is the value used in libmaxminddb.
	maximumDataStructureDepth = 512
)

// NewDataDecoder creates a [DataDecoder].
func NewDataDecoder(buffer []byte) DataDecoder {
	return DataDecoder{
		buffer:      buffer,
		stringCache: NewStringCache(),
	}
}

// Buffer returns the underlying buffer for direct access.
func (d *DataDecoder) Buffer() []byte {
	return d.buffer
}

// DecodeCtrlData decodes the control byte and data info at the given offset.
func (d *DataDecoder) DecodeCtrlData(offset uint) (Kind, uint, uint, error) {
	newOffset := offset + 1
	if offset >= uint(len(d.buffer)) {
		return 0, 0, 0, mmdberrors.NewOffsetError()
	}
	ctrlByte := d.buffer[offset]

	kindNum := Kind(ctrlByte >> 5)
	if kindNum == KindExtended {
		if newOffset >= uint(len(d.buffer)) {
			return 0, 0, 0, mmdberrors.NewOffsetError()
		}
		kindNum = Kind(d.buffer[newOffset] + 7)
		newOffset++
	}

	var size uint
	size, newOffset, err := d.sizeFromCtrlByte(ctrlByte, newOffset, kindNum)
	return kindNum, size, newOffset, err
}

// DecodeBytes decodes a byte slice from the given offset with the given size.
func (d *DataDecoder) DecodeBytes(size, offset uint) ([]byte, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return nil, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bytes := make([]byte, size)
	copy(bytes, d.buffer[offset:newOffset])
	return bytes, newOffset, nil
}

// DecodeFloat64 decodes a 64-bit float from the given offset.
func (d *DataDecoder) DecodeFloat64(size, offset uint) (float64, uint, error) {
	if size != 8 {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float 64 size of %v)",
			size,
		)
	}
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bits := binary.BigEndian.Uint64(d.buffer[offset:newOffset])
	return math.Float64frombits(bits), newOffset, nil
}

// DecodeFloat32 decodes a 32-bit float from the given offset.
func (d *DataDecoder) DecodeFloat32(size, offset uint) (float32, uint, error) {
	if size != 4 {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float32 size of %v)",
			size,
		)
	}
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bits := binary.BigEndian.Uint32(d.buffer[offset:newOffset])
	return math.Float32frombits(bits), newOffset, nil
}

// DecodeInt32 decodes a 32-bit signed integer from the given offset.
func (d *DataDecoder) DecodeInt32(size, offset uint) (int32, uint, error) {
	if size > 4 {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (int32 size of %v)",
			size,
		)
	}
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

// DecodePointer decodes a pointer from the given offset.
func (d *DataDecoder) DecodePointer(
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

// DecodeBool decodes a boolean from the given offset.
func (*DataDecoder) DecodeBool(size, offset uint) (bool, uint, error) {
	if size > 1 {
		return false, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (bool size of %v)",
			size,
		)
	}
	value, newOffset := decodeBool(size, offset)
	return value, newOffset, nil
}

// DecodeString decodes a string from the given offset.
func (d *DataDecoder) DecodeString(size, offset uint) (string, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return "", 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	value := d.stringCache.InternAt(offset, size, d.buffer)
	return value, newOffset, nil
}

// DecodeUint16 decodes a 16-bit unsigned integer from the given offset.
func (d *DataDecoder) DecodeUint16(size, offset uint) (uint16, uint, error) {
	if size > 2 {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint16 size of %v)",
			size,
		)
	}
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

// DecodeUint32 decodes a 32-bit unsigned integer from the given offset.
func (d *DataDecoder) DecodeUint32(size, offset uint) (uint32, uint, error) {
	if size > 4 {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint32 size of %v)",
			size,
		)
	}
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

// DecodeUint64 decodes a 64-bit unsigned integer from the given offset.
func (d *DataDecoder) DecodeUint64(size, offset uint) (uint64, uint, error) {
	if size > 8 {
		return 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint64 size of %v)",
			size,
		)
	}
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

// DecodeUint128 decodes a 128-bit unsigned integer from the given offset.
// Returns the value as high and low 64-bit unsigned integers.
func (d *DataDecoder) DecodeUint128(size, offset uint) (hi, lo uint64, newOffset uint, err error) {
	if size > 16 {
		return 0, 0, 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint128 size of %v)",
			size,
		)
	}
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset = offset + size

	// Process bytes from most significant to least significant
	for _, b := range d.buffer[offset:newOffset] {
		var carry byte
		lo, carry = append64(lo, b)
		hi, _ = append64(hi, carry)
	}

	return hi, lo, newOffset, nil
}

func append64(val uint64, b byte) (uint64, byte) {
	return (val << 8) | uint64(b), byte(val >> 56)
}

// DecodeKey decodes a map key into []byte slice. We use a []byte so that we
// can take advantage of https://github.com/golang/go/issues/3512 to avoid
// copying the bytes when decoding a struct. Previously, we achieved this by
// using unsafe.
func (d *DataDecoder) DecodeKey(offset uint) ([]byte, uint, error) {
	kindNum, size, dataOffset, err := d.DecodeCtrlData(offset)
	if err != nil {
		return nil, 0, err
	}
	if kindNum == KindPointer {
		pointer, ptrOffset, err := d.DecodePointer(size, dataOffset)
		if err != nil {
			return nil, 0, err
		}
		key, _, err := d.DecodeKey(pointer)
		return key, ptrOffset, err
	}
	if kindNum != KindString {
		return nil, 0, mmdberrors.NewInvalidDatabaseError(
			"unexpected type when decoding string: %v",
			kindNum,
		)
	}
	newOffset := dataOffset + size
	if newOffset > uint(len(d.buffer)) {
		return nil, 0, mmdberrors.NewOffsetError()
	}
	return d.buffer[dataOffset:newOffset], newOffset, nil
}

// NextValueOffset skips ahead to the next value without decoding
// the one at the offset passed in. The size bits have different meanings for
// different data types.
func (d *DataDecoder) NextValueOffset(offset, numberToSkip uint) (uint, error) {
	if numberToSkip == 0 {
		return offset, nil
	}
	kindNum, size, offset, err := d.DecodeCtrlData(offset)
	if err != nil {
		return 0, err
	}
	switch kindNum {
	case KindPointer:
		_, offset, err = d.DecodePointer(size, offset)
		if err != nil {
			return 0, err
		}
	case KindMap:
		numberToSkip += 2 * size
	case KindSlice:
		numberToSkip += size
	case KindBool:
	default:
		offset += size
	}
	return d.NextValueOffset(offset, numberToSkip-1)
}

func (d *DataDecoder) sizeFromCtrlByte(
	ctrlByte byte,
	offset uint,
	kindNum Kind,
) (uint, uint, error) {
	size := uint(ctrlByte & 0x1f)
	if kindNum == KindExtended {
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

func decodeBool(size, offset uint) (bool, uint) {
	return size != 0, offset
}

func uintFromBytes(prefix uint, uintBytes []byte) uint {
	val := prefix
	for _, b := range uintBytes {
		val = (val << 8) | uint(b)
	}
	return val
}
