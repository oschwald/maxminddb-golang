package main

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/gcexportdata"
	"golang.org/x/tools/go/packages"
)

// Reuse loaded dependencies, but type-check every file in the completed package.
func validateLoadedPackage(
	pkg *packages.Package,
	mmdbdata *packages.Package,
	generated *ast.File,
	path string,
	overlay map[string][]byte,
	diagnostics io.Writer,
) (bool, error) {
	name := filepath.Base(path)
	_, suffix, _ := splitBuildSuffix(name)
	if pkg.Module == nil || pkg.Module.GoVersion == "" || len(pkg.CompiledGoFiles) == 0 ||
		!slices.Equal(pkg.GoFiles, pkg.CompiledGoFiles) ||
		filepath.Dir(path) != filepath.Dir(pkg.CompiledGoFiles[0]) ||
		filepath.Ext(name) != goFileExtension || strings.HasPrefix(name, ".") ||
		strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_test.go") || suffix != "" {
		return false, nil
	}
	if generatedBuildConstraints(generated) != "" {
		return false, nil
	}

	files := make([]*ast.File, 0, len(pkg.Syntax)+1)
	for i, filename := range pkg.CompiledGoFiles {
		if filename == path {
			continue
		}
		file := pkg.Syntax[i]
		if source, ok := overlay[filename]; ok {
			var err error
			file, err = parsePackageFile(pkg.Fset, filename, source)
			if err != nil {
				return true, err
			}
		}
		files = append(files, file)
	}
	files = append(files, generated)

	// Let the normal loader handle remapped import paths.
	for _, file := range files {
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return true, err
			}
			if imported := pkg.Imports[path]; imported != nil && imported.PkgPath != path {
				return false, nil
			}
		}
	}
	// Keep the compiler and build-system checks without reparsing dependency sources.
	validated, err := packages.Load(&packages.Config{
		Dir:     pkg.Dir,
		Mode:    packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedExportFile,
		Overlay: overlay,
		Tests:   false,
	}, ".")
	if err != nil {
		return true, err
	}
	if writePackageErrors(diagnostics, validated) != 0 || len(validated) != 1 {
		return true, errors.New("validating generated package")
	}
	imports := &loadedPackageImporter{
		packages: make(map[string]*packages.Package),
		types:    make(map[string]*types.Package),
		fset:     pkg.Fset,
	}
	packages.Visit([]*packages.Package{pkg, mmdbdata}, nil, func(dependency *packages.Package) {
		imports.packages[dependency.PkgPath] = dependency
		imports.types[dependency.PkgPath] = dependency.Types
	})
	config := types.Config{
		Importer:  imports,
		Sizes:     pkg.TypesSizes,
		GoVersion: "go" + pkg.Module.GoVersion,
		Error: func(err error) {
			_, _ = fmt.Fprintln(diagnostics, err)
		},
	}
	if _, err := config.Check(pkg.PkgPath, pkg.Fset, files, nil); err != nil {
		return true, errors.New("validating generated package")
	}
	return true, nil
}

type loadedPackageImporter struct {
	packages map[string]*packages.Package
	types    map[string]*types.Package
	fset     *token.FileSet
}

func (imports *loadedPackageImporter) Import(path string) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	if pkg := imports.types[path]; pkg != nil && pkg.Complete() {
		return pkg, nil
	}
	pkg := imports.packages[path]
	if pkg == nil || pkg.ExportFile == "" {
		return nil, fmt.Errorf("package %q was not loaded", path)
	}
	file, err := os.Open(pkg.ExportFile)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	reader := bufio.NewReader(file)
	header, err := reader.Peek(8)
	if err != nil {
		return nil, err
	}
	var data io.Reader = reader
	if string(header) == "!<arch>\n" {
		//nolint:staticcheck // Go 1.26 and 1.27 export files still use the archive format.
		data, err = gcexportdata.NewReader(reader)
		if err != nil {
			return nil, err
		}
	}
	return gcexportdata.Read(data, imports.fset, imports.types, path)
}
