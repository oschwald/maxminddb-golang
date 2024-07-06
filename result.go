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

// DecodePath unmarshals a value from data section into v, following the
// specified path.
//
// The v parameter should be a pointer to the value where the decoded data
// will be stored. If v is nil or not a pointer, an error is returned. If the
// data in the database record cannot be stored in v because of type
// differences, an UnmarshalTypeError is returned.
//
// The path is a variadic list of keys (strings) and/or indices (ints) that
// describe the nested structure to traverse in the data to reach the desired
// value.
//
// For maps, string path elements are used as keys.
// For arrays, int path elements are used as indices.
//
// If the path is empty, the entire data structure is decoded into v.
//
// Returns an error if:
//   - the path is invalid
//   - the data cannot be decoded into the type of v
//   - v is not a pointer or the database record cannot be stored in v due to
//     type mismatch
//   - the Result does not contain valid data
//
// Example usage:
//
//	var city string
//	err := result.DecodePath(&city, "location", "city", "names", "en")
//
//	var geonameID int
//	err := result.DecodePath(&geonameID, "subdivisions", 0, "geoname_id")
func (r Result) DecodePath(v any, path ...any) error {
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
	return r.decoder.decodePath(r.offset, path, rv)
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
