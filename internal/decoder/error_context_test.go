package decoder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

func TestWrapError_ZeroAllocationHappyPath(t *testing.T) {
	buffer := []byte{0x44, 't', 'e', 's', 't'} // String "test"
	dd := NewDataDecoder(buffer)
	decoder := NewDecoder(dd, 0)

	// Test that no error wrapping has zero allocation
	err := decoder.wrapError(nil)
	require.NoError(t, err)

	// DataDecoder should always have path tracking enabled
	require.NotNil(t, decoder.d)
}

func TestWrapError_ContextWhenError(t *testing.T) {
	buffer := []byte{0x44, 't', 'e', 's', 't'} // String "test"
	dd := NewDataDecoder(buffer)
	decoder := NewDecoder(dd, 0)

	// Simulate an error with context
	originalErr := mmdberrors.NewInvalidDatabaseError("test error")
	wrappedErr := decoder.wrapError(originalErr)

	require.Error(t, wrappedErr)

	// Should be a ContextualError
	var contextErr mmdberrors.ContextualError
	require.ErrorAs(t, wrappedErr, &contextErr)

	// Should have offset information
	require.Equal(t, uint(0), contextErr.Offset)
	require.Equal(t, originalErr, contextErr.Err)
}

func TestPathBuilder(t *testing.T) {
	builder := mmdberrors.NewPathBuilder()

	// Test basic path building
	require.Equal(t, "/", builder.Build())

	builder.PushMap("city")
	require.Equal(t, "/city", builder.Build())

	builder.PushMap("names")
	require.Equal(t, "/city/names", builder.Build())

	builder.PushSlice(0)
	require.Equal(t, "/city/names/0", builder.Build())

	// Test pop
	builder.Pop()
	require.Equal(t, "/city/names", builder.Build())

	// Test reset
	builder.Reset()
	require.Equal(t, "/", builder.Build())
}

// Benchmark to verify zero allocation on happy path.
func BenchmarkWrapError_HappyPath(b *testing.B) {
	buffer := []byte{0x44, 't', 'e', 's', 't'} // String "test"
	dd := NewDataDecoder(buffer)
	decoder := NewDecoder(dd, 0)

	b.ReportAllocs()

	for b.Loop() {
		err := decoder.wrapError(nil)
		if err != nil {
			b.Fatal("unexpected error")
		}
	}
}

// Benchmark to show allocation only occurs on error path.
func BenchmarkWrapError_ErrorPath(b *testing.B) {
	buffer := []byte{0x44, 't', 'e', 's', 't'} // String "test"
	dd := NewDataDecoder(buffer)
	decoder := NewDecoder(dd, 0)

	originalErr := mmdberrors.NewInvalidDatabaseError("test error")

	b.ReportAllocs()

	for b.Loop() {
		err := decoder.wrapError(originalErr)
		if err == nil {
			b.Fatal("expected error")
		}
	}
}

// Example showing the API in action.
func ExampleContextualError() {
	// This would be internal to the decoder, shown for illustration
	builder := mmdberrors.NewPathBuilder()
	builder.PushMap("city")
	builder.PushMap("names")
	builder.PushMap("en")

	// Simulate an error with context
	originalErr := mmdberrors.NewInvalidDatabaseError("string too long")

	contextTracker := &errorContext{path: builder}
	wrappedErr := mmdberrors.WrapWithContext(originalErr, 1234, contextTracker)

	fmt.Println(wrappedErr.Error())
	// Output: at offset 1234, path /city/names/en: string too long
}

func TestContextualErrorIntegration(t *testing.T) {
	t.Run("InvalidStringLength", func(t *testing.T) {
		// String claims size 4 but buffer only has 3 bytes total
		buffer := []byte{0x44, 't', 'e', 's'}

		// Test ReflectionDecoder
		rd := New(buffer)
		var result string
		err := rd.Decode(0, &result)
		require.Error(t, err)

		var contextErr mmdberrors.ContextualError
		require.ErrorAs(t, err, &contextErr)
		require.Equal(t, uint(0), contextErr.Offset)
		require.Contains(t, contextErr.Error(), "offset 0")

		// Test new Decoder API
		dd := NewDataDecoder(buffer)
		decoder := NewDecoder(dd, 0)
		_, err = decoder.ReadString()
		require.Error(t, err)

		require.ErrorAs(t, err, &contextErr)
		require.Equal(t, uint(0), contextErr.Offset)
		require.Contains(t, contextErr.Error(), "offset 0")
	})

	t.Run("NestedMapWithPath", func(t *testing.T) {
		// Map with nested structure that has an error deep inside
		// Map { "key": invalid_string }
		buffer := []byte{
			0xe1,                // Map with 1 item
			0x43, 'k', 'e', 'y', // Key "key" (3 bytes)
			0x44, 't', 'e', // Invalid string (claims size 4, only has 2 bytes)
		}

		// Test ReflectionDecoder with map decoding
		rd := New(buffer)
		var result map[string]string
		err := rd.Decode(0, &result)
		require.Error(t, err)

		// Should get a wrapped error with path information
		var contextErr mmdberrors.ContextualError
		require.ErrorAs(t, err, &contextErr)
		require.Equal(t, "/key", contextErr.Path)
		require.Contains(t, contextErr.Error(), "path /key")

		// Test new Decoder API - no automatic path tracking
		dd := NewDataDecoder(buffer)
		decoder := NewDecoder(dd, 0)
		mapIter, _, err := decoder.ReadMap()
		require.NoError(t, err, "ReadMap failed")

		var mapErr error
		for _, iterErr := range mapIter {
			if iterErr != nil {
				mapErr = iterErr
				break
			}

			// Try to read the value (this should fail)
			_, mapErr = decoder.ReadString()
			if mapErr != nil {
				break
			}
		}

		require.Error(t, mapErr)
		require.ErrorAs(t, mapErr, &contextErr)
		// New API should have offset but no path
		require.Contains(t, contextErr.Error(), "offset")
		require.Empty(t, contextErr.Path)
	})

	t.Run("SliceIndexInPath", func(t *testing.T) {
		// Create nested map-slice-map structure: { "list": [{"name": invalid_string}] }
		// This will test path like /list/0/name
		buffer := []byte{
			0xe1,                     // Map with 1 item
			0x44, 'l', 'i', 's', 't', // Key "list" (4 bytes)
			0x01, 0x04, // Array with 1 item (extended type: type=4 (slice), count=1)
			0xe1,                     // Map with 1 item (array element)
			0x44, 'n', 'a', 'm', 'e', // Key "name" (4 bytes)
			0x44, 't', 'e', // Invalid string (claims size 4, only has 2 bytes)
		}

		// Test ReflectionDecoder with slice index in path
		rd := New(buffer)
		var result map[string][]map[string]string
		err := rd.Decode(0, &result)
		require.Error(t, err)

		// Debug: print the actual error and path
		t.Logf("Error: %v", err)

		// Should get a wrapped error with slice index in path
		var contextErr mmdberrors.ContextualError
		require.ErrorAs(t, err, &contextErr)
		t.Logf("Path: %s", contextErr.Path)

		// Verify we get the exact path with correct order
		require.Equal(t, "/list/0/name", contextErr.Path)
		require.Contains(t, contextErr.Error(), "path /list/0/name")
		require.Contains(t, contextErr.Error(), "offset")

		// Test new Decoder API - manual iteration, no automatic path tracking
		dd := NewDataDecoder(buffer)
		decoder := NewDecoder(dd, 0)

		// Navigate through the nested structure manually
		mapIter, _, err := decoder.ReadMap()
		require.NoError(t, err, "ReadMap failed")
		var mapErr error

		for key, iterErr := range mapIter {
			if iterErr != nil {
				mapErr = iterErr
				break
			}
			require.Equal(t, "list", string(key))

			// Read the array
			sliceIter, _, err := decoder.ReadSlice()
			require.NoError(t, err, "ReadSlice failed")
			sliceIndex := 0
			for sliceIterErr := range sliceIter {
				if sliceIterErr != nil {
					mapErr = sliceIterErr
					break
				}
				require.Equal(t, 0, sliceIndex) // Should be first element

				// Read the nested map (array element)
				innerMapIter, _, err := decoder.ReadMap()
				require.NoError(t, err, "ReadMap failed")
				for innerKey, innerIterErr := range innerMapIter {
					if innerIterErr != nil {
						mapErr = innerIterErr
						break
					}
					require.Equal(t, "name", string(innerKey))

					// Try to read the invalid string (this should fail)
					_, mapErr = decoder.ReadString()
					if mapErr != nil {
						break
					}
				}
				if mapErr != nil {
					break
				}
				sliceIndex++
			}
			if mapErr != nil {
				break
			}
		}

		require.Error(t, mapErr)
		require.ErrorAs(t, mapErr, &contextErr)
		// New API should have offset but no path (since it's manual iteration)
		require.Contains(t, contextErr.Error(), "offset")
		require.Empty(t, contextErr.Path)
	})
}
