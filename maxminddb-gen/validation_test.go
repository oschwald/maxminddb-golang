package main

import (
	"bytes"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/gcexportdata"
	"golang.org/x/tools/go/packages"
)

func TestPackageParsingPreservesDirectives(t *testing.T) {
	for _, tt := range []struct {
		name, source, want string
	}{
		{
			"language version",
			"//go:build go1.22\n\npackage fixture\nfunc f() { for range func(func(int) bool) {} {} }\n",
			"requires go1.23",
		},
		{
			"line directive",
			"package fixture\n//line original.go:123\nvar value int = \"invalid\"\n",
			"original.go:123",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parsePackageFile(fset, "model.go", []byte(tt.source))
			require.NoError(t, err)
			config := types.Config{GoVersion: "go1.26"}
			_, err = config.Check("fixture", fset, []*ast.File{file}, nil)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestLoadedValidationChecksBodiesSizesAndLanguageVersion(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	pkgs, err := packages.Load(&packages.Config{
		Dir: dir,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesSizes | packages.NeedModule |
			packages.NeedImports | packages.NeedExportFile,
	}, ".", mmdbdataPath)
	require.NoError(t, err)
	require.Zero(t, writePackageErrors(nil, pkgs))
	var target, mmdbdata *packages.Package
	for _, pkg := range pkgs {
		if pkg.PkgPath == mmdbdataPath {
			mmdbdata = pkg
		} else {
			target = pkg
		}
	}
	require.NotNil(t, target)
	require.NotNil(t, mmdbdata)
	for _, tt := range []struct {
		name, source, version, want string
	}{
		{"function body", `func broken() int { return "invalid" }`, "1.26", "as int value"},
		{"compiler directive", "//go:noescape\nfunc broken() {}", "1.26", "go:noescape"},
		{"32-bit overflow", "const tooLarge int = 1 << 40", "1.26", "overflows"},
		{"language version", "func generic[T any]() {}", "1.17", "requires go1.18"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pkg := *target
			pkg.TypesSizes = types.SizesFor("gc", "386")
			module := *pkg.Module
			module.GoVersion = tt.version
			pkg.Module = &module
			path := filepath.Join(dir, "generated.go")
			overlay := map[string][]byte{path: []byte("package fixture\n\n" + tt.source + "\n")}
			file, _, err := parseGeneratedFile(pkg.Fset, path, overlay[path])
			require.NoError(t, err)
			var diagnostics bytes.Buffer
			handled, err := validateLoadedPackage(&pkg, mmdbdata, file, path, overlay, &diagnostics)
			require.True(t, handled)
			require.ErrorContains(t, err, "validating generated package")
			require.Contains(t, diagnostics.String(), tt.want)
		})
	}
}

func TestGeneratedImportRetainsInternalPackageChecks(t *testing.T) {
	dir := newTestModule(t, "package fixture\n\ntype Record struct{}\n")
	t.Chdir(dir)
	require.NoError(t, run([]string{"model.go"}))
	previous := readTestFile(t, "model_maxminddb.go")
	writeTestPackage(t, dir, "custom", `package custom

import "example.com/generatortest/custom/internal/secret"

type Texts []secret.Text
`)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "custom", "internal"), 0o700))
	writeTestPackage(
		t,
		filepath.Join(dir, "custom", "internal"),
		"secret",
		"package secret\n\ntype Text string\n",
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.go"), []byte(`package fixture

import "example.com/generatortest/custom"

type Record struct { Value custom.Texts }
`), 0o600))
	var diagnostics bytes.Buffer
	err := runWithIO([]string{"model.go"}, nil, &diagnostics)
	require.ErrorContains(t, err, "validating generated package")
	require.Contains(t, diagnostics.String(), "use of internal package")
	require.Equal(t, previous, readTestFile(t, "model_maxminddb.go"))
}

func TestLoadedPackageImporterReadsExportData(t *testing.T) {
	const path = "example.com/exported"
	source := types.NewPackage(path, "exported")
	source.Scope().Insert(types.NewVar(token.NoPos, source, "Value", types.Typ[types.String]))
	source.MarkComplete()
	var data bytes.Buffer
	require.NoError(t, gcexportdata.Write(&data, token.NewFileSet(), source))
	export := filepath.Join(t.TempDir(), "export")
	require.NoError(t, os.WriteFile(export, data.Bytes(), 0o600))
	partial := types.NewPackage(path, "exported")
	imports := &loadedPackageImporter{
		packages: map[string]*packages.Package{path: {ExportFile: export}},
		types:    map[string]*types.Package{path: partial},
		fset:     token.NewFileSet(),
	}
	loaded, err := imports.Import(path)
	require.NoError(t, err)
	require.Same(t, partial, loaded)
	require.True(t, loaded.Complete())
	require.Equal(t, types.Typ[types.String], loaded.Scope().Lookup("Value").Type())
	require.NoError(t, os.Remove(export))
	again, err := imports.Import(path)
	require.NoError(t, err)
	require.Same(t, loaded, again)
}
