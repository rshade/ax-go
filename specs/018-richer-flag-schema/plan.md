# Implementation Plan: Richer Per-Flag `__schema` Semantics

**Branch**: `018-richer-flag-schema` | **Date**: 2026-10-07 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/018-richer-flag-schema/spec.md`

## Summary

Add three author-declared facts to the machine-discoverability contract:

- An allowed-value set (enum) per flag. ax-go enforces it when flags are parsed.
- An example value per flag.
- A capability class per command, drawn from a fixed six-member vocabulary, with
  an optional note.

Authors declare these through three functions in the import-isolated `schema`
package: `WithFlagEnum`, `WithFlagExample` and `WithCapability`. The root package
`ax` re-exports them. They follow the idiom `WithNonDeterministicFields[T](cmd)`
already uses: the function annotates the Cobra object directly, and reflection
reads the annotation later. Unlike that function, they return an error, so an
authoring inconsistency (FR-013) reaches the author at declaration time.

Enforcement replaces the flag's `pflag.Value` with a validating wrapper. pflag
calls `Set` while it parses flags, which is before `PersistentPreRunE` and
`RunE`. The rejection therefore always happens before `--dry-run` or any side
effect (FR-014). The same mechanism applies under `ax.Execute` and under the
live `mcp-server` dispatcher, because both parse flags through Cobra.

The rejection is an `*contract.Error` carrying `validation_error` and exit code
2. pflag's `InvalidValueError` exposes it through `Unwrap`, so the existing
`errors.As` path in `ax.Execute` surfaces it unchanged. The wrapper is the
single source of truth for the enum: both `__schema` and the MCP input schema
read the allowed set from it.

The new output fields are all `omitempty` or nil pointers. The fixtures in
`testdata/schema_*.golden.json` describe a CLI that declares nothing, so they
must stay byte-identical. That is the SC-004 backward-compatibility proof. New
"enriched" golden files pin the declared case.

## Technical Context

**Language/Version**: Go 1.26.5 (module `github.com/rshade/ax-go`)

**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2, `github.com/spf13/pflag`
v1.0.10 (`InvalidValueError` + `Unwrap`, verified in `flag.go:495` /
`errors.go:120`), `github.com/modelcontextprotocol/go-sdk` v1.6.1 (live server
only; `sdk.ToolAnnotations` exists). **No new dependencies.**

**Storage**: N/A. Declarations live on the in-memory Cobra and pflag objects:
the flag's `Value` wrapper, `pflag.Flag.Annotations`, and
`cobra.Command.Annotations`.

**Testing**: `go test -race ./...` across the 4-configuration build-tag matrix;
table-driven unit tests; golden files (`testdata/`, `examples/integration/testdata/`);
fuzz test for the enum-membership canonicaliser (parser surface); `ExampleXxx`
for the three declaration functions; `BenchmarkBuildCommand` stays within
budget.

**Target Platform**: Library; the 6 GOOS/GOARCH profiles in `surfacecheck`.

**Project Type**: Go library (CLI foundation).

**Performance Goals**: `BenchmarkBuildCommand` stays inside the benchcheck
budget: ns/op may grow at most 5%, and allocs/op at most +1, for a tree that
declares nothing. A flag with no declaration adds no allocations: the code does
one type assertion and reads one map entry.

**Constraints**: The `schema` package stays import-isolated, so it must not
import the MCP SDK, `net/http`, or the root facade. Output stays deterministic.
Library code never panics. The `contract`, `config`, `schema` and `id` packages
keep their import isolation.

**Scale/Scope**: About 3 new exported functions, 1 type, 6 constants, 2 structs
and 5 struct fields in `schema`, mirrored as aliases in `ax`. About 400 to 600
lines of production code plus tests.

**Governing ADR(s)**: N/A. Issue #28 names ADR-0003, but `docs/adr/` holds only
0004 (trace-ID format) and 0008 (Cobra). Neither governs flag-schema semantics.
ADR-0008's Cobra-only decision is a constraint this feature obeys, not a
decision it changes, so no ADR is absorbed or retired.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Status |
|-----------|------|--------|
| I. Stream Separation | `__schema` output stays on stdout; the enum rejection is an `ax.Error` on stderr, written by `ax.Execute` | PASS |
| II. Determinism & Exit Codes | The enum keeps the author's order and rejects duplicates. Struct fields are used, not maps, except the existing `inputSchema` map, which `encoding/json` key-sorts. The rejection is exit `2`, and the declaration error has no exit code of its own. Identical input produces an identical rejection envelope. | PASS |
| III. Machine Discoverability | Additive fields in both `__schema` and `--as=mcp`. Golden files guard the enriched output, and the existing golden files stay unchanged. | PASS |
| IV. Agent-Safety Primitives | Enforcement happens in pflag `Set`, so it precedes `PersistentPreRunE` and the `Guard`/`Perform` dry-run helpers (FR-014, US1-AS5) | PASS |
| V. Asymmetric JSON I/O | No change | N/A |
| VI. Library Scope | Cobra stays the only framework; no state is persisted; the change is specified through Spec Kit, not an ADR | PASS |
| VII. Test-First | Failing tests come first for each FR. Golden, fuzz (canonicaliser) and `ExampleXxx` tests are added, and every new export has a doc comment. | PASS (gated in tasks) |
| VIII. Observability | No logging; the rejection envelope keeps `trace_id` because `normalizeExecuteError` fills it | PASS |
| IX. Security | No panic: declarations return errors. The user's value appears only in the JSON-escaped envelope's `context` field. It is never formatted into a log message and never into the error `message`. | PASS |
| X. Idiomatic Go | No new dependency and no package-level mutable state. The vocabulary is a `const` block, and membership is checked with a `switch`. Errors wrap with `%w` (`ErrInvalidDeclaration`). | PASS |
| XI. Stability & SemVer | Additive Go API and additive payload fields, so the commit type is `feat:` and the release is a minor bump. No `breaking-change-approved` label is needed. | PASS |
| XII. Deprecation | Nothing deprecated | N/A |

**ADR absorption gate**: N/A, because no ADR governs this feature (see Technical
Context).

**Additional gates touched**: `make surface-check` (new `schema` + `ax`
features, so the baseline regenerates and the root additions get audit rows),
`apidiff` (additive only), `make doc-coverage` (examples added; the required
list is unchanged), `make cover-check` (`internal/schema` floor 93%,
`internal/mcp` 96.9%, `internal/cli` 98%, root 85%), `make bench-check`.

**Post-design re-check (after Phase 1)**: PASS. The design adds no violation.
The one judgement call is that `mutate` maps to MCP `destructiveHint: true`.
That choice is conservative toward safety and is recorded in research.md R6. It
does not violate any principle.

## Project Structure

### Documentation (this feature)

```text
specs/018-richer-flag-schema/
├── plan.md              # This file
├── research.md          # Phase 0: decisions R1–R10
├── data-model.md        # Phase 1: entities, validation, wrapper semantics
├── quickstart.md        # Phase 1: author + agent walkthrough, verification commands
├── contracts/
│   ├── declaration-api.md   # Go API contract (schema + ax facade)
│   └── schema-output.md     # __schema / --as=mcp / mcp-server payload contract
├── checklists/requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
internal/schema/
├── schema.go             # Flag gains Enum/Example; Command gains Capability; CollectFlags reads them
├── declare.go            # NEW: enumValue wrapper, annotation keys, declaration + validation logic,
│                         #      capability vocabulary, fail-closed readers
├── convert.go            # NEW: type-aware canonicalise/convert helpers (moved from internal/mcp)
├── declare_test.go       # NEW: table-driven FR-013/FR-014/FR-009 tests
└── declare_fuzz_test.go  # NEW: FuzzEnumCanonicalise

internal/mcp/
├── mcp.go                # flagProperty emits "enum"/"examples"; Tool gains Capability + Annotations
└── *_test.go             # enum/example/annotations property tests

internal/mcpserver/
├── server.go             # sdk.Tool gets Annotations from the mapped hints
└── dispatch.go           # FlagErrorFunc preserves an existing *contract.Error (errors.As) instead of re-wrapping

schema/
├── schema.go             # FlagSchema.Enum/Example, CommandSchema.Capability, MCPTool.Capability/Annotations,
│                         # CapabilitySchema, MCPToolAnnotations, conversion
├── declare.go            # NEW: Capability type + 6 consts, ErrInvalidDeclaration,
│                         #      WithFlagEnum / WithFlagExample / WithCapability
├── declare_test.go       # NEW
└── example_test.go       # ExampleWithFlagEnum, ExampleWithFlagExample, ExampleWithCapability

schema.go (root ax)       # aliases + forwarding funcs + typed constants
execute_test.go           # enum rejection → exit 2, stderr envelope, no stdout, dry-run still rejected
testdata/
├── schema_ax_enriched.golden.json    # NEW
└── schema_mcp_enriched.golden.json   # NEW  (existing schema_*.golden.json UNCHANGED)

examples/integration/     # declare an enum, example, and capability on existing commands; regenerate goldens
internal/cmd/surfacecheck/baseline.json          # regenerated (make surface-update)
specs/015-internalize-helpers/public-surface-audit.json  # append root-package audit rows
README.md, AGENTS.md      # document the declaration API and output fields
```

**Structure Decision**: The feature follows the layering spec 015 established.
The mechanics live in `internal/schema` and `internal/mcp`; the public surface
is the import-isolated `schema` package; root `ax` forwards to it. No new public
package is added, so the apidiff allowlist and the surfacecheck package list do
not change.

## Complexity Tracking

No constitution violations. The table is empty.
