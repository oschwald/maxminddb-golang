# maxminddb-gen

`maxminddb-gen` generates reflection-free decoder methods for types owned by
the package where generation runs. It is part of the
`maxminddb-golang` module and uses the same version as the decoder support API.
Generation is optional: a type with neither generated nor handwritten custom
unmarshaling methods continues to use the reflection decoder.

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
input file. Unexported structs, aliases, and non-struct types are skipped.
Generated targets receive both `UnmarshalMaxMindDBCursor` and, throughout v2,
the deprecated `UnmarshalMaxMindDB` compatibility bridge. A handwritten
implementation of either callback causes that target to be skipped with a
diagnostic; new handwritten decoders should implement
`mmdbdata.CursorUnmarshaler`. Generation fails when no eligible exported
structs remain, avoiding a successful header-only output.
The default output name is derived from the source
filename: a directive in `models.go` writes `models_maxminddb.go`. Recognized
build suffixes remain at the end, so `models_linux.go` writes
`models_maxminddb_linux.go`. Source build constraints are reproduced in the
generated file. Generation loads the package using the current `GOOS`, `GOARCH`,
and build tags, so every constrained input must be selected by that environment.
Use `-output` to override the default. Multiple input files may be passed when
`-output` is set; their explicit and filename constraints are combined with
`&&` on the shared output.

The generator ignores prior `maxminddb-gen` files while analyzing the package,
so multiple per-file directives produce the same output regardless of their
execution order or which generated files already exist. When changing an output
path, remove the superseded generated file after verifying the new output;
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
`go tool maxminddb-gen -version` reports the version of the containing module, or
`devel` for an unversioned local build.

## Supported types

The initial generator supports:

- structs with exported, non-embedded fields and `maxminddb` tags
- `bool`, `string`, `[]byte`, `float32`, and `float64`
- signed and unsigned integer destinations up to 64 bits (excluding `uintptr`),
  with overflow checks
- named types whose underlying type is a supported scalar
- pointers recursively composed from any supported type
- slices of supported values
- maps with string or named-string keys and supported values
- nested package-owned structs
- nested types that implement `mmdbdata.CursorUnmarshaler`; the deprecated
  `mmdbdata.Unmarshaler` remains supported throughout v2, and cursor unmarshaling
  takes precedence when both are present

Named-string map keys may also implement either custom unmarshaling interface.
Their callback receives the original MMDB key, and cursor unmarshaling takes
precedence when both interfaces are implemented.

Non-embedded fields tagged `maxminddb:"-"` are ignored. Fields without a tag,
or with an explicit empty tag, use their Go field name, matching reflection
decoding.

Tag options follow the comma-separated `encoding/json/v2` grammar. The
`maxsize:N` option can be applied to maps, MMDB arrays decoded into slices,
strings, and bytes:

```go
type City struct {
	Subdivisions []Subdivision `maxminddb:"subdivisions,maxsize:32"`
}
```

It rejects a map or array with more than `N` entries, or a string or byte value
with more than `N` bytes, before allocating or mutating the matching field. A
`[]byte` field accepts both the MMDB Bytes and array encodings, and the same
limit covers both. Reflection decoding enforces the option with the same
semantics. Because a comma delimits options, quote a literal field name
containing a comma, for example `maxminddb:"'city,name'"`. For a supported
custom field type, `maxsize` checks every size-bearing MMDB kind (map, array,
string, and bytes) before invoking the unmarshaler because the encodings
accepted by a callback cannot be inferred from its Go type. Unknown tag options
are ignored for forward compatibility. Spelling and case variants that
normalize to `maxsize`, such as `max_size` and `MAXSIZE`, are rejected.

The generator rejects embedded fields, including those tagged `maxminddb:"-"`,
generic structs, recursive type graphs, interfaces, unsupported map keys,
arrays, complex numbers, channels, functions, unsafe pointers, struct types
owned by another package without a custom unmarshaler, and unsupported target
fields. Diagnostics include the source position and field name.

Generated slice decoding matches reflection reuse behavior: it reuses adequate
capacity, clears visible elements before decoding, and clears a hidden tail
that could otherwise retain references. Existing maps and pointers are reused
when possible, and absent struct fields remain unchanged. Reused maps are not
cleared, so keys absent from the newly decoded map retain their previous values.
The exception is an exact `[]byte` decoded from the MMDB Bytes kind: generated
code may reuse and mutate an adequately sized destination backing array,
whereas reflection decoding allocates a replacement slice. If another value
must retain the old contents, copy its contents into a newly allocated slice
(for example, `preserved := append([]byte(nil), value...)`) or detach the decode
destination from the shared backing array (for example, assign the destination
`nil`) before decoding. Clearing or reslicing the destination does not detach
its backing array and does not protect aliases.

Generated and handwritten unmarshalers control their own traversal and do not
inherit reflection decoding's aggregate per-record work and payload budgets.
Generated code rejects duplicate recognized map keys and enforces each declared
`maxsize` constraint, but schema authors must place constraints on fields that
could otherwise amplify work. Unknown keys are skipped without following
pointer targets. Handwritten implementations must also share an aggregate
budget across nested calls when they traverse untrusted data. Calling
`Reader.Verify` before decoding provides a complete-database validation
boundary for untrusted database files.

## Ownership

Generation must run in the package that owns the target type. The command does
not add methods to types from another package and does not replace handwritten
`UnmarshalMaxMindDBCursor` methods. It also leaves deprecated handwritten
`UnmarshalMaxMindDB` methods in place for v2 compatibility.
