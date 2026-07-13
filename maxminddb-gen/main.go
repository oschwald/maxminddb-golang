// Command maxminddb-gen generates reflection-free MaxMind DB decoders.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
)

const minimumLibrary = "v2.5.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "maxminddb-gen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	return runWithIO(args, os.Stdout, os.Stderr)
}

func runWithIO(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("maxminddb-gen", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var output string
	var showVersion bool
	flags.StringVar(
		&output,
		"output",
		"",
		"generated output file; default derived from the source filename",
	)
	flags.BoolVar(&showVersion, "version", false, "print the generator version")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(stderr)
			flags.Usage()
		}
		return err
	}
	if showVersion {
		_, err := fmt.Fprintf(stdout, "maxminddb-gen %s\n", buildVersion())
		return err
	}

	request, err := discoverTargets(flags.Args(), output)
	if err != nil {
		return err
	}
	return generatePackage(request.targets, request.output, request.buildConstraint, stderr)
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	return versionFromBuildInfo(info, ok)
}

func versionFromBuildInfo(info *debug.BuildInfo, ok bool) string {
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "devel"
	}
	return info.Main.Version
}

type generationRequest struct {
	output          string
	buildConstraint string
	targets         []string
}

func discoverTargets(paths []string, output string) (generationRequest, error) {
	if len(paths) == 0 {
		return generationRequest{}, errors.New("at least one Go source file is required")
	}
	if len(paths) > 1 && output == "" {
		return generationRequest{}, errors.New("-output is required with multiple source files")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return generationRequest{}, err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return generationRequest{}, err
	}

	fset := token.NewFileSet()
	seenPaths := make(map[string]bool, len(paths))
	candidates := make([]string, 0)
	candidateSet := make(map[string]bool)
	ignored := make(map[string]token.Position)
	var buildConstraints []string
	packageName := ""
	for _, path := range paths {
		file, constraints, err := loadInputFile(fset, cwd, path, seenPaths)
		if err != nil {
			return generationRequest{}, err
		}
		for _, buildConstraint := range constraints {
			buildConstraints = appendUnique(buildConstraints, buildConstraint)
		}
		if packageName == "" {
			packageName = file.Name.Name
		} else if file.Name.Name != packageName {
			return generationRequest{}, fmt.Errorf(
				"input %q declares package %s; expected %s",
				path,
				file.Name.Name,
				packageName,
			)
		}

		for _, commentGroup := range file.Comments {
			for _, comment := range commentGroup.List {
				if err := parseIgnoreDirective(fset, comment, ignored); err != nil {
					return generationRequest{}, err
				}
			}
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if typeSpec.Assign.IsValid() || !ast.IsExported(typeSpec.Name.Name) {
					continue
				}
				if _, ok := typeSpec.Type.(*ast.StructType); !ok {
					continue
				}
				candidates = append(candidates, typeSpec.Name.Name)
				candidateSet[typeSpec.Name.Name] = true
			}
		}
	}

	for name, position := range ignored {
		if !candidateSet[name] {
			return generationRequest{}, fmt.Errorf(
				"%s: maxminddb:ignore names unknown exported struct %q",
				position,
				name,
			)
		}
	}
	targets := candidates[:0]
	for _, name := range candidates {
		if _, skip := ignored[name]; !skip {
			targets = append(targets, name)
		}
	}
	if len(targets) == 0 {
		return generationRequest{}, errors.New("no exported struct targets remain after exclusions")
	}

	if output == "" {
		stem, suffix, _ := splitBuildSuffix(filepath.Base(paths[0]))
		output = stem + "_maxminddb" + suffix + ".go"
	}
	return generationRequest{
		targets:         targets,
		output:          output,
		buildConstraint: combineBuildConstraints(buildConstraints),
	}, nil
}

func loadInputFile(
	fset *token.FileSet,
	cwd, path string,
	seenPaths map[string]bool,
) (*ast.File, []string, error) {
	if filepath.Ext(path) != ".go" {
		return nil, nil, fmt.Errorf("input %q is not a Go source file", path)
	}
	if strings.HasSuffix(filepath.Base(path), "_test.go") {
		return nil, nil, fmt.Errorf("test source file %q is not supported", path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	if filepath.Dir(absolute) != cwd {
		return nil, nil, fmt.Errorf("input %q is not in the current package directory", path)
	}
	if seenPaths[absolute] {
		return nil, nil, fmt.Errorf("duplicate input file %q", path)
	}
	seenPaths[absolute] = true

	file, err := parser.ParseFile(fset, absolute, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	explicitConstraint, err := sourceBuildConstraint(file)
	if err != nil {
		return nil, nil, fmt.Errorf("reading build constraint from %s: %w", path, err)
	}
	_, _, suffixConstraint := splitBuildSuffix(filepath.Base(path))
	return file, []string{explicitConstraint, suffixConstraint}, nil
}

func sourceBuildConstraint(file *ast.File) (string, error) {
	var goBuild constraint.Expr
	var plusBuild constraint.Expr
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, comment := range group.List {
			switch {
			case constraint.IsGoBuild(comment.Text):
				expr, err := constraint.Parse(comment.Text)
				if err != nil {
					return "", err
				}
				if goBuild != nil {
					return "", errors.New("multiple //go:build lines")
				}
				goBuild = expr
			case constraint.IsPlusBuild(comment.Text):
				expr, err := constraint.Parse(comment.Text)
				if err != nil {
					return "", err
				}
				if plusBuild == nil {
					plusBuild = expr
				} else {
					plusBuild = &constraint.AndExpr{X: plusBuild, Y: expr}
				}
			default:
			}
		}
	}
	if goBuild != nil {
		return goBuild.String(), nil
	}
	if plusBuild != nil {
		return plusBuild.String(), nil
	}
	return "", nil
}

func appendUnique(values []string, value string) []string {
	if value == "" || slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func combineBuildConstraints(values []string) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return values[0]
	}
	for i, value := range values {
		values[i] = "(" + value + ")"
	}
	return strings.Join(values, " && ")
}

func splitBuildSuffix(filename string) (stem, suffix, buildConstraint string) {
	stem = strings.TrimSuffix(filename, filepath.Ext(filename))
	parts := strings.Split(stem, "_")
	if len(parts) < 2 {
		return stem, "", ""
	}
	last := parts[len(parts)-1]
	if len(parts) >= 3 && knownGOOS[parts[len(parts)-2]] && knownGOARCH[last] {
		goos := parts[len(parts)-2]
		return strings.Join(parts[:len(parts)-2], "_"), "_" + goos + "_" + last,
			goos + " && " + last
	}
	if knownGOOS[last] || knownGOARCH[last] {
		return strings.Join(parts[:len(parts)-1], "_"), "_" + last, last
	}
	return stem, "", ""
}

// These lists mirror the filename suffixes recognized by go/build. They retain
// historical targets because Go continues to reserve those suffixes.
var knownGOOS = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true,
	"freebsd": true, "hurd": true, "illumos": true, "ios": true,
	"js": true, "linux": true, "nacl": true, "netbsd": true,
	"openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
	"windows": true, "zos": true,
}

var knownGOARCH = map[string]bool{
	"386": true, "amd64": true, "amd64p32": true, "arm": true,
	"armbe": true, "arm64": true, "arm64be": true, "loong64": true,
	"mips": true, "mipsle": true, "mips64": true, "mips64le": true,
	"mips64p32": true, "mips64p32le": true, "ppc": true, "ppc64": true,
	"ppc64le": true, "riscv": true, "riscv64": true, "s390": true,
	"s390x": true, "sparc": true, "sparc64": true, "wasm": true,
}

func parseIgnoreDirective(
	fset *token.FileSet,
	comment *ast.Comment,
	ignored map[string]token.Position,
) error {
	text := strings.TrimSpace(comment.Text)
	switch {
	case strings.HasPrefix(text, "//"):
		text = strings.TrimSpace(strings.TrimPrefix(text, "//"))
	case strings.HasPrefix(text, "/*") && strings.HasSuffix(text, "*/"):
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "/*"), "*/"))
	default:
		return nil
	}
	const directive = "maxminddb:ignore"
	if !strings.HasPrefix(text, directive) {
		return nil
	}
	remainder := strings.TrimPrefix(text, directive)
	if remainder != "" && !strings.ContainsAny(remainder[:1], " \t\r\n") {
		return nil
	}
	names := strings.Fields(remainder)
	position := fset.Position(comment.Pos())
	if len(names) == 0 {
		return fmt.Errorf("%s: maxminddb:ignore requires at least one type name", position)
	}
	for _, name := range names {
		if !token.IsIdentifier(name) {
			return fmt.Errorf("%s: invalid type name %q in maxminddb:ignore", position, name)
		}
		if previous, exists := ignored[name]; exists {
			return fmt.Errorf(
				"%s: duplicate maxminddb:ignore for %q (previously at %s)",
				position,
				name,
				previous,
			)
		}
		ignored[name] = position
	}
	return nil
}
