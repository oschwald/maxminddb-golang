// Package decoder provides low-level decoding utilities for MaxMind DB data.
package decoder

import (
	"fmt"
	"iter"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

// Decoder allows decoding of a single value stored at a specific offset
// in the database.
type Decoder struct {
	d             DataDecoder
	offset        uint
	nextOffset    uint
	hasNextOffset bool
}

type decoderOptions struct {
	// Intentionally empty for now. DecoderOption callbacks are still invoked so
	// adding options in a future release is non-breaking.
}

// DecoderOption configures a Decoder.
//
//nolint:revive // name follows existing library pattern (ReaderOption, NetworksOption)
type DecoderOption func(*decoderOptions)

// NewDecoder creates a new Decoder with the given DataDecoder, offset, and options.
func NewDecoder(d DataDecoder, offset uint, options ...DecoderOption) *Decoder {
	opts := decoderOptions{}
	for _, option := range options {
		option(&opts)
	}

	return &Decoder{
		d:      d,
		offset: offset,
	}
}

// ReadBool reads the value pointed by the decoder as a bool.
//
// Returns an error if the database is malformed or if the pointed value is not a bool.
func (d *Decoder) ReadBool() (bool, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindBool)
	if err != nil {
		return false, d.wrapError(err)
	}

	value, newOffset, err := d.d.decodeBool(size, offset)
	if err != nil {
		return false, d.wrapError(err)
	}
	d.setNextOffset(newOffset)
	return value, nil
}

// ReadString reads the value pointed by the decoder as a string.
//
// Returns an error if the database is malformed or if the pointed value is not a string.
func (d *Decoder) ReadString() (string, error) {
	value, nextOffset, err := d.d.decodeStringValue(d.offset)
	if err != nil {
		return "", d.wrapError(err)
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadBytes reads the value pointed by the decoder as bytes.
//
// Returns an error if the database is malformed or if the pointed value is not bytes.
func (d *Decoder) ReadBytes() ([]byte, error) {
	val, err := d.readBytes(KindBytes)
	if err != nil {
		return nil, d.wrapError(err)
	}
	return val, nil
}

// ReadFloat32 reads the value pointed by the decoder as a float32.
//
// Returns an error if the database is malformed or if the pointed value is not a float.
func (d *Decoder) ReadFloat32() (float32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindFloat32)
	if err != nil {
		return 0, d.wrapError(err)
	}

	value, nextOffset, err := d.d.decodeFloat32(size, offset)
	if err != nil {
		return 0, d.wrapError(err)
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadFloat64 reads the value pointed by the decoder as a float64.
//
// Returns an error if the database is malformed or if the pointed value is not a double.
func (d *Decoder) ReadFloat64() (float64, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindFloat64)
	if err != nil {
		return 0, d.wrapError(err)
	}

	value, nextOffset, err := d.d.decodeFloat64(size, offset)
	if err != nil {
		return 0, d.wrapError(err)
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadInt32 reads the value pointed by the decoder as an int32.
//
// Returns an error if the database is malformed or if the pointed value is not an int32.
func (d *Decoder) ReadInt32() (int32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindInt32)
	if err != nil {
		return 0, d.wrapError(err)
	}

	value, nextOffset, err := d.d.decodeInt32(size, offset)
	if err != nil {
		return 0, d.wrapError(err)
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadUint16 reads the value pointed by the decoder as a uint16.
//
// Returns an error if the database is malformed or if the pointed value is not a uint16.
func (d *Decoder) ReadUint16() (uint16, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindUint16)
	if err != nil {
		return 0, d.wrapError(err)
	}

	value, nextOffset, err := d.d.decodeUint16(size, offset)
	if err != nil {
		return 0, d.wrapError(err)
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadUint32 reads the value pointed by the decoder as a uint32.
//
// Returns an error if the database is malformed or if the pointed value is not a uint32.
func (d *Decoder) ReadUint32() (uint32, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindUint32)
	if err != nil {
		return 0, d.wrapError(err)
	}

	value, nextOffset, err := d.d.decodeUint32(size, offset)
	if err != nil {
		return 0, d.wrapError(err)
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadUint64 reads the value pointed by the decoder as a uint64.
//
// Returns an error if the database is malformed or if the pointed value is not a uint64.
func (d *Decoder) ReadUint64() (uint64, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindUint64)
	if err != nil {
		return 0, d.wrapError(err)
	}

	value, nextOffset, err := d.d.decodeUint64(size, offset)
	if err != nil {
		return 0, d.wrapError(err)
	}

	d.setNextOffset(nextOffset)
	return value, nil
}

// ReadUint128 reads the value pointed by the decoder as a uint128.
//
// Returns an error if the database is malformed or if the pointed value is not a uint128.
func (d *Decoder) ReadUint128() (hi, lo uint64, err error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindUint128)
	if err != nil {
		return 0, 0, d.wrapError(err)
	}

	hi, lo, nextOffset, err := d.d.decodeUint128(size, offset)
	if err != nil {
		return 0, 0, d.wrapError(err)
	}

	d.setNextOffset(nextOffset)
	return hi, lo, nil
}

// ReadMap returns an iterator to read the map along with the map size. The
// size can be used to pre-allocate a map with the correct capacity for better
// performance. The first value from the iterator is the key. Please note that
// this byte slice is only valid during the iteration. This is done to avoid
// an unnecessary allocation. You must make a copy of it if you are storing it
// for later use. The second value is an error indicating that the database is
// malformed or that the pointed value is not a map.
func (d *Decoder) ReadMap() (iter.Seq2[[]byte, error], uint, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindMap)
	if err != nil {
		return nil, 0, d.wrapError(err)
	}

	iterator := func(yield func([]byte, error) bool) {
		currentOffset := offset

		for range size {
			key, keyEndOffset, err := d.d.decodeKey(currentOffset)
			if err != nil {
				yield(nil, d.wrapErrorAtOffset(err, currentOffset))
				return
			}

			// Position decoder to read value after yielding key
			d.reset(keyEndOffset)

			ok := yield(key, nil)
			if !ok {
				return
			}

			// Skip the value to get to next key-value pair
			valueEndOffset, err := d.d.nextValueOffset(keyEndOffset, 1)
			if err != nil {
				yield(nil, d.wrapError(err))
				return
			}
			currentOffset = valueEndOffset
		}

		// Set the final offset after map iteration
		d.reset(currentOffset)
	}

	return iterator, size, nil
}

// ReadSlice returns an iterator over the values of the slice along with the
// slice size. The size can be used to pre-allocate a slice with the correct
// capacity for better performance. The iterator returns an error if the
// database is malformed or if the pointed value is not a slice.
func (d *Decoder) ReadSlice() (iter.Seq[error], uint, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(KindSlice)
	if err != nil {
		return nil, 0, d.wrapError(err)
	}

	iterator := func(yield func(error) bool) {
		currentOffset := offset

		for i := range size {
			// Position decoder to read current element
			d.reset(currentOffset)

			ok := yield(nil)
			if !ok {
				// Skip the unvisited elements
				remaining := size - i - 1
				if remaining > 0 {
					endOffset, err := d.d.nextValueOffset(currentOffset, remaining)
					if err == nil {
						d.reset(endOffset)
					}
				}
				return
			}

			// Advance to next element
			nextOffset, err := d.d.nextValueOffset(currentOffset, 1)
			if err != nil {
				yield(d.wrapError(err))
				return
			}
			currentOffset = nextOffset
		}

		// Set final offset after slice iteration
		d.reset(currentOffset)
	}

	return iterator, size, nil
}

// SkipValue skips over the current value without decoding it.
// This is useful in custom decoders when encountering unknown fields.
// The decoder will be positioned after the skipped value.
func (d *Decoder) SkipValue() error {
	// We can reuse the existing nextValueOffset logic by jumping to the next value
	nextOffset, err := d.d.nextValueOffset(d.offset, 1)
	if err != nil {
		return d.wrapError(err)
	}
	d.reset(nextOffset)
	return nil
}

// PeekKind returns the kind of the current value without consuming it.
// This allows for look-ahead parsing similar to jsontext.Decoder.PeekKind().
func (d *Decoder) PeekKind() (Kind, error) {
	kindNum, _, _, err := d.d.decodeCtrlData(d.offset)
	if err != nil {
		return 0, d.wrapError(err)
	}

	// Follow pointers to get the actual kind
	if kindNum == KindPointer {
		// We need to follow the pointer to get the real kind
		dataOffset := d.offset
		for {
			var size uint
			kindNum, size, dataOffset, err = d.d.decodeCtrlData(dataOffset)
			if err != nil {
				return 0, d.wrapError(err)
			}
			if kindNum != KindPointer {
				break
			}
			dataOffset, _, err = d.d.decodePointer(size, dataOffset)
			if err != nil {
				return 0, d.wrapError(err)
			}
		}
	}

	return kindNum, nil
}

// Offset returns the current offset position in the database.
// If the current position points to a pointer, this method resolves the
// pointer chain and returns the offset of the actual data. This ensures
// that multiple pointers to the same data return the same offset, which
// is important for caching purposes.
func (d *Decoder) Offset() uint {
	// Follow pointer chain to get resolved data location
	dataOffset := d.offset
	for {
		kindNum, size, ctrlEndOffset, err := d.d.decodeCtrlData(dataOffset)
		if err != nil {
			// Return original offset to avoid breaking the public API.
			// Offset() returns uint (not (uint, error)), so we can't propagate errors.
			// In practice, errors here are rare and the original offset is still valid.
			return d.offset
		}
		if kindNum != KindPointer {
			// dataOffset is now pointing at the actual data (not a pointer)
			// Return this offset, which is where the data's control bytes start
			break
		}
		// Follow the pointer to get the target offset
		dataOffset, _, err = d.d.decodePointer(size, ctrlEndOffset)
		if err != nil {
			// Return original offset to avoid breaking the public API.
			// The caller will encounter the same error when they try to read.
			return d.offset
		}
		// dataOffset is now the pointer target; loop to check if it's also a pointer
	}
	return dataOffset
}

// IsPointerAt reports whether the value at the provided offset is a pointer
// in the original container stream.
func (d *Decoder) IsPointerAt(offset uint) (bool, error) {
	kindNum, _, _, err := d.d.decodeCtrlData(offset)
	if err != nil {
		return false, d.wrapError(err)
	}
	return kindNum == KindPointer, nil
}

// ReadStringAt reads a string value at the provided offset and returns the
// decoded value and end offset for the value in the original container stream.
func (d *Decoder) ReadStringAt(offset uint) (string, uint, error) {
	value, nextOffset, err := d.d.decodeStringValue(offset)
	if err != nil {
		return "", 0, d.wrapError(err)
	}
	return value, nextOffset, nil
}

// ReadBoolAt reads a bool value at the provided offset.
func (d *Decoder) ReadBoolAt(offset uint) (bool, uint, error) {
	size, dataOffset, nextOffset, err := d.decodeCtrlDataAndFollowAt(offset, KindBool)
	if err != nil {
		return false, 0, err
	}
	value, _, err := d.d.decodeBool(size, dataOffset)
	if err != nil {
		return false, 0, d.wrapError(err)
	}
	return value, nextOffset, nil
}

// ReadUint16At reads a uint16 value at the provided offset.
func (d *Decoder) ReadUint16At(offset uint) (uint16, uint, error) {
	size, dataOffset, nextOffset, err := d.decodeCtrlDataAndFollowAt(offset, KindUint16)
	if err != nil {
		return 0, 0, err
	}
	value, _, err := d.d.decodeUint16(size, dataOffset)
	if err != nil {
		return 0, 0, d.wrapError(err)
	}
	return value, nextOffset, nil
}

// ReadUint32At reads a uint32 value at the provided offset.
func (d *Decoder) ReadUint32At(offset uint) (uint32, uint, error) {
	size, dataOffset, nextOffset, err := d.decodeCtrlDataAndFollowAt(offset, KindUint32)
	if err != nil {
		return 0, 0, err
	}
	value, _, err := d.d.decodeUint32(size, dataOffset)
	if err != nil {
		return 0, 0, d.wrapError(err)
	}
	return value, nextOffset, nil
}

// ReadUint64At reads a uint64 value at the provided offset.
func (d *Decoder) ReadUint64At(offset uint) (uint64, uint, error) {
	size, dataOffset, nextOffset, err := d.decodeCtrlDataAndFollowAt(offset, KindUint64)
	if err != nil {
		return 0, 0, err
	}
	value, _, err := d.d.decodeUint64(size, dataOffset)
	if err != nil {
		return 0, 0, d.wrapError(err)
	}
	return value, nextOffset, nil
}

// ReadFloat64At reads a float64 value at the provided offset.
func (d *Decoder) ReadFloat64At(offset uint) (float64, uint, error) {
	size, dataOffset, nextOffset, err := d.decodeCtrlDataAndFollowAt(offset, KindFloat64)
	if err != nil {
		return 0, 0, err
	}
	value, _, err := d.d.decodeFloat64(size, dataOffset)
	if err != nil {
		return 0, 0, d.wrapError(err)
	}
	return value, nextOffset, nil
}

// ReadUintAt reads an unsigned integer value (uint16/uint32/uint64) at the
// provided offset in a single pass and returns it as uint64.
func (d *Decoder) ReadUintAt(offset uint) (uint64, uint, error) {
	dataOffset := offset
	hasNextOffset := false
	var nextOffset uint

	for {
		kindNum, size, ctrlEndOffset, err := d.d.decodeCtrlData(dataOffset)
		if err != nil {
			return 0, 0, d.wrapError(err)
		}

		if kindNum == KindPointer {
			dataOffset, nextOffset, err = d.d.decodePointer(size, ctrlEndOffset)
			if err != nil {
				return 0, 0, d.wrapError(err)
			}
			hasNextOffset = true
			continue
		}

		if !hasNextOffset {
			nextOffset = endOffsetForValue(kindNum, size, ctrlEndOffset)
		}

		switch kindNum {
		case KindUint16:
			v, _, err := d.d.decodeUint16(size, ctrlEndOffset)
			if err != nil {
				return 0, 0, d.wrapError(err)
			}
			return uint64(v), nextOffset, nil
		case KindUint32:
			v, _, err := d.d.decodeUint32(size, ctrlEndOffset)
			if err != nil {
				return 0, 0, d.wrapError(err)
			}
			return uint64(v), nextOffset, nil
		case KindUint64:
			v, _, err := d.d.decodeUint64(size, ctrlEndOffset)
			if err != nil {
				return 0, 0, d.wrapError(err)
			}
			return v, nextOffset, nil
		default:
			return 0, 0, d.wrapError(
				mmdberrors.NewInvalidDatabaseError(
					"unexpected uint kind at offset %d: %s",
					offset,
					kindNum.String(),
				),
			)
		}
	}
}

// ReadMapHeaderAt reads a map header at the given offset and returns the map
// size and the offset of the first key.
func (d *Decoder) ReadMapHeaderAt(offset uint) (uint, uint, error) {
	size, dataOffset, _, err := d.decodeCtrlDataAndFollowAt(offset, KindMap)
	if err != nil {
		return 0, 0, err
	}
	return size, dataOffset, nil
}

// ReadMapEntryStringValueOffsetAt reads one map entry key at keyOffset and
// returns the decoded key string and value offset.
func (d *Decoder) ReadMapEntryStringValueOffsetAt(
	keyOffset uint,
) (string, uint, error) {
	key, valueOffset, err := d.d.decodeStringValue(keyOffset)
	if err != nil {
		return "", 0, d.wrapErrorAtOffset(err, keyOffset)
	}
	return key, valueOffset, nil
}

// ReadMapEntryKeyValueOffsetAt reads one map entry key at keyOffset and
// returns the key bytes (pointing into decoder buffer) and value offset.
func (d *Decoder) ReadMapEntryKeyValueOffsetAt(
	keyOffset uint,
) ([]byte, uint, error) {
	key, valueOffset, err := d.d.decodeKey(keyOffset)
	if err != nil {
		return nil, 0, d.wrapErrorAtOffset(err, keyOffset)
	}
	return key, valueOffset, nil
}

// ReadSliceHeaderAt reads a slice header at the given offset and returns the
// slice length and the offset of the first element.
func (d *Decoder) ReadSliceHeaderAt(offset uint) (uint, uint, error) {
	size, dataOffset, _, err := d.decodeCtrlDataAndFollowAt(offset, KindSlice)
	if err != nil {
		return 0, 0, err
	}
	return size, dataOffset, nil
}

// NextValueOffsetAt returns the offset immediately after the value at offset.
func (d *Decoder) NextValueOffsetAt(offset uint) (uint, error) {
	nextOffset, err := d.d.nextValueOffset(offset, 1)
	if err != nil {
		return 0, d.wrapError(err)
	}
	return nextOffset, nil
}

func (d *Decoder) reset(offset uint) {
	d.offset = offset
	d.hasNextOffset = false
	d.nextOffset = 0
}

func (d *Decoder) setNextOffset(offset uint) {
	if !d.hasNextOffset {
		d.hasNextOffset = true
		d.nextOffset = offset
	}
}

func unexpectedKindErr(expectedKind, actualKind Kind) error {
	return fmt.Errorf("unexpected kind %d, expected %d", actualKind, expectedKind)
}

func (d *Decoder) decodeCtrlDataAndFollow(expectedKind Kind) (uint, uint, error) {
	dataOffset := d.offset
	for {
		var kindNum Kind
		var size uint
		var err error
		kindNum, size, dataOffset, err = d.d.decodeCtrlData(dataOffset)
		if err != nil {
			return 0, 0, err // Don't wrap here, let caller wrap
		}

		if kindNum == KindPointer {
			var nextOffset uint
			dataOffset, nextOffset, err = d.d.decodePointer(size, dataOffset)
			if err != nil {
				return 0, 0, err // Don't wrap here, let caller wrap
			}
			d.setNextOffset(nextOffset)
			continue
		}

		if kindNum != expectedKind {
			return 0, 0, unexpectedKindErr(expectedKind, kindNum)
		}

		return size, dataOffset, nil
	}
}

func (d *Decoder) readBytes(kind Kind) ([]byte, error) {
	size, offset, err := d.decodeCtrlDataAndFollow(kind)
	if err != nil {
		return nil, err // Return unwrapped - caller will wrap
	}

	if offset+size > uint(len(d.d.getBuffer())) {
		return nil, mmdberrors.NewInvalidDatabaseError(
			"the MaxMind DB file's data section contains bad data (offset+size %d exceeds buffer length %d)",
			offset+size,
			len(d.d.getBuffer()),
		)
	}
	d.setNextOffset(offset + size)
	return d.d.getBuffer()[offset : offset+size], nil
}

func endOffsetForValue(kind Kind, size, dataOffset uint) uint {
	switch kind {
	case KindBool, KindMap, KindSlice:
		return dataOffset
	default:
		return dataOffset + size
	}
}

func (d *Decoder) decodeCtrlDataAndFollowAt(
	offset uint,
	expectedKind Kind,
) (uint, uint, uint, error) {
	dataOffset := offset
	hasNextOffset := false
	var nextOffset uint

	for {
		kindNum, size, ctrlEndOffset, err := d.d.decodeCtrlData(dataOffset)
		if err != nil {
			return 0, 0, 0, d.wrapError(err)
		}

		if kindNum == KindPointer {
			var err error
			dataOffset, nextOffset, err = d.d.decodePointer(size, ctrlEndOffset)
			if err != nil {
				return 0, 0, 0, d.wrapError(err)
			}
			if !hasNextOffset {
				hasNextOffset = true
			}
			continue
		}

		if kindNum != expectedKind {
			return 0, 0, 0, d.wrapError(unexpectedKindErr(expectedKind, kindNum))
		}

		if !hasNextOffset {
			nextOffset = endOffsetForValue(kindNum, size, ctrlEndOffset)
		}

		return size, ctrlEndOffset, nextOffset, nil
	}
}
