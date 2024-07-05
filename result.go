package maxminddb

import (
	"errors"
	"math"
	"reflect"
)

const notFound uint = math.MaxUint

type Result struct {
	err     error
	decoder decoder
	offset  uint
}

// Decode unmarshals the data from the data section into the value pointed to
// by v. If v is nil or not a pointer, an error is returned. If the data in
// the database record cannot be stored in v because of type differences, an
// UnmarshalTypeError is returned. If the database is invalid or otherwise
// cannot be read, an InvalidDatabaseError is returned.
//
// An error will also be returned if there was an error during the
// Reader.Lookup call.
//
// If the Reader.Lookup call did not find a value for the IP address, no error
// will be returned and v will be unchanged.
func (r Result) Decode(v any) error {
	if r.err != nil {
		return r.err
	}
	if r.offset == notFound {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("result param must be a pointer")
	}

	if dser, ok := v.(deserializer); ok {
		_, err := r.decoder.decodeToDeserializer(r.offset, dser, 0, false)
		return err
	}

	_, err := r.decoder.decode(r.offset, rv, 0)
	return err
}

// Err provides a way to check whether there was an error during the lookup
// without clling Result.Decode. If there was an error, it will also be
// returned from Result.Decode.
func (r Result) Err() error {
	return r.err
}

// Found will return true if the IP was found in the search tree. It will
// return false if the IP was not found or if there was an error.
func (r Result) Found() bool {
	return r.err == nil && r.offset != notFound
}
