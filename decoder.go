package maxminddb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
)

type decoder struct {
	buffer      []byte
	pointerBase uint
}

func (d *decoder) decodeArray(size uint, offset uint) ([]interface{}, uint) {
	array := make([]interface{}, size)
	for i := range array {
		var value interface{}
		value, offset = d.decode(offset)
		array[i] = value
	}
	return array, offset
}

func (d *decoder) decodeBool(size uint, offset uint) (bool, uint) {
	return size != 0, offset
}

func (d *decoder) decodeBytes(size uint, offset uint) ([]byte, uint) {
	newOffset := offset + size
	return d.buffer[offset:newOffset], newOffset
}

func (d *decoder) decodeFloat64(size uint, offset uint) (float64, uint) {
	newOffset := offset + size
	var dbl float64
	binary.Read(bytes.NewBuffer(d.buffer[offset:newOffset]), binary.BigEndian, &dbl)
	return dbl, newOffset
}

func (d *decoder) decodeFloat32(size uint, offset uint) (float32, uint) {
	newOffset := offset + size
	var flt float32
	binary.Read(bytes.NewBuffer(d.buffer[offset:newOffset]), binary.BigEndian, &flt)
	return flt, newOffset
}

func (d *decoder) decodeInt(size uint, offset uint) (int, uint) {
	newOffset := offset + size
	intBytes := d.buffer[offset:newOffset]
	if size != 4 {
		pad := make([]byte, 4-size)
		intBytes = append(pad, intBytes...)
	}

	var val int32
	binary.Read(bytes.NewBuffer(intBytes), binary.BigEndian, &val)

	return int(val), newOffset
}

func (d *decoder) decodeMap(size uint, offset uint) (map[string]interface{}, uint) {
	container := make(map[string]interface{})
	for i := uint(0); i < size; i++ {
		var key interface{}
		var value interface{}
		key, offset = d.decode(offset)
		value, offset = d.decode(offset)
		container[key.(string)] = value
	}
	return container, offset
}

var pointerValueOffset map[uint]uint = map[uint]uint{
	1: 0,
	2: 2048,
	3: 526336,
	4: 0,
}

func (d *decoder) decodePointer(size uint, offset uint) (interface{}, uint) {
	pointerSize := ((size >> 3) & 0x3) + 1
	newOffset := offset + pointerSize
	pointerBytes := d.buffer[offset:newOffset]
	var packed []byte
	if pointerSize == 4 {
		packed = pointerBytes
	} else {
		packed = append([]byte{byte(size & 0x7)}, pointerBytes...)
	}
	unpacked := uintFromBytes(packed)

	pointer := unpacked + d.pointerBase + pointerValueOffset[pointerSize]
	// if self._pointer_test:
	//     return pointer, new_offset
	value, _ := d.decode(pointer)
	return value, newOffset
}

func (d *decoder) decodeUint(size uint, offset uint) (uint, uint) {
	newOffset := offset + size
	val := uintFromBytes(d.buffer[offset:newOffset])

	return val, newOffset
}

func (d *decoder) decodeUint128(size uint, offset uint) (*big.Int, uint) {
	newOffset := offset + size
	val := new(big.Int)
	val.SetBytes(d.buffer[offset:newOffset])

	return val, newOffset
}

func uintFromBytes(uintBytes []byte) uint {
	var val uint = 0
	for _, b := range uintBytes {
		val = (val << 8) | uint(b)
	}
	return val
}

func (d *decoder) decodeString(size uint, offset uint) (string, uint) {
	new_offset := offset + size
	return string(d.buffer[offset:new_offset]), new_offset
}

func (d *decoder) decode(offset uint) (interface{}, uint) {
	newOffset := offset + 1
	ctrlByte := d.buffer[offset]

	typeNum := dataType(ctrlByte >> 5)
	// Extended type
	if typeNum == 0 {
		typeNum = dataType(d.buffer[newOffset] + 7)
		newOffset++
	}

	var size uint
	size, newOffset = d.sizeFromCtrlByte(ctrlByte, newOffset, typeNum)
	return d.decodeFromType(typeNum, size, newOffset)
}

type dataType int

const (
	EXTENDED dataType = iota
	POINTER
	STRING
	FLOAT64
	BYTES
	UINT16
	UINT32
	MAP
	INT32
	UINT64
	UINT128
	ARRAY
	CONTAINER
	MARKER
	BOOL
	FLOAT32
)

func (d *decoder) decodeFromType(dtype dataType, size uint, offset uint) (interface{}, uint) {
	var value interface{}
	switch dtype {
	case POINTER:
		value, offset = d.decodePointer(size, offset)
	case BOOL:
		value, offset = d.decodeBool(size, offset)
	case INT32:
		value, offset = d.decodeInt(size, offset)
	case UINT16, UINT32, UINT64:
		value, offset = d.decodeUint(size, offset)
	case UINT128:
		value, offset = d.decodeUint128(size, offset)
	case FLOAT32:
		value, offset = d.decodeFloat32(size, offset)
	case FLOAT64:
		value, offset = d.decodeFloat64(size, offset)
	case STRING:
		value, offset = d.decodeString(size, offset)
	case BYTES:
		value, offset = d.decodeBytes(size, offset)
	case ARRAY:
		value, offset = d.decodeArray(size, offset)
	case MAP:
		value, offset = d.decodeMap(size, offset)
	default:
		panic(fmt.Sprintf("Unknown type: %d", dtype))
	}
	return value, offset
}

func (d *decoder) sizeFromCtrlByte(ctrlByte byte, offset uint, typeNum dataType) (uint, uint) {
	size := uint(ctrlByte & 0x1f)
	if typeNum == 1 {
		return size, offset
	}

	var bytesToRead uint = 0
	if size > 28 {
		bytesToRead = size - 28
	}

	newOffset := offset + bytesToRead
	sizeBytes := d.buffer[offset:newOffset]

	switch {
	case size == 29:
		size = 29 + uint(sizeBytes[0])
	case size == 30:
		size = 285 + uintFromBytes(sizeBytes)
	case size > 30:
		size = uintFromBytes(sizeBytes) + 65821
	}
	return size, newOffset
}
