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

// Unmarshaler is implemented by types that can unmarshal MaxMind DB data.
// This is used internally for reflection-based decoding.
type Unmarshaler interface {
	UnmarshalMaxMindDB(d *Decoder) error
}

// unmarshalerType is cached for efficient interface checking.
var unmarshalerType = reflect.TypeFor[Unmarshaler]()

// ReflectionDecoder is a decoder for the MMDB data section.
type ReflectionDecoder struct {
	DataDecoder
}

// New creates a [ReflectionDecoder].
func New(buffer []byte) ReflectionDecoder {
	return ReflectionDecoder{
		DataDecoder: NewDataDecoder(buffer),
	}
}

// Decode decodes the data value at offset and stores it in the value
// pointed at by v.
func (d *ReflectionDecoder) Decode(offset uint, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("result param must be a pointer")
	}

	// Check if the type implements Unmarshaler interface using cached type check
	if rv.Type().Implements(unmarshalerType) {
		unmarshaler := v.(Unmarshaler) // Safe, we know it implements
		decoder := NewDecoder(d.DataDecoder, offset)
		return unmarshaler.UnmarshalMaxMindDB(decoder)
	}

	if dser, ok := v.(deserializer); ok {
		_, err := d.decodeToDeserializer(offset, dser, 0, false)
		return d.wrapError(err, offset)
	}

	_, err := d.decode(offset, rv, 0)
	if err == nil {
		return nil
	}

	// Check if error already has context (including path), if so just add offset if missing
	var contextErr mmdberrors.ContextualError
	if errors.As(err, &contextErr) {
		// If the outermost error already has offset and path info, return as-is
		if contextErr.Offset != 0 || contextErr.Path != "" {
			return err
		}
		// Otherwise, just add offset to root
		return mmdberrors.WrapWithContext(contextErr.Err, offset, nil)
	}

	// Plain error, add offset
	return mmdberrors.WrapWithContext(err, offset, nil)
}

// DecodePath decodes the data value at offset and stores the value assocated
// with the path in the value pointed at by v.
func (d *ReflectionDecoder) DecodePath(
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
			typeNum Kind
			size    uint
			err     error
		)
		typeNum, size, offset, err = d.DecodeCtrlData(offset)
		if err != nil {
			return err
		}

		if typeNum == KindPointer {
			pointer, _, err := d.DecodePointer(size, offset)
			if err != nil {
				return err
			}

			typeNum, size, offset, err = d.DecodeCtrlData(pointer)
			if err != nil {
				return err
			}
		}

		switch v := v.(type) {
		case string:
			// We are expecting a map
			if typeNum != KindMap {
				// XXX - use type names in errors.
				return fmt.Errorf("expected a map for %s but found %d", v, typeNum)
			}
			for range size {
				var key []byte
				key, offset, err = d.DecodeKey(offset)
				if err != nil {
					return err
				}
				if string(key) == v {
					continue PATH
				}
				offset, err = d.NextValueOffset(offset, 1)
				if err != nil {
					return err
				}
			}
			// Not found. Maybe return a boolean?
			return nil
		case int:
			// We are expecting an array
			if typeNum != KindSlice {
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
			offset, err = d.NextValueOffset(offset, i)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected type for %d value in path, %v: %T", i, v, v)
		}
	}
	_, err := d.decode(offset, result, len(path))
	return d.wrapError(err, offset)
}

// wrapError wraps an error with context information when an error occurs.
// Zero allocation on happy path - only allocates when error != nil.
func (*ReflectionDecoder) wrapError(err error, offset uint) error {
	if err == nil {
		return nil
	}
	// Only wrap with context when an error actually occurs
	return mmdberrors.WrapWithContext(err, offset, nil)
}

// wrapErrorWithMapKey wraps an error with map key context, building path retroactively.
// Zero allocation on happy path - only allocates when error != nil.
func (*ReflectionDecoder) wrapErrorWithMapKey(err error, key string) error {
	if err == nil {
		return nil
	}

	// Build path context retroactively by checking if the error already has context
	var pathBuilder *mmdberrors.PathBuilder
	var contextErr mmdberrors.ContextualError
	if errors.As(err, &contextErr) {
		// Error already has context, extract existing path and extend it
		pathBuilder = mmdberrors.NewPathBuilder()
		if contextErr.Path != "" && contextErr.Path != "/" {
			// Parse existing path and rebuild
			pathBuilder.ParseAndExtend(contextErr.Path)
		}
		pathBuilder.PrependMap(key)
		// Return unwrapped error with extended path, preserving original offset
		return mmdberrors.WrapWithContext(contextErr.Err, contextErr.Offset, pathBuilder)
	}

	// New error, start building path - extract offset if it's already a contextual error
	pathBuilder = mmdberrors.NewPathBuilder()
	pathBuilder.PrependMap(key)

	// Try to get existing offset from any wrapped contextual error
	var existingOffset uint
	var existingErr mmdberrors.ContextualError
	if errors.As(err, &existingErr) {
		existingOffset = existingErr.Offset
	}

	return mmdberrors.WrapWithContext(err, existingOffset, pathBuilder)
}

// wrapErrorWithSliceIndex wraps an error with slice index context, building path retroactively.
// Zero allocation on happy path - only allocates when error != nil.
func (*ReflectionDecoder) wrapErrorWithSliceIndex(err error, index int) error {
	if err == nil {
		return nil
	}

	// Build path context retroactively by checking if the error already has context
	var pathBuilder *mmdberrors.PathBuilder
	var contextErr mmdberrors.ContextualError
	if errors.As(err, &contextErr) {
		// Error already has context, extract existing path and extend it
		pathBuilder = mmdberrors.NewPathBuilder()
		if contextErr.Path != "" && contextErr.Path != "/" {
			// Parse existing path and rebuild
			pathBuilder.ParseAndExtend(contextErr.Path)
		}
		pathBuilder.PrependSlice(index)
		// Return unwrapped error with extended path, preserving original offset
		return mmdberrors.WrapWithContext(contextErr.Err, contextErr.Offset, pathBuilder)
	}

	// New error, start building path - extract offset if it's already a contextual error
	pathBuilder = mmdberrors.NewPathBuilder()
	pathBuilder.PrependSlice(index)

	// Try to get existing offset from any wrapped contextual error
	var existingOffset uint
	var existingErr mmdberrors.ContextualError
	if errors.As(err, &existingErr) {
		existingOffset = existingErr.Offset
	}

	return mmdberrors.WrapWithContext(err, existingOffset, pathBuilder)
}

func (d *ReflectionDecoder) decode(offset uint, result reflect.Value, depth int) (uint, error) {
	if depth > maximumDataStructureDepth {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"exceeded maximum data structure depth; database is likely corrupt",
		)
	}

	// First handle pointers by creating the value if needed, similar to indirect()
	// but we don't want to fully indirect yet as we need to check for Unmarshaler
	if result.Kind() == reflect.Ptr {
		if result.IsNil() {
			result.Set(reflect.New(result.Type().Elem()))
		}
		// Now check if the pointed-to type implements Unmarshaler using cached type check
		if result.Type().Implements(unmarshalerType) {
			unmarshaler := result.Interface().(Unmarshaler) // Safe, we know it implements
			decoder := NewDecoder(d.DataDecoder, offset)
			if err := unmarshaler.UnmarshalMaxMindDB(decoder); err != nil {
				return 0, err
			}
			return decoder.getNextOffset()
		}
		// Continue with the pointed-to value
		return d.decode(offset, result.Elem(), depth)
	}

	// Check if the value implements Unmarshaler interface
	// We need to check if result can be addressed and if the pointer type implements Unmarshaler
	if result.CanAddr() {
		ptrType := result.Addr().Type()
		if ptrType.Implements(unmarshalerType) {
			unmarshaler := result.Addr().Interface().(Unmarshaler) // Safe, we know it implements
			decoder := NewDecoder(d.DataDecoder, offset)
			if err := unmarshaler.UnmarshalMaxMindDB(decoder); err != nil {
				return 0, err
			}
			return decoder.getNextOffset()
		}
	}

	typeNum, size, newOffset, err := d.DecodeCtrlData(offset)
	if err != nil {
		return 0, err
	}

	if typeNum != KindPointer && result.Kind() == reflect.Uintptr {
		result.Set(reflect.ValueOf(uintptr(offset)))
		return d.NextValueOffset(offset, 1)
	}
	return d.decodeFromType(typeNum, size, newOffset, result, depth+1)
}

func (d *ReflectionDecoder) decodeFromType(
	dtype Kind,
	size uint,
	offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	result = indirect(result)

	// For these types, size has a special meaning
	switch dtype {
	case KindBool:
		return unmarshalBool(size, offset, result)
	case KindMap:
		return d.unmarshalMap(size, offset, result, depth)
	case KindPointer:
		return d.unmarshalPointer(size, offset, result, depth)
	case KindSlice:
		return d.unmarshalSlice(size, offset, result, depth)
	case KindBytes:
		return d.unmarshalBytes(size, offset, result)
	case KindFloat32:
		return d.unmarshalFloat32(size, offset, result)
	case KindFloat64:
		return d.unmarshalFloat64(size, offset, result)
	case KindInt32:
		return d.unmarshalInt32(size, offset, result)
	case KindUint16:
		return d.unmarshalUint(size, offset, result, 16)
	case KindUint32:
		return d.unmarshalUint(size, offset, result, 32)
	case KindUint64:
		return d.unmarshalUint(size, offset, result, 64)
	case KindString:
		return d.unmarshalString(size, offset, result)
	case KindUint128:
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

func (d *ReflectionDecoder) unmarshalBytes(size, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.DecodeBytes(size, offset)
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

func (d *ReflectionDecoder) unmarshalFloat32(
	size, offset uint, result reflect.Value,
) (uint, error) {
	if size != 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float32 size of %v)",
			size,
		)
	}
	value, newOffset, err := d.DecodeFloat32(size, offset)
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

func (d *ReflectionDecoder) unmarshalFloat64(
	size, offset uint, result reflect.Value,
) (uint, error) {
	if size != 8 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (float 64 size of %v)",
			size,
		)
	}
	value, newOffset, err := d.DecodeFloat64(size, offset)
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

func (d *ReflectionDecoder) unmarshalInt32(size, offset uint, result reflect.Value) (uint, error) {
	if size > 4 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (int32 size of %v)",
			size,
		)
	}

	value, newOffset, err := d.DecodeInt32(size, offset)
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

func (d *ReflectionDecoder) unmarshalMap(
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

func (d *ReflectionDecoder) unmarshalPointer(
	size, offset uint,
	result reflect.Value,
	depth int,
) (uint, error) {
	pointer, newOffset, err := d.DecodePointer(size, offset)
	if err != nil {
		return 0, err
	}
	_, err = d.decode(pointer, result, depth)
	return newOffset, err
}

func (d *ReflectionDecoder) unmarshalSlice(
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

func (d *ReflectionDecoder) unmarshalString(size, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.DecodeString(size, offset)
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

func (d *ReflectionDecoder) unmarshalUint(
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

	value, newOffset, err := d.DecodeUint64(size, offset)
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

func (d *ReflectionDecoder) unmarshalUint128(
	size, offset uint, result reflect.Value,
) (uint, error) {
	if size > 16 {
		return 0, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (uint128 size of %v)",
			size,
		)
	}

	value, newOffset, err := d.DecodeUint128(size, offset)
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

func (d *ReflectionDecoder) decodeMap(
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
		var err error

		offset, err = d.decode(offset, keyValue, depth)
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
			return 0, d.wrapErrorWithMapKey(err, keyValue.String())
		}

		result.SetMapIndex(keyValue, elemValue)
	}
	return offset, nil
}

func (d *ReflectionDecoder) decodeSlice(
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
			return 0, d.wrapErrorWithSliceIndex(err, int(i))
		}
	}
	return offset, nil
}

func (d *ReflectionDecoder) decodeStruct(
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
		key, offset, err = d.DecodeKey(offset)
		if err != nil {
			return 0, err
		}
		// The string() does not create a copy due to this compiler
		// optimization: https://github.com/golang/go/issues/3512
		j, ok := fields.namedFields[string(key)]
		if !ok {
			offset, err = d.NextValueOffset(offset, 1)
			if err != nil {
				return 0, err
			}
			continue
		}

		offset, err = d.decode(offset, result.Field(j), depth)
		if err != nil {
			return 0, d.wrapErrorWithMapKey(err, string(key))
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
