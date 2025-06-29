// Package mmdbdata provides types and interfaces for working with MaxMind DB data.
package mmdbdata

import "github.com/oschwald/maxminddb-golang/v2/internal/decoder"

// Type represents MMDB data types.
type Type = decoder.Type

// Decoder provides methods for decoding MMDB data.
type Decoder = decoder.Decoder

// Type constants for MMDB data.
const (
	TypeExtended  = decoder.TypeExtended
	TypePointer   = decoder.TypePointer
	TypeString    = decoder.TypeString
	TypeFloat64   = decoder.TypeFloat64
	TypeBytes     = decoder.TypeBytes
	TypeUint16    = decoder.TypeUint16
	TypeUint32    = decoder.TypeUint32
	TypeMap       = decoder.TypeMap
	TypeInt32     = decoder.TypeInt32
	TypeUint64    = decoder.TypeUint64
	TypeUint128   = decoder.TypeUint128
	TypeSlice     = decoder.TypeSlice
	TypeContainer = decoder.TypeContainer
	TypeEndMarker = decoder.TypeEndMarker
	TypeBool      = decoder.TypeBool
	TypeFloat32   = decoder.TypeFloat32
)
