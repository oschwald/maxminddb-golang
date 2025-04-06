// Package decoder decodes values in the data section.
package decoder

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"sync"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

// Decoder is a decoder for the MMDB data section.
type Decoder struct {
	DataDecoder
}

// New creates a [Decoder].
func New(buffer []byte) Decoder {
	return Decoder{DataDecoder: NewDataDecoder(buffer)}
}

// Decode decodes the data value at offset and stores it in the value
// pointed at by v.
func (d *Decoder) Decode(offset uint, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("result param must be a pointer")
	}

	if dser, ok := v.(deserializer); ok {
		_, err := d.decodeToDeserializer(offset, dser, 0, false)
		return err
	}

	_, err := d.decode(offset, rv, 0)
	return err
}

func (d *Decoder) decode(offset uint, result reflect.Value, depth int) (uint, error) {
	if depth > maximumDataStructureDepth {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"exceeded maximum data structure depth; database is likely corrupt",
		)
	}
	typeNum, size, newOffset, err := d.decodeCtrlData(offset)
	if err != nil {
		return 0, err
	}

	if typeNum != TypePointer && result.Kind() == reflect.Uintptr {
		result.Set(reflect.ValueOf(uintptr(offset)))
		return d.nextValueOffset(offset, 1)
	}
	return d.decodeFromType(typeNum, size, newOffset, result, depth+1)
}

// DecodePath decodes the data value at offset and stores the value assocated
// with the path in the value pointed at by v.
func (d *Decoder) DecodePath(
	offset uint,
	path []any,
	v any,
) error {
	result := reflect.ValueOf(v)
	if result.Kind() != reflect.Ptr || result.IsNil() {
		return errors.New("result param must be a pointer")
	}

PATH:
	for i, v := range path {
		var (
			typeNum Type
			size    uint
			err     error
		)
		typeNum, size, offset, err = d.decodeCtrlData(offset)
		if err != nil {
			return err
		}

		if typeNum == TypePointer {
			pointer, _, err := d.decodePointer(size, offset)
			if err != nil {
				return err
			}

			typeNum, size, offset, err = d.decodeCtrlData(pointer)
			if err != nil {
				return err
			}
		}

		switch v := v.(type) {
		case string:
			// We are expecting a map
			if typeNum != TypeMap {
				// XXX - use type names in errors.
				return fmt.Errorf("expected a map for %s but found %d", v, typeNum)
			}
			for range size {
				var key []byte
				key, offset, err = d.decodeKey(offset)
				if err != nil {
					return err
				}
				if string(key) == v {
					continue PATH
				}
				offset, err = d.nextValueOffset(offset, 1)
				if err != nil {
					return err
				}
			}
			// Not found. Maybe return a boolean?
			return nil
		case int:
			// We are expecting an array
			if typeNum != TypeSlice {
				// XXX - use type names in errors.
				return fmt.Errorf("expected a slice for %d but found %d", v, typeNum)
			}
			var i uint
			if v < 0 {
				if size < uint(-v) {
					// Slice is smaller than negative index, not found
					return nil
				}
				i = size - uint(-v)
			} else {
				if size <= uint(v) {
					// Slice is smaller than index, not found
					return nil
				}
				i = uint(v)
			}
			offset, err = d.nextValueOffset(offset, i)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected type for %d value in path, %v: %T", i, v, v)
		}
	}
	_, err := d.decode(offset, result, len(path))
	return err
}

func (d *Decoder) decodeFromType(
	dtype Type,
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	result = indirect(result)

	// For these types, size has a special meaning
	switch dtype {
	case TypeBool:
		return unmarshalBool(size, offset, result)
	case TypeMap:
		return d.unmarshalMap(size, offset, result, depth)
	case TypePointer:
		return d.unmarshalPointer(size, offset, result, depth)
	case TypeSlice:
		return d.unmarshalSlice(size, offset, result, depth)
	case TypeBytes:
		return d.unmarshalBytes(size, offset, result)
	case TypeFloat32:
		return d.unmarshalFloat32(size, offset, result)
	case TypeFloat64:
		return d.unmarshalFloat64(size, offset, result)
	case TypeInt32:
		return d.unmarshalInt32(size, offset, result)
	case TypeUint16:
		return d.unmarshalUint(size, offset, result, 16)
	case TypeUint32:
		return d.unmarshalUint(size, offset, result, 32)
	case TypeUint64:
		return d.unmarshalUint(size, offset, result, 64)
	case TypeString:
		return d.unmarshalString(size, offset, result)
	case TypeUint128:
		return d.unmarshalUint128(size, offset, result)
	default:
		return 0, mmdberrors.NewInvalidDatabaseError("unknown type: %d", dtype)
	}
}

func unmarshalBool(size, offset uint, result reflect.Value) (uint, error) {
	if size > 1 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (bool size of %v)",
			size,
		)
	}
	value, newOffset := decodeBool(size, offset)

	switch result.Kind() {
	case reflect.Bool:
		result.SetBool(value)
		return newOffset, nil
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, mmdberrors.NewUnmarshalTypeError(value, result.Type())
}

// indirect follows pointers and create values as necessary. This is
// heavily based on encoding/json as my original version had a subtle
// bug. This method should be considered to be licensed under
// https://golang.org/LICENSE
func indirect(result reflect.Value) reflect.Value {
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if result.Kind() == reflect.Interface && !result.IsNil() {
			e := result.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() {
				result = e
				continue
			}
		}

		if result.Kind() != reflect.Ptr {
			break
		}

		if result.IsNil() {
			result.Set(reflect.New(result.Type().Elem()))
		}

		result = result.Elem()
	}
	return result
}

var sliceType = reflect.TypeOf([]byte{})

func (d *Decoder) unmarshalBytes(size, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.decodeBytes(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	case reflect.Slice:
		if result.Type() == sliceType {
			result.SetBytes(value)
			return newOffset, nil
		}
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, mmdberrors.NewUnmarshalTypeError(value, result.Type())
}

func (d *Decoder) unmarshalFloat32(size, offset uint, result reflect.Value) (uint, error) {
	if size != 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float32 size of %v)",
			size,
		)
	}
	value, newOffset, err := d.decodeFloat32(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	case reflect.Float32, reflect.Float64:
		result.SetFloat(float64(value))
		return newOffset, nil
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, mmdberrors.NewUnmarshalTypeError(value, result.Type())
}

func (d *Decoder) unmarshalFloat64(size, offset uint, result reflect.Value) (uint, error) {
	if size != 8 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float 64 size of %v)",
			size,
		)
	}
	value, newOffset, err := d.decodeFloat64(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	case reflect.Float32, reflect.Float64:
		if result.OverflowFloat(value) {
			return 0, mmdberrors.NewUnmarshalTypeError(value, result.Type())
		}
		result.SetFloat(value)
		return newOffset, nil
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, mmdberrors.NewUnmarshalTypeError(value, result.Type())
}

func (d *Decoder) unmarshalInt32(size, offset uint, result reflect.Value) (uint, error) {
	if size > 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (int32 size of %v)",
			size,
		)
	}

	value, newOffset, err := d.decodeInt(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(value)
		if !result.OverflowInt(n) {
			result.SetInt(n)
			return newOffset, nil
		}
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		n := uint64(value)
		if !result.OverflowUint(n) {
			result.SetUint(n)
			return newOffset, nil
		}
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, mmdberrors.NewUnmarshalTypeError(value, result.Type())
}

func (d *Decoder) unmarshalMap(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	result = indirect(result)
	switch result.Kind() {
	default:
		return 0, mmdberrors.NewUnmarshalTypeStrError("map", result.Type())
	case reflect.Struct:
		return d.decodeStruct(size, offset, result, depth)
	case reflect.Map:
		return d.decodeMap(size, offset, result, depth)
	case reflect.Interface:
		if result.NumMethod() == 0 {
			rv := reflect.ValueOf(make(map[string]any, size))
			newOffset, err := d.decodeMap(size, offset, rv, depth)
			result.Set(rv)
			return newOffset, err
		}
		return 0, mmdberrors.NewUnmarshalTypeStrError("map", result.Type())
	}
}

func (d *Decoder) unmarshalPointer(
	size, offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	pointer, newOffset, err := d.decodePointer(size, offset)
	if err != nil {
		return 0, err
	}
	_, err = d.decode(pointer, result, depth)
	return newOffset, err
}

func (d *Decoder) unmarshalSlice(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	switch result.Kind() {
	case reflect.Slice:
		return d.decodeSlice(size, offset, result, depth)
	case reflect.Interface:
		if result.NumMethod() == 0 {
			a := []any{}
			rv := reflect.ValueOf(&a).Elem()
			newOffset, err := d.decodeSlice(size, offset, rv, depth)
			result.Set(rv)
			return newOffset, err
		}
	}
	return 0, mmdberrors.NewUnmarshalTypeStrError("array", result.Type())
}

func (d *Decoder) unmarshalString(size, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.decodeString(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	case reflect.String:
		result.SetString(value)
		return newOffset, nil
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, mmdberrors.NewUnmarshalTypeError(value, result.Type())
}

func (d *Decoder) unmarshalUint(
	size, offset uint,
	result reflect.Value,
	uintType uint,
) (uint, error) {
	if size > uintType/8 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint%v size of %v)",
			uintType,
			size,
		)
	}

	value, newOffset, err := d.decodeUint(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(value)
		if !result.OverflowInt(n) {
			result.SetInt(n)
			return newOffset, nil
		}
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		if !result.OverflowUint(value) {
			result.SetUint(value)
			return newOffset, nil
		}
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, mmdberrors.NewUnmarshalTypeError(value, result.Type())
}

var bigIntType = reflect.TypeOf(big.Int{})

func (d *Decoder) unmarshalUint128(size, offset uint, result reflect.Value) (uint, error) {
	if size > 16 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint128 size of %v)",
			size,
		)
	}

	value, newOffset, err := d.decodeUint128(size, offset)
	if err != nil {
		return 0, err
	}

	switch result.Kind() {
	case reflect.Struct:
		if result.Type() == bigIntType {
			result.Set(reflect.ValueOf(*value))
			return newOffset, nil
		}
	case reflect.Interface:
		if result.NumMethod() == 0 {
			result.Set(reflect.ValueOf(value))
			return newOffset, nil
		}
	}
	return newOffset, mmdberrors.NewUnmarshalTypeError(value, result.Type())
}

func (d *Decoder) decodeMap(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	if result.IsNil() {
		result.Set(reflect.MakeMapWithSize(result.Type(), int(size)))
	}

	mapType := result.Type()
	keyValue := reflect.New(mapType.Key()).Elem()
	elemType := mapType.Elem()
	var elemValue reflect.Value
	for range size {
		var key []byte
		var err error
		key, offset, err = d.decodeKey(offset)
		if err != nil {
			return 0, err
		}

		if elemValue.IsValid() {
			elemValue.SetZero()
		} else {
			elemValue = reflect.New(elemType).Elem()
		}

		offset, err = d.decode(offset, elemValue, depth)
		if err != nil {
			return 0, fmt.Errorf("decoding value for %s: %w", key, err)
		}

		keyValue.SetString(string(key))
		result.SetMapIndex(keyValue, elemValue)
	}
	return offset, nil
}

func (d *Decoder) decodeSlice(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	result.Set(reflect.MakeSlice(result.Type(), int(size), int(size)))
	for i := range size {
		var err error
		offset, err = d.decode(offset, result.Index(int(i)), depth)
		if err != nil {
			return 0, err
		}
	}
	return offset, nil
}

func (d *Decoder) decodeStruct(
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	fields := cachedFields(result)

	// This fills in embedded structs
	for _, i := range fields.anonymousFields {
		_, err := d.unmarshalMap(size, offset, result.Field(i), depth)
		if err != nil {
			return 0, err
		}
	}

	// This handles named fields
	for range size {
		var (
			err error
			key []byte
		)
		key, offset, err = d.decodeKey(offset)
		if err != nil {
			return 0, err
		}
		// The string() does not create a copy due to this compiler
		// optimization: https://github.com/golang/go/issues/3512
		j, ok := fields.namedFields[string(key)]
		if !ok {
			offset, err = d.nextValueOffset(offset, 1)
			if err != nil {
				return 0, err
			}
			continue
		}

		offset, err = d.decode(offset, result.Field(j), depth)
		if err != nil {
			return 0, fmt.Errorf("decoding value for %s: %w", key, err)
		}
	}
	return offset, nil
}

type fieldsType struct {
	namedFields     map[string]int
	anonymousFields []int
}

var fieldsMap sync.Map

func cachedFields(result reflect.Value) *fieldsType {
	resultType := result.Type()

	if fields, ok := fieldsMap.Load(resultType); ok {
		return fields.(*fieldsType)
	}
	numFields := resultType.NumField()
	namedFields := make(map[string]int, numFields)
	var anonymous []int
	for i := range numFields {
		field := resultType.Field(i)

		fieldName := field.Name
		if tag := field.Tag.Get("maxminddb"); tag != "" {
			if tag == "-" {
				continue
			}
			fieldName = tag
		}
		if field.Anonymous {
			anonymous = append(anonymous, i)
			continue
		}
		namedFields[fieldName] = i
	}
	fields := &fieldsType{namedFields, anonymous}
	fieldsMap.Store(resultType, fields)

	return fields
}
