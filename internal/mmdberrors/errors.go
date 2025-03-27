package mmdberrors

import (
	"fmt"
	"reflect"
)

// InvalidDatabaseError is returned when the database contains invalid data
// and cannot be parsed.
type InvalidDatabaseError struct {
	message string
}

func NewOffsetError() InvalidDatabaseError {
	return InvalidDatabaseError{"unexpected end of database"}
}

func NewInvalidDatabaseError(format string, args ...any) InvalidDatabaseError {
	return InvalidDatabaseError{fmt.Sprintf(format, args...)}
}

func (e InvalidDatabaseError) Error() string {
	return e.message
}

type CacheTypeError struct {
	Type  string
	Value any
}

func NewCacheTypeStrError(value any, expType string) CacheTypeError {
	return CacheTypeError{
		Type:  expType,
		Value: value,
	}
}

func (e CacheTypeError) Error() string {
	return fmt.Sprintf("maxminddb: expected %s type in cache but found %T", e.Type, e.Value)
}

// UnmarshalTypeError is returned when the value in the database cannot be
// assigned to the specified data type.
type UnmarshalTypeError struct {
	Type  reflect.Type
	Value string
}

func NewUnmarshalTypeStrError(value string, rType reflect.Type) UnmarshalTypeError {
	return UnmarshalTypeError{
		Type:  rType,
		Value: value,
	}
}

func NewUnmarshalTypeError(value any, rType reflect.Type) UnmarshalTypeError {
	return NewUnmarshalTypeStrError(fmt.Sprintf("%v (%T)", value, value), rType)
}

func (e UnmarshalTypeError) Error() string {
	return fmt.Sprintf("maxminddb: cannot unmarshal %s into type %s", e.Value, e.Type)
}
