# Implementation Plan: Register every error code and guard the registry

**Branch**: `033-register-error-codes` | **Date**: 2026-10-09 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/033-register-error-codes/spec.md`

## Summary

`contract.KnownErrorCodes()` and the emit sites are not connected: 19 sites pass
a string literal and 2 name an unexported local constant. This feature
publishes the four missing code constants in `contract`, makes the registry
name only constants (with `invalid_schema_declaration` in a separate,
commented, unexported authoring-time list), converts every emit site, deletes
the duplicate `contractErrorWarnings`, and adds a drift guard: a stdlib-only
syntactic checker in `internal/testutil` that the `contract` tests run over
the whole module, proven against a committed fixture. No machine payload
changes. Research R1 to R8 hold the decisions.

## Technical Context

**Language/Version**: Go 1.27.2 (pinned in `mise.toml`, matched by `go.mod`)

**Primary Dependencies**: none new; the guard uses stdlib `go/ast`,
`go/parser`, `go/printer`, and `go/token`

**Storage**: N/A

**Testing**: `go test -race` across the 4-tag matrix; a fixture-driven test
for the checker; a module-wide guard test; a registry consistency test; the
existing goldens unchanged

**Target Platform**: any platform an ax-go CLI builds for (the 6
surfacecheck profiles); the guard parses constrained files on every host

**Project Type**: Go library (`github.com/rshade/ax-go`)

**Performance Goals**: no runtime change. The guard parses the module's
non-test sources once per test run, well under a second

**Constraints**: `contract` stays import-isolated with no new imports; code
spellings, exit codes, and `known_codes` are frozen; no golden changes; no
new root `ax` symbol

**Scale/Scope**: about 25 changed emit-site lines across 11 files, 4
constants, about 300 lines of checker plus tests and fixture, 1 baseline
update, README and AGENTS.md notes

**Governing ADR(s)**: N/A

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment |
| --- | --- |
| I Stream separation | Unchanged. Envelopes still go to stderr. ✅ |
| II Deterministic output and exit codes | Every spelling and exit code is unchanged; constants hold the same bytes. The guard sorts its output. ✅ |
| III Machine discoverability via `__schema` | `known_codes` is unchanged and is now enforced to be complete. ✅ |
| IV Agent-safety primitives | `confirmation_required` keeps its spelling and exit code. ✅ |
| V Asymmetric JSON I/O | Untouched. ✅ |
| VI Library, not application | Only constants are added to the public surface. No mutable state: both lists are functions returning fresh slices. ✅ |
| VII Test-first | The fixture test, the module guard (seen failing on 21 sites), and the consistency test land before the registry and emit-site changes (R7). ✅ |
| VIII Observability and ID discipline | Untouched. ✅ |
| IX Security and resource safety | No input handling changes. The guard reads only repository files in tests. ✅ |
| X Idiomatic Go, minimal deps | Stdlib only. Errors wrapped with `%w`. The helper takes no context because it does no cancelable I/O. ✅ |
| XI Stability and SemVer | Additive: four `contract` constants. `feat:` with no break label. ✅ |
| XII Deprecation lifecycle | Nothing deprecated. The removed identifiers are unexported. ✅ |

**ADR absorption gate**: N/A, because no ADR governs this feature.

**Post-design re-check**: the Phase 1 artifacts add the four constants, one
unexported list function, and an `internal/` test helper. Nothing else is
exported. All gates still pass.

## Project Structure

### Documentation (this feature)

```text
specs/033-register-error-codes/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── error-code-registry.md
├── checklists/
│   └── requirements.md
└── tasks.md             # /speckit-tasks
```

### Source Code (repository root)

```text
contract/
├── codes.go               # +4 constants, registry names only constants, authoringTimeErrorCodes()
├── codes_test.go          # + TestErrorCodeRegistryConsistency
├── codes_guard_test.go    # NEW: TestErrorCodesAreRegistered (module policy)
└── error.go               # WriteError fallback -> ErrorCodeInternalError
internal/testutil/
├── errorcodes.go          # NEW: FindErrorCodeViolations, FormatErrorCodeViolations, ParseStringConstants
├── errorcodes_test.go     # NEW: fixture test + config validation table
└── testdata/errorcodes/   # NEW: fixture tree with planted violations and excluded paths
execute.go                 # literals -> constants; contractErrorWarnings removed
confirm.go                 # literal -> constant
internal/mcpserver/
├── dispatch.go            # 3 calls + JSON fallback -> constants
├── server.go              # 5 calls -> constant
└── transport.go           # 2 calls -> constant
internal/schema/
├── declare.go             # 1 call -> constant
└── conflict_error.go      # 2 calls -> constant
mcp/command.go             # 1 call -> constant
schema/
├── schema.go              # 1 call -> constant
└── declarations.go        # invalidDeclarationCode removed -> constant
internal/cmd/surfacecheck/baseline.json   # +4 contract const features
README.md                  # envelope section: every ax-go code has a contract constant
AGENTS.md                  # how to add a code; SPECKIT marker
```

**Structure Decision**: a single Go module. The registry stays in `contract`,
which owns codes. The checker goes in `internal/testutil` beside the
import-isolation helpers it is modeled on, and the module policy (exclusions,
pass-throughs, registered set) lives in the `contract` test that owns the
registry. No new package is created.

## Complexity Tracking

No constitution violations to justify.
