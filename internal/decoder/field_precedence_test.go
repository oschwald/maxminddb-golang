package decoder

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
