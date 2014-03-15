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

func (d *decoder) decodeArray(size uint, offset uint) ([]interface{}, uint, error) {
	array := make([]interface{}, size)
	for i := range array {
		var value interface{}
		var err error
		value, offset, err = d.decode(offset)
		if err != nil {
			return nil, 0, err
		}
		array[i] = value
	}
	return array, offset, nil
}

func (d *decoder) decodeBool(size uint, offset uint) (bool, uint, error) {
	return size != 0, offset, nil
}

func (d *decoder) decodeBytes(size uint, offset uint) ([]byte, uint, error) {
	newOffset := offset + size
	return d.buffer[offset:newOffset], newOffset, nil
}

func (d *decoder) decodeFloat64(size uint, offset uint) (float64, uint, error) {
	newOffset := offset + size
	var dbl float64
	binary.Read(bytes.NewBuffer(d.buffer[offset:newOffset]), binary.BigEndian, &dbl)
	return dbl, newOffset, nil
}

func (d *decoder) decodeFloat32(size uint, offset uint) (float32, uint, error) {
	newOffset := offset + size
	var flt float32
	binary.Read(bytes.NewBuffer(d.buffer[offset:newOffset]), binary.BigEndian, &flt)
	return flt, newOffset, nil
}

func (d *decoder) decodeInt(size uint, offset uint) (int, uint, error) {
	newOffset := offset + size
	intBytes := d.buffer[offset:newOffset]
	if size != 4 {
		pad := make([]byte, 4-size)
		intBytes = append(pad, intBytes...)
	}

	var val int32
	binary.Read(bytes.NewBuffer(intBytes), binary.BigEndian, &val)

	return int(val), newOffset, nil
}

func (d *decoder) decodeMap(size uint, offset uint) (map[string]interface{}, uint, error) {
	container := make(map[string]interface{})
	for i := uint(0); i < size; i++ {
		var key interface{}
		var value interface{}
		var err error
		key, offset, err = d.decode(offset)
		if err != nil {
			return nil, 0, err
		}
		value, offset, err = d.decode(offset)
		if err != nil {
			return nil, 0, err
		}
		container[key.(string)] = value
	}
	return container, offset, nil
}

var pointerValueOffset = map[uint]uint{
	1: 0,
	2: 2048,
	3: 526336,
	4: 0,
}

func (d *decoder) decodePointer(size uint, offset uint) (interface{}, uint, error) {
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
	value, _, err := d.decode(pointer)
	return value, newOffset, err
}

func (d *decoder) decodeUint(size uint, offset uint) (uint, uint, error) {
	newOffset := offset + size
	val := uintFromBytes(d.buffer[offset:newOffset])

	return val, newOffset, nil
}

func (d *decoder) decodeUint128(size uint, offset uint) (*big.Int, uint, error) {
	newOffset := offset + size
	val := new(big.Int)
	val.SetBytes(d.buffer[offset:newOffset])

	return val, newOffset, nil
}

func uintFromBytes(uintBytes []byte) uint {
	var val uint
	for _, b := range uintBytes {
		val = (val << 8) | uint(b)
	}
	return val
}

func (d *decoder) decodeString(size uint, offset uint) (string, uint, error) {
	newOffset := offset + size
	return string(d.buffer[offset:newOffset]), newOffset, nil
}

func (d *decoder) decode(offset uint) (interface{}, uint, error) {
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
	_Extended dataType = iota
	_Pointer
	_String
	_Float64
	_Bytes
	_Uint16
	_Uint32
	_Map
	_Int32
	_Uint64
	_Uint128
	_Array
	_Container
	_Marker
	_Bool
	_Float32
)

func (d *decoder) decodeFromType(dtype dataType, size uint, offset uint) (interface{}, uint, error) {
	switch dtype {
	case _Pointer:
		return d.decodePointer(size, offset)
	case _Bool:
		return d.decodeBool(size, offset)
	case _Int32:
		return d.decodeInt(size, offset)
	case _Uint16, _Uint32, _Uint64:
		return d.decodeUint(size, offset)
	case _Uint128:
		return d.decodeUint128(size, offset)
	case _Float32:
		return d.decodeFloat32(size, offset)
	case _Float64:
		return d.decodeFloat64(size, offset)
	case _String:
		return d.decodeString(size, offset)
	case _Bytes:
		return d.decodeBytes(size, offset)
	case _Array:
		return d.decodeArray(size, offset)
	case _Map:
		return d.decodeMap(size, offset)
	default:
		return nil, 0, fmt.Errorf("unknown type: %d", dtype)
	}
}

func (d *decoder) sizeFromCtrlByte(ctrlByte byte, offset uint, typeNum dataType) (uint, uint) {
	size := uint(ctrlByte & 0x1f)
	if typeNum == _Extended {
		return size, offset
	}

	var bytesToRead uint
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
