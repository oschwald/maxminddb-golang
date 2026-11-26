package decoder

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

// TestFieldPrecedenceRules tests json/v2 style field precedence behavior.
func TestFieldPrecedenceRules(t *testing.T) {
	// Test data: {"en": "Foo"}
	testData := "e142656e43466f6f"
	testBytes, err := hex.DecodeString(testData)
	require.NoError(t, err)

	t.Run("DirectFieldWinsOverEmbedded", func(t *testing.T) {
		type Embedded struct {
			En string `maxminddb:"en"`
		}
		target := &struct {
			Embedded

			En string `maxminddb:"en"` // Direct field should win
		}{}

		decoder := New(testBytes)
		err := decoder.Decode(0, target)
		require.NoError(t, err)

		assert.Equal(t, "Foo", target.En, "Direct field should be set")
		assert.Empty(t, target.Embedded.En, "Embedded field should not be set due to precedence")
	})

	t.Run("TaggedFieldWinsOverUntagged", func(t *testing.T) {
		type Untagged struct {
			En string // Untagged field
		}
		target := &struct {
			Untagged

			En string `maxminddb:"en"` // Tagged field should win
		}{}

		decoder := New(testBytes)
		err := decoder.Decode(0, target)
		require.NoError(t, err)

		assert.Equal(t, "Foo", target.En, "Tagged field should be set")
		assert.Empty(t, target.Untagged.En, "Untagged field should not be set")
	})

	t.Run("ShallowFieldWinsOverDeep", func(t *testing.T) {
		type DeepNested struct {
			En string `maxminddb:"en"` // Deeper field
		}
		type MiddleNested struct {
			DeepNested
		}
		target := &struct {
			MiddleNested

			En string `maxminddb:"en"` // Shallow field should win
		}{}

		decoder := New(testBytes)
		err := decoder.Decode(0, target)
		require.NoError(t, err)

		assert.Equal(t, "Foo", target.En, "Shallow field should be set")
		assert.Empty(t, target.DeepNested.En, "Deep field should not be set due to precedence")
	})
}

// TestEmbeddedPointerSupport tests support for embedded pointer types.
func TestEmbeddedPointerSupport(t *testing.T) {
	// Test data: {"data": "test"}
	testData := "e144646174614474657374"
	testBytes, err := hex.DecodeString(testData)
	require.NoError(t, err)

	type EmbeddedPointer struct {
		Data string `maxminddb:"data"`
	}

	target := &struct {
		*EmbeddedPointer

		Other string `maxminddb:"other"`
	}{}

	decoder := New(testBytes)
	err = decoder.Decode(0, target)
	require.NoError(t, err)

	// Test embedded pointer field access - this was causing nil pointer dereference before fix
	require.NotNil(t, target.EmbeddedPointer, "Embedded pointer should be initialized")
	assert.Equal(t, "test", target.Data)
}

// TestFieldCaching tests the field caching mechanism works with new precedence rules.
func TestFieldCaching(t *testing.T) {
	type Embedded struct {
		Field1 string `maxminddb:"field1"`
	}

	type TestStruct struct {
		Embedded

		Field2 int  `maxminddb:"field2"`
		Field3 bool `maxminddb:"field3"`
	}

	// Test that multiple instances use cached fields
	s1 := TestStruct{}
	s2 := TestStruct{}

	fields1 := cachedFields(reflect.ValueOf(s1))
	fields2 := cachedFields(reflect.ValueOf(s2))

	// Should be the same cached instance
	assert.Same(t, fields1, fields2, "Same struct type should use cached fields")

	// Verify field mapping includes embedded fields
	expectedFieldNames := []string{"field1", "field2", "field3"}

	assert.Len(t, fields1.namedFields, 3, "Should have 3 named fields")
	for _, name := range expectedFieldNames {
		assert.Contains(t, fields1.namedFields, name, "Should contain field: "+name)
		assert.NotNil(t, fields1.namedFields[name], "Field info should not be nil: "+name)
	}
}

func TestPointerStructFieldCaching(t *testing.T) {
	type inner struct {
		En string `maxminddb:"en"`
	}

	type outer struct {
		Inner *inner `maxminddb:"inner"`
	}

	fields := makeStructFields(reflect.TypeFor[outer]())
	require.Contains(t, fields.namedFields, "inner")
	require.NotNil(t, fields.namedFields["inner"].structFields)

	// Test data: {"inner": {"en": "Foo"}}
	testData := "e145696e6e6572e142656e43466f6f"
	testBytes, err := hex.DecodeString(testData)
	require.NoError(t, err)

	var target outer
	decoder := New(testBytes)
	require.NoError(t, decoder.Decode(0, &target))
	require.NotNil(t, target.Inner)
	assert.Equal(t, "Foo", target.Inner.En)
}

func TestNestedStructWrongKindUsesFieldOffset(t *testing.T) {
	type inner struct {
		Name string `maxminddb:"name"`
	}
	tests := map[string][]byte{
		"direct scalar": {0xe1, 0x46, 'n', 'e', 's', 't', 'e', 'd', 0x80},
		"pointer to scalar": {
			0xe1, 0x46, 'n', 'e', 's', 't', 'e', 'd',
			0x20, 0x0a, // pointer to the scalar at offset 10
			0x80,
		},
	}

	for name, data := range tests {
		t.Run(name+"/value", func(t *testing.T) {
			var target struct {
				Nested inner `maxminddb:"nested"`
			}
			decoder := New(data)
			err := decoder.Decode(0, &target)
			require.Error(t, err)
			require.ErrorContains(t, err, "cannot unmarshal")
			var typeError mmdberrors.UnmarshalTypeError
			require.ErrorAs(t, err, &typeError)
		})

		t.Run(name+"/pointer", func(t *testing.T) {
			var target struct {
				Nested *inner `maxminddb:"nested"`
			}
			decoder := New(data)
			err := decoder.Decode(0, &target)
			require.Error(t, err)
			require.ErrorContains(t, err, "cannot unmarshal")
			var typeError mmdberrors.UnmarshalTypeError
			require.ErrorAs(t, err, &typeError)
			require.Nil(t, target.Nested)
		})
	}
}

func TestRecursivePointerStructFieldCaching(t *testing.T) {
	type node struct {
		Next *node  `maxminddb:"next"`
		Name string `maxminddb:"name"`
	}

	fields := makeStructFields(reflect.TypeFor[node]())
	require.Contains(t, fields.namedFields, "next")
	assert.Nil(t, fields.namedFields["next"].structFields)
}

func TestPointerStructFieldCachingChecksDepth(t *testing.T) {
	type inner struct {
		En string `maxminddb:"en"`
	}

	fields := makeStructFields(reflect.TypeFor[inner]())
	tests := map[string][]byte{
		"direct map": {0xe0},
		"pointer": {
			0x20, 0x02,
			0xe0,
		},
	}

	for name, data := range tests {
		t.Run(name+"/value struct", func(t *testing.T) {
			decoder := New(data)
			var target inner
			_, ok, err := decoder.tryDecodeStructWithFields(
				0,
				addressableValue{Value: reflect.ValueOf(&target).Elem()},
				maximumDataStructureDepth,
				fields,
			)

			require.True(t, ok)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "exceeded maximum data structure depth")
		})

		t.Run(name+"/pointer struct", func(t *testing.T) {
			decoder := New(data)
			var target *inner
			_, ok, err := decoder.tryDecodePointerStructWithFields(
				0,
				addressableValue{Value: reflect.ValueOf(&target).Elem()},
				maximumDataStructureDepth,
				fields,
			)

			require.True(t, ok)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "exceeded maximum data structure depth")
			assert.Nil(t, target)
		})
	}
}

type unexportedEmbeddedUnmarshalerA struct {
	V string
}

func (u *unexportedEmbeddedUnmarshalerA) UnmarshalMaxMindDB(d *Decoder) error {
	v, err := d.ReadString()
	u.V = v
	return err
}

type unexportedEmbeddedUnmarshalerB struct {
	V string
}

func (u *unexportedEmbeddedUnmarshalerB) UnmarshalMaxMindDB(d *Decoder) error {
	v, err := d.ReadString()
	u.V = v
	return err
}

type unexportedEmbeddedCursorUnmarshalerA struct {
	V string
}

func (u *unexportedEmbeddedCursorUnmarshalerA) UnmarshalMaxMindDBCursor(
	c Cursor,
) (Cursor, error) {
	v, next, err := c.ReadString()
	u.V = v
	return next, err
}

type unexportedEmbeddedCursorUnmarshalerB struct {
	V string
}

func (u *unexportedEmbeddedCursorUnmarshalerB) UnmarshalMaxMindDBCursor(
	c Cursor,
) (Cursor, error) {
	v, next, err := c.ReadString()
	u.V = v
	return next, err
}

// TestUnexportedTaggedEmbeddedUnmarshaler tests that decoding returns an error
// instead of panicking when a record has the key of a named field that is an
// unexported embedded struct with a custom unmarshaler. Each parent is a named
// type with two such fields. The two methods conflict, so neither is promoted
// to the parent, and reflection decodes the parent field by field.
func TestUnexportedTaggedEmbeddedUnmarshaler(t *testing.T) {
	// {"a": "x", "b": "z"}
	data, err := hex.DecodeString("e2416141784162417a")
	require.NoError(t, err)

	t.Run("used", func(t *testing.T) {
		type record struct {
			unexportedEmbeddedUnmarshalerA `maxminddb:"a"`
			unexportedEmbeddedUnmarshalerB `maxminddb:"b"`
		}

		var got record

		d := New(data)
		require.NotPanics(t, func() {
			err = d.Decode(0, &got)
		})
		require.EqualError(
			t,
			err,
			"at offset 0, path /a: cannot decode into unexported embedded field of type "+
				"decoder.unexportedEmbeddedUnmarshalerA: its type has a custom unmarshaler",
		)
	})

	t.Run("cursor unmarshaler", func(t *testing.T) {
		type record struct {
			unexportedEmbeddedCursorUnmarshalerA `maxminddb:"a"`
			unexportedEmbeddedCursorUnmarshalerB `maxminddb:"b"`
		}

		var got record

		d := New(data)
		require.NotPanics(t, func() {
			err = d.Decode(0, &got)
		})
		require.EqualError(
			t,
			err,
			"at offset 0, path /a: cannot decode into unexported embedded field of type "+
				"decoder.unexportedEmbeddedCursorUnmarshalerA: its type has a custom unmarshaler",
		)
	})

	t.Run("slice element and map value", func(t *testing.T) {
		type record struct {
			unexportedEmbeddedUnmarshalerA `maxminddb:"a"`
			unexportedEmbeddedUnmarshalerB `maxminddb:"b"`
		}
		const want = "cannot decode into unexported embedded field of type " +
			"decoder.unexportedEmbeddedUnmarshalerA: its type has a custom unmarshaler"

		// [{"a": "x", "b": "z"}]
		sliceData, err := hex.DecodeString("0104e2416141784162417a")
		require.NoError(t, err)
		var gotSlice []record
		d := New(sliceData)
		require.NotPanics(t, func() {
			err = d.Decode(0, &gotSlice)
		})
		require.ErrorContains(t, err, want)

		// {"k": {"a": "x", "b": "z"}}
		mapData, err := hex.DecodeString("e1416be2416141784162417a")
		require.NoError(t, err)
		var gotMap map[string]record
		d = New(mapData)
		require.NotPanics(t, func() {
			err = d.Decode(0, &gotMap)
		})
		require.ErrorContains(t, err, want)
	})

	t.Run("key not in record", func(t *testing.T) {
		// {"c": "y"}
		other, err := hex.DecodeString("e141634179")
		require.NoError(t, err)

		type record struct {
			unexportedEmbeddedUnmarshalerA `maxminddb:"a"`
			unexportedEmbeddedUnmarshalerB `maxminddb:"b"`

			C string `maxminddb:"c"`
		}

		var got record

		d := New(other)
		require.NoError(t, d.Decode(0, &got))
		assert.Equal(t, "y", got.C)
	})

	t.Run("shadowed", func(t *testing.T) {
		// The direct fields are shallower than the fields of Inner, so they
		// win for keys "a" and "b".
		type Inner struct {
			unexportedEmbeddedUnmarshalerA `maxminddb:"a"`
			unexportedEmbeddedUnmarshalerB `maxminddb:"b"`
		}
		type record struct {
			Inner

			A string `maxminddb:"a"`
			B string `maxminddb:"b"`
		}

		var got record

		d := New(data)
		require.NoError(t, d.Decode(0, &got))
		assert.Equal(t, "x", got.A)
		assert.Equal(t, "z", got.B)
	})
}

type unexportedEmbeddedUnmarshalerHolder struct {
	A unexportedEmbeddedUnmarshalerA `maxminddb:"a"`
}

// TestUnexportedTaggedEmbeddedStructWithUnmarshalerField tests that an
// unexported embedded struct with a name tag decodes when its own type has no
// custom unmarshaler but one of its exported fields does.
func TestUnexportedTaggedEmbeddedStructWithUnmarshalerField(t *testing.T) {
	// {"w": {"a": "x"}}
	data, err := hex.DecodeString("e14177e141614178")
	require.NoError(t, err)

	var got struct {
		unexportedEmbeddedUnmarshalerHolder `maxminddb:"w"`
	}
	d := New(data)
	require.NoError(t, d.Decode(0, &got))
	assert.Equal(t, "x", got.A.V)
}
