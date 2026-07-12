package main

import (
	"bytes"
	"context"
	"flag"
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

	t.Chdir(fixture)
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
	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), "valueCursor.UnmarshalCursor(&out.Local)")
	require.Contains(t, string(generated), "valueCursor.UnmarshalCursor(&out.External)")
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
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), "out.Bytes = append(out.Bytes[:0]")
	require.Contains(t, string(generated), "out.Named = make([]MyByte")
	require.Contains(t, string(generated), "out.Data = make(Blob")
	testGeneratedPackage(t)
}

func TestGenerateRejectsEmbeddedFields(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Embedded struct{ Name string }
type Record struct{ Embedded }
`)
	t.Chdir(dir)
	err := run([]string{"model.go"})
	require.ErrorContains(t, err, "embedded field Embedded is not yet supported")
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

func TestRunDiscoversExportedStructsAndDerivesOutput(t *testing.T) {
	dir := newTestModule(t, `package fixture

type Label string
type private struct{}
type City struct{}
type Enterprise struct{}
`)
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))

	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), "func (out *City) UnmarshalMaxMindDB")
	require.Contains(t, string(generated), "func (out *Enterprise) UnmarshalMaxMindDB")
	require.Contains(t, string(generated), "func (out *City) UnmarshalMaxMindDBCursor")
	require.Contains(t, string(generated), "func (out *Enterprise) UnmarshalMaxMindDBCursor")
	require.Contains(t, string(generated), "cursor.MapReader()")
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
	Custom custom.Value `+"`maxminddb:\"custom\"`"+`
	Left left.Value `+"`maxminddb:\"left\"`"+`
	Right right.Value `+"`maxminddb:\"right\"`"+`
	Local localdata.Value `+"`maxminddb:\"local\"`"+`
}
`)
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
	generated, err := os.ReadFile("model_maxminddb.go")
	require.NoError(t, err)
	require.Contains(t, string(generated), `"time"`)
	require.Contains(t, string(generated), `collision2 "example.com/generatortest/second"`)
	require.Contains(t, string(generated), `mmdbdata2 "example.com/generatortest/localdata"`)
	testGeneratedPackage(t)
}

func TestRunHonorsOutputOverride(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"-output", "custom_generated.go", "model.go"}))
	_, err := os.Stat("custom_generated.go")
	require.NoError(t, err)
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
