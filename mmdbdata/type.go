// Package mmdbdata provides types and interfaces for working with MaxMind DB data.
package mmdbdata

import "github.com/oschwald/maxminddb-golang/v2/internal/decoder"

// Kind represents MMDB data kinds.
type Kind = decoder.Kind

// Decoder provides methods for decoding MMDB data.
type Decoder = decoder.Decoder

// Kind constants for MMDB data.
const (
	KindExtended  = decoder.KindExtended
	KindPointer   = decoder.KindPointer
	KindString    = decoder.KindString
	KindFloat64   = decoder.KindFloat64
	KindBytes     = decoder.KindBytes
	KindUint16    = decoder.KindUint16
	KindUint32    = decoder.KindUint32
	KindMap       = decoder.KindMap
	KindInt32     = decoder.KindInt32
	KindUint64    = decoder.KindUint64
	KindUint128   = decoder.KindUint128
	KindSlice     = decoder.KindSlice
	KindContainer = decoder.KindContainer
	KindEndMarker = decoder.KindEndMarker
	KindBool      = decoder.KindBool
	KindFloat32   = decoder.KindFloat32
)
