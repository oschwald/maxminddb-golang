package decoder

import (
	"fmt"
)

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
