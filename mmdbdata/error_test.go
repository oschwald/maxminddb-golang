package mmdbdata

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

func TestNormalizeUnmarshalErrorPreservesDirectOffset(t *testing.T) {
	decoder := NewDecoder([]byte{0x41, 'x'}, 0)
	_, _, err := decoder.Cursor().ReadBool()
	require.Error(t, err)

	normalized := NormalizeUnmarshalError[bool](err)
	var contextError mmdberrors.ContextualError
	require.ErrorAs(t, normalized, &contextError)
	require.Zero(t, contextError.Offset)
	var typeError mmdberrors.UnmarshalTypeError
	require.ErrorAs(t, normalized, &typeError)
	require.Equal(t, reflect.TypeFor[bool](), typeError.Type)
}

func TestNormalizeUnmarshalErrorLeavesNestedMismatchAlone(t *testing.T) {
	decoder := NewDecoder([]byte{0x41, 'x'}, 0)
	_, _, err := decoder.Cursor().ReadBool()
	require.Error(t, err)
	custom := fmt.Errorf("custom unmarshaler: %w", err)

	normalized := NormalizeUnmarshalError[string](custom)
	require.Equal(t, custom, normalized)
	var mismatch UnexpectedKindError
	require.ErrorAs(t, normalized, &mismatch)
	var typeError mmdberrors.UnmarshalTypeError
	require.NotErrorAs(t, normalized, &typeError)
}

func TestNewUnmarshalTypeError(t *testing.T) {
	err := NewUnmarshalTypeError[uint8](uint64(256))
	var typeError mmdberrors.UnmarshalTypeError
	require.ErrorAs(t, err, &typeError)
	require.Equal(t, reflect.TypeFor[uint8](), typeError.Type)
}
