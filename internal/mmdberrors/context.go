package mmdberrors

import (
	"errors"
	"fmt"
	"strconv"
)

// ContextualError provides detailed error context with offset and path information.
// This is only allocated when an error actually occurs, ensuring zero allocation
// on the happy path.
type ContextualError struct {
	Err    error
	Path   string
	Offset uint
}

func (e ContextualError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("at offset %d, path %s: %v", e.Offset, e.Path, e.Err)
	}
	return fmt.Sprintf("at offset %d: %v", e.Offset, e.Err)
}

func (e ContextualError) Unwrap() error {
	return e.Err
}

// WrapWithContext wraps an error with offset context.
// This function is designed to have zero allocation on the happy path -
// it only allocates when an error actually occurs.
func WrapWithContext(err error, offset uint) error {
	if err == nil {
		return nil
	}
	return ContextualError{
		Offset: offset,
		Err:    err,
	}
}

// PrependPath prepends a JSON-pointer-like segment to an error's path context.
func PrependPath(err error, segment string) error {
	if err == nil {
		return nil
	}
	prefix := "/" + segment
	if contextErr, ok := errors.AsType[ContextualError](err); ok {
		contextErr.Path = prefix + contextErr.Path
		return contextErr
	}
	return ContextualError{
		Err:  err,
		Path: prefix,
	}
}

// PrependIndex prepends a slice index segment to an error's path context.
func PrependIndex(err error, index int) error {
	if err == nil {
		return nil
	}
	return PrependPath(err, strconv.Itoa(index))
}
