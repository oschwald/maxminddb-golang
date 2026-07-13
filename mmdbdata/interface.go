package mmdbdata

// Unmarshaler is implemented by types that can unmarshal MaxMind DB data. The
// Decoder is valid only for the duration of UnmarshalMaxMindDB and must not be
// retained by the implementation.
type Unmarshaler interface {
	UnmarshalMaxMindDB(d *Decoder) error
}

// CursorUnmarshaler is implemented by generated and handwritten decoders that
// can unmarshal directly from a Cursor. Implementations must consume and
// validate the complete value and return its proven successor.
type CursorUnmarshaler interface {
	UnmarshalMaxMindDBCursor(cursor Cursor) (Cursor, error)
}
