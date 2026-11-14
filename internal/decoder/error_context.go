package decoder

import "github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"

// wrapError wraps an error with context information when an error occurs.
// Zero allocation on happy path - only allocates when error != nil.
func (d *Decoder) wrapError(err error) error {
	return wrapErrorAtOffset(err, d.offset)
}

func wrapErrorAtOffset(err error, offset uint) error {
	return mmdberrors.WrapWithContext(err, offset)
}
