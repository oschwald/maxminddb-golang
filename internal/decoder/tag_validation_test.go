package decoder

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTag(t *testing.T) {
	tests := []struct {
		name        string
		fieldName   string
		tag         string
		expectError bool
		description string
	}{
		{
			name:        "ValidTag",
			fieldName:   "TestField",
			tag:         "valid_field",
			expectError: false,
			description: "Valid tag should not error",
		},
		{
			name:        "IgnoreTag",
			fieldName:   "TestField",
			tag:         "-",
			expectError: false,
			description: "Ignore tag should not error",
		},
		{
			name:        "EmptyTag",
			fieldName:   "TestField",
			tag:         "",
			expectError: false,
			description: "Empty tag should not error",
		},
		{
			name:        "ComplexValidTag",
			fieldName:   "TestField",
			tag:         "some_complex_field_name_123",
			expectError: false,
			description: "Complex valid tag should not error",
		},
		{
			name:        "InvalidUTF8",
			fieldName:   "TestField",
			tag:         "field\xff\xfe",
			expectError: true,
			description: "Invalid UTF-8 should error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock struct field
			field := reflect.StructField{
				Name: tt.fieldName,
				Type: reflect.TypeOf(""),
			}

			err := validateTag(field, tt.tag)

			if tt.expectError {
				require.Error(t, err, tt.description)
				assert.Contains(t, err.Error(), tt.fieldName, "Error should mention field name")
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

// TestTagValidationIntegration tests that tag validation works during field processing.
func TestTagValidationIntegration(t *testing.T) {
	// Test that makeStructFields processes tags without panicking
	// even when there are validation errors

	type TestStruct struct {
		ValidField   string `maxminddb:"valid"`
		IgnoredField string `maxminddb:"-"`
		NoTagField   string
	}

	// This should not panic even with invalid tags
	structType := reflect.TypeOf(TestStruct{})
	fields := makeStructFields(structType)

	// Verify that valid fields are still processed
	assert.Contains(t, fields.namedFields, "valid", "Valid field should be processed")
	assert.Contains(t, fields.namedFields, "NoTagField", "Field without tag should use field name")

	// The important thing is that it doesn't crash
	assert.NotNil(t, fields.namedFields, "Fields map should be created")
}

