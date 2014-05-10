package maxminddb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"reflect"
)

type decoder struct {
	buffer      []byte
	pointerBase uint
}

func (d *decoder) decodeSlice(size uint, offset uint, result reflect.Value) (uint, error) {
	result.Set(reflect.MakeSlice(result.Type(), int(size), int(size)))
	for i := 0; i < int(size); i++ {
		var err error
		offset, err = d.decode(offset, result.Index(i))
		if err != nil {
			return 0, err
		}
	}
	return offset, nil
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

func (d *decoder) decodeMap(size uint, offset uint, result reflect.Value) (uint, error) {
	if result.IsNil() {
		result.Set(reflect.MakeMap(result.Type()))
	}

	for i := uint(0); i < size; i++ {
		key := reflect.New(result.Type().Key())

		var err error
		offset, err = d.decode(offset, key)
		if err != nil {
			return 0, err
		}

		value := reflect.New(result.Type().Elem())
		offset, err = d.decode(offset, value)
		if err != nil {
			return 0, err
		}
		result.SetMapIndex(key.Elem(), value.Elem())
	}
	return offset, nil
}

func (d *decoder) decodeStruct(size uint, offset uint, result reflect.Value) (uint, error) {

	resultType := result.Type()
	numFields := resultType.NumField()

	fields := make(map[string]reflect.Value)
	for i := 0; i < numFields; i++ {
		fieldType := resultType.Field(i)

		fieldName := fieldType.Name
		tag := fieldType.Tag.Get("maxminddb")
		if tag != "" {
			fieldName = tag
		}
		fields[fieldName] = result.Field(i)
	}

	for i := uint(0); i < size; i++ {

		var key string
		keyValue := reflect.ValueOf(&key)

		var err error
		offset, err = d.decode(offset, keyValue)
		if err != nil {
			return 0, err
		}
		field, ok := fields[key]
		if !ok {
			// XXX - Ideally we should not bother decoding values we skip.
			// This doesn't matter for the geoip2 reader as we want to decode
			// everything, but it may matter for other use cases that just
			// want one or two values quickly.
			var skip interface{}
			field = reflect.ValueOf(&skip)
		}
		offset, err = d.decode(offset, field)
		if err != nil {
			return 0, err
		}
	}
	return offset, nil
}

var pointerValueOffset = map[uint]uint{
	1: 0,
	2: 2048,
	3: 526336,
	4: 0,
}

func (d *decoder) decodePointer(size uint, offset uint, result reflect.Value) (uint, error) {
	pointerSize := ((size >> 3) & 0x3) + 1
	newOffset := offset + pointerSize
	pointerBytes := d.buffer[offset:newOffset]
	var prefix uint64
	if pointerSize == 4 {
		prefix = 0
	} else {
		prefix = uint64(size & 0x7)
	}
	unpacked := uint(uintFromBytes(prefix, pointerBytes))

	pointer := unpacked + d.pointerBase + pointerValueOffset[pointerSize]
	_, err := d.decode(pointer, result)
	return newOffset, err
}

func (d *decoder) decodeUint(size uint, offset uint) (uint64, uint, error) {
	newOffset := offset + size
	val := uintFromBytes(0, d.buffer[offset:newOffset])

	return val, newOffset, nil
}

func (d *decoder) decodeUint128(size uint, offset uint) (*big.Int, uint, error) {
	newOffset := offset + size
	val := new(big.Int)
	val.SetBytes(d.buffer[offset:newOffset])

	return val, newOffset, nil
}

func uintFromBytes(prefix uint64, uintBytes []byte) uint64 {
	val := prefix
	for _, b := range uintBytes {
		val = (val << 8) | uint64(b)
	}
	return val
}

func (d *decoder) decodeString(size uint, offset uint) (string, uint, error) {
	newOffset := offset + size
	return string(d.buffer[offset:newOffset]), newOffset, nil
}

func (d *decoder) decode(offset uint, result reflect.Value) (uint, error) {
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
	return d.decodeFromType(typeNum, size, newOffset, result)
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
	_Slice
	_Container
	_Marker
	_Bool
	_Float32
)

func (d *decoder) decodeFromType(dtype dataType, size uint, offset uint, result reflect.Value) (uint, error) {

	if result.Kind() == reflect.Ptr {
		result = reflect.Indirect(result)
	}

	switch dtype {
	case _Pointer:
		return d.decodePointer(size, offset, result)
	case _Bool:
		value, newOffset, err := d.decodeBool(size, offset)
		if err != nil {
			return 0, err
		}
		switch result.Kind() {
		default:
			return newOffset, fmt.Errorf("trying to unmarshal %v into %v", value, result.Type())
		case reflect.Bool:
			result.SetBool(value)
			return newOffset, nil
		case reflect.Interface:
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	case _Int32:
		value, newOffset, err := d.decodeInt(size, offset)
		if err != nil {
			return 0, err
		}

		switch result.Kind() {
		default:
			return newOffset, fmt.Errorf("trying to unmarshal %v into %v", value, result.Type())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result.SetInt(int64(value))
			return newOffset, nil
		case reflect.Interface:
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	case _Uint16, _Uint32, _Uint64:
		value, newOffset, err := d.decodeUint(size, offset)
		if err != nil {
			return 0, err
		}

		switch result.Kind() {
		default:
			return newOffset, fmt.Errorf("trying to unmarshal %v into %v", value, result.Type())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			result.SetUint(value)
			return newOffset, nil
		case reflect.Interface:
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	case _Uint128:
		value, newOffset, err := d.decodeUint128(size, offset)
		if err != nil {
			return 0, err
		}

		// XXX - this should allow *big.Int rather than just bigInt
		// Currently this is reported as invalid
		switch result.Kind() {
		default:
			return newOffset, fmt.Errorf("trying to unmarshal %v into %v", value, result.Type())
		case reflect.Struct:
			result.Set(reflect.ValueOf(*value))
			return newOffset, nil
		case reflect.Interface, reflect.Ptr:
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	case _Float32:
		value, newOffset, err := d.decodeFloat32(size, offset)
		if err != nil {
			return 0, err
		}

		switch result.Kind() {
		default:
			return newOffset, fmt.Errorf("trying to unmarshal %v into %v", value, result.Type())
		case reflect.Float32, reflect.Float64:
			result.SetFloat(float64(value))
			return newOffset, nil
		case reflect.Interface:
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	case _Float64:
		value, newOffset, err := d.decodeFloat64(size, offset)
		if err != nil {
			return 0, err
		}
		switch result.Kind() {
		default:
			return newOffset, fmt.Errorf("trying to unmarshal %v into %v", value, result.Type())
		case reflect.Float32, reflect.Float64:
			result.SetFloat(value)
			return newOffset, nil
		case reflect.Interface:
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	case _String:
		value, newOffset, err := d.decodeString(size, offset)

		if err != nil {
			return 0, err
		}
		switch result.Kind() {
		default:
			return newOffset, fmt.Errorf("trying to unmarshal %v into %v", value, result.Type())
		case reflect.String:
			result.SetString(value)
			return newOffset, nil
		case reflect.Interface:
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	case _Bytes:
		value, newOffset, err := d.decodeBytes(size, offset)
		if err != nil {
			return 0, err
		}
		switch result.Kind() {
		default:
			return newOffset, fmt.Errorf("trying to unmarshal %v into %v", value, result.Type())
		case reflect.Slice:
			result.SetBytes(value)
			return newOffset, nil
		case reflect.Interface:
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	case _Slice:
		switch result.Kind() {
		default:
			return 0, fmt.Errorf("trying to unmarshal an array into %v", result.Type())
		case reflect.Slice:
			return d.decodeSlice(size, offset, result)
		case reflect.Interface:
			a := []interface{}{}
			rv := reflect.ValueOf(&a).Elem()
			newOffset, err := d.decodeSlice(size, offset, rv)
			result.Set(rv)
			return newOffset, err
		}
	case _Map:
		switch result.Kind() {
		default:
			return 0, fmt.Errorf("trying to unmarshal a map into %v", result.Type())
		case reflect.Struct:
			return d.decodeStruct(size, offset, result)
		case reflect.Map:
			return d.decodeMap(size, offset, result)
		case reflect.Interface:
			rv := reflect.ValueOf(make(map[string]interface{}))
			newOffset, err := d.decodeMap(size, offset, rv)
			result.Set(rv)
			return newOffset, err
		}
	default:
		return 0, fmt.Errorf("unknown type: %d", dtype)
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
		size = 285 + uint(uintFromBytes(0, sizeBytes))
	case size > 30:
		size = uint(uintFromBytes(0, sizeBytes)) + 65821
	}
	return size, newOffset
}
