# Implementation Plan: __schema Non-Deterministic Field Enumeration

**Branch**: `015-schema-nondeterministic-fields` | **Date**: 2026-07-08 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/015-schema-nondeterministic-fields/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Extend `__schema` output so every command node declares which of its output
fields are expected to vary between otherwise-identical runs
(`non_deterministic_fields`), so agents can diff two runs' output reliably
without hardcoding a guess. Commands that emit the standard
`contract.Envelope[T]` success shape register their payload type once, at
command-construction time, via a new generic
`WithNonDeterministicFields[T any](cmd *cobra.Command)`. That registration
marks the command as an envelope emitter, adds the built-in metadata fields
(`meta.trace_id`, `meta.span_id`, `meta.idempotency_key`) from a fixed literal,
and reflects any domain-specific `ax:"nondeterministic"` payload tags into
`data.` locators. Commands that do not register a success-envelope payload get
an explicit empty command-scoped list, so locators always map to that command's
actual output shape. The `ax.Error` envelope's single global shape gets a fixed
`["trace_id"]` entry alongside the existing `Required`/`Optional` lists. The
`--as=mcp` adapter carries the same information under
`nonDeterministicFields`. No `schema_version` bump (additive change, per the
precedent in `specs/013-error-recovery-fields`).

## Technical Context

**Language/Version**: Go 1.26.5 (module `github.com/rshade/ax-go`)

**Primary Dependencies**: stdlib `reflect` and `encoding/json` (new usage,
no new dependency); existing `github.com/spf13/cobra`/`github.com/spf13/pflag`
(Cobra command tree and `Annotations` extension point). No new third-party
dependency is introduced.

**Storage**: N/A — ax-go persists no state (Constitution Principle VI).

**Testing**: `go test -race ./...`; table-driven unit tests for the
reflection/registration helper, including registered envelope commands,
unregistered/raw-output commands with explicit empty lists, and nil-command
no-op behavior; golden-file tests (root `schema_test.go` +
`testdata/schema_{ax,mcp}.golden.json`, and the canonical
`examples/integration/golden_test.go` +
`examples/integration/testdata/schema_{ax,mcp}.golden.json`, per AGENTS.md);
a drift-detection unit test asserting the hardcoded metadata literal matches
reflecting `contract.Metadata`'s tags (research.md D2); and an integration
schema assertion showing an existing generated `data.entity_id` payload field.
`WithX` functional options are demonstrated inside a parent `ExampleXxx`, not gated
individually (AGENTS.md Test-First Discipline), so no new gated example is
required, but `WithNonDeterministicFields` is demonstrated inside an
existing/updated example in `examples/integration`.

**Target Platform**: Cross-platform CLI binary (Linux/Darwin/Windows,
amd64/arm64 — existing cross-compile matrix); no platform-specific code.

**Project Type**: Single Go module (library) — Option 1 structure, no
web/mobile split.

**Performance Goals**: No new numeric target. Must stay within the existing
CI-enforced performance regression budget for `BenchmarkBuildCommand`
(`__schema` reflection path, tracked under AGENTS.md's Performance
Regression Budget): ≤5% `ns/op` regression (when statistically significant),
≤+1 `allocs/op`. Achieved by design (research.md D2): the built-in metadata
fields are a hardcoded literal merged only for registered envelope commands,
and author-registered fields are reflected once, at command construction, not
on the `__schema` request path.

**Constraints**: `TestRootSchemaOutputMatchesIsolatedPackage` must keep
passing — the root `ax` facade and the `schema` package must stay
byte-identical. `__schema` output for an unchanged command tree must stay
byte-identical run over run (locator lists sorted + deduplicated,
research.md D8).

**Scale/Scope**: Small — touches `contract/json.go` (tag-only, no behavior
change), `internal/schema/schema.go`, `schema/schema.go`, `internal/mcp/mcp.go`,
the root `ax` facade (`schema.go`), one `examples/integration/main.go`
payload, and golden fixtures at both layers; roughly 6–8 files, no new
packages.

**Governing ADR(s)**: N/A. The GitHub issue references ADR-0003/ADR-0002,
neither of which exists as a file in `docs/adr/` — see `research.md`
"Governing ADR Status" for detail; the standing decisions already live as
constitution principles II, III, and XI.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Status |
|-----------|------|--------|
| I. Stream Separation | No new stdout/stderr writes introduced beyond existing `__schema`/error-envelope paths. | PASS |
| II. Deterministic Output & Exit Codes | The enumeration itself must be deterministic (sorted/deduplicated, research.md D8); no new envelope field bypasses struct-based modeling. | PASS |
| III. Machine Discoverability via `__schema` | This feature directly extends `__schema`'s contract; golden-file coverage required and planned (FR-008, research.md D11, Phase 1 contracts). | PASS |
| IV. Agent-Safety Primitives | Idempotency-key auto-generation behavior is unchanged; registered success-envelope commands now document `meta.idempotency_key` as non-deterministic, not altered. | PASS |
| V. Asymmetric JSON I/O | N/A — no config/Hujson surface touched. | PASS |
| VI. ADR-Governed Scope | New exported API (`WithNonDeterministicFields[T]`, extended schema structs) stays a cross-cutting AX primitive (schema discoverability), not a domain command; specified via this Spec Kit feature, not a new ADR. | PASS |
| VII. Test-First Discipline | Tests land before implementation per tasks.md ordering: reflection-helper unit tests, registered-vs-empty command schema tests, golden-file updates, drift-detection test (research.md D2) all precede/accompany the implementation tasks. `WithX` demonstrated inside an existing example, not separately gated. | PASS (verify at task-authoring time) |
| VIII. Observability & ID Discipline | No change to trace/span/idempotency-key generation or the cardinality split — this feature only documents existing behavior. | PASS |
| IX. Security & Resource Safety | Reflection/registration helper must never panic (research.md D9) — fail-closed on unusual types by skipping, and treat nil `cmd` as a no-op. | PASS (design constraint carried into tasks) |
| X. Idiomatic Go & Dependency Minimalism | Generic `WithNonDeterministicFields[T any]` avoids `any`/`interface{}`; no new dependency; no new mutable package-level state (registration data lives on the per-instance `cmd.Annotations`). | PASS |
| XI. Stability & SemVer | All struct field additions are additive/non-breaking (Go API + machine-payload shape); no `schema_version` bump (research.md D7); FR-010's "removal is breaking" is a future-review policy, not violated by this change. | PASS |
| XII. Deprecation Lifecycle | N/A — nothing deprecated or removed. | PASS |

**ADR absorption gate (Constitution §Governance)**: "Governing ADR(s)" above
is N/A — no ADR to absorb. `research.md`'s "Governing ADR Status" section
documents why (issue-referenced ADR-0002/ADR-0003 do not exist as files);
no ADR-retirement task is required in `tasks.md`.

## Project Structure

### Documentation (this feature)

```text
specs/015-schema-nondeterministic-fields/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — no governing ADR to absorb (see "Governing ADR Status")
├── data-model.md         # Phase 1 output
├── quickstart.md         # Phase 1 output
├── contracts/
│   └── schema-non-deterministic-fields.md  # Phase 1 output — JSON + Go API contract
└── tasks.md              # Phase 2 output (/speckit-tasks command — NOT created by /speckit-plan)
```

### Source Code (repository root)

Single Go module (Option 1: single project) — no new packages, existing
package boundaries hold:

```text
contract/
└── json.go              # add ax:"nondeterministic" tags to Metadata fields (doc-only, no behavior change)

internal/schema/
└── schema.go             # add Command.Annotations passthrough; add shared envelope-registration +
                           # locator-union helper used by both schema/ and internal/mcp/

internal/mcp/
└── mcp.go                 # source MCPTool.NonDeterministicFields via the internal/schema helper

schema/
├── schema.go              # add CommandSchema/ErrorSchemaInfo/MCPTool.NonDeterministicFields;
│                           # add WithNonDeterministicFields[T any]; wire convertCommandSchema
└── schema_test.go          # drift-detection test (built-in literal vs. contract.Metadata reflection);
                             # reflection-helper + registered/empty command unit tests

schema.go                   # root ax package facade — forward WithNonDeterministicFields[T]
schema_test.go               # root golden-file assertions (hand-edited testdata/*.golden.json)
testdata/
├── schema_ax.golden.json    # regenerated (no -update flag; hand-edited)
└── schema_mcp.golden.json   # regenerated (no -update flag; hand-edited)

examples/integration/
├── main.go                  # existing EntityID payload field gains ax:"nondeterministic";
│                             # success-envelope commands register their payload types (research.md D11)
├── golden_test.go            # existing TestGoldenSchema/TestGoldenSchemaMCP cover registered and empty lists
└── testdata/
    ├── schema_ax.golden.json   # regenerated via `go test ./examples/integration -run TestGolden -update`
    └── schema_mcp.golden.json  # regenerated via the same -update run

AGENTS.md                     # Core AX Mandates: short paragraph pointing at non_deterministic_fields
                               # as the authoritative enumeration (FR-009)
```

**Structure Decision**: No new top-level packages. This feature extends
three existing packages (`contract`, `internal/schema`, `schema`, plus
`internal/mcp`) that already jointly implement `__schema`, matching the
layering documented in `research.md`'s "Existing Architecture" section:
`internal/schema` stays Cobra-metadata-only for command reflection (gains a
passthrough field, not new output-shape inference); the actual
envelope-registration check + locator union/merge logic lives once in a small
`internal/schema` helper shared by both the ax-native path
(`schema/schema.go`) and the MCP path (`internal/mcp/mcp.go`), avoiding
duplicated merge logic between the two output formats.

## Complexity Tracking

*No entries — the Constitution Check above has no violations to justify.*
