# Implementation Plan: Known codes include the config package's runtime codes

**Branch**: `032-known-config-codes` | **Date**: 2026-10-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/032-known-config-codes/spec.md`

## Summary

`__schema`'s `error_envelope.known_codes` lists four codes, and the public
`config` package emits five more during a command run. That list is what an
agent reads. This feature makes the list complete for the runtime: it adds
five exported `contract` constants for the `config_*` codes, routes
`config/config.go` through them, extends `contract.KnownErrorCodes()` to nine
sorted codes, and states the scope rule (spec FR-001) on the `KnownCodes`
field and in spec 029. The machine payload change is additive. Goldens and
the surface baseline move in the same change. Research R1 to R7 hold the
decisions.

## Technical Context

**Language/Version**: Go 1.27.2 (pinned in `mise.toml`, matched by `go.mod`)

**Primary Dependencies**: none new; stdlib plus the existing module graph

**Storage**: N/A

**Testing**: `go test -race` across the 4-tag matrix, golden files, a
verified `ExampleKnownErrorCodes`

**Target Platform**: any platform an ax-go CLI builds for (the 6 surfacecheck
profiles)

**Project Type**: Go library (`github.com/rshade/ax-go`)

**Performance Goals**: no change. No tracked benchmark path is affected
(research R7)

**Constraints**: `contract` stays import-isolated, with no new imports;
the `logging` binary-size budget is unaffected; existing code spellings and
exit codes are frozen

**Scale/Scope**: about 60 changed Go lines across 3 packages, 4 goldens, 1
baseline, README, spec 029

**Governing ADR(s)**: N/A

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment |
| --- | --- |
| I Stream separation | Unchanged. Envelopes still go to stderr. ✅ |
| II Deterministic output and exit codes | The list is a fixed sorted value. No exit code moves. ✅ |
| III Machine discoverability via `__schema` | This is the point of the feature: `__schema` stops under-reporting. ✅ |
| IV Agent-safety primitives | Untouched. ✅ |
| V Asymmetric JSON I/O | Config parsing behavior unchanged. Only the code source becomes a constant. ✅ |
| VI Library, not application | No new runtime surface, only contract constants. ✅ |
| VII Test-first | Tests (R6) land first and fail before the implementation. A verified example pins the list. ✅ |
| VIII Observability and ID discipline | Untouched. ✅ |
| IX Security and resource safety | Untouched. The read cap and its code are unchanged. ✅ |
| X Idiomatic Go, minimal deps | Untyped constants and no dependency. A copied slice per call. ✅ |
| XI Stability and SemVer | Additive on both surfaces. `feat:` with no break label (R2). ✅ |
| XII Deprecation lifecycle | Nothing deprecated or removed. ✅ |

**ADR absorption gate**: N/A, because no ADR governs this feature.

**Post-design re-check**: the Phase 1 artifacts add no surface beyond the
five constants and one field doc comment. All gates still pass.

## Project Structure

### Documentation (this feature)

```text
specs/032-known-config-codes/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── known-codes.md
├── checklists/
│   └── requirements.md
└── tasks.md             # /speckit-tasks
```

### Source Code (repository root)

```text
contract/
├── codes.go             # NEW: code constants + KnownErrorCodes (moved from warnings.go)
├── codes_test.go        # NEW: sorted / unique / exclusion / fresh-copy properties
├── example_test.go      # + ExampleKnownErrorCodes (exact nine codes)
└── warnings.go          # loses ErrorCodeWarningsAsErrors + KnownErrorCodes
config/
├── config.go            # literals -> contract constants
└── config_test.go       # six failure paths -> constant + membership
schema/
└── schema.go            # KnownCodes field doc comment
examples/integration/
├── main_test.go         # SC-002 end to end: emitted code is in known_codes
└── testdata/schema_ax.golden.json
testdata/
├── schema_ax.golden.json
├── schema_ax_declarations.golden.json
└── schema_ax_enriched.golden.json
internal/cmd/surfacecheck/baseline.json   # +5 contract const features
specs/029-success-warnings/spec.md        # amended known-codes definition
README.md                                 # config codes + pointer to known_codes
```

**Structure Decision**: this is a single Go module, and the change stays
inside the packages that already own each concern. `contract` owns codes,
`config` emits them, and `schema` publishes them. No new package is created.

## Complexity Tracking

No constitution violations to justify.
