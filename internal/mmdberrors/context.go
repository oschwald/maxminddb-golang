package mmdberrors

import (
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
		return fmt.Sprintf("at offset %d, %v", e.Offset, e.WithoutOffset())
	}
	return fmt.Sprintf("at offset %d: %v", e.Offset, e.Err)
}

// WithoutOffset returns the error with its path and without its offset. Use
// it in a message that states the offset itself.
func (e ContextualError) WithoutOffset() error {
	if e.Path == "" {
		return e.Err
	}
	return fmt.Errorf("path %s: %w", e.Path, e.Err)
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
	// A wrapper that a custom unmarshaler adds above a ContextualError stays
	// in the chain, so only a direct ContextualError is extended in place.
	if contextErr, ok := err.(ContextualError); ok { //nolint:errorlint // Preserve wrappers.
		contextErr.Path = "/" + segment + contextErr.Path
		return contextErr
	}
	return ContextualError{
		Err:  err,
		Path: "/" + segment,
	}
}

// PrependIndex prepends a slice index segment to an error's path context.
func PrependIndex(err error, index int) error {
	if err == nil {
		return nil
	}
	return PrependPath(err, strconv.Itoa(index))
}
