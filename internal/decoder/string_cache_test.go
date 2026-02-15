package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringCacheOffsetZero(t *testing.T) {
	cache := newStringCache()
	data := []byte("hello world, this is test data")

	// Test string at offset 0
	str1 := cache.InternAt(0, 5, data)
	require.Equal(t, "hello", str1)

	// Second call should hit cache and return same interned string
	str2 := cache.InternAt(0, 5, data)
	require.Equal(t, "hello", str2)

	// Note: Both strings should be identical (cache hit)
	// We can't easily test if they're the same object without unsafe,
	// but correctness is verified by the equal values
}

func TestStringCacheVariousOffsets(t *testing.T) {
	cache := newStringCache()
	data := []byte("abcdefghijklmnopqrstuvwxyz")

	testCases := []struct {
		offset   uint
		size     uint
		expected string
	}{
		{0, 3, "abc"},
		{5, 3, "fgh"},
		{10, 5, "klmno"},
		{23, 3, "xyz"},
	}

	for _, tc := range testCases {
		// First call
		str1 := cache.InternAt(tc.offset, tc.size, data)
		require.Equal(t, tc.expected, str1)

		// Second call should hit cache
		str2 := cache.InternAt(tc.offset, tc.size, data)
		require.Equal(t, tc.expected, str2)
		// Verify cache hit returns correct value (interning tested via behavior)
	}
}
