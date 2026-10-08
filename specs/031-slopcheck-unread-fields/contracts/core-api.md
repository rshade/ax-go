# Contract: `internal/unreadfield` core and analyzer adapter

Internal API. It is exported only within the module, appears in no
`surfacecheck` baseline, and is not apidiff-gated.

## Core (`internal/unreadfield`, stdlib only)

```go
// Analyze classifies one type-checked unit. info must have Types, Defs,
// Uses, and Selections populated. It never panics on well-typed input and
// keeps no state between calls.
func Analyze(fset *token.FileSet, files []*ast.File, pkg *types.Package,
    info *types.Info) Result

// BuildConfigurations returns the four supported build-tag configurations in
// canonical order: default, ax_no_grpc, ax_no_otlp, both. It returns a fresh
// slice on every call. A test keeps it equal to the Makefile BUILD_TAG_MATRIX.
func BuildConfigurations() []Configuration

// Configuration is one build-tag set.
type Configuration struct {
    Name string
    Tags []string
}

// Run loads every package under dir (./..., tests included) once per
// configuration with go list -deps -export -test -json, analyzes each unit,
// and intersects results across configurations. A load or type-check failure
// returns *AnalysisError; a timeout returns an error wrapping
// context.DeadlineExceeded, including one that fires while go list runs;
// cancellation wraps context.Canceled; permission failures wrap
// fs.ErrPermission.
func Run(ctx context.Context, dir string, configs []Configuration,
    perConfigTimeout time.Duration) (Report, error)

// AnalysisError reports which configuration failed to load or type-check.
type AnalysisError struct {
    Configuration string
    Err           error
}
```

Guarantees:

- `Analyze` output is a pure function of its inputs; `Findings` and
  `Assigned` are sorted (see [data-model.md](../data-model.md)).
- Positions in a `Result` are module-relative only when `Run` produced them.
  `Analyze` returns file paths as `fset` reports them, and `Run` relativizes
  them against `dir`.
- `errors.Is` and `errors.As` work through every returned error (`%w`
  wrapping).

## Adapter (`internal/unreadfield/analyzer`)

```go
// New returns the unreadfield analyzer. Each call returns a fresh value.
// Run reports one diagnostic per finding at the field's declaration, with
// message "struct field <Type>.<Field> is assigned but never read".
func New() *analysis.Analyzer
```

- `Requires`: none. The analyzer uses `pass.Fset`, `pass.Files`,
  `pass.Pkg` and `pass.TypesInfo` directly.
- It contains no classification logic: `Run` calls `unreadfield.Analyze` and
  converts each `Finding` into `pass.Reportf`.
- It analyzes each unit in isolation, so it does not apply the cross-
  configuration intersection. The gate (`internal/cmd/slopcheck`) is the
  enforcing front end.
