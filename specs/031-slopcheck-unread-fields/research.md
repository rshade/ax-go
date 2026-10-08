# Research: Unread struct-field gate (slopcheck)

Phase 0 output for `specs/031-slopcheck-unread-fields`. There is no governing
ADR. Every unknown from the plan's Technical Context is resolved below.

## R1. Harness: one core, two front ends

**Decision**: Three packages.

| Package | Role | Imports |
| --- | --- | --- |
| `internal/unreadfield` | Harness-agnostic core: classification, scope rules, the `go list` loader, cross-configuration intersection | stdlib only |
| `internal/unreadfield/analyzer` | `go/analysis` adapter: one `*analysis.Analyzer` built by a constructor | core + `golang.org/x/tools/go/analysis` |
| `internal/cmd/slopcheck` | The gate: flags, timeout, stream and exit contract | core + `contract` |

The core exposes `Analyze(fset, files, pkg, info) Result`, which takes exactly
the four values both harnesses already hold (`*analysis.Pass` carries
`Fset`, `Files`, `Pkg` and `TypesInfo`). It also exposes `Run(ctx, dir,
configurations)`, which loads and analyzes a module tree and intersects the
results across build configurations.

**Rationale**: The maintainer chose both harnesses. Putting all
classification in the core is what makes "both front ends agree" a structural
property rather than something tested after the fact. Keeping the adapter in
its own package keeps the gate binary stdlib-only: `internal/cmd/slopcheck`
never links `x/tools`, so the issue's Option B property (full stream control,
no dependency in the gate itself) holds exactly.

**Alternatives considered**:

- *Adapter in the core package*: one fewer package, but every core importer,
  the gate included, links `x/tools/go/analysis`. Rejected.
- *Gate driven through `singlechecker`/`checker`*: `singlechecker` owns
  output and `os.Exit`, which conflicts with the one-envelope contract. Rejected,
  as the issue predicted.
- *`go/packages` as the gate's loader*: it handles tests and tags, but it would
  put `x/tools` back into the gate and give up the surfacecheck precedent
  below. Rejected.

## R2. Loader: `go list -deps -export -test -json`

**Decision**: Reuse `surfacecheck`'s proven loader shape and add `-test`:

```text
go list -deps -export -test -json [-tags=<config>] ./...
```

Verified on `internal/schema` (0.3 s warm). The stream contains, per tested
package:

| `ImportPath` | `ForTest` | Files | Action |
| --- | --- | --- | --- |
| `p` | — | production only | skip when a test variant exists, else analyze |
| `p [p.test]` | `p` | production + internal `_test.go` | analyze, keyed as `p` |
| `p_test [p.test]` | `p` | external `_test.go` | analyze, keyed as `p_test` |
| `p.test` | — | generated test main in the build cache | skip |
| `DepOnly: true` | — | dependencies | export data only |

Each analyzed unit is parsed from `Dir` + `GoFiles` + `CgoFiles` with
`parser.ParseComments` and type-checked with `go/types`. Imports resolve
through `importer.ForCompiler(fset, "gc", lookup)`, where `lookup` first
rewrites the path through the unit's `ImportMap` (so `p_test` gets `p
[p.test]`, including `export_test.go` symbols) and then opens the `Export`
file. Output is bounded by a byte ceiling (16 MiB, as `deadcheck`'s
`maxReportBytes`; a whole-repository listing measured 2.4 MB). Any type-check error, missing export data, or `go list` failure fails
the run closed (`slopcheck_analysis_failed`).

**Rationale**: Analyzing the test variant instead of the plain package gives
one unit that already contains production code and internal tests, which is
exactly the "tests are readers" rule. The plain variant would report fields
read only in tests.

**Alternatives considered**: `importer.ForCompiler(..., "source", ...)`
ignores `-tags` (it uses `build.Default`), so a tagged configuration would
type-check against the wrong imports. Rejected.

## R3. Build configurations and intersection

**Decision**: Analyze the four configurations `""`, `ax_no_grpc`,
`ax_no_otlp`, `ax_no_grpc,ax_no_otlp` on the host platform, in that order, as a
Go constant function, `unreadfield.BuildConfigurations()`, owned by the core so
that the gate and the core's own `Run` tests share one definition. A single
core test asserts it matches the Makefile's `BUILD_TAG_MATRIX` (the
`deadcheck` pattern). Each configuration yields
`Assigned` (candidate fields assigned in a literal) and `Findings` (the subset
never read). A field is reported if and only if it is a finding in **every**
configuration whose `Assigned` set contains it.

**Rationale**: A field read only under `ax_no_grpc` is used. Intersecting
over "configurations where it is assigned" also covers a field declared in a
tagged file: it is judged only where it exists.

**Alternatives considered**: default configuration only (reports tag-gated
reads as false positives; rejected); union (most findings, most false
positives; rejected for a blocking gate).

## R4. Load/store classification

**Decision**: Each field selector `x.F` (field resolved through
`Info.Selections`, canonicalized with `(*types.Var).Origin()` so generic
instantiations collapse onto the declared field) is classified by its parent:

| Parent context | Classification |
| --- | --- |
| LHS of `=` | store |
| LHS of `op=` | load + store |
| `x.F++`, `x.F--` | load + store |
| operand of `&` | load (address may be read through) |
| base of a further selector, index, or slice (`x.F.G`, `x.F[i]`) | load |
| `for x.F = range …` key/value | store |
| RHS of `x.F = x.F` with the same field and same base expression | not a read |
| anything else | load |

A promoted selector also marks every embedded field on its path as read,
because the path dereferences each one.

**Rationale**: This is the issue's correctness core, encoded as a closed table
so each row is one fixture case.

## R5. Whole-value uses count as reading every field

**Decision**: Use an allow-list of harmless positions. A **carrier** is any
expression whose type contains a candidate struct `T`: `T` itself, or a
pointer, array, slice, map (key or element) or channel whose element type is
or contains `T`. Every use of a carrier outside the allow-list marks all fields
of every candidate struct the type contains as read, recursing through nested
struct field types with a cycle guard. The harmless positions are:

- the base of a field or method selector;
- the left-hand side of `=` or `:=`, a range key or value variable;
- the **right-hand side** of `=` or `:=`, or a `var` initializer, when the
  destination's type is not an interface (so `x := T{F: 1}`, `tc := tc` and
  `got := build()` are harmless). A blank destination (`_ = x`) is harmless:
  go/types records no type for `_`, so it must be special-cased before any
  interface check, never treated as an interface;
- the range expression of a `for … range` (so `range tests` is harmless);
- an element or keyed value of a composite literal whose element or field
  type is not an interface;
- an argument to a function or method that resolves **statically** to a
  `*types.Func` declared in the analyzed package, when the parameter type is
  not an interface or a type parameter. Dynamic callees (func values, method
  values, interface methods) are not allow-listed;
- a `return` operand of an **unexported** function whose result type is not
  an interface;
- an argument to the builtins `append`, `len`, `cap`, `copy`, `delete`;
- an index or slice base (`tests[i]`, `tests[1:]`), when the result is itself
  in a harmless position;
- the operand of `&` or `*`, or a parenthesized expression, when the
  resulting expression is itself in a harmless position.

**Rationale**: The gate cannot see reflection readers (`reflect.DeepEqual`,
`encoding/json`, `fmt`), equality comparison, or downstream callers. It is a
blocking gate, so it fails toward silence. An allow-list is safer than an
enumerated deny-list: a form nobody thought of counts as a read rather than
becoming a false positive.

**Alternatives considered**: enumerating escape forms (`==`, interface
conversion, out-of-package call) misses implicit conversions such as sending
on a `chan any` or storing into a `map[string]any`. Rejected. Tracking only
`T` and `*T` (not containers) misses `t.Logf("%v", tests)`, where `fmt`
reads every row's fields through a `[]T`. Rejected.

## R6. Scope rules

**Decision**: A field is a candidate when all of these hold:

1. It is declared in a file of the analyzed unit (`field.Pkg() == pkg`).
2. Its declaring file is not generated (`ast.IsGenerated`).
3. Its name is not `_`.
4. It is not exported-and-externally-visible. An exported field is skipped
   when its struct type expression lies lexically inside a package-level
   declaration that declares an exported name (an exported `TypeSpec`, a
   `ValueSpec` with an exported name, or an exported `FuncDecl`'s signature),
   or when its named type is the target of an exported package-level alias
   (`type Foo = foo`, the `export_test.go` pattern).

Exported fields of unexported types and every field of an anonymous struct
inside a function body (the table-test shape) remain in scope.

**Rationale**: Encodes the maintainer's "skip exported fields of exported
types" decision so that it also covers the two ways an unexported type
becomes nameable downstream.

## R7. Positional literals

**Decision**: A positional literal assigns every field of its type, so every
in-scope field becomes an assignment site at the literal's position.

**Rationale**: The issue requires an explicit choice. Ignoring positional
literals would create false negatives. Excluding their types from reporting
would hide real findings.

## R8. Gate output contract

**Decision**: It mirrors `deadcheck` exactly. See
[contracts/gate-cli.md](contracts/gate-cli.md). The error codes are:

| Code | Exit |
| --- | --- |
| `slopcheck_unread_field` | `2` |
| `invalid_slopcheck_artifact` | `2` |
| `slopcheck_analysis_failed` | `2` |
| `slopcheck_timeout` | `3` |
| `slopcheck_permission` | `4` |
| `slopcheck_internal` | `1` |

Findings go in `context.cause` as sorted `path:line:col: pkg.Type.Field
(assigned at path:line:col, …)` entries joined by a semicolon and a space, with module-relative
paths.

**Rationale**: Consistency with the seven sibling gates (spec User Story 2).

## R9. Timeout and cancellation

**Decision**: Five minutes per configuration, applied to each `go list`
invocation through `exec.CommandContext` and checked between package units.
`context.DeadlineExceeded` maps to `slopcheck_timeout`, and cancellation maps to
`slopcheck_internal`. Both mirror `deadcheck`. When the context kills `go
list` mid-run, `exec.Cmd.Wait` returns an `*exec.ExitError` ("signal:
killed"), not `ctx.Err()`. So after any failed `go list`, the loader checks
`ctx.Err()` and wraps it when it is non-nil (as `deadcheck` `runner.go`
does). Otherwise a real timeout would surface as `slopcheck_analysis_failed`.

## R10. Dependency: `golang.org/x/tools`

**Decision**: Promote `golang.org/x/tools` to a direct requirement at the
version MVS already selects (`v0.50.0`, currently pulled in by the MCP SDK).

**Verified**:

- `go list -m golang.org/x/tools` → `v0.50.0`. It is in `go.sum` but has no
  `go.mod` line at all; it is pulled in only by the MCP SDK's tests.
- The first import (`analysistest`, in T021) makes the build fail with
  "updates to go.mod needed" until `go mod tidy` runs. Tidy then adds
  `golang.org/x/tools v0.50.0` as a direct requirement and
  `golang.org/x/mod v0.41.0 // indirect`, plus `go.sum` entries. Both modules
  are already in the module graph, so this is a promotion, not new code.
- `.golangci.yml` `depguard` has deny-lists only (deprecated modules,
  `math/rand`, `log`). Nothing to edit, so the rule against touching
  `.golangci.yml` is respected.
- Only `internal/unreadfield/analyzer` imports it. No public package and no
  `logging` dependency changes, so `size-check` and `surface-check` are
  unaffected (spec SC-007, FR-013).

**Rationale**: Constitution X ("justify every new dependency") is met: no new
module enters the graph, the stdlib cannot provide the analyzer contract, and
the maintainer chose the analyzer front end.

## R11. Lint-driven shape constraints

**Decision**:

- The analyzer is built by `New() *analysis.Analyzer`, not a package-level
  `var`. `gochecknoglobals` and Constitution X forbid mutable package state.
- The build-configuration list is a function returning a fresh slice,
  following `deadcheck.buildTags()`.

## R12. Test strategy

**Decision**:

- **Shared fixture module** at `internal/unreadfield/testdata/fixtures/` with
  its own `go.mod`. It holds one package per acceptance shape (spec User Story
  1, cases 1–7), plus whole-value, scope, and generic cases, annotated with
  `// want` comments. It contains **no `_test.go` files**, because
  `analysistest` loads with `Tests: true` and would analyze the plain variant
  of a package whose only read is in a test.
- **Core tests** run `unreadfield.Run` over the fixture module and compare
  against a table of expected fields.
- **Adapter tests** run `analysistest.Run` over the same fixture module.
  Verified in `x/tools@v0.50.0` `analysistest.go`: a `dir` containing
  `go.mod` loads in module mode with `GOPROXY=off`. The source labels this
  "Undocumented module mode. Will be replaced by something better", so a future
  `x/tools` bump may require moving the fixtures to a GOPATH-style
  `testdata/src/` tree. The fixture module has no requirements, so
  `GOPROXY=off` is harmless. A parity test asserts that
  `analysistest`'s diagnostics equal `Run`'s findings field for field
  (FR-012, SC-003).
- **`Run` fixtures** at `internal/unreadfield/testdata/`: a module whose
  only read is in a `_test.go` file, a module with a tag-gated read (proves R3),
  and a module that does not type-check. These test the core's loader and
  intersection, and they stay out of the `analysistest` module.
- **Gate fixtures** at `internal/cmd/slopcheck/testdata/`: a clean module, a
  failing module, and a broken module, one per stream golden.
- **Fuzz**: `FuzzDecodeListing` over the `go list -json` stream decoder,
  seeded with a real listing. It must never panic and must either return
  units or a wrapped error, and must respect the byte ceiling. Constitution VII
  requires a fuzz test for every parser surface, and every sibling gate fuzzes
  its tool-output parser (`deadcheck` `FuzzParseReport`, `covercheck`,
  `surfacecheck`).
- **Golden files**: `pass.stdout.golden`, `fail.stderr.golden`,
  `analysis.stderr.golden`. Each stream that must be empty is asserted empty.
- **Manual (FR-018, SC-002)**: a whole-repository run is recorded in the PR
  description with triage, and a throwaway `expectError bool` is planted, seen
  reported, and removed.

## R13. Whole-repository findings and the blocking gate

**Decision**: Run `go run ./internal/cmd/slopcheck` over the repository
before wiring `make ci`. With zero findings, wire the gate. With only false
positives, fix the rule (they are core defects) and re-run. With any **real**
finding, stop and ask the maintainer (spec Assumptions) before wiring.
