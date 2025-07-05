// Package decoder decodes values in the data section.
package decoder

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"sync"
	"unicode/utf8"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

// Unmarshaler is implemented by types that can unmarshal MaxMind DB data.
// This is used internally for reflection-based decoding.
type Unmarshaler interface {
	UnmarshalMaxMindDB(d *Decoder) error
}

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
	// Check if the type implements Unmarshaler interface without reflection
	if unmarshaler, ok := v.(Unmarshaler); ok {
		decoder := NewDecoder(d.DataDecoder, offset)
		return unmarshaler.UnmarshalMaxMindDB(decoder)
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("result param must be a pointer")
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
		typeNum, size, offset, err = d.decodeCtrlData(offset)
		if err != nil {
			return err
		}

		if typeNum == KindPointer {
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
			if typeNum != KindMap {
				return fmt.Errorf("expected a map for %s but found %s", v, typeNum.String())
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
			if typeNum != KindSlice {
				return fmt.Errorf("expected a slice for %d but found %s", v, typeNum.String())
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
	if result.Kind() == reflect.Ptr {
		if result.IsNil() {
			result.Set(reflect.New(result.Type().Elem()))
		}
		// Continue with the pointed-to value - interface check will happen in recursive call
		return d.decode(offset, result.Elem(), depth)
	}

	// Check if the value implements Unmarshaler interface using type assertion
	if result.CanAddr() {
		if unmarshaler, ok := result.Addr().Interface().(Unmarshaler); ok {
			decoder := NewDecoder(d.DataDecoder, offset)
			if err := unmarshaler.UnmarshalMaxMindDB(decoder); err != nil {
				return 0, err
			}
			return decoder.getNextOffset()
		}
	}

	typeNum, size, newOffset, err := d.decodeCtrlData(offset)
	if err != nil {
		return 0, err
	}

	if typeNum != KindPointer && result.Kind() == reflect.Uintptr {
		result.Set(reflect.ValueOf(uintptr(offset)))
		return d.nextValueOffset(offset, 1)
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
		return d.unmarshalBool(size, offset, result)
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

func (d *ReflectionDecoder) unmarshalBool(size, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.decodeBool(size, offset)
	if err != nil {
		return 0, err
	}

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

func (d *ReflectionDecoder) unmarshalFloat32(
	size, offset uint, result reflect.Value,
) (uint, error) {
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

func (d *ReflectionDecoder) unmarshalFloat64(
	size, offset uint, result reflect.Value,
) (uint, error) {
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

func (d *ReflectionDecoder) unmarshalInt32(size, offset uint, result reflect.Value) (uint, error) {
	value, newOffset, err := d.decodeInt32(size, offset)
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
	pointer, newOffset, err := d.decodePointer(size, offset)
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

func (d *ReflectionDecoder) unmarshalUint(
	size, offset uint,
	result reflect.Value,
	uintType uint,
) (uint, error) {
	// Use the appropriate DataDecoder method based on uint type
	var value uint64
	var newOffset uint
	var err error

	switch uintType {
	case 16:
		v16, off, e := d.decodeUint16(size, offset)
		value, newOffset, err = uint64(v16), off, e
	case 32:
		v32, off, e := d.decodeUint32(size, offset)
		value, newOffset, err = uint64(v32), off, e
	case 64:
		value, newOffset, err = d.decodeUint64(size, offset)
	default:
		return 0, mmdberrors.NewInvalidDatabaseError(
			"unsupported uint type: %d", uintType)
	}

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
	hi, lo, newOffset, err := d.decodeUint128(size, offset)
	if err != nil {
		return 0, err
	}

	// Convert hi/lo representation to big.Int
	value := new(big.Int)
	if hi == 0 {
		value.SetUint64(lo)
	} else {
		value.SetUint64(hi)
		value.Lsh(value, 64)                        // Shift high part left by 64 bits
		value.Or(value, new(big.Int).SetUint64(lo)) // OR with low part
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

	// Single-phase processing: decode only the dominant fields
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
		fieldInfo, ok := fields.namedFields[string(key)]
		if !ok {
			offset, err = d.nextValueOffset(offset, 1)
			if err != nil {
				return 0, err
			}
			continue
		}

		// Use optimized field access with addressable value wrapper
		av := newAddressableValue(result)
		fieldValue := av.fieldByIndex(fieldInfo.index0, fieldInfo.index, true)
		if !fieldValue.IsValid() {
			// Field access failed, skip this field
			offset, err = d.nextValueOffset(offset, 1)
			if err != nil {
				return 0, err
			}
			continue
		}
		offset, err = d.decode(offset, fieldValue.Value, depth)
		if err != nil {
			return 0, d.wrapErrorWithMapKey(err, string(key))
		}
	}
	return offset, nil
}

type fieldInfo struct {
	name   string
	index  []int // Remaining indices (nil if single field)
	index0 int   // First field index (avoids bounds check)
	depth  int
	hasTag bool
}

type fieldsType struct {
	namedFields map[string]*fieldInfo // Map from field name to field info
}

type queueEntry struct {
	typ   reflect.Type
	index []int // Field index path
	depth int   // Embedding depth
}

// getEmbeddedStructType returns the struct type for embedded fields.
// Returns nil if the field is not an embeddable struct type.
func getEmbeddedStructType(fieldType reflect.Type) reflect.Type {
	if fieldType.Kind() == reflect.Struct {
		return fieldType
	}
	if fieldType.Kind() == reflect.Ptr && fieldType.Elem().Kind() == reflect.Struct {
		return fieldType.Elem()
	}
	return nil
}

// handleEmbeddedField processes an embedded struct field and returns true if the field should be skipped.
func handleEmbeddedField(
	field reflect.StructField,
	hasTag bool,
	queue *[]queueEntry,
	seen *map[reflect.Type]bool,
	fieldIndex []int,
	depth int,
) bool {
	embeddedType := getEmbeddedStructType(field.Type)
	if embeddedType == nil {
		return false
	}

	// For embedded structs (and pointer to structs), add to queue for further traversal
	if !(*seen)[embeddedType] {
		*queue = append(*queue, queueEntry{embeddedType, fieldIndex, depth + 1})
		(*seen)[embeddedType] = true
	}

	// If embedded struct has no explicit tag, don't add it as a named field
	return !hasTag
}

// validateTag performs basic validation of maxminddb struct tags.
func validateTag(field reflect.StructField, tag string) error {
	if tag == "" || tag == "-" {
		return nil
	}

	// Check for invalid UTF-8
	if !utf8.ValidString(tag) {
		return fmt.Errorf("field %s has tag with invalid UTF-8: %q", field.Name, tag)
	}

	// Only flag very obvious mistakes - don't be too restrictive
	return nil
}

var fieldsMap sync.Map

func cachedFields(result reflect.Value) *fieldsType {
	resultType := result.Type()

	if fields, ok := fieldsMap.Load(resultType); ok {
		return fields.(*fieldsType)
	}

	fields := makeStructFields(resultType)
	fieldsMap.Store(resultType, fields)

	return fields
}

// makeStructFields implements json/v2 style field precedence rules.
func makeStructFields(rootType reflect.Type) *fieldsType {
	// Breadth-first traversal to collect all fields with depth information

	queue := []queueEntry{{rootType, nil, 0}}
	var allFields []fieldInfo
	seen := make(map[reflect.Type]bool)
	seen[rootType] = true

	// Collect all reachable fields using breadth-first search
	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		for i := range entry.typ.NumField() {
			field := entry.typ.Field(i)

			// Skip unexported fields (except embedded structs)
			if !field.IsExported() && (!field.Anonymous || field.Type.Kind() != reflect.Struct) {
				continue
			}

			// Build field index path
			fieldIndex := make([]int, len(entry.index)+1)
			copy(fieldIndex, entry.index)
			fieldIndex[len(entry.index)] = i

			// Parse maxminddb tag
			fieldName := field.Name
			hasTag := false
			if tag := field.Tag.Get("maxminddb"); tag != "" {
				// Validate tag syntax
				if err := validateTag(field, tag); err != nil {
					// Log warning but continue processing
					// In a real implementation, you might want to use a proper logger
					_ = err // For now, just ignore validation errors
				}

				if tag == "-" {
					continue // Skip ignored fields
				}
				fieldName = tag
				hasTag = true
			}

			// Handle embedded structs and embedded pointers to structs
			if field.Anonymous && handleEmbeddedField(
				field, hasTag, &queue, &seen, fieldIndex, entry.depth,
			) {
				continue
			}

			// Add field to collection
			allFields = append(allFields, fieldInfo{
				index:  fieldIndex, // Will be reindexed later for optimization
				name:   fieldName,
				hasTag: hasTag,
				depth:  entry.depth,
			})
		}
	}

	// Apply precedence rules to resolve field conflicts
	// Pre-size the map based on field count for better memory efficiency
	namedFields := make(map[string]*fieldInfo, len(allFields))
	fieldsByName := make(map[string][]fieldInfo, len(allFields))

	// Group fields by name
	for _, field := range allFields {
		fieldsByName[field.name] = append(fieldsByName[field.name], field)
	}

	// Apply precedence rules for each field name
	// Store results in a flattened slice to allow pointer references
	flatFields := make([]fieldInfo, 0, len(fieldsByName))

	for name, fields := range fieldsByName {
		if len(fields) == 1 {
			// No conflict, use the field
			flatFields = append(flatFields, fields[0])
			namedFields[name] = &flatFields[len(flatFields)-1]
			continue
		}

		// Find the dominant field using json/v2 precedence rules:
		// 1. Shallowest depth wins
		// 2. Among same depth, explicitly tagged field wins
		// 3. Among same depth with same tag status, first declared wins

		dominant := fields[0]
		for i := 1; i < len(fields); i++ {
			candidate := fields[i]

			// Shallowest depth wins
			if candidate.depth < dominant.depth {
				dominant = candidate
				continue
			}
			if candidate.depth > dominant.depth {
				continue
			}

			// Same depth: explicitly tagged field wins
			if candidate.hasTag && !dominant.hasTag {
				dominant = candidate
				continue
			}
			if !candidate.hasTag && dominant.hasTag {
				continue
			}

			// Same depth and tag status: first declared wins (keep current dominant)
		}

		flatFields = append(flatFields, dominant)
		namedFields[name] = &flatFields[len(flatFields)-1]
	}

	fields := &fieldsType{
		namedFields: namedFields,
	}

	// Reindex all fields for optimized access
	fields.reindex()

	return fields
}

// reindex optimizes field indices to avoid bounds checks during runtime.
// This follows the json/v2 pattern of splitting the first index from the remainder.
func (fs *fieldsType) reindex() {
	for _, field := range fs.namedFields {
		if len(field.index) > 0 {
			field.index0 = field.index[0]
			field.index = field.index[1:]
			if len(field.index) == 0 {
				field.index = nil // avoid pinning the backing slice
			}
		}
	}
}

// addressableValue wraps a reflect.Value to optimize field access and
// embedded pointer handling. Based on encoding/json/v2 patterns.
type addressableValue struct {
	reflect.Value

	forcedAddr bool
}

// newAddressableValue creates an addressable value wrapper.
func newAddressableValue(v reflect.Value) addressableValue {
	return addressableValue{Value: v, forcedAddr: false}
}

// fieldByIndex efficiently accesses a field by its index path,
// initializing embedded pointers as needed.
func (av addressableValue) fieldByIndex(
	index0 int,
	remainingIndex []int,
	mayAlloc bool,
) addressableValue {
	// First field access (optimized with no bounds check)
	av = addressableValue{av.Field(index0), av.forcedAddr}

	// Handle remaining indices if any
	if len(remainingIndex) > 0 {
		for _, i := range remainingIndex {
			av = av.indirect(mayAlloc)
			if !av.IsValid() {
				return av
			}
			av = addressableValue{av.Field(i), av.forcedAddr}
		}
	}

	return av
}

// indirect handles pointer dereferencing and initialization.
func (av addressableValue) indirect(mayAlloc bool) addressableValue {
	if av.Kind() == reflect.Ptr {
		if av.IsNil() {
			if !mayAlloc || !av.CanSet() {
				return addressableValue{} // Return invalid value
			}
			av.Set(reflect.New(av.Type().Elem()))
		}
		av = addressableValue{av.Elem(), false}
	}
	return av
}
