# maxminddb-gen

`maxminddb-gen` generates reflection-free `UnmarshalMaxMindDB` methods for
types owned by the package where generation runs. It is part of the
`maxminddb-golang` module and uses the same version as the decoder support API.
Generation is optional: a type without a generated method continues to use the
reflection decoder.

## Reproducible use

Pin `maxminddb-golang` and declare its generator as a module tool:

```go
require github.com/oschwald/maxminddb-golang/v2 v2.5.0

tool github.com/oschwald/maxminddb-golang/v2/maxminddb-gen
```

Add a directive to a non-generated file in the package that declares the
target types:

```go
//go:generate go tool maxminddb-gen $GOFILE
```

The command generates methods for every exported, named struct declared in each
input file. Unexported structs, aliases, and non-struct types are skipped. A
handwritten `UnmarshalMaxMindDB` or `UnmarshalMaxMindDBCursor` method also takes
precedence and is left alone with a diagnostic. Generation fails when no
eligible exported structs remain, avoiding a successful header-only output.
The default output name is derived from the source
filename: a directive in `models.go` writes `models_maxminddb.go`. Recognized
build suffixes remain at the end, so `models_linux.go` writes
`models_maxminddb_linux.go`. Source build constraints are reproduced in the
generated file. Use `-output` to override the default. Multiple input files may
be passed when `-output` is set.

When changing the output path, the generator ignores an overlapping prior
`maxminddb-gen` file while analyzing the package so the replacement remains
complete. Remove the superseded generated file after verifying the new output;
keeping both files will define duplicate methods.

To exclude an exported struct that is not an MMDB model, name it in a source
directive:

```go
//maxminddb:ignore ServerConfig InternalRecord
```

Unknown names are rejected so that stale or misspelled exclusions do not pass
silently.

Then generate and verify the checked-in output:

```sh
go generate ./...
git diff --exit-code
go test ./...
```

The command writes formatted output atomically. Its output contains no
timestamps or local paths, and rerunning it fully replaces stale declarations.
It refuses to overwrite a file that does not have its generated-file header.
`maxminddb-gen -version` reports the version of the containing module, or
`devel` for an unversioned local build.

## Supported types

The initial generator supports:

- structs with exported, non-embedded fields and `maxminddb` tags
- `bool`, `string`, `[]byte`, `float32`, and `float64`
- signed and unsigned integer destinations up to 64 bits, with overflow checks
- named types whose underlying type is a supported scalar
- pointers recursively composed from any supported type
- slices of supported values
- maps with string or named-string keys and supported values
- nested package-owned structs
- nested types that already implement `mmdbdata.CursorUnmarshaler` or
  `mmdbdata.Unmarshaler`; cursor unmarshaling takes precedence

Fields tagged `maxminddb:"-"` are ignored. Fields without a tag, or with an
explicit empty tag, use their Go field name, matching reflection decoding.

The generator rejects embedded fields, generic structs, recursive type graphs,
interfaces, unsupported map keys, arrays, complex numbers, channels, functions,
unsafe pointers, struct types owned by another package without a custom unmarshaler,
and unsupported target fields. Diagnostics include the source position and
field name.

Generated slice decoding matches reflection reuse behavior: it reuses adequate
capacity, clears visible elements before decoding, and clears a hidden tail
that could otherwise retain references. Existing maps and pointers are reused
when possible, and absent struct fields remain unchanged.

## Ownership

Generation must run in the package that owns the target type. The command does
not add methods to types from another package and does not replace handwritten
`UnmarshalMaxMindDB` or `UnmarshalMaxMindDBCursor` methods.
