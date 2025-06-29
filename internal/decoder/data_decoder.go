// Package decoder decodes values in the data section.
package decoder

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

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
	buffer []byte
}

const (
	// This is the value used in libmaxminddb.
	maximumDataStructureDepth = 512
)

// NewDataDecoder creates a [DataDecoder].
func NewDataDecoder(buffer []byte) DataDecoder {
	return DataDecoder{buffer: buffer}
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
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bits := binary.BigEndian.Uint64(d.buffer[offset:newOffset])
	return math.Float64frombits(bits), newOffset, nil
}

// DecodeFloat32 decodes a 32-bit float from the given offset.
func (d *DataDecoder) DecodeFloat32(size, offset uint) (float32, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return 0, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	bits := binary.BigEndian.Uint32(d.buffer[offset:newOffset])
	return math.Float32frombits(bits), newOffset, nil
}

// DecodeInt32 decodes a 32-bit signed integer from the given offset.
func (d *DataDecoder) DecodeInt32(size, offset uint) (int32, uint, error) {
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

// DecodeString decodes a string from the given offset.
func (d *DataDecoder) DecodeString(size, offset uint) (string, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return "", 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	return string(d.buffer[offset:newOffset]), newOffset, nil
}

// DecodeUint16 decodes a 16-bit unsigned integer from the given offset.
func (d *DataDecoder) DecodeUint16(size, offset uint) (uint16, uint, error) {
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
func (d *DataDecoder) DecodeUint128(size, offset uint) (*big.Int, uint, error) {
	if offset+size > uint(len(d.buffer)) {
		return nil, 0, mmdberrors.NewOffsetError()
	}

	newOffset := offset + size
	val := new(big.Int)
	val.SetBytes(d.buffer[offset:newOffset])

	return val, newOffset, nil
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
			return d.NextValueOffset(offset, 1)
		}
		return 0, nil
	}

	kindNum, size, newOffset, err := d.DecodeCtrlData(offset)
	if err != nil {
		return 0, err
	}

	return d.decodeFromTypeToDeserializer(kindNum, size, newOffset, dser, depth+1)
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

func (d *DataDecoder) decodeFromTypeToDeserializer(
	dtype Kind,
	size uint,
	offset uint,
	dser deserializer,
	depth int,
) (uint, error) {
	// For these types, size has a special meaning
	switch dtype {
	case KindBool:
		v, offset := decodeBool(size, offset)
		return offset, dser.Bool(v)
	case KindMap:
		return d.decodeMapToDeserializer(size, offset, dser, depth)
	case KindPointer:
		pointer, newOffset, err := d.DecodePointer(size, offset)
		if err != nil {
			return 0, err
		}
		_, err = d.decodeToDeserializer(pointer, dser, depth, false)
		return newOffset, err
	case KindSlice:
		return d.decodeSliceToDeserializer(size, offset, dser, depth)
	case KindBytes:
		v, offset, err := d.DecodeBytes(size, offset)
		if err != nil {
			return 0, err
		}
		return offset, dser.Bytes(v)
	case KindFloat32:
		v, offset, err := d.DecodeFloat32(size, offset)
		if err != nil {
			return 0, err
		}
		return offset, dser.Float32(v)
	case KindFloat64:
		v, offset, err := d.DecodeFloat64(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Float64(v)
	case KindInt32:
		v, offset, err := d.DecodeInt32(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Int32(v)
	case KindString:
		v, offset, err := d.DecodeString(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.String(v)
	case KindUint16:
		v, offset, err := d.DecodeUint16(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Uint16(v)
	case KindUint32:
		v, offset, err := d.DecodeUint32(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Uint32(v)
	case KindUint64:
		v, offset, err := d.DecodeUint64(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Uint64(v)
	case KindUint128:
		v, offset, err := d.DecodeUint128(size, offset)
		if err != nil {
			return 0, err
		}

		return offset, dser.Uint128(v)
	default:
		return 0, mmdberrors.NewInvalidDatabaseError("unknown type: %d", dtype)
	}
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
