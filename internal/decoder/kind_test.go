package decoder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKind_String(t *testing.T) {
	tests := []struct {
		kind     Kind
		expected string
	}{
		{KindExtended, "Extended"},
		{KindPointer, "Pointer"},
		{KindString, "String"},
		{KindFloat64, "Float64"},
		{KindBytes, "Bytes"},
		{KindUint16, "Uint16"},
		{KindUint32, "Uint32"},
		{KindMap, "Map"},
		{KindInt32, "Int32"},
		{KindUint64, "Uint64"},
		{KindUint128, "Uint128"},
		{KindSlice, "Slice"},
		{KindContainer, "Container"},
		{KindEndMarker, "EndMarker"},
		{KindBool, "Bool"},
		{KindFloat32, "Float32"},
		{Kind(999), "Unknown(999)"}, // Test unknown kind
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.kind.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestKind_IsContainer(t *testing.T) {
	tests := []struct {
		kind     Kind
		expected bool
		name     string
	}{
		{KindMap, true, "Map is container"},
		{KindSlice, true, "Slice is container"},
		{KindString, false, "String is not container"},
		{KindUint32, false, "Uint32 is not container"},
		{KindBool, false, "Bool is not container"},
		{KindPointer, false, "Pointer is not container"},
		{KindExtended, false, "Extended is not container"},
		{
			KindContainer,
			false,
			"Container is not container",
		}, // Container is special, not a data container
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsContainer()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestKind_IsScalar(t *testing.T) {
	tests := []struct {
		kind     Kind
		expected bool
		name     string
	}{
		{KindString, true, "String is scalar"},
		{KindFloat64, true, "Float64 is scalar"},
		{KindBytes, true, "Bytes is scalar"},
		{KindUint16, true, "Uint16 is scalar"},
		{KindUint32, true, "Uint32 is scalar"},
		{KindInt32, true, "Int32 is scalar"},
		{KindUint64, true, "Uint64 is scalar"},
		{KindUint128, true, "Uint128 is scalar"},
		{KindBool, true, "Bool is scalar"},
		{KindFloat32, true, "Float32 is scalar"},
		{KindMap, false, "Map is not scalar"},
		{KindSlice, false, "Slice is not scalar"},
		{KindPointer, false, "Pointer is not scalar"},
		{KindExtended, false, "Extended is not scalar"},
		{KindContainer, false, "Container is not scalar"},
		{KindEndMarker, false, "EndMarker is not scalar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.kind.IsScalar()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestKind_Classification(t *testing.T) {
	// Test that IsContainer and IsScalar are mutually exclusive for data types
	for k := KindExtended; k <= KindFloat32; k++ {
		isContainer := k.IsContainer()
		isScalar := k.IsScalar()

		// For actual data types (not meta types), they should be either container or scalar
		switch k {
		case KindMap, KindSlice:
			require.True(t, isContainer, "Kind %s should be container", k.String())
			require.False(t, isScalar, "Kind %s should not be scalar", k.String())
		case KindString,
			KindFloat64,
			KindBytes,
			KindUint16,
			KindUint32,
			KindInt32,
			KindUint64,
			KindUint128,
			KindBool,
			KindFloat32:
			require.True(t, isScalar, "Kind %s should be scalar", k.String())
			require.False(t, isContainer, "Kind %s should not be container", k.String())
		default:
			// Meta types like Extended, Pointer, Container, EndMarker are neither
			require.False(t, isContainer, "Meta kind %s should not be container", k.String())
			require.False(t, isScalar, "Meta kind %s should not be scalar", k.String())
		}
	}
}

// ExampleKind_String demonstrates human-readable Kind names.
func ExampleKind_String() {
	kinds := []Kind{KindString, KindMap, KindSlice, KindUint32, KindBool}

	for _, k := range kinds {
		fmt.Printf("%s\n", k.String())
	}

	// Output:
	// String
	// Map
	// Slice
	// Uint32
	// Bool
}

// ExampleKind_IsContainer demonstrates container type detection.
func ExampleKind_IsContainer() {
	kinds := []Kind{KindString, KindMap, KindSlice, KindUint32}

	for _, k := range kinds {
		if k.IsContainer() {
			fmt.Printf("%s is a container type\n", k.String())
		} else {
			fmt.Printf("%s is not a container type\n", k.String())
		}
	}

	// Output:
	// String is not a container type
	// Map is a container type
	// Slice is a container type
	// Uint32 is not a container type
}

// ExampleKind_IsScalar demonstrates scalar type detection.
func ExampleKind_IsScalar() {
	kinds := []Kind{KindString, KindMap, KindUint32, KindPointer}

	for _, k := range kinds {
		if k.IsScalar() {
			fmt.Printf("%s is a scalar value\n", k.String())
		} else {
			fmt.Printf("%s is not a scalar value\n", k.String())
		}
	}

	// Output:
	// String is a scalar value
	// Map is not a scalar value
	// Uint32 is a scalar value
	// Pointer is not a scalar value
}
