package decoder

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Helper function to create a Decoder for a given hex string.
func newDecoderFromHex(t *testing.T, hexStr string) *Decoder {
	t.Helper()
	inputBytes, err := hex.DecodeString(hexStr)
	require.NoError(t, err, "Failed to decode hex string: %s", hexStr)
	dd := NewDataDecoder(inputBytes) // [cite: 11]
	return NewDecoder(dd, 0)         // [cite: 26]
}

// Helper function to create reasonable test names from potentially long hex strings.
func makeTestName(hexStr string) string {
	if len(hexStr) <= 20 {
		return hexStr
	}
	return hexStr[:16] + "..." + hexStr[len(hexStr)-4:]
}

func TestDecodeBool(t *testing.T) {
	tests := map[string]bool{
		"0007": false, // [cite: 29]
		"0107": true,  // [cite: 30]
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadBool() // [cite: 30]
			require.NoError(t, err)
			require.Equal(t, expected, result)
			// Check if offset was advanced correctly (simple check)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeDouble(t *testing.T) {
	tests := map[string]float64{
		"680000000000000000": 0.0,
		"683FE0000000000000": 0.5,
		"68400921FB54442EEA": 3.14159265359,
		"68405EC00000000000": 123.0,
		"6841D000000007F8F4": 1073741824.12457,
		"68BFE0000000000000": -0.5,
		"68C00921FB54442EEA": -3.14159265359,
		"68C1D000000007F8F4": -1073741824.12457,
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadFloat64() // [cite: 38]
			require.NoError(t, err)
			if expected == 0 {
				require.InDelta(t, expected, result, 0)
			} else {
				require.InEpsilon(t, expected, result, 1e-15)
			}
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeFloat(t *testing.T) {
	tests := map[string]float32{
		"040800000000": float32(0.0),
		"04083F800000": float32(1.0),
		"04083F8CCCCD": float32(1.1),
		"04084048F5C3": float32(3.14),
		"0408461C3FF6": float32(9999.99),
		"0408BF800000": float32(-1.0),
		"0408BF8CCCCD": float32(-1.1),
		"0408C048F5C3": -float32(3.14),
		"0408C61C3FF6": float32(-9999.99),
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadFloat32() // [cite: 36]
			require.NoError(t, err)
			if expected == 0 {
				require.InDelta(t, expected, result, 0)
			} else {
				require.InEpsilon(t, expected, result, 1e-6)
			}
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeInt32(t *testing.T) {
	tests := map[string]int32{
		"0001":         int32(0), // [cite: 39]
		"0401ffffffff": int32(-1),
		"0101ff":       int32(255),
		"0401ffffff01": int32(-255),
		"020101f4":     int32(500),
		"0401fffffe0c": int32(-500),
		"0201ffff":     int32(65535),
		"0401ffff0001": int32(-65535),
		"0301ffffff":   int32(16777215),
		"0401ff000001": int32(-16777215),
		"04017fffffff": int32(2147483647), // [cite: 86]
		"040180000001": int32(-2147483647),
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadInt32() // [cite: 40]
			require.NoError(t, err)
			require.Equal(t, expected, result)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeMap(t *testing.T) {
	tests := map[string]map[string]any{
		"e0":                             {}, // [cite: 50]
		"e142656e43466f6f":               {"en": "Foo"},
		"e242656e43466f6f427a6843e4baba": {"en": "Foo", "zh": "人"},
		// Nested map test needs separate handling or more complex validation logic
		// "e1446e616d65e242656e43466f6f427a6843e4baba": map[string]any{
		// 	"name": map[string]any{"en": "Foo", "zh": "人"},
		// },
		// Map containing slice needs separate handling
		// "e1496c616e677561676573020442656e427a68": map[string]any{
		// 	"languages": []any{"en", "zh"},
		// },
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			mapIter, size, err := decoder.ReadMap() // [cite: 53]
			require.NoError(t, err, "ReadMap failed")
			resultMap := make(map[string]any, size) // Pre-allocate with correct capacity

			// Iterate through the map [cite: 54]
			for keyBytes, err := range mapIter { // [cite: 50]
				require.NoError(t, err, "Iterator returned error for key")
				key := string(keyBytes) // [cite: 51] - Need to copy if stored

				// Now decode the value corresponding to the key
				// For simplicity, we'll read as string here. Needs adjustment for mixed types.
				value, err := decoder.ReadString() // [cite: 32]
				require.NoError(t, err, "Failed to decode value for key %s", key)
				resultMap[key] = value
			}

			// Final check on the accumulated map
			require.Equal(t, expected, resultMap)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeSlice(t *testing.T) {
	tests := map[string][]any{
		"0004":                 {}, // [cite: 55]
		"010443466f6f":         {"Foo"},
		"020443466f6f43e4baba": {"Foo", "人"},
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			sliceIter, size, err := decoder.ReadSlice() // [cite: 56]
			require.NoError(t, err, "ReadSlice failed")
			results := make([]any, 0, size) // Pre-allocate with correct capacity

			// Iterate through the slice [cite: 57]
			for err := range sliceIter {
				require.NoError(t, err, "Iterator returned error")

				// Read the current element
				// For simplicity, reading as string. Needs adjustment for mixed types.
				elem, err := decoder.ReadString() // [cite: 32]
				require.NoError(t, err, "Failed to decode slice element")
				results = append(results, elem)
			}

			require.Equal(t, expected, results)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeString(t *testing.T) {
	for hexStr, expected := range testStrings {
		t.Run(makeTestName(hexStr), func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadString() // [cite: 32]
			require.NoError(t, err)
			require.Equal(t, expected, result)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeByte(t *testing.T) {
	byteTests := make(map[string][]byte)
	for key, val := range testStrings {
		oldCtrl, err := hex.DecodeString(key[0:2])
		require.NoError(t, err)
		// Adjust control byte for Bytes type (assuming String=0x2, Bytes=0x5)
		// This mapping might need verification based on the actual type codes.
		// Assuming TypeString=2 (010.....) -> TypeBytes=4 (100.....)
		// Need to check the actual constants [cite: 4, 5]
		newCtrlByte := (oldCtrl[0] & 0x1f) | (byte(KindBytes) << 5)
		newCtrl := []byte{newCtrlByte}

		newKey := hex.EncodeToString(newCtrl) + key[2:]
		byteTests[newKey] = []byte(val.(string))
	}

	for hexStr, expected := range byteTests {
		t.Run(makeTestName(hexStr), func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadBytes() // [cite: 34]
			require.NoError(t, err)
			require.Equal(t, expected, result)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeUint16(t *testing.T) {
	tests := map[string]uint16{
		"a0":     uint16(0), // [cite: 41]
		"a1ff":   uint16(255),
		"a201f4": uint16(500),
		"a22a78": uint16(10872),
		"a2ffff": uint16(65535),
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadUint16() // [cite: 42]
			require.NoError(t, err)
			require.Equal(t, expected, result)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeUint32(t *testing.T) {
	tests := map[string]uint32{
		"c0":         uint32(0), // [cite: 43]
		"c1ff":       uint32(255),
		"c201f4":     uint32(500),
		"c22a78":     uint32(10872),
		"c2ffff":     uint32(65535),
		"c3ffffff":   uint32(16777215),
		"c4ffffffff": uint32(4294967295),
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadUint32() // [cite: 44]
			require.NoError(t, err)
			require.Equal(t, expected, result)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeUint64(t *testing.T) {
	ctrlByte := "02" // Extended type for Uint64 [cite: 10]

	tests := map[string]uint64{
		"00" + ctrlByte:          uint64(0), // [cite: 45]
		"02" + ctrlByte + "01f4": uint64(500),
		"02" + ctrlByte + "2a78": uint64(10872),
		// Add max value tests similar to reflection_test [cite: 89]
		"08" + ctrlByte + "ffffffffffffffff": uint64(18446744073709551615),
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			result, err := decoder.ReadUint64() // [cite: 46]
			require.NoError(t, err)
			require.Equal(t, expected, result)
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestDecodeUint128(t *testing.T) {
	ctrlByte := "03" // Extended type for Uint128 [cite: 10]
	bits := uint(128)

	tests := map[string]*big.Int{
		"00" + ctrlByte:          big.NewInt(0), // [cite: 47]
		"02" + ctrlByte + "01f4": big.NewInt(500),
		"02" + ctrlByte + "2a78": big.NewInt(10872),
		// Add max value tests similar to reflection_test [cite: 91]
		"10" + ctrlByte + strings.Repeat("ff", 16): func() *big.Int { // 16 bytes = 128 bits
			expected := powBigInt(big.NewInt(2), bits)
			return expected.Sub(expected, big.NewInt(1))
		}(),
	}

	for hexStr, expected := range tests {
		t.Run(hexStr, func(t *testing.T) {
			decoder := newDecoderFromHex(t, hexStr)
			hi, lo, err := decoder.ReadUint128() // [cite: 48]
			require.NoError(t, err)

			// Reconstruct the big.Int from hi and lo parts for comparison
			result := new(big.Int)
			result.SetUint64(hi)
			result.Lsh(result, 64)                        // Shift high part left by 64 bits
			result.Or(result, new(big.Int).SetUint64(lo)) // OR with low part

			require.Equal(t, 0, expected.Cmp(result),
				"Expected %v, got %v", expected.String(), result.String())
			require.True(t, decoder.hasNextOffset || decoder.offset > 0, "Offset was not advanced")
		})
	}
}

func TestPointersInDecoder(t *testing.T) {
	bytes, err := os.ReadFile(testFile("maps-with-pointers.raw"))
	require.NoError(t, err)
	dd := NewDataDecoder(bytes)

	expected := map[uint]map[string]string{
		0:  {"long_key": "long_value1"},
		22: {"long_key": "long_value2"},
		37: {"long_key2": "long_value1"},
		50: {"long_key2": "long_value2"},
		55: {"long_key": "long_value1"},
		57: {"long_key2": "long_value2"},
	}

	for startOffset, expectedValue := range expected {
		t.Run(fmt.Sprintf("Offset_%d", startOffset), func(t *testing.T) {
			decoder := NewDecoder(dd, startOffset) // Start at the specific offset

			// Expecting a map at the target offset (may be behind a pointer)
			mapIter, size, err := decoder.ReadMap()
			require.NoError(t, err, "ReadMap failed")
			actualValue := make(map[string]string, int(size))
			for keyBytes, errIter := range mapIter {
				require.NoError(t, errIter)
				key := string(keyBytes)
				// Value is expected to be a string
				value, errDecode := decoder.ReadString()
				require.NoError(t, errDecode)
				actualValue[key] = value
			}

			require.Equal(t, expectedValue, actualValue)
			// Offset check might be complex here due to pointer jumps
		})
	}
}

// TestBoundsChecking verifies that buffer access is properly bounds-checked
// to prevent panics on malformed databases.
func TestBoundsChecking(t *testing.T) {
	// Create a very small buffer that would cause out-of-bounds access
	// if bounds checking is not working
	smallBuffer := []byte{0x44, 0x41} // Type string (0x4), size 4, but only 2 bytes total
	dd := NewDataDecoder(smallBuffer)
	decoder := NewDecoder(dd, 0)

	// This should fail gracefully with an error instead of panicking
	_, err := decoder.ReadString()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected end of database")

	// Test DecodeBytes bounds checking with a separate buffer
	bytesBuffer := []byte{
		0x84,
		0x41,
	} // Type bytes (4 << 5 = 0x80), size 4 (0x04), but only 2 bytes total
	dd3 := NewDataDecoder(bytesBuffer)
	decoder3 := NewDecoder(dd3, 0)

	_, err = decoder3.ReadBytes()
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds buffer length")

	// Test DecodeUint128 bounds checking
	uint128Buffer := []byte{
		0x0B,
		0x03,
	} // Extended type (0x0), size 11, TypeUint128-7=3, but only 2 bytes total
	dd2 := NewDataDecoder(uint128Buffer)
	decoder2 := NewDecoder(dd2, 0)

	_, _, err = decoder2.ReadUint128()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected end of database")
}

func TestPeekKind(t *testing.T) {
	tests := []struct {
		name     string
		buffer   []byte
		expected Kind
	}{
		{
			name:     "string type",
			buffer:   []byte{0x44, 't', 'e', 's', 't'}, // String "test" (TypeString=2, (2<<5)|4)
			expected: KindString,
		},
		{
			name:     "map type",
			buffer:   []byte{0xE0}, // Empty map (TypeMap=7, (7<<5)|0)
			expected: KindMap,
		},
		{
			name: "slice type",
			buffer: []byte{
				0x00,
				0x04,
			}, // Empty slice (TypeSlice=11, extended type: 0x00, TypeSlice-7=4)
			expected: KindSlice,
		},
		{
			name: "bool type",
			buffer: []byte{
				0x01,
				0x07,
			}, // Bool true (TypeBool=14, extended type: size 1, TypeBool-7=7)
			expected: KindBool,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(NewDataDecoder(tt.buffer), 0)

			actualType, err := decoder.PeekKind()
			require.NoError(t, err, "PeekKind failed")

			require.Equal(
				t,
				tt.expected,
				actualType,
				"Expected type %d, got %d",
				tt.expected,
				actualType,
			)

			// Verify that PeekKind doesn't consume the value
			actualType2, err := decoder.PeekKind()
			require.NoError(t, err, "Second PeekKind failed")

			require.Equal(
				t,
				tt.expected,
				actualType2,
				"Second PeekKind gave different result: expected %d, got %d",
				tt.expected,
				actualType2,
			)
		})
	}
}

// TestPeekKindWithPointer tests that PeekKind correctly follows pointers
// to get the actual kind of the pointed-to value.
func TestPeekKindWithPointer(t *testing.T) {
	// Create a buffer with a pointer at offset 0 and a string at offset 5.
	buffer := []byte{
		0x20, 0x05,
		0x00, 0x00, 0x00,
		0x44, 't', 'e', 's', 't',
	}

	decoder := NewDecoder(NewDataDecoder(buffer), 0)

	actualType, err := decoder.PeekKind()
	require.NoError(t, err, "PeekKind with pointer failed")
	require.Equal(t, KindString, actualType)
}

// ExampleDecoder_PeekKind demonstrates how to use PeekKind for
// look-ahead parsing without consuming values.
func ExampleDecoder_PeekKind() {
	// Create test data with different types
	testCases := [][]byte{
		{0x44, 't', 'e', 's', 't'}, // String
		{0xE0},                     // Empty map
		{0x00, 0x04},               // Empty slice (extended type)
		{0x01, 0x07},               // Bool true (extended type)
	}

	typeNames := []string{"String", "Map", "Slice", "Bool"}

	for i, buffer := range testCases {
		decoder := NewDecoder(NewDataDecoder(buffer), 0)

		// Peek at the kind without consuming it
		typ, err := decoder.PeekKind()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Type %d: %s (value: %d)\n", i+1, typeNames[i], typ)

		// PeekKind doesn't consume, so we can peek again
		typ2, err := decoder.PeekKind()
		if err != nil {
			panic(err)
		}

		if typ != typ2 {
			fmt.Println("ERROR: PeekKind consumed the value!")
		}
	}

	// Output:
	// Type 1: String (value: 2)
	// Type 2: Map (value: 7)
	// Type 3: Slice (value: 11)
	// Type 4: Bool (value: 14)
}

func TestDecoderOptions(t *testing.T) {
	buffer := []byte{0x44, 't', 'e', 's', 't'} // String "test"
	dd := NewDataDecoder(buffer)
	optionCalled := false
	option := func(*decoderOptions) {
		optionCalled = true
	}

	// Test that options infrastructure works (even with no current options).
	decoder1 := NewDecoder(dd, 0)
	require.NotNil(t, decoder1)

	// Test that passing options invokes each option callback.
	decoderWithOption := NewDecoder(dd, 0, option)
	require.NotNil(t, decoderWithOption)
	require.True(t, optionCalled)

	// Test that passing empty options slice works.
	decoder2 := NewDecoder(dd, 0)
	require.NotNil(t, decoder2)
}
