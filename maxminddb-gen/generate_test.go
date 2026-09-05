package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateGoldenIsDeterministicAndCompiles(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("testdata", "basic"))
	require.NoError(t, err)
	goldenPath := filepath.Join(fixture, "model_maxminddb.go")
	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	testFixture := t.TempDir()
	require.NoError(t, os.CopyFS(testFixture, os.DirFS(fixture)))
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	modulePath := filepath.Join(testFixture, "go.mod")
	module, err := os.ReadFile(modulePath)
	require.NoError(t, err)
	module = bytes.ReplaceAll(
		module,
		[]byte("=> ../../.."),
		[]byte("=> "+filepath.ToSlash(repository)),
	)
	require.NoError(
		t,
		os.WriteFile(modulePath, module, 0o600), //nolint:gosec // Path is under t.TempDir.
	)

	t.Chdir(testFixture)
	require.NoError(t, run([]string{"model.go"}))
	first, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Equal(t, normalizeLineEndings(golden), string(first))

	require.NoError(t, run([]string{"model.go"}))
	second, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Equal(t, string(first), string(second))

	command := exec.CommandContext(context.Background(), "go", "test", ".")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func TestNormalizeLineEndings(t *testing.T) {
	require.Equal(t, "first\nsecond\n", normalizeLineEndings([]byte("first\r\nsecond\r\n")))
}

func normalizeLineEndings(contents []byte) string {
	return strings.ReplaceAll(string(contents), "\r\n", "\n")
}

func TestRunWritesPackageErrorsToDiagnostics(t *testing.T) {
	t.Run("syntax loading", func(t *testing.T) {
		dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "broken.go"),
			[]byte("package fixture\n\nfunc broken(\n"),
			0o600,
		))
		t.Chdir(dir)
		var diagnostics bytes.Buffer
		err := runWithIO([]string{"model.go"}, io.Discard, &diagnostics)
		require.ErrorContains(t, err, "loading target package syntax")
		require.Contains(t, diagnostics.String(), "broken.go")
	})

	t.Run("type loading", func(t *testing.T) {
		dir := newTestModule(t, "package fixture\n\ntype Record struct { Value Missing }\n")
		t.Chdir(dir)
		var diagnostics bytes.Buffer
		err := runWithIO([]string{"model.go"}, io.Discard, &diagnostics)
		require.ErrorContains(t, err, "loading target package")
		require.Contains(t, diagnostics.String(), "undefined: Missing")
	})
}

func TestGenerateRejectsUnsupportedTypeWithPosition(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Bad struct {
	Unsupported chan int `+"`maxminddb:\"unsupported\"`"+`
}
`)
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.Error(t, err)
	require.ErrorContains(t, err, "model.go:4")
	require.ErrorContains(t, err, "field Unsupported: unsupported type chan int")
}

func TestGenerateRejectsInterfaceFieldsBeforeCustomUnmarshalerDetection(t *testing.T) {
	tests := []struct {
		name      string
		fieldType string
		decl      string
	}{
		{name: "unmarshaler", fieldType: "mmdbdata.Unmarshaler"},
		{name: "cursor unmarshaler", fieldType: "mmdbdata.CursorUnmarshaler"},
		{
			name:      "anonymous unmarshaler interface",
			fieldType: "interface { mmdbdata.Unmarshaler }",
		},
		{
			name:      "embedded cursor interface",
			fieldType: "CustomCursorUnmarshaler",
			decl:      "type CustomCursorUnmarshaler interface { mmdbdata.CursorUnmarshaler }\n\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newTestModule(t, `package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

`+tt.decl+`type Record struct {
	Value `+tt.fieldType+` `+"`maxminddb:\"value\"`"+`
}
`)
			t.Chdir(dir)
			err := run([]string{"model.go"})
			require.ErrorContains(t, err, "field Value: interface type")
			require.ErrorContains(t, err, "is not supported")
			_, statErr := os.Stat("model_maxminddb.go")
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestGenerateRejectsOnlyHandwrittenMethod(t *testing.T) {
	dir := newTestModule(t, `package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type Record struct{}

func (*Record) UnmarshalMaxMindDB(*mmdbdata.Decoder) error { return nil }
`)
	t.Chdir(dir)
	var stderr bytes.Buffer
	err := runWithIO([]string{"model.go"}, io.Discard, &stderr)
	require.ErrorContains(t, err, "no decoders remain")
	require.Contains(t, stderr.String(), "skipping Record: handwritten UnmarshalMaxMindDB method")
}

func TestGenerateRejectsIncompatibleHandwrittenMethod(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record struct{}

func (*Record) UnmarshalMaxMindDB(int) error { return nil }
`)
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "model.go:5")
	require.ErrorContains(t, err, "incompatible UnmarshalMaxMindDB method")
}

func TestGenerateRejectsOnlyHandwrittenCursorMethod(t *testing.T) {
	dir := newTestModule(t, `package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type Record struct{}

func (*Record) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	return cursor.Skip()
}
`)
	t.Chdir(dir)
	var stderr bytes.Buffer
	err := runWithIO([]string{"model.go"}, io.Discard, &stderr)
	require.ErrorContains(t, err, "no decoders remain")
	require.Contains(
		t,
		stderr.String(),
		"skipping Record: handwritten UnmarshalMaxMindDBCursor method",
	)
}

func TestGenerateRejectsIncompatibleHandwrittenCursorMethod(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record struct{}

func (*Record) UnmarshalMaxMindDBCursor(int) error { return nil }
`)
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "model.go:5")
	require.ErrorContains(t, err, "incompatible UnmarshalMaxMindDBCursor method")
}

func TestGenerateRejectsGenericTarget(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record[T any] struct {
	Name string
}
`)
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "model.go:3")
	require.ErrorContains(t, err, "generic target Record is not supported")
}

func TestGenerateRejectsNestedGenericStruct(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record struct {
	Box Box[string]
}
`)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "box.go"),
		[]byte("package fixture\n\ntype Box[T any] struct { Value T }\n"),
		0o600,
	))
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "model.go:4")
	require.ErrorContains(t, err, "field Box: generic struct Box is not supported")
}

func TestGenerateTreatsEmptyMaxMindDBTagAsUntagged(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\n"+
		"type Record struct {\n"+
		"\tName string `maxminddb:\"\"`\n"+
		"}\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), `case "Name":`)
	testGeneratedPackage(t)
}

func TestGenerateQuotesMaxMindDBTagsInErrors(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\n"+
		"type Record struct {\n"+
		"\tQuote string `maxminddb:\"a\\\"b\"`\n"+
		"\tBackslash string `maxminddb:\"a\\\\b\"`\n"+
		"\tNewline string `maxminddb:\"a\\nb\"`\n"+
		"\tPercent string `maxminddb:\"a%b\"`\n"+
		"}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record_test.go"), []byte(`package fixture

import (
	"strings"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestEscapedTagErrors(t *testing.T) {
	for _, key := range []string{"a\"b", "a\\b", "a\nb", "a%b"} {
		data := []byte{0xe1, byte(0x40 + len(key))}
		data = append(data, key...)
		data = append(data, 0xa1, 1)
		var record Record
		err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
		if err == nil {
			t.Fatalf("expected an error for key %q", key)
		}
		if want := "decoding field " + key + ": "; !strings.HasPrefix(err.Error(), want) {
			t.Fatalf("error %q does not start with %q", err, want)
		}
	}
}
`), 0o600))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGenerateUsesNestedCursorUnmarshalers(t *testing.T) {
	dir := newTestModule(t, `package fixture

import (
	"example.com/generatortest/custom"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

type Local string

func (value *Local) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	decoded, next, err := cursor.ReadString()
	if err == nil {
		*value = Local("local:" + decoded)
	}
	return next, err
}

type Record struct {
	Local Local `+"`maxminddb:\"local\"`"+`
	External custom.Value `+"`maxminddb:\"external\"`"+`
}
`)
	writeTestPackage(t, dir, "custom", `package custom

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type Value string

func (value *Value) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	decoded, next, err := cursor.ReadString()
	if err == nil {
		*value = Value("external:" + decoded)
	}
	return next, err
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record_test.go"), []byte(`package fixture

import (
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestNestedCursorUnmarshalers(t *testing.T) {
	data := []byte{
		0xe2,
		0x45, 'l', 'o', 'c', 'a', 'l', 0x41, 'a',
		0x48, 'e', 'x', 't', 'e', 'r', 'n', 'a', 'l', 0x41, 'b',
	}
	var record Record
	if err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0)); err != nil {
		t.Fatal(err)
	}
	if record.Local != "local:a" || record.External != "external:b" {
		t.Fatalf("unexpected record: %#v", record)
	}
}
`), 0o600))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGenerateUsesPointerCustomUnmarshalers(t *testing.T) {
	dir := newTestModule(t, `package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type CursorValue string

func (value *CursorValue) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	decoded, next, err := cursor.ReadString()
	if err == nil {
		*value = CursorValue("cursor:" + decoded)
	}
	return next, err
}

type LegacyValue string

func (value *LegacyValue) UnmarshalMaxMindDB(decoder *mmdbdata.Decoder) error {
	decoded, err := decoder.ReadString()
	if err == nil {
		*value = LegacyValue("legacy:" + decoded)
	}
	return err
}

type Record struct {
	Cursor *CursorValue `+"`maxminddb:\"cursor\"`"+`
	Legacy *LegacyValue `+"`maxminddb:\"legacy\"`"+`
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record_test.go"), []byte(`package fixture

import (
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func decodeRecord(t *testing.T, cursorValue, legacyValue byte, record *Record) {
	t.Helper()
	data := []byte{
		0xe2,
		0x46, 'c', 'u', 'r', 's', 'o', 'r', 0x41, cursorValue,
		0x46, 'l', 'e', 'g', 'a', 'c', 'y', 0x41, legacyValue,
	}
	if err := record.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0)); err != nil {
		t.Fatal(err)
	}
}

func TestPointerCustomUnmarshalers(t *testing.T) {
	var record Record
	decodeRecord(t, 'a', 'b', &record)
	if record.Cursor == nil || *record.Cursor != "cursor:a" {
		t.Fatalf("unexpected cursor value: %#v", record.Cursor)
	}
	if record.Legacy == nil || *record.Legacy != "legacy:b" {
		t.Fatalf("unexpected legacy value: %#v", record.Legacy)
	}

	cursorPointer, legacyPointer := record.Cursor, record.Legacy
	decodeRecord(t, 'c', 'd', &record)
	if record.Cursor != cursorPointer || record.Legacy != legacyPointer {
		t.Fatal("existing custom unmarshaler pointers were not reused")
	}
	if *record.Cursor != "cursor:c" || *record.Legacy != "legacy:d" {
		t.Fatalf("unexpected reused values: %#v", record)
	}
}
`), 0o600))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGenerateSupportsPointerContainers(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record struct {
	Values *[]uint16 `+"`maxminddb:\"values\"`"+`
	Lookup *map[string]string `+"`maxminddb:\"lookup\"`"+`
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record_test.go"), []byte(`package fixture

import (
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func pointerContainerData(values []byte, key, value byte) []byte {
	data := []byte{
		0xe2, 0x46, 'v', 'a', 'l', 'u', 'e', 's', byte(len(values) / 2), 0x04,
	}
	data = append(data, values...)
	data = append(data, 0x46, 'l', 'o', 'o', 'k', 'u', 'p', 0xe1, 0x41, key, 0x41, value)
	return data
}

func TestPointerContainers(t *testing.T) {
	var record Record
	data := pointerContainerData([]byte{0xa1, 1, 0xa1, 2}, 'a', 'x')
	if _, err := record.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor()); err != nil {
		t.Fatal(err)
	}
	if record.Values == nil || !reflect.DeepEqual(*record.Values, []uint16{1, 2}) ||
		record.Lookup == nil || !reflect.DeepEqual(*record.Lookup, map[string]string{"a": "x"}) {
		t.Fatalf("decoded %#v", record)
	}

	valuesPointer, lookupPointer := record.Values, record.Lookup
	firstElement := &(*record.Values)[0]
	data = pointerContainerData([]byte{0xa1, 3}, 'b', 'y')
	if _, err := record.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor()); err != nil {
		t.Fatal(err)
	}
	if record.Values != valuesPointer || record.Lookup != lookupPointer || &(*record.Values)[0] != firstElement {
		t.Fatal("existing pointer containers were not reused")
	}
	if !reflect.DeepEqual(*record.Values, []uint16{3}) ||
		!reflect.DeepEqual(*record.Lookup, map[string]string{"a": "x", "b": "y"}) {
		t.Fatalf("decoded reused record %#v", record)
	}
}
`), 0o600))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGeneratedIntegerBoundaries(t *testing.T) {
	dir := newTestModule(t, `package fixture

type IntegerBounds struct {
	Unsigned uint8 `+"`maxminddb:\"unsigned\"`"+`
	Signed int8 `+"`maxminddb:\"signed\"`"+`
	PlatformUnsigned uint `+"`maxminddb:\"platform_unsigned\"`"+`
	PlatformSigned int `+"`maxminddb:\"platform_signed\"`"+`
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "integer_test.go"), []byte(`package fixture

import (
	"strconv"
	"strings"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestIntegerBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   []byte
		initial IntegerBounds
		want    IntegerBounds
		wantErr bool
		only32  bool
	}{
		{name: "uint8 maximum", key: "unsigned", value: []byte{0xa1, 0xff}, want: IntegerBounds{Unsigned: 255}},
		{
			name: "uint8 overflow", key: "unsigned", value: []byte{0xa2, 0x01, 0x00},
			initial: IntegerBounds{Unsigned: 7}, want: IntegerBounds{Unsigned: 7}, wantErr: true,
		},
		{
			name: "negative to unsigned", key: "unsigned",
			value: []byte{0x04, 0x01, 0xff, 0xff, 0xff, 0xff},
			initial: IntegerBounds{Unsigned: 7}, want: IntegerBounds{Unsigned: 7}, wantErr: true,
		},
		{
			name: "int8 overflow", key: "signed", value: []byte{0xa1, 0x80},
			initial: IntegerBounds{Signed: 7}, want: IntegerBounds{Signed: 7}, wantErr: true,
		},
		{
			name: "negative int8", key: "signed",
			value: []byte{0x04, 0x01, 0xff, 0xff, 0xff, 0xfb},
			initial: IntegerBounds{Signed: 7}, want: IntegerBounds{Signed: -5},
		},
		{
			name: "int8 minimum", key: "signed",
			value: []byte{0x04, 0x01, 0xff, 0xff, 0xff, 0x80},
			initial: IntegerBounds{Signed: 7}, want: IntegerBounds{Signed: -128},
		},
		{
			name: "uint maximum on 32-bit", key: "platform_unsigned",
			value: []byte{0xc4, 0xff, 0xff, 0xff, 0xff},
			want: IntegerBounds{PlatformUnsigned: uint(0xffffffff)}, only32: true,
		},
		{
			name: "uint overflow on 32-bit", key: "platform_unsigned",
			value: []byte{0x05, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00},
			initial: IntegerBounds{PlatformUnsigned: 7},
			want: IntegerBounds{PlatformUnsigned: 7}, wantErr: true, only32: true,
		},
		{
			name: "int maximum on 32-bit", key: "platform_signed",
			value: []byte{0xc4, 0x7f, 0xff, 0xff, 0xff},
			want: IntegerBounds{PlatformSigned: int(0x7fffffff)}, only32: true,
		},
		{
			name: "int overflow on 32-bit", key: "platform_signed",
			value: []byte{0xc4, 0x80, 0x00, 0x00, 0x00},
			initial: IntegerBounds{PlatformSigned: 7},
			want: IntegerBounds{PlatformSigned: 7}, wantErr: true, only32: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.only32 && strconv.IntSize != 32 {
				t.Skip("32-bit platform boundary")
			}
			data := []byte{0xe1, byte(0x40 + len(tt.key))}
			data = append(data, tt.key...)
			data = append(data, tt.value...)
			got := tt.initial
			err := got.UnmarshalMaxMindDB(mmdbdata.NewDecoder(data, 0))
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "cannot unmarshal") {
					t.Fatalf("expected conversion error, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}
`), 0o600))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGenerateRejectsRecursiveNamedContainers(t *testing.T) {
	tests := []struct {
		name      string
		container string
	}{
		{name: "slice", container: "[]Recursive"},
		{name: "map", container: "map[string]Recursive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newTestModule(t, `package fixture

type Recursive `+tt.container+`

type Record struct {
	Values Recursive
}
`)
			t.Chdir(dir)
			err := run([]string{"model.go"})
			require.ErrorContains(t, err, "recursive type graph through Recursive is not supported")
		})
	}
}

func TestGenerateHandlesNamedByteSlicesLikeReflection(t *testing.T) {
	dir := newTestModule(t, `package fixture

type MyByte byte
type Blob []byte

type Record struct {
	Bytes []byte
	Named []MyByte
	Data Blob
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record_test.go"), []byte(`package fixture

import (
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestNamedByteSliceBehavior(t *testing.T) {
	integerSlice := []byte{0x03, 0x04, 0xa1, 1, 0xa1, 2, 0xa1, 3}
	tests := []struct {
		name    string
		key     string
		value   []byte
		want    Record
		wantErr bool
	}{
		{
			name:  "plain bytes from bytes kind",
			key:   "Bytes",
			value: []byte{0x83, 1, 2, 3},
			want:  Record{Bytes: []byte{1, 2, 3}},
		},
		{
			name:  "named elements from integer slice",
			key:   "Named",
			value: integerSlice,
			want:  Record{Named: []MyByte{1, 2, 3}},
		},
		{
			name:  "named slice from integer slice",
			key:   "Data",
			value: integerSlice,
			want:  Record{Data: Blob{1, 2, 3}},
		},
		{
			name:    "named elements reject bytes kind",
			key:     "Named",
			value:   []byte{0x83, 1, 2, 3},
			wantErr: true,
		},
		{
			name:    "named slice rejects bytes kind",
			key:     "Data",
			value:   []byte{0x83, 1, 2, 3},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte{0xe1, byte(0x40 + len(tt.key))}
			data = append(data, tt.key...)
			data = append(data, tt.value...)
			data = append(data, 0x44, 'd', 'o', 'n', 'e')

			decoder := mmdbdata.NewDecoder(data, 0)
			var got Record
			next, err := got.UnmarshalMaxMindDBCursor(decoder.Cursor())
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected decoding error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
			trailing, _, err := next.ReadString()
			if err != nil {
				t.Fatal(err)
			}
			if trailing != "done" {
				t.Fatalf("trailing value = %q, want done", trailing)
			}
		})
	}
}
`), 0o600))
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGeneratedRecursivelyComposedContainers(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Nested struct {
	Name string `+"`maxminddb:\"name\"`"+`
}

type Record struct {
	Groups map[string][]*Nested `+"`maxminddb:\"groups\"`"+`
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record_test.go"), []byte(`package fixture

import (
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestRecursivelyComposedContainers(t *testing.T) {
	data := []byte{
		0xe1,
		0x46, 'g', 'r', 'o', 'u', 'p', 's',
		0xe1,
		0x43, 's', 'e', 't',
		0x02, 0x04,
		0xe1, 0x44, 'n', 'a', 'm', 'e', 0x41, 'a',
		0xe1, 0x44, 'n', 'a', 'm', 'e', 0x41, 'b',
		0x45, 'a', 'f', 't', 'e', 'r',
	}
	decoder := mmdbdata.NewDecoder(data, 0)
	var record Record
	next, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor())
	if err != nil {
		t.Fatal(err)
	}
	values := record.Groups["set"]
	if len(values) != 2 || values[0] == nil || values[1] == nil ||
		values[0].Name != "a" || values[1].Name != "b" {
		t.Fatalf("unexpected groups: %#v", record.Groups)
	}
	trailing, _, err := next.ReadString()
	if err != nil {
		t.Fatal(err)
	}
	if trailing != "after" {
		t.Fatalf("trailing value %q, want %q", trailing, "after")
	}
}
`), 0o600))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGeneratedSuccessfulSliceReuseClearsHiddenTail(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Value struct {
	Name string `+"`maxminddb:\"name\"`"+`
}

type Record struct {
	Values []*Value `+"`maxminddb:\"values\"`"+`
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record_test.go"), []byte(`package fixture

import (
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestSuccessfulSliceReuseClearsHiddenTail(t *testing.T) {
	data := []byte{
		0xe1,
		0x46, 'v', 'a', 'l', 'u', 'e', 's',
		0x01, 0x04,
		0xe1, 0x44, 'n', 'a', 'm', 'e', 0x43, 'n', 'e', 'w',
		0x45, 'a', 'f', 't', 'e', 'r',
	}
	backing := []*Value{
		{Name: "visible"},
		{Name: "hidden one"},
		{Name: "hidden two"},
	}
	record := Record{Values: backing}
	firstElement := &record.Values[0]
	decoder := mmdbdata.NewDecoder(data, 0)
	next, err := record.UnmarshalMaxMindDBCursor(decoder.Cursor())
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Values) != 1 || &record.Values[0] != firstElement {
		t.Fatalf("slice backing array was not reused: %#v", record.Values)
	}
	if record.Values[0] == nil || record.Values[0].Name != "new" {
		t.Fatalf("unexpected decoded slice: %#v", record.Values)
	}
	if backing[1] != nil || backing[2] != nil {
		t.Fatalf("hidden tail retained pointers: %#v", backing)
	}
	trailing, _, err := next.ReadString()
	if err != nil {
		t.Fatal(err)
	}
	if trailing != "after" {
		t.Fatalf("trailing value %q, want %q", trailing, "after")
	}
}
`), 0o600))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGenerateRejectsEmbeddedFields(t *testing.T) {
	t.Run("exported", func(t *testing.T) {
		dir := newTestModule(t, `package fixture

type Embedded struct{ Name string }
type Record struct{ Embedded }
`)
		t.Chdir(dir)
		err := run([]string{"model.go"})
		require.ErrorContains(t, err, "embedded field Embedded is not yet supported")
	})

	t.Run("unexported", func(t *testing.T) {
		dir := newTestModule(t, `package fixture

type embedded struct{ Name string }
type Record struct{ embedded }
`)
		t.Chdir(dir)
		err := run([]string{"model.go"})
		require.ErrorContains(t, err, "embedded field embedded is not yet supported")
	})
}

func TestGenerateRejectsRecursiveStructs(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Node struct {
	Next *Node
}
`)
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "recursive type graph through Node is not supported")
}

func TestGenerateRejectsDuplicateMMDBKeys(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record struct {
	First string `+"`maxminddb:\"same\"`"+`
	Second string `+"`maxminddb:\"same\"`"+`
}
`)
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "conflicts with another field")
	require.ErrorContains(t, err, `MMDB key "same"`)
}

func TestGenerateIgnoresMaxMindDBTextInUnrelatedTag(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\n"+
		"type Record struct {\n"+
		"\tValue string `json:\"maxminddb:value\"`\n"+
		"}\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), `case "Value":`)
	testGeneratedPackage(t)
}

func TestGenerateIgnoresInvalidUTF8InUnrelatedTag(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\n"+
		"type Record struct {\n"+
		"\tName string \"json:\\\"\\xff\\\"\"\n"+
		"}\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), `case "Name":`)
	testGeneratedPackage(t)
}

func TestGenerateIgnoresMalformedUnrelatedPrefixedTag(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\n"+
		"type Record struct {\n"+
		"\tName string `maxminddbx\"name\"`\n"+
		"}\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGenerateRejectsInvalidUTF8MaxMindDBTag(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\n"+
		"type Record struct {\n"+
		"\tName string \"maxminddb:\\\"\\xff\\\"\"\n"+
		"}\n")
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "model.go:4")
	require.ErrorContains(t, err, "invalid UTF-8 maxminddb struct tag")
}

func TestGenerateRejectsMalformedMaxMindDBTags(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{name: "missing colon", tag: `maxminddb"name"`},
		{name: "unterminated", tag: `maxminddb:"name`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newTestModule(
				t,
				"package fixture\n\ntype Record struct {\n\tName string `"+tt.tag+"`\n}\n",
			)
			t.Chdir(dir)
			err := run([]string{"model.go"})
			require.ErrorContains(t, err, "maxminddb struct tag")
		})
	}
}

func TestGenerateSupportsQuotedCommaFieldName(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record struct {
	Value string `+"`maxminddb:\"'city,name'\"`"+`
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model_test.go"), []byte(`package fixture

import (
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestQuotedCommaFieldName(t *testing.T) {
	data := []byte{0xe1, 0x49, 'c', 'i', 't', 'y', ',', 'n', 'a', 'm', 'e', 0x41, 'x'}
	var got Record
	_, err := got.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "x" {
		t.Fatalf("Value = %q, want x", got.Value)
	}
}
`), 0o600))
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated := string(readTestFile(t, "model_maxminddb.go"))
	require.Contains(t, generated, `case "city,name":`)
	testGeneratedPackage(t)
}

func TestGenerateMaxSizeTags(t *testing.T) {
	dir := newTestModule(t, `package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type LimitedString string

var limitedStringCalls int

func (out *LimitedString) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	limitedStringCalls++
	value, next, err := cursor.ReadString()
	*out = LimitedString(value)
	return next, err
}

type Record struct {
	Text        string          `+"`maxminddb:\"text,maxsize:3\"`"+`
	Bytes       []byte          `+"`maxminddb:\"bytes,maxsize:3\"`"+`
	Values      []uint16        `+"`maxminddb:\"values,maxsize:3\"`"+`
	Lookup      map[string]bool `+"`maxminddb:\"lookup,maxsize:2\"`"+`
	TextPointer *string         `+"`maxminddb:\"text_pointer,maxsize:3\"`"+`
	Custom      LimitedString   `+"`maxminddb:\"custom,maxsize:3\"`"+`
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model_test.go"), []byte(`package fixture

import (
	"errors"
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func appendField(data []byte, key string, value []byte) []byte {
	data = append(data, byte(0x40+len(key)))
	data = append(data, key...)
	return append(data, value...)
}

func TestMaxSizeExact(t *testing.T) {
	data := []byte{0xe6}
	data = appendField(data, "text", []byte{0x43, 'a', 'b', 'c'})
	data = appendField(data, "bytes", []byte{0x83, 1, 2, 3})
	data = appendField(data, "values", []byte{0x03, 0x04, 0xa0, 0xa0, 0xa0})
	data = appendField(data, "lookup", []byte{
		0xe2,
		0x41, 'a', 0x00, 0x07,
		0x41, 'b', 0x01, 0x07,
	})
	data = appendField(data, "text_pointer", []byte{0x43, 'd', 'e', 'f'})
	data = appendField(data, "custom", []byte{0x43, 'g', 'h', 'i'})

	limitedStringCalls = 0
	var got Record
	_, err := got.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err != nil {
		t.Fatal(err)
	}
	if limitedStringCalls != 1 {
		t.Fatalf("custom unmarshaler calls = %d, want 1", limitedStringCalls)
	}
	wantPointer := "def"
	want := Record{
		Text:        "abc",
		Bytes:       []byte{1, 2, 3},
		Values:      []uint16{0, 0, 0},
		Lookup:      map[string]bool{"a": false, "b": true},
		TextPointer: &wantPointer,
		Custom:      "ghi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded record: got %#v, want %#v", got, want)
	}
}

func TestMaxSizeRejectsBeforeMutation(t *testing.T) {
	mapValue := []byte{0xe3}
	for key := byte('a'); key <= 'c'; key++ {
		mapValue = append(mapValue, 0x41, key, 0x00, 0x07)
	}
	tests := []struct {
		name  string
		key   string
		value []byte
	}{
		{name: "string", key: "text", value: []byte{0x44, 't', 'e', 'x', 't'}},
		{name: "bytes", key: "bytes", value: []byte{0x84, 1, 2, 3, 4}},
		{name: "bytes array", key: "bytes", value: []byte{0x04, 0x04, 0xa0, 0xa0, 0xa0, 0xa0}},
		{name: "array", key: "values", value: []byte{0x04, 0x04, 0xa0, 0xa0, 0xa0, 0xa0}},
		{name: "map", key: "lookup", value: mapValue},
		{name: "pointer field", key: "text_pointer", value: []byte{0x44, 't', 'e', 'x', 't'}},
		{name: "custom unmarshaler", key: "custom", value: []byte{0x44, 't', 'e', 'x', 't'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keep := "keep"
			newRecord := func() Record {
				return Record{
					Text:        "keep",
					Bytes:       []byte{9},
					Values:      []uint16{9},
					Lookup:      map[string]bool{"keep": true},
					TextPointer: &keep,
					Custom:      "keep",
				}
			}
			got := newRecord()
			want := newRecord()
			data := appendField([]byte{0xe1}, tt.key, tt.value)
			limitedStringCalls = 0
			_, err := got.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
			if err == nil {
				t.Fatal("expected maxsize error")
			}
			var invalid mmdbdata.InvalidDatabaseError
			if !errors.As(err, &invalid) {
				t.Fatalf("error type = %T, want InvalidDatabaseError: %v", err, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("destination mutated: got %#v, want %#v", got, want)
			}
			if tt.key == "custom" && limitedStringCalls != 0 {
				t.Fatalf("custom unmarshaler called %d times before maxsize rejection", limitedStringCalls)
			}
		})
	}
}
`), 0o600))
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated := string(readTestFile(t, "model_maxminddb.go"))
	require.Contains(t, generated, "ReadStringMaxSize(3)")
	require.Contains(
		t,
		generated,
		"CheckMaxSize(mmdbdata.NewKindSet(mmdbdata.KindBytes, mmdbdata.KindSlice), 3)",
	)
	require.Contains(t, generated, "SliceMaxSize(3)")
	require.Contains(t, generated, "CheckMaxSize(mmdbdata.NewKindSet(mmdbdata.KindMap), 2)")
	testGeneratedPackage(t)
}

func TestGenerateMaxSizeCustomContainerTypes(t *testing.T) {
	//nolint:dupword // The generated fixture intentionally repeats its declared type names.
	dir := newTestModule(t, `package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type CursorMap map[string]bool

var cursorMapCalls int

func (out *CursorMap) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	cursorMapCalls++
	next, err := cursor.Skip()
	if err == nil {
		*out = CursorMap{"decoded": true}
	}
	return next, err
}

type CursorSlice []bool

var cursorSliceCalls int

func (out *CursorSlice) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	cursorSliceCalls++
	next, err := cursor.Skip()
	if err == nil {
		*out = CursorSlice{true, false}
	}
	return next, err
}

type CursorBytes []byte

var cursorBytesCalls int

func (out *CursorBytes) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	cursorBytesCalls++
	value, next, err := cursor.ReadBytes()
	if err == nil {
		*out = append((*out)[:0], value...)
	}
	return next, err
}

type LegacyMap map[string]bool

var legacyMapCalls int

func (out *LegacyMap) UnmarshalMaxMindDB(decoder *mmdbdata.Decoder) error {
	legacyMapCalls++
	if err := decoder.SkipValue(); err != nil {
		return err
	}
	*out = LegacyMap{"decoded": true}
	return nil
}

type Record struct {
	CursorMap   CursorMap   `+"`maxminddb:\"cursor_map,maxsize:2\"`"+`
	CursorSlice CursorSlice `+"`maxminddb:\"cursor_slice,maxsize:2\"`"+`
	CursorBytes CursorBytes `+"`maxminddb:\"cursor_bytes,maxsize:2\"`"+`
	LegacyMap   LegacyMap   `+"`maxminddb:\"legacy_map,maxsize:2\"`"+`
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model_test.go"), []byte(`package fixture

import (
	"errors"
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func appendField(data []byte, key string, value []byte) []byte {
	data = append(data, byte(0x40+len(key)))
	data = append(data, key...)
	return append(data, value...)
}

func mapValue(size int) []byte {
	data := []byte{0xe0 | byte(size)}
	for i := range size {
		data = append(data, 0x41, byte('a'+i), 0x00, 0x07)
	}
	return data
}

func sliceValue(size int) []byte {
	data := []byte{byte(size), 0x04}
	for range size {
		data = append(data, 0x00, 0x07)
	}
	return data
}

func TestMaxSizeCustomContainersExact(t *testing.T) {
	data := []byte{0xe4}
	data = appendField(data, "cursor_map", mapValue(2))
	data = appendField(data, "cursor_slice", sliceValue(2))
	data = appendField(data, "cursor_bytes", []byte{0x82, 1, 2})
	data = appendField(data, "legacy_map", mapValue(2))

	cursorMapCalls = 0
	cursorSliceCalls = 0
	cursorBytesCalls = 0
	legacyMapCalls = 0
	var got Record
	_, err := got.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err != nil {
		t.Fatal(err)
	}
	want := Record{
		CursorMap:   CursorMap{"decoded": true},
		CursorSlice: CursorSlice{true, false},
		CursorBytes: CursorBytes{1, 2},
		LegacyMap:   LegacyMap{"decoded": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded record: got %#v, want %#v", got, want)
	}
	if cursorMapCalls != 1 || cursorSliceCalls != 1 || cursorBytesCalls != 1 || legacyMapCalls != 1 {
		t.Fatalf(
			"callback calls = (%d, %d, %d, %d), want (1, 1, 1, 1)",
			cursorMapCalls,
			cursorSliceCalls,
			cursorBytesCalls,
			legacyMapCalls,
		)
	}
}

func TestMaxSizeCustomContainersRejectBeforeCallback(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value []byte
	}{
		{name: "cursor map", key: "cursor_map", value: mapValue(3)},
		{name: "cursor slice", key: "cursor_slice", value: sliceValue(3)},
		{name: "cursor bytes", key: "cursor_bytes", value: []byte{0x83, 1, 2, 3}},
		{name: "legacy map", key: "legacy_map", value: mapValue(3)},
	}
	newRecord := func() Record {
		return Record{
			CursorMap:   CursorMap{"keep": true},
			CursorSlice: CursorSlice{true},
			CursorBytes: CursorBytes{9},
			LegacyMap:   LegacyMap{"keep": true},
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursorMapCalls = 0
			cursorSliceCalls = 0
			cursorBytesCalls = 0
			legacyMapCalls = 0
			got := newRecord()
			want := newRecord()
			data := appendField([]byte{0xe1}, tt.key, tt.value)
			_, err := got.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
			var invalid mmdbdata.InvalidDatabaseError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v, want InvalidDatabaseError", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("destination mutated: got %#v, want %#v", got, want)
			}
			if cursorMapCalls != 0 || cursorSliceCalls != 0 || cursorBytesCalls != 0 || legacyMapCalls != 0 {
				t.Fatalf(
					"callback calls = (%d, %d, %d, %d), want (0, 0, 0, 0)",
					cursorMapCalls,
					cursorSliceCalls,
					cursorBytesCalls,
					legacyMapCalls,
				)
			}
		})
	}
}
`), 0o600))
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated := string(readTestFile(t, "model_maxminddb.go"))
	require.Contains(
		t,
		generated,
		"NewKindSet(mmdbdata.KindMap, mmdbdata.KindSlice, mmdbdata.KindString, mmdbdata.KindBytes)",
	)
	testGeneratedPackage(t)
}

func TestGenerateRejectsInvalidMaxSizeTags(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{
			name:    "unsupported type",
			field:   "Value uint16 `maxminddb:\"value,maxsize:1\"`",
			wantErr: "only supported",
		},
		{
			name:    "underscore",
			field:   "Value string `maxminddb:\"value,max_size:1\"`",
			wantErr: `specify "maxsize"`,
		},
		{
			name:    "equals",
			field:   "Value string `maxminddb:\"value,maxsize=1\"`",
			wantErr: "missing value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newTestModule(t, "package fixture\n\ntype Record struct {\n\t"+tt.field+"\n}\n")
			t.Chdir(dir)
			err := run([]string{"model.go"})
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestRunPreservesBuildConstraints(t *testing.T) {
	source := "//go:build " + runtime.GOOS + " && !maxminddb_generator_excluded\n\n" +
		"package fixture\n\ntype Record struct{}\n"
	dir := newTestModule(t, source)
	input := "model_" + runtime.GOOS + ".go"
	require.NoError(t, os.Rename(filepath.Join(dir, "model.go"), filepath.Join(dir, input)))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "common.go"),
		[]byte("package fixture\n"),
		0o600,
	))

	t.Chdir(dir)
	require.NoError(t, run([]string{input}))
	output := "model_maxminddb_" + runtime.GOOS + ".go"
	generated, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(generated), "//go:build ")
	require.Contains(t, string(generated), runtime.GOOS)
	require.Contains(t, string(generated), "!maxminddb_generator_excluded")
	require.NoError(t, run([]string{input}))
	regenerated, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, generated, regenerated)
	testGeneratedPackage(t)

	otherOS := "windows"
	if runtime.GOOS == otherOS {
		otherOS = "linux"
	}
	testGeneratedPackageWithEnv(
		t,
		"GOOS="+otherOS,
		"GOARCH=amd64",
		"CGO_ENABLED=0",
	)
}

func TestRunCombinesLegacyMultipleFileBuildConstraints(t *testing.T) {
	legacyConstraint := runtime.GOOS
	modernConstraint := runtime.GOARCH
	dir := newTestModule(t, "// +build "+legacyConstraint+"\n\n"+
		"package fixture\n\ntype First struct { Name string }\n")
	require.NoError(t, os.Rename(
		filepath.Join(dir, "model.go"),
		filepath.Join(dir, "first.go"),
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "second.go"),
		[]byte("//go:build "+modernConstraint+"\n\n"+
			"package fixture\n\ntype Second struct { Count uint }\n"),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "common.go"),
		[]byte("package fixture\n"),
		0o600,
	))

	t.Chdir(dir)
	const output = "combined_maxminddb.go"
	require.NoError(t, run([]string{
		"-output", output, "first.go", "second.go",
	}))
	generated, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(
		t,
		string(generated),
		"//go:build "+legacyConstraint+" && "+modernConstraint,
	)
	testGeneratedPackage(t)

	otherOS := "windows"
	if runtime.GOOS == otherOS {
		otherOS = "linux"
	}
	testGeneratedPackageWithEnv(
		t,
		"GOOS="+otherOS,
		"GOARCH=amd64",
		"CGO_ENABLED=0",
	)
}

func TestSplitBuildSuffix(t *testing.T) {
	tests := []struct {
		input      string
		wantStem   string
		wantSuffix string
		wantTag    string
	}{
		{input: "models.go", wantStem: "models"},
		{input: "models_types.go", wantStem: "models_types"},
		{
			input:      "models_linux.go",
			wantStem:   "models",
			wantSuffix: "_linux",
			wantTag:    "linux",
		},
		{
			input:      "models_linux_amd64.go",
			wantStem:   "models",
			wantSuffix: "_linux_amd64",
			wantTag:    "linux && amd64",
		},
		{
			input:      "models_amd64.go",
			wantStem:   "models",
			wantSuffix: "_amd64",
			wantTag:    "amd64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			stem, suffix, tag := splitBuildSuffix(tt.input)
			require.Equal(t, tt.wantStem, stem)
			require.Equal(t, tt.wantSuffix, suffix)
			require.Equal(t, tt.wantTag, tag)
		})
	}
}

func TestGenerateDoesNotReplaceHandwrittenOutput(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	output := filepath.Join(dir, "model_maxminddb.go")
	const handwritten = "package fixture\n\nconst Handwritten = true\n"
	require.NoError(t, os.WriteFile(output, []byte(handwritten), 0o600))
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "was not created by maxminddb-gen")
	contents, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	require.Equal(t, handwritten, string(contents))
}

func TestGenerateRejectsNearGeneratedMarker(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	output := filepath.Join(dir, "model_maxminddb.go")
	const handwritten = `// Code generated by maxminddb-generator documentation; this file is maintained manually.
package fixture

const PreserveMe = true
`
	require.NoError(t, os.WriteFile(output, []byte(handwritten), 0o600))
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "was not created by maxminddb-gen")
	contents, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	require.Equal(t, handwritten, string(contents))
}

func TestGenerateReplacesOlderGeneratedVersion(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	output := filepath.Join(dir, "model_maxminddb.go")
	const previous = `// Code generated by maxminddb-gen 0.0.1; DO NOT EDIT.
package fixture

const StaleGeneratedDeclaration = true
`
	require.NoError(t, os.WriteFile(output, []byte(previous), 0o600))
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	contents, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(contents), generatedPrefix+"; DO NOT EDIT.")
	require.Contains(t, string(contents), "func (out *Record) UnmarshalMaxMindDB")
	require.NotContains(t, string(contents), "StaleGeneratedDeclaration")
	testGeneratedPackage(t)
}

type failingAtomicFile struct {
	*os.File

	err       error
	operation string
}

func (f *failingAtomicFile) Write(contents []byte) (int, error) {
	if f.operation == "write" {
		return 0, f.err
	}
	return f.File.Write(contents)
}

func (f *failingAtomicFile) Chmod(mode os.FileMode) error {
	if f.operation == "chmod" {
		return f.err
	}
	return f.File.Chmod(mode)
}

func (f *failingAtomicFile) Sync() error {
	if f.operation == "sync" {
		return f.err
	}
	return f.File.Sync()
}

func (f *failingAtomicFile) Close() error {
	err := f.File.Close()
	if f.operation == "close" {
		return f.err
	}
	return err
}

func TestWriteAtomicFailurePreservesOutput(t *testing.T) {
	for _, operation := range []string{"write", "chmod", "sync", "close", "replace"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "model_maxminddb.go")
			original := []byte("original output\n")
			require.NoError(t, os.WriteFile(output, original, 0o600))
			failure := errors.New("controlled atomic output failure")

			createTemp := func(dir, pattern string) (atomicOutputFile, error) {
				file, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				return &failingAtomicFile{
					File:      file,
					err:       failure,
					operation: operation,
				}, nil
			}
			replace := replaceFile
			if operation == "replace" {
				replace = func(string, string) error { return failure }
			}

			err := writeAtomicWith(output, []byte("replacement output\n"), createTemp, replace)
			require.ErrorIs(t, err, failure)
			contents, readErr := os.ReadFile(output)
			require.NoError(t, readErr)
			require.Equal(t, original, contents)
			tempFiles, globErr := filepath.Glob(filepath.Join(dir, ".maxminddb-gen-*"))
			require.NoError(t, globErr)
			require.Empty(t, tempFiles)
		})
	}
}

func TestRegenerationPreservesGeneratedMethodSignaturesDuringLoad(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct { Name string }\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	first := readTestFile(t, "model_maxminddb.go")

	usage := []byte(`package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

var _ mmdbdata.Unmarshaler = (*Record)(nil)
var _ mmdbdata.CursorUnmarshaler = (*Record)(nil)

func decodeRecord(record *Record, decoder *mmdbdata.Decoder) error {
	return record.UnmarshalMaxMindDB(decoder)
}

func decodeRecordCursor(
	record *Record,
	cursor mmdbdata.Cursor,
) (mmdbdata.Cursor, error) {
	return record.UnmarshalMaxMindDBCursor(cursor)
}
`)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "generated_api_usage.go"),
		usage,
		0o600,
	))

	require.NoError(t, run([]string{"model.go"}))
	require.Equal(t, first, readTestFile(t, "model_maxminddb.go"))
	testGeneratedPackage(t)
}

func TestRegenerationPreservesOutputWhenRemovedTargetIsReferenced(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Old struct { Name string }
type New struct { Name string }
`)
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	previous := readTestFile(t, "model_maxminddb.go")

	usage := []byte(`package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

func decodeOld(value *Old, decoder *mmdbdata.Decoder) error {
	return value.UnmarshalMaxMindDB(decoder)
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage.go"), usage, 0o600))

	model := []byte(`package fixture

//maxminddb:ignore Old
type Old struct { Name string }
type New struct { Name string }
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.go"), model, 0o600))

	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "validating generated package")
	require.Equal(t, previous, readTestFile(t, "model_maxminddb.go"))
	testGeneratedPackage(t)

	require.NoError(t, os.Remove(filepath.Join(dir, "usage.go")))
	require.NoError(t, run([]string{"model.go"}))
	generated := readTestFile(t, "model_maxminddb.go")
	require.NotContains(t, string(generated), "func (out *Old) UnmarshalMaxMindDB")
	require.Contains(t, string(generated), "func (out *New) UnmarshalMaxMindDB")
	testGeneratedPackage(t)
}

func TestRegenerationCanonicalizesCompatibilityAliasReceiver(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Old struct { Name string }\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))

	model := []byte(`package fixture

type New struct { Name string }
type Old = New
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.go"), model, 0o600))
	usage := []byte(`package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

var _ mmdbdata.Unmarshaler = (*New)(nil)
var _ mmdbdata.Unmarshaler = (*Old)(nil)
var _ mmdbdata.CursorUnmarshaler = (*New)(nil)
var _ mmdbdata.CursorUnmarshaler = (*Old)(nil)
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage.go"), usage, 0o600))

	const output = "renamed_maxminddb.go"
	require.NoError(t, run([]string{"-output", output, "model.go"}))
	generated := readTestFile(t, output)
	require.Contains(t, string(generated), "func (out *New) UnmarshalMaxMindDB")
	require.NotContains(t, string(generated), "func (out *Old) UnmarshalMaxMindDB")
	require.NoError(t, os.Remove("model_maxminddb.go"))
	testGeneratedPackage(t)
}

func TestRegenerationDropsInvalidAliasReceivers(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		external bool
		wantNew  bool
	}{
		{
			name: "pointer",
			model: `package fixture

type New struct { Name string }
type Old = *New
type Keep struct { Name string }
`,
			wantNew: true,
		},
		{
			name: "instantiated generic",
			model: `package fixture

//maxminddb:ignore New
type New[T any] struct { Value T }
type Old = New[int]
type Keep struct { Name string }
`,
		},
		{
			name: "external",
			model: `package fixture

import external "example.com/generatortest/external"

type Old = external.New
type Keep struct { Name string }
`,
			external: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newTestModule(t, `package fixture

type Old struct { Name string }
type Keep struct { Name string }
`)
			if tt.external {
				writeTestPackage(t, dir, "external", "package external\n\ntype New struct{}\n")
			}
			t.Chdir(dir)
			require.NoError(t, run([]string{"model.go"}))
			require.NoError(t, os.WriteFile("model.go", []byte(tt.model), 0o600))

			require.NoError(t, run([]string{"model.go"}))
			generated := readTestFile(t, "model_maxminddb.go")
			require.NotContains(t, string(generated), "func (out *Old) UnmarshalMaxMindDB")
			require.Contains(t, string(generated), "func (out *Keep) UnmarshalMaxMindDB")
			if tt.wantNew {
				require.Contains(t, string(generated), "func (out *New) UnmarshalMaxMindDB")
			}
			testGeneratedPackage(t)
		})
	}
}

func TestRegenerationDropsInvalidDefinedReceivers(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		external bool
	}{
		{
			name: "direct pointer",
			model: `package fixture

type Old *int
type Keep struct { Name string }
`,
		},
		{
			name: "indirect pointer",
			model: `package fixture

type Base *int
type Old Base
type Keep struct { Name string }
`,
		},
		{
			name: "direct interface",
			model: `package fixture

type Old interface{}
type Keep struct { Name string }
`,
		},
		{
			name: "indirect interface",
			model: `package fixture

type Interface interface{}
type Old Interface
type Keep struct { Name string }
`,
		},
		{
			name: "predeclared interface",
			model: `package fixture

type Old any
type Keep struct { Name string }
`,
		},
		{
			name: "imported pointer",
			model: `package fixture

import external "example.com/generatortest/external"

type Old external.Pointer
type Keep struct { Name string }
`,
			external: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newTestModule(t, `package fixture

type Old struct { Name string }
type Keep struct { Name string }
`)
			if tt.external {
				writeTestPackage(t, dir, "external", "package external\n\ntype Pointer *int\n")
			}
			t.Chdir(dir)
			require.NoError(t, run([]string{"model.go"}))
			require.NoError(t, os.WriteFile("model.go", []byte(tt.model), 0o600))

			require.NoError(t, run([]string{"model.go"}))
			generated := readTestFile(t, "model_maxminddb.go")
			require.NotContains(t, string(generated), "func (out *Old) UnmarshalMaxMindDB")
			require.Contains(t, string(generated), "func (out *Keep) UnmarshalMaxMindDB")
			testGeneratedPackage(t)
		})
	}
}

func TestRegenerationSupportsHandwrittenTakeover(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Takeover struct { Name string }
type Generated struct { Name string }
`)
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	previous := readTestFile(t, "model_maxminddb.go")

	legacyMethod := []byte(`package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

func (*Takeover) UnmarshalMaxMindDB(*mmdbdata.Decoder) error { return nil }
`)
	require.NoError(t, os.WriteFile("handwritten.go", legacyMethod, 0o600))
	usage := []byte(`package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

func decodeTakeoverCursor(
	value *Takeover,
	cursor mmdbdata.Cursor,
) (mmdbdata.Cursor, error) {
	return value.UnmarshalMaxMindDBCursor(cursor)
}
`)
	require.NoError(t, os.WriteFile("usage.go", usage, 0o600))

	var diagnostics bytes.Buffer
	err := runWithIO([]string{"model.go"}, io.Discard, &diagnostics)
	require.ErrorContains(t, err, "validating generated package")
	require.Contains(t, diagnostics.String(), "skipping Takeover")
	require.Equal(t, previous, readTestFile(t, "model_maxminddb.go"))

	completeTakeover := []byte(`package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

func (*Takeover) UnmarshalMaxMindDB(*mmdbdata.Decoder) error { return nil }

func (*Takeover) UnmarshalMaxMindDBCursor(
	cursor mmdbdata.Cursor,
) (mmdbdata.Cursor, error) {
	return cursor, nil
}
`)
	require.NoError(t, os.WriteFile("handwritten.go", completeTakeover, 0o600))
	diagnostics.Reset()
	require.NoError(t, runWithIO([]string{"model.go"}, io.Discard, &diagnostics))
	require.Contains(t, diagnostics.String(), "skipping Takeover")
	generated := readTestFile(t, "model_maxminddb.go")
	require.NotContains(t, string(generated), "func (out *Takeover) UnmarshalMaxMindDB")
	require.Contains(t, string(generated), "func (out *Generated) UnmarshalMaxMindDB")
	testGeneratedPackage(t)
}

func TestRunDiscoversExportedStructsAndDerivesOutput(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Label string
type private struct{}
type City struct{}
type Enterprise struct{}
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "record_test.go"), []byte(`package fixture

import (
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestEmptyGeneratedStructSkipsFieldsAndReturnsSuccessor(t *testing.T) {
	data := []byte{0xe1, 0x41, 'x', 0x41, 'y', 0x45, 'a', 'f', 't', 'e', 'r'}
	var city City
	next, err := city.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := next.ReadString()
	if err != nil {
		t.Fatal(err)
	}
	if value != "after" {
		t.Fatalf("trailing value = %q", value)
	}
}
`), 0o600))
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))

	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), "func (out *City) UnmarshalMaxMindDB")
	require.Contains(t, string(generated), "func (out *Enterprise) UnmarshalMaxMindDB")
	require.Contains(t, string(generated), "func (out *City) UnmarshalMaxMindDBCursor")
	require.Contains(t, string(generated), "func (out *Enterprise) UnmarshalMaxMindDBCursor")
	require.NotContains(t, string(generated), "*private")
	require.NotContains(t, string(generated), "*Label")
	testGeneratedPackage(t)
}

func TestRunReportsHandwrittenTypesInMixedInput(t *testing.T) {
	dir := newTestModule(t, `package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type Handwritten struct{}
func (*Handwritten) UnmarshalMaxMindDB(*mmdbdata.Decoder) error { return nil }

type Generated struct{ Name string }
`)
	t.Chdir(dir)
	var stderr bytes.Buffer
	require.NoError(t, runWithIO([]string{"model.go"}, io.Discard, &stderr))
	require.Contains(t, stderr.String(), "skipping Handwritten")
	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.NotContains(t, string(generated), "*Handwritten")
	require.Contains(t, string(generated), "*Generated")
}

func TestRunRejectsNoExportedTargets(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype private struct{}\n")
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "no exported struct targets remain")
}

func TestRunHelpAndVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runWithIO([]string{"-h"}, &stdout, &stderr)
	require.ErrorIs(t, err, flag.ErrHelp)
	require.Empty(t, stdout.String())
	require.Equal(t, 1, strings.Count(stderr.String(), "Usage of maxminddb-gen:"))
	require.NotContains(t, stderr.String(), "flag: help requested")

	stdout.Reset()
	stderr.Reset()
	require.NoError(t, runWithIO([]string{"-version"}, &stdout, &stderr))
	require.Equal(t, "maxminddb-gen "+buildVersion()+"\n", stdout.String())
	require.Empty(t, stderr.String())
}

func TestVersionFromBuildInfo(t *testing.T) {
	require.Equal(t, "devel", versionFromBuildInfo(nil, false))
	require.Equal(
		t,
		"devel",
		versionFromBuildInfo(&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true),
	)
	require.Equal(
		t,
		"v2.5.0",
		versionFromBuildInfo(&debug.BuildInfo{Main: debug.Module{Version: "v2.5.0"}}, true),
	)
}

func TestGenerateTracksImportsAndPackageNameCollisions(t *testing.T) {
	dir := newTestModule(t, `package fixture

import (
	"time"

	custom "example.com/generatortest/custom"
	left "example.com/generatortest/first"
	localdata "example.com/generatortest/localdata"
	right "example.com/generatortest/second"
)

type Record struct {
	Delay time.Duration `+"`maxminddb:\"delay\"`"+`
	Ratio float32 `+"`maxminddb:\"ratio\"`"+`
	Bytes []byte `+"`maxminddb:\"bytes\"`"+`
	Custom custom.Value `+"`maxminddb:\"custom\"`"+`
	Left left.Value `+"`maxminddb:\"left\"`"+`
	Right right.Value `+"`maxminddb:\"right\"`"+`
	Local localdata.Value `+"`maxminddb:\"local\"`"+`
}
`)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "collisions.go"),
		[]byte(`package fixture

var (
	errors int
	fmt int
	math int
	mmdbdata int
	_maxminddbgenImport1 int
)
`),
		0o600,
	))
	writeTestPackage(t, dir, "custom", `package custom

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type Value string

func (value *Value) UnmarshalMaxMindDB(decoder *mmdbdata.Decoder) error {
	decoded, err := decoder.ReadString()
	if err == nil {
		*value = Value(decoded)
	}
	return err
}
`)
	writeTestPackage(t, dir, "first", "package collision\n\ntype Value string\n")
	writeTestPackage(t, dir, "second", "package collision\n\ntype Value string\n")
	writeTestPackage(t, dir, "localdata", "package mmdbdata\n\ntype Value string\n")

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestIsThirdPartyImport(t *testing.T) {
	tests := map[string]bool{
		"errors":                        false,
		"encoding/json":                 false,
		"example/module":                false,
		"example.com/module":            true,
		"github.com/example/dependency": true,
	}
	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			require.Equal(t, want, isThirdPartyImport(path))
		})
	}
}

func TestGeneratedImportAliasesCannotBeShadowed(t *testing.T) {
	dir := newTestModule(t, `package fixture

import (
	outpkg "example.com/generatortest/out"
	stringpkg "example.com/generatortest/stringpkg"
)

type Record struct {
	Out outpkg.Value
	String stringpkg.Value
}
`)
	writeTestPackage(t, dir, "out", "package out\n\ntype Value string\n")
	writeTestPackage(t, dir, "stringpkg", "package string\n\ntype Value string\n")

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated := readTestFile(t, "model_maxminddb.go")
	testGeneratedPackage(t)

	require.NoError(t, run([]string{"model.go"}))
	require.Equal(t, generated, readTestFile(t, "model_maxminddb.go"))
}

func TestRunHonorsOutputOverride(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"-output", "custom_generated.go", "model.go"}))
	_, err := os.Stat("custom_generated.go")
	require.NoError(t, err)
}

func TestRunPerFileGenerationIsOrderIndependent(t *testing.T) {
	type outputs struct {
		record []byte
		nested []byte
	}
	generate := func(t *testing.T, order []string) outputs {
		t.Helper()
		dir := newTestModule(t, `package fixture

type Record struct {
	Nested Nested `+"`maxminddb:\"nested\"`"+`
}
`)
		require.NoError(t, os.Rename(
			filepath.Join(dir, "model.go"),
			filepath.Join(dir, "record.go"),
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "nested.go"),
			[]byte(`package fixture

type Nested struct {
	Name string `+"`maxminddb:\"name\"`"+`
}
`),
			0o600,
		))
		t.Chdir(dir)
		for _, input := range order {
			require.NoError(t, run([]string{input}))
		}
		first := outputs{
			record: readTestFile(t, "record_maxminddb.go"),
			nested: readTestFile(t, "nested_maxminddb.go"),
		}
		testGeneratedPackage(t)

		for _, input := range order {
			require.NoError(t, run([]string{input}))
		}
		require.Equal(t, first.record, readTestFile(t, "record_maxminddb.go"))
		require.Equal(t, first.nested, readTestFile(t, "nested_maxminddb.go"))
		testGeneratedPackage(t)
		return first
	}

	var parentFirst outputs
	t.Run("parent first", func(t *testing.T) {
		parentFirst = generate(t, []string{"record.go", "nested.go"})
	})
	t.Run("nested first", func(t *testing.T) {
		nestedFirst := generate(t, []string{"nested.go", "record.go"})
		require.Equal(t, parentFirst, nestedFirst)
	})
}

func TestRunPerFileGenerationSupportsSharedNestedType(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Alpha struct { Value Nested }\n")
	require.NoError(t, os.Rename(
		filepath.Join(dir, "model.go"),
		filepath.Join(dir, "alpha.go"),
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "beta.go"),
		[]byte("package fixture\n\ntype Beta struct { Value Nested }\n"),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "nested.go"),
		[]byte("package fixture\n\ntype Nested struct { Name string }\n"),
		0o600,
	))
	t.Chdir(dir)
	for _, input := range []string{"alpha.go", "beta.go", "nested.go"} {
		require.NoError(t, run([]string{input}))
	}
	outputs := map[string][]byte{
		"alpha":  readTestFile(t, "alpha_maxminddb.go"),
		"beta":   readTestFile(t, "beta_maxminddb.go"),
		"nested": readTestFile(t, "nested_maxminddb.go"),
	}
	testGeneratedPackage(t)

	for _, input := range []string{"nested.go", "beta.go", "alpha.go"} {
		require.NoError(t, run([]string{input}))
	}
	for name, want := range outputs {
		require.Equal(t, want, readTestFile(t, name+"_maxminddb.go"))
	}
	testGeneratedPackage(t)
}

func TestGenerateRejectsNestedHelperCollision(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct { Value Nested }\n")
	require.NoError(t, os.Rename(
		filepath.Join(dir, "model.go"),
		filepath.Join(dir, "record.go"),
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "nested.go"),
		[]byte("package fixture\n\ntype Nested struct { Name string }\n"),
		0o600,
	))
	t.Chdir(dir)
	require.NoError(t, run([]string{"record.go"}))
	helper := generatedPointerParameterHelper(
		t,
		readTestFile(t, "record_maxminddb.go"),
		"Nested",
	)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "collision.go"),
		[]byte("package fixture\n\nfunc "+helper+"() {}\n"),
		0o600,
	))
	err := run([]string{"record.go"})
	require.ErrorContains(t, err, "generated helper "+helper+" collides")
}

func TestGenerateSupportsByteHelpersAlongsideNestedByteNames(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record struct {
	Raw []byte `+"`maxminddb:\"raw\"`"+`
	Bytes Bytes `+"`maxminddb:\"bytes\"`"+`
	ByteSlice ByteSlice `+"`maxminddb:\"byte_slice\"`"+`
}
`)
	require.NoError(t, os.Rename(
		filepath.Join(dir, "model.go"),
		filepath.Join(dir, "record.go"),
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "nested.go"),
		[]byte(`package fixture

type Bytes struct { Name string `+"`maxminddb:\"name\"`"+` }
type ByteSlice struct { Name string `+"`maxminddb:\"name\"`"+` }
`),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "record_test.go"),
		[]byte(`package fixture

import (
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestDecode(t *testing.T) {
	data := []byte{
		0xe3,
		0x43, 'r', 'a', 'w', 0x82, 1, 2,
		0x45, 'b', 'y', 't', 'e', 's', 0xe1, 0x44, 'n', 'a', 'm', 'e', 0x41, 'b',
		0x4a, 'b', 'y', 't', 'e', '_', 's', 'l', 'i', 'c', 'e', 0xe1, 0x44, 'n', 'a', 'm', 'e', 0x41, 's',
	}
	var record Record
	_, err := record.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record.Raw, []byte{1, 2}) || record.Bytes.Name != "b" || record.ByteSlice.Name != "s" {
		t.Fatalf("decoded %#v", record)
	}
}
`),
		0o600,
	))

	t.Chdir(dir)
	require.NoError(t, run([]string{"record.go"}))
	testGeneratedPackage(t)
}

func TestGenerateDiscoversOnlyNestedBytesAndFloat32(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Record struct {
	BytePointer *[]byte `+"`maxminddb:\"byte_pointer\"`"+`
	ByteSlices [][]byte `+"`maxminddb:\"byte_slices\"`"+`
	ByteMap map[string][]byte `+"`maxminddb:\"byte_map\"`"+`
	FloatPointer *float32 `+"`maxminddb:\"float_pointer\"`"+`
	FloatSlices []float32 `+"`maxminddb:\"float_slices\"`"+`
	FloatMap map[string]float32 `+"`maxminddb:\"float_map\"`"+`
}
`)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "model_test.go"),
		[]byte(`package fixture

import (
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestDecode(t *testing.T) {
	f := []byte{0x04, 0x08, 0x3f, 0xc0, 0x00, 0x00}
	data := []byte{0xe6}
	appendKey := func(key string) {
		data = append(data, byte(0x40+len(key)))
		data = append(data, key...)
	}
	appendKey("byte_pointer")
	data = append(data, 0x82, 1, 2)
	appendKey("byte_slices")
	data = append(data, 0x02, 0x04, 0x81, 3, 0x81, 4)
	appendKey("byte_map")
	data = append(data, 0xe1, 0x41, 'k', 0x81, 5)
	appendKey("float_pointer")
	data = append(data, f...)
	appendKey("float_slices")
	data = append(data, 0x01, 0x04)
	data = append(data, f...)
	appendKey("float_map")
	data = append(data, 0xe1, 0x41, 'k')
	data = append(data, f...)

	var record Record
	_, err := record.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err != nil {
		t.Fatal(err)
	}
	if record.BytePointer == nil || !reflect.DeepEqual(*record.BytePointer, []byte{1, 2}) ||
		!reflect.DeepEqual(record.ByteSlices, [][]byte{{3}, {4}}) ||
		!reflect.DeepEqual(record.ByteMap, map[string][]byte{"k": {5}}) ||
		record.FloatPointer == nil || *record.FloatPointer != 1.5 ||
		!reflect.DeepEqual(record.FloatSlices, []float32{1.5}) ||
		!reflect.DeepEqual(record.FloatMap, map[string]float32{"k": 1.5}) {
		t.Fatalf("decoded %#v", record)
	}
}
`),
		0o600,
	))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestGenerateSupportsCustomMapKeys(t *testing.T) {
	dir := newTestModule(t, `package fixture

import (
	"errors"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

type CursorKey string

func (key *CursorKey) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	value, next, err := cursor.ReadString()
	if err == nil { *key = CursorKey("cursor:" + value) }
	return next, err
}

type LegacyKey string

func (key *LegacyKey) UnmarshalMaxMindDB(decoder *mmdbdata.Decoder) error {
	value, err := decoder.ReadString()
	if err == nil { *key = LegacyKey("legacy:" + value) }
	return err
}

type DualKey string

func (key *DualKey) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	value, next, err := cursor.ReadString()
	if err == nil { *key = DualKey("cursor:" + value) }
	return next, err
}

func (key *DualKey) UnmarshalMaxMindDB(decoder *mmdbdata.Decoder) error {
	value, err := decoder.ReadString()
	if err == nil { *key = DualKey("legacy:" + value) }
	return err
}

type SkipKey string

func (*SkipKey) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	return cursor.Skip()
}

var ErrKeyCallback = errors.New("key callback failed")

type ErrorKey string

func (*ErrorKey) UnmarshalMaxMindDBCursor(cursor mmdbdata.Cursor) (mmdbdata.Cursor, error) {
	return cursor, ErrKeyCallback
}

type Record struct {
	Cursor map[CursorKey]bool `+"`maxminddb:\"cursor\"`"+`
	Legacy map[LegacyKey]bool `+"`maxminddb:\"legacy\"`"+`
	Dual map[DualKey]bool `+"`maxminddb:\"dual\"`"+`
	Skip map[SkipKey]bool `+"`maxminddb:\"skip\"`"+`
	Failure map[ErrorKey]bool `+"`maxminddb:\"failure\"`"+`
}
`)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "model_test.go"),
		[]byte(`package fixture

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func appendKey(data []byte, key string) []byte {
	data = append(data, byte(0x40+len(key)))
	return append(data, key...)
}

func appendLargeMapHeader(data []byte, size int) []byte {
	extended := size - 285
	return append(data, 0xfe, byte(extended>>8), byte(extended))
}

func TestCustomKeysAndSuccessor(t *testing.T) {
	data := []byte{0xe3}
	for _, field := range []string{"cursor", "legacy", "dual"} {
		data = appendKey(data, field)
		data = append(data, 0xe1, 0x42, 'e', 'n', 0x00, 0x07)
	}
	data = append(data, 0x45, 'a', 'f', 't', 'e', 'r')
	var record Record
	next, err := record.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err != nil { t.Fatal(err) }
	_, cursorOK := record.Cursor["cursor:en"]
	_, legacyOK := record.Legacy["legacy:en"]
	_, dualOK := record.Dual["cursor:en"]
	if !cursorOK || !legacyOK || !dualOK {
		t.Fatalf("decoded %#v", record)
	}
	trailing, _, err := next.ReadString()
	if err != nil || trailing != "after" { t.Fatalf("trailing %q: %v", trailing, err) }
}

func TestLargeCustomKeyMapBoundaries(t *testing.T) {
	for _, size := range []int{511, 512} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			data := appendKey([]byte{0xe1}, "cursor")
			data = appendLargeMapHeader(data, size)
			for i := range size {
				data = appendKey(data, fmt.Sprintf("%04x", i))
				data = append(data, 0x00, 0x07)
			}
			data = append(data, 0x45, 'a', 'f', 't', 'e', 'r')

			var record Record
			next, err := record.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
			if err != nil { t.Fatal(err) }
			if len(record.Cursor) != size {
				t.Fatalf("decoded %d entries, want %d", len(record.Cursor), size)
			}
			for i := range size {
				key := CursorKey("cursor:" + fmt.Sprintf("%04x", i))
				if value, ok := record.Cursor[key]; !ok || value {
					t.Fatalf("entry %q: value=%v present=%v", key, value, ok)
				}
			}
			trailing, _, err := next.ReadString()
			if err != nil || trailing != "after" {
				t.Fatalf("trailing %q: %v", trailing, err)
			}
		})
	}
}

func TestMalformedRawKeyIsRejected(t *testing.T) {
	data := appendKey([]byte{0xe1}, "skip")
	data = append(data, 0xe1, 0xa1, 1, 0x00, 0x07)
	var record Record
	_, err := record.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err == nil || !strings.Contains(err.Error(), "unexpected map key type: Uint16") {
		t.Fatalf("error %v", err)
	}
}

func TestKeyCallbackError(t *testing.T) {
	data := appendKey([]byte{0xe1}, "failure")
	data = append(data, 0xe1, 0x42, 'e', 'n', 0x00, 0x07)
	var record Record
	_, err := record.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if !errors.Is(err, ErrKeyCallback) || !strings.Contains(err.Error(), "en") {
		t.Fatalf("error %v", err)
	}
}

func TestLargeMalformedCustomKeyMapDoesNotMutateDestination(t *testing.T) {
	data := appendKey([]byte{0xe1}, "cursor")
	data = append(data, 0xfe, 0x00, 0xe3)
	data = append(data, bytes.Repeat([]byte{0xff}, 1024)...)

	var empty Record
	_, err := empty.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err == nil || empty.Cursor != nil {
		t.Fatalf("nil destination after error: map=%#v err=%v", empty.Cursor, err)
	}

	existing := Record{Cursor: map[CursorKey]bool{"keep": true}}
	_, err = existing.UnmarshalMaxMindDBCursor(mmdbdata.NewDecoder(data, 0).Cursor())
	if err == nil || len(existing.Cursor) != 1 || !existing.Cursor["keep"] {
		t.Fatalf("existing destination after error: map=%#v err=%v", existing.Cursor, err)
	}
}
`),
		0o600,
	))

	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	testGeneratedPackage(t)
}

func TestRunMigratesGeneratedOutputPath(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))

	const replacement = "record_decoders.go"
	require.NoError(t, run([]string{"-output", replacement, "model.go"}))
	generated, err := os.ReadFile(replacement)
	require.NoError(t, err)
	require.Contains(t, string(generated), "func (out *Record) UnmarshalMaxMindDB")
	require.Contains(t, string(generated), "func (out *Record) UnmarshalMaxMindDBCursor")
	externalOutput := filepath.Join(t.TempDir(), "record_decoders.go")
	require.NoError(t, run([]string{"-output", externalOutput, "model.go"}))
	externalGenerated, err := os.ReadFile(externalOutput)
	require.NoError(t, err)
	require.Contains(t, string(externalGenerated), "func (out *Record) UnmarshalMaxMindDB")

	require.NoError(t, os.Remove("model_maxminddb.go"))
	testGeneratedPackage(t)
}

func TestRunSupportsIgnoreDirective(t *testing.T) {
	dir := newTestModule(t, `package fixture

//maxminddb:ignore Unsupported
type Unsupported struct { Value chan int }
type Record struct{}
`)
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.NotContains(t, string(generated), "Unsupported")
	require.Contains(t, string(generated), "func (out *Record) UnmarshalMaxMindDB")
}

func TestRunRejectsInvalidIgnoreDirectives(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{
			name:  "missing name",
			model: "package fixture\n\n//maxminddb:ignore\ntype Record struct{}\n",
			want:  "requires at least one type name",
		},
		{
			name:  "unknown name",
			model: "package fixture\n\n//maxminddb:ignore Typo\ntype Record struct{}\n",
			want:  `unknown exported struct "Typo"`,
		},
		{
			name:  "invalid name",
			model: "package fixture\n\n//maxminddb:ignore Not-A-Type\ntype Record struct{}\n",
			want:  `invalid type name "Not-A-Type"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newTestModule(t, tt.model)
			t.Chdir(dir)
			err := run([]string{"model.go"})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestRunAggregatesMultipleFilesWithExplicitOutput(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype City struct{}\n")
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "enterprise.go"),
		[]byte("package fixture\n\ntype Enterprise struct{}\n"),
		0o600,
	))
	t.Chdir(dir)
	err := run([]string{"model.go", "enterprise.go"})
	require.ErrorContains(t, err, "-output is required")

	require.NoError(t, run([]string{
		"-output", "models_maxminddb.go", "model.go", "enterprise.go",
	}))
	generated, err := os.ReadFile("models_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), "func (out *City) UnmarshalMaxMindDB")
	require.Contains(t, string(generated), "func (out *Enterprise) UnmarshalMaxMindDB")
}

func TestRunRejectsInvalidInputs(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "model_test.go"),
		[]byte("package fixture\n"),
		0o600,
	))
	t.Chdir(dir)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing", want: "at least one Go source file is required"},
		{name: "not go", args: []string{"README.md"}, want: "not a Go source file"},
		{name: "test file", args: []string{"model_test.go"}, want: "not supported"},
		{
			name: "duplicate",
			args: []string{"-output", "out.go", "model.go", "model.go"},
			want: "duplicate input file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func newTestModule(t *testing.T, model string) string {
	t.Helper()
	dir := t.TempDir()
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	module := strings.ReplaceAll(`module example.com/generatortest

go 1.25.0

require github.com/oschwald/maxminddb-golang/v2 v2.5.0

replace github.com/oschwald/maxminddb-golang/v2 => REPOSITORY
`, "REPOSITORY", filepath.ToSlash(repository))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.go"), []byte(model), 0o600))
	return dir
}

func writeTestPackage(t *testing.T, root, name, source string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".go"), []byte(source), 0o600))
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return contents
}

func generatedPointerParameterHelper(t *testing.T, source []byte, parameterType string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, 0)
	require.NoError(t, err)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil {
			continue
		}
		for _, parameter := range function.Type.Params.List {
			pointer, ok := parameter.Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			name, ok := pointer.X.(*ast.Ident)
			if ok && name.Name == parameterType {
				return function.Name.Name
			}
		}
	}
	t.Fatalf("generated helper with *%s parameter not found", parameterType)
	return ""
}

func testGeneratedPackage(t *testing.T) {
	t.Helper()
	testGeneratedPackageWithEnv(t)
}

func testGeneratedPackageWithEnv(t *testing.T, env ...string) {
	t.Helper()
	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Env = append(os.Environ(), env...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
}
