package decoder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Inner type with UnmarshalMaxMindDB.
type testInnerNested struct {
	Value  string
	custom bool // track if custom unmarshaler was called
}

func (i *testInnerNested) UnmarshalMaxMindDB(d *Decoder) error {
	i.custom = true
	str, err := d.ReadString()
	if err != nil {
		return err
	}
	i.Value = "custom:" + str
	return nil
}

// TestNestedUnmarshaler tests that UnmarshalMaxMindDB is called for nested struct fields.
func TestNestedUnmarshaler(t *testing.T) {
	// Outer type without UnmarshalMaxMindDB
	type Outer struct {
		Field testInnerNested
		Name  string
	}

	// Create test data: a map with "Field" -> "test" and "Name" -> "example"
	data := []byte{
		// Map with 2 items
		0xe2,
		// Key "Field"
		0x45, 'F', 'i', 'e', 'l', 'd',
		// Value "test" (string)
		0x44, 't', 'e', 's', 't',
		// Key "Name"
		0x44, 'N', 'a', 'm', 'e',
		// Value "example" (string)
		0x47, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
	}

	t.Run("nested field with UnmarshalMaxMindDB", func(t *testing.T) {
		d := New(data)
		var result Outer

		err := d.Decode(0, &result)
		require.NoError(t, err)

		// Check that custom unmarshaler WAS called for nested field
		require.True(
			t,
			result.Field.custom,
			"Custom unmarshaler should be called for nested fields",
		)
		require.Equal(t, "custom:test", result.Field.Value)
		require.Equal(t, "example", result.Name)
	})
}

// testInnerPointer with UnmarshalMaxMindDB for pointer test.
type testInnerPointer struct {
	Value  string
	custom bool
}

func (i *testInnerPointer) UnmarshalMaxMindDB(d *Decoder) error {
	i.custom = true
	str, err := d.ReadString()
	if err != nil {
		return err
	}
	i.Value = "ptr:" + str
	return nil
}

// TestNestedUnmarshalerPointer tests UnmarshalMaxMindDB with pointer fields.
func TestNestedUnmarshalerPointer(t *testing.T) {
	type Outer struct {
		Field *testInnerPointer
		Name  string
	}

	// Test data
	data := []byte{
		// Map with 2 items
		0xe2,
		// Key "Field"
		0x45, 'F', 'i', 'e', 'l', 'd',
		// Value "test"
		0x44, 't', 'e', 's', 't',
		// Key "Name"
		0x44, 'N', 'a', 'm', 'e',
		// Value "example"
		0x47, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
	}

	t.Run("pointer field with UnmarshalMaxMindDB", func(t *testing.T) {
		d := New(data)
		var result Outer
		err := d.Decode(0, &result)
		require.NoError(t, err)

		// The pointer should be created and unmarshaled with custom unmarshaler
		require.NotNil(t, result.Field)
		require.True(
			t,
			result.Field.custom,
			"Custom unmarshaler should be called for pointer fields",
		)
		require.Equal(t, "ptr:test", result.Field.Value)
		require.Equal(t, "example", result.Name)
	})
}

// testItem with UnmarshalMaxMindDB for slice test.
type testItem struct {
	ID     int
	custom bool
}

func (item *testItem) UnmarshalMaxMindDB(d *Decoder) error {
	item.custom = true
	id, err := d.ReadUint32()
	if err != nil {
		return err
	}
	item.ID = int(id) * 2
	return nil
}

// TestNestedUnmarshalerInSlice tests UnmarshalMaxMindDB for slice elements.
func TestNestedUnmarshalerInSlice(t *testing.T) {
	type Container struct {
		Items []testItem
	}

	// Test data: a map with "Items" -> [1, 2, 3]
	data := []byte{
		// Map with 1 item (KindMap=7 << 5 | size=1)
		0xe1,
		// Key "Items" (KindString=2 << 5 | size=5)
		0x45, 'I', 't', 'e', 'm', 's',
		// Slice with 3 items - KindSlice=11, which is > 7, so we need extended type
		// Extended type: ctrl_byte = (KindExtended << 5) | size = (0 << 5) | 3 = 0x03
		// Next byte: KindSlice - 7 = 11 - 7 = 4
		0x03, 0x04,
		// Value 1 (KindUint32=6 << 5 | size=1)
		0xc1, 0x01,
		// Value 2 (KindUint32=6 << 5 | size=1)
		0xc1, 0x02,
		// Value 3 (KindUint32=6 << 5 | size=1)
		0xc1, 0x03,
	}

	t.Run("slice elements with UnmarshalMaxMindDB", func(t *testing.T) {
		d := New(data)
		var result Container
		err := d.Decode(0, &result)
		require.NoError(t, err)

		require.Len(t, result.Items, 3)
		// With custom unmarshaler, values should be doubled
		require.True(
			t,
			result.Items[0].custom,
			"Custom unmarshaler should be called for slice elements",
		)
		require.Equal(t, 2, result.Items[0].ID) // 1 * 2
		require.Equal(t, 4, result.Items[1].ID) // 2 * 2
		require.Equal(t, 6, result.Items[2].ID) // 3 * 2
	})
}

// testValue with UnmarshalMaxMindDB for map test.
type testValue struct {
	Data   string
	custom bool
}

func (v *testValue) UnmarshalMaxMindDB(d *Decoder) error {
	v.custom = true
	str, err := d.ReadString()
	if err != nil {
		return err
	}
	v.Data = "map:" + str
	return nil
}

// TestNestedUnmarshalerInMap tests UnmarshalMaxMindDB for map values.
func TestNestedUnmarshalerInMap(t *testing.T) {
	// Test data: {"key1": "value1", "key2": "value2"}
	data := []byte{
		// Map with 2 items
		0xe2,
		// Key "key1"
		0x44, 'k', 'e', 'y', '1',
		// Value "value1"
		0x46, 'v', 'a', 'l', 'u', 'e', '1',
		// Key "key2"
		0x44, 'k', 'e', 'y', '2',
		// Value "value2"
		0x46, 'v', 'a', 'l', 'u', 'e', '2',
	}

	t.Run("map values with UnmarshalMaxMindDB", func(t *testing.T) {
		d := New(data)
		var result map[string]testValue
		err := d.Decode(0, &result)
		require.NoError(t, err)

		require.Len(t, result, 2)
		require.True(t, result["key1"].custom, "Custom unmarshaler should be called for map values")
		require.Equal(t, "map:value1", result["key1"].Data)
		require.Equal(t, "map:value2", result["key2"].Data)
	})
}

// testMapIterator uses ReadMap() iterator to simulate mmdbtype.Map behavior.
type testMapIterator struct {
	Values map[string]string
	custom bool
}

func (m *testMapIterator) UnmarshalMaxMindDB(d *Decoder) error {
	m.custom = true
	iter, size, err := d.ReadMap()
	if err != nil {
		return err
	}

	m.Values = make(map[string]string, size)
	for key, iterErr := range iter {
		if iterErr != nil {
			return iterErr
		}

		// Read the value as a string
		value, err := d.ReadString()
		if err != nil {
			return err
		}

		m.Values[string(key)] = value
	}
	return nil
}

// TestCustomUnmarshalerWithIterator tests that custom unmarshalers using iterators
// work correctly in struct fields. This reproduces the original "no next offset available"
// issue that occurred when mmdbtype.Map was used in structs.
func TestCustomUnmarshalerWithIterator(t *testing.T) {
	type Record struct {
		Name     string
		Location testMapIterator // This field uses ReadMap() iterator
		Country  string
	}

	data := []byte{
		// Map with 3 items
		0xe3,
		// Key "Name"
		0x44, 'N', 'a', 'm', 'e',
		// Value "Test" (string)
		0x44, 'T', 'e', 's', 't',
		// Key "Location"
		0x48, 'L', 'o', 'c', 'a', 't', 'i', 'o', 'n',
		// Value: Map with 2 items (latitude and longitude)
		0xe2,
		// Key "lat"
		0x43, 'l', 'a', 't',
		// Value "40.7"
		0x44, '4', '0', '.', '7',
		// Key "lng"
		0x43, 'l', 'n', 'g',
		// Value "-74.0"
		0x45, '-', '7', '4', '.', '0',
		// Key "Country"
		0x47, 'C', 'o', 'u', 'n', 't', 'r', 'y',
		// Value "US"
		0x42, 'U', 'S',
	}

	d := New(data)
	var result Record

	err := d.Decode(0, &result)
	require.NoError(t, err)

	require.Equal(t, "Test", result.Name)
	assert.True(t, result.Location.custom, "Custom unmarshaler should be called")
	assert.Len(t, result.Location.Values, 2)
	assert.Equal(t, "40.7", result.Location.Values["lat"])
	assert.Equal(t, "-74.0", result.Location.Values["lng"])
	assert.Equal(t, "US", result.Country)
}

type testInterfaceUnmarshaler struct {
	Value  string
	custom bool
}

func (u *testInterfaceUnmarshaler) UnmarshalMaxMindDB(d *Decoder) error {
	u.custom = true
	value, err := d.ReadString()
	if err != nil {
		return err
	}
	u.Value = "interface:" + value
	return nil
}

func TestPreinitializedInterfaceFieldUsesUnmarshaler(t *testing.T) {
	type Record struct {
		Field any `maxminddb:"field"`
	}

	data := []byte{
		0xe1,
		0x45, 'f', 'i', 'e', 'l', 'd',
		0x44, 't', 'e', 's', 't',
	}

	inner := &testInterfaceUnmarshaler{}
	result := Record{Field: inner}
	d := New(data)
	require.NoError(t, d.Decode(0, &result))
	assert.Same(t, inner, result.Field)
	assert.True(t, inner.custom)
	assert.Equal(t, "interface:test", inner.Value)
}

// markedString and friends below are named primitive types implementing
// UnmarshalMaxMindDB with a transform whose output is distinguishable
// from the raw decoded value. If the fast path in tryFastDecodeTyped
// bypasses the unmarshaler, the field decodes to the raw value; if the
// slow path runs, it decodes to the transformed value.

type markedString string

func (m *markedString) UnmarshalMaxMindDB(d *Decoder) error {
	s, err := d.ReadString()
	if err != nil {
		return err
	}
	*m = markedString("X:" + s)
	return nil
}

type markedBool bool

func (m *markedBool) UnmarshalMaxMindDB(d *Decoder) error {
	b, err := d.ReadBool()
	if err != nil {
		return err
	}
	*m = markedBool(!b)
	return nil
}

type markedUint uint

func (m *markedUint) UnmarshalMaxMindDB(d *Decoder) error {
	n, err := d.ReadUint64()
	if err != nil {
		return err
	}
	*m = markedUint(n) + 1000
	return nil
}

type markedUint16 uint16

func (m *markedUint16) UnmarshalMaxMindDB(d *Decoder) error {
	n, err := d.ReadUint16()
	if err != nil {
		return err
	}
	*m = markedUint16(n) + 1000
	return nil
}

type markedUint32 uint32

func (m *markedUint32) UnmarshalMaxMindDB(d *Decoder) error {
	n, err := d.ReadUint32()
	if err != nil {
		return err
	}
	*m = markedUint32(n) + 1000
	return nil
}

type markedUint64 uint64

func (m *markedUint64) UnmarshalMaxMindDB(d *Decoder) error {
	n, err := d.ReadUint64()
	if err != nil {
		return err
	}
	*m = markedUint64(n) + 1000
	return nil
}

type markedFloat64 float64

func (m *markedFloat64) UnmarshalMaxMindDB(d *Decoder) error {
	f, err := d.ReadFloat64()
	if err != nil {
		return err
	}
	*m = markedFloat64(-f)
	return nil
}

// TestFastPathPreservesUnmarshalerForNamedTypes guards every fast-path
// kind against a regression where decodeStruct's isFastType
// short-circuit runs before the checkUnmarshaler branch and silently
// bypasses UnmarshalMaxMindDB on named primitive fields. The hazard
// applies to String, Bool, Uint, Uint16/32/64, and Float64 — and to
// pointers to each (isFastDecodeType recurses through Pointer).
func TestFastPathPreservesUnmarshalerForNamedTypes(t *testing.T) {
	mapPrefix := []byte{0xe1, 0x41, 'v'} // map size 1, key "v"
	wrap := func(value ...byte) []byte {
		return append(append([]byte{}, mapPrefix...), value...)
	}

	cases := []struct {
		name       string
		data       []byte
		runValue   func(t *testing.T, data []byte)
		runPointer func(t *testing.T, data []byte)
	}{
		{
			name: "string",
			data: wrap(0x43, 'f', 'o', 'o'),
			runValue: func(t *testing.T, data []byte) {
				type record struct {
					V markedString `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.Equal(t, markedString("X:foo"), r.V)
			},
			runPointer: func(t *testing.T, data []byte) {
				type record struct {
					V *markedString `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.NotNil(t, r.V)
				require.Equal(t, markedString("X:foo"), *r.V)
			},
		},
		{
			name: "bool",
			// extended (high=0), size=1; kind byte 7 means Kind(7+7)=14=Bool; size=1 -> true
			data: wrap(0x01, 0x07),
			runValue: func(t *testing.T, data []byte) {
				type record struct {
					V markedBool `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.Equal(t, markedBool(false), r.V)
			},
			runPointer: func(t *testing.T, data []byte) {
				type record struct {
					V *markedBool `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.NotNil(t, r.V)
				require.Equal(t, markedBool(false), *r.V)
			},
		},
		{
			name: "uint",
			// extended, size=1; kind byte 2 -> Kind(9)=Uint64; value 42
			data: wrap(0x01, 0x02, 0x2a),
			runValue: func(t *testing.T, data []byte) {
				type record struct {
					V markedUint `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.Equal(t, markedUint(1042), r.V)
			},
			runPointer: func(t *testing.T, data []byte) {
				type record struct {
					V *markedUint `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.NotNil(t, r.V)
				require.Equal(t, markedUint(1042), *r.V)
			},
		},
		{
			name: "uint16",
			data: wrap(0xa1, 0x2a), // KindUint16, size=1, value 42
			runValue: func(t *testing.T, data []byte) {
				type record struct {
					V markedUint16 `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.Equal(t, markedUint16(1042), r.V)
			},
			runPointer: func(t *testing.T, data []byte) {
				type record struct {
					V *markedUint16 `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.NotNil(t, r.V)
				require.Equal(t, markedUint16(1042), *r.V)
			},
		},
		{
			name: "uint32",
			data: wrap(0xc1, 0x2a), // KindUint32, size=1, value 42
			runValue: func(t *testing.T, data []byte) {
				type record struct {
					V markedUint32 `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.Equal(t, markedUint32(1042), r.V)
			},
			runPointer: func(t *testing.T, data []byte) {
				type record struct {
					V *markedUint32 `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.NotNil(t, r.V)
				require.Equal(t, markedUint32(1042), *r.V)
			},
		},
		{
			name: "uint64",
			data: wrap(0x01, 0x02, 0x2a), // extended Uint64, size=1, value 42
			runValue: func(t *testing.T, data []byte) {
				type record struct {
					V markedUint64 `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.Equal(t, markedUint64(1042), r.V)
			},
			runPointer: func(t *testing.T, data []byte) {
				type record struct {
					V *markedUint64 `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.NotNil(t, r.V)
				require.Equal(t, markedUint64(1042), *r.V)
			},
		},
		{
			name: "float64",
			// KindFloat64 (high=3), size=8; IEEE 754 of 3.14159265359
			data: wrap(0x68, 0x40, 0x09, 0x21, 0xfb, 0x54, 0x44, 0x2e, 0xea),
			runValue: func(t *testing.T, data []byte) {
				type record struct {
					V markedFloat64 `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.InDelta(t, -3.14159265359, float64(r.V), 1e-9)
			},
			runPointer: func(t *testing.T, data []byte) {
				type record struct {
					V *markedFloat64 `maxminddb:"v"`
				}
				var r record
				d := New(data)
				require.NoError(t, d.Decode(0, &r))
				require.NotNil(t, r.V)
				require.InDelta(t, -3.14159265359, float64(*r.V), 1e-9)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/value", func(t *testing.T) { tc.runValue(t, tc.data) })
		t.Run(tc.name+"/pointer", func(t *testing.T) { tc.runPointer(t, tc.data) })
	}
}

func TestPreinitializedPointerToInterfaceFieldUsesUnmarshaler(t *testing.T) {
	type Record struct {
		Field *any `maxminddb:"field"`
	}

	data := []byte{
		0xe1,
		0x45, 'f', 'i', 'e', 'l', 'd',
		0x44, 't', 'e', 's', 't',
	}

	inner := &testInterfaceUnmarshaler{}
	holder := any(inner)
	result := Record{Field: &holder}
	d := New(data)
	require.NoError(t, d.Decode(0, &result))
	require.NotNil(t, result.Field)
	assert.Same(t, inner, (*result.Field).(*testInterfaceUnmarshaler))
	assert.True(t, inner.custom)
	assert.Equal(t, "interface:test", inner.Value)
}
