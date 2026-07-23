# Implementation Plan: Certify and Internalize the Public Boundary Before v1.0

**Branch**: `015-internalize-helpers` | **Date**: 2026-07-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from
`/specs/015-internalize-helpers/spec.md`

## Summary

Deliver feature 015 as the non-breaking audit, enforcement, internalization,
and deprecation phase of the pre-v1 boundary work:

1. Generate a type-aware inventory of the complete compiler-visible root `ax`
   surface for all six supported GOOS/GOARCH profiles.
2. Commit a permanent identifier-by-identifier decision record at
   `public-surface-audit.json`; records are retained through later lifecycle
   transitions.
3. Commit a separate operational baseline used by a new
   `internal/cmd/surfacecheck` CI gate.
4. Move audit-approved non-public mechanics into cohesive `internal/`
   packages where a compatibility-preserving root forwarder is possible, and
   publish Go-recognized deprecation notices without removing or re-typing any
   export.
5. Defer removal to a follow-up Spec Kit feature after a real published minor
   satisfies Constitution Principle XII.

The gate uses `go list` compiler export data plus stdlib `go/importer` and
`go/types`, not a declaration-only AST approximation. Successful output is
minified strict JSON; failures are one deterministic `ax.Error` on stderr.

## Technical Context

**Language/Version**: Go 1.26.5 (module `github.com/rshade/ax-go`, package `ax`)

**Primary Dependencies**: Standard library only for the new gate:
`context`, `encoding/json`, `flag`, `go/importer`, `go/token`, `go/types`,
`io`, `os/exec`, `runtime`, `sort`, and path utilities. The gate invokes the
existing Go toolchain with `go list -deps -export -json`; no module dependency
is added.

**Storage**:

- `specs/015-internalize-helpers/public-surface-audit.json`: permanent
  classification and lifecycle history; rows are never deleted.
- `internal/cmd/surfacecheck/baseline.json`: current approved canonical API
  feature IDs and signatures; updated with reviewed live-surface changes.

The gate cross-validates active audit rows against the live baseline. Separate
artifacts are intentional because historical decisions and current compiler
state have different lifecycles.

**Testing**:

- Tests land before implementation.
- Table-driven type-surface fixtures cover declarations, aliases, fields,
  interface embedding, value/pointer methods, promotion, ambiguity, hidden
  concrete reachability, build tags, and excluded test files.
- Golden tests pin minified success/inventory output and `ax.Error` failures.
- A fuzz test covers the new strict baseline/audit JSON parser surface.
- Stream/exit matrices verify stdout/stderr separation and exit codes.
- Repeated scans assert byte determinism.
- `go test -race ./...`, `go vet ./...`, `golangci-lint run`,
  `make doc-coverage`, `make cover-check`, and `make bench-check` remain green.
- The new command package faces the existing 25% default package coverage
  floor; no floor is reduced.

**Target Platform**: Developer machines and GitHub Actions. The inventory
program runs as a host binary and passes each supported target profile to its
child `go list`; it verifies:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`

**Project Type**: Go library plus internal maintainer tooling.

**Performance Goals**: None. `surfacecheck` is a one-shot maintainer/CI tool,
not a runtime hot path. No numeric performance claim is made. Runtime
reorganization must keep the existing benchmark budget green.

**Constraints**:

- Zero new module dependencies.
- No exported removal, rename, re-type, or semantic change in feature 015.
- Existing `ax.Error` and `__schema` goldens remain byte-identical.
- Clear non-public mechanics live under cohesive `internal/` packages;
  `internal/helpers` is forbidden.
- A safe root compatibility forwarder and valid `Deprecated:` paragraph are
  required before an implementation is moved.
- If compatibility cannot be proved, deprecate in place and defer relocation
  to the follow-up removal feature.
- No ADR is created or edited.
- Commands are documented as module-root invocations.

**Scale/Scope**: The complete compiler-visible feature graph rooted at the
current `ax` package, not an approximate top-level declaration count. One
historical audit, one live baseline, one new internal command, one Make target,
and CI validation wiring.

**Governing ADR(s)**: N/A. Issue #18's ADR-0012 reference is stale; only
ADR-0004 and ADR-0008 remain, neither governs this feature. Constitution
Principles X–XII govern package boundaries, breaking changes, and deprecation.
The decision is recorded in `research.md` without creating or editing an ADR.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Verdict | Evidence |
|-----------|---------|----------|
| I. Stream Separation | PASS | Pass/inventory results are minified strict JSON on stdout; every failure leaves stdout empty and emits exactly one minified `ax.Error` on stderr. Make recipe echo is suppressed. |
| II. Deterministic Output & Exit Codes | PASS | Output uses structs and canonical sorting. Exit `0` is success, `2` is drift/invalid repository input, `1` is unexpected internal failure, and `4` is permission failure. |
| III. Machine Discoverability via `__schema` | PASS | Runtime command trees and existing schema goldens are unchanged. `surfacecheck` is internal build tooling rather than an adopting CLI runtime. |
| IV. Agent-Safety Primitives | N/A | No adopting CLI behavior changes. |
| V. Asymmetric JSON I/O | PASS | Audit/baseline reads are strict JSON and size-capped; stdout writes are strict minified JSON. |
| VI. ADR-Governed Scope | PASS | The feature is Spec Kit governed, creates only internal maintainer tooling, adds no domain behavior, and creates/edits no ADR. |
| VII. Test-First Discipline | PASS | Table, golden, fuzz, stream/exit, deterministic-output, race, lint, vet, documentation, coverage, and existing benchmark gates are explicit. |
| VIII. Observability & ID Discipline | N/A | No telemetry, logging, or identifier behavior changes. |
| IX. Security & Resource Safety | PASS | File reads and subprocess output are bounded; subprocesses receive context cancellation; errors wrap with `%w`; no panic or network surface is introduced. |
| X. Idiomatic Go & Dependency Minimalism | PASS | Complete type information comes from compiler export data using stdlib packages. Clear non-public mechanics move to role-specific `internal/` packages. |
| XI. Stability & SemVer | PASS | Feature 015 makes no breaking Go or payload change. Deprecations ship through a non-breaking `feat:` minor. The follow-up removal is separately specified and uses the breaking-change process. |
| XII. Deprecation Lifecycle | PASS | Every retirement candidate remains exported with a valid notice. Removal is forbidden until at least one published minor carried that notice and a follow-up feature verifies it. |

**ADR absorption gate**: N/A. No governing ADR exists and no ADR is deleted.

## Phase 0: Research Decisions

Research is complete in [research.md](research.md):

- D1: Type-aware compiler export inventory across supported profiles.
- D2: Canonical API feature identity and reachability rules.
- D3: Permanent audit separated from the live baseline.
- D4: Constitution-compliant deprecate/publish/remove lifecycle.
- D5: Compatibility-preserving internalization policy.
- D6: Deterministic JSON and exit/stream contract.
- D7: Local/CI wiring and module-root invocation.
- D8: Package allowlist versus identifier classification.
- D9: Boundary governance and downstream evidence.

No `NEEDS CLARIFICATION` items remain.

## Phase 1: Design

### Data and contracts

- [data-model.md](data-model.md) defines API Feature, Audit Record, Live
  Baseline Entry, Target Profile, Drift Item, and Gate Result.
- [contracts/audit-schema.md](contracts/audit-schema.md) defines the permanent
  historical decision artifact.
- [contracts/baseline-schema.md](contracts/baseline-schema.md) defines the live
  operational surface projection.
- [contracts/surfacecheck-output.md](contracts/surfacecheck-output.md) defines
  flags, streams, JSON shapes, errors, and exit codes.
- [quickstart.md](quickstart.md) documents module-root usage, audit approval,
  safe baseline updates, deprecation publication, and the follow-up removal
  boundary.

### Delivery checkpoints

1. **Inventory and gate foundation**: tests first; implement the type-aware
   six-profile scanner, strict parsers, deterministic output, and CI wiring.
2. **Audit approval**: generate `public-surface-audit.json`; record every
   classification, rationale, downstream search, internal target, and
   compatibility strategy. No migration task starts until this artifact is
   reviewed.
3. **Non-breaking internalization**: move only approved mechanics with proven
   compatibility forwarders; add deprecation notices; migrate in-repo call
   sites; keep API and payload goldens unchanged.
4. **Notice release handoff**: land with a non-breaking `feat:` commit and
   record that an actual published `0.MINOR.0` is required before removal.
   Create/reference the follow-up Spec Kit feature; do not perform removal in
   feature 015.

## Project Structure

### Documentation

```text
specs/015-internalize-helpers/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── public-surface-audit.json    # P1 permanent decision record
├── quickstart.md
├── contracts/
│   ├── audit-schema.md
│   ├── baseline-schema.md
│   └── surfacecheck-output.md
└── tasks.md                     # generated by /speckit-tasks
```

### Source

```text
internal/cmd/surfacecheck/
├── main.go
├── main_test.go
├── inventory.go
├── inventory_test.go
├── testdata/
└── baseline.json

internal/<role>/                 # approved implementation relocation targets
*.go                            # temporary deprecated compatibility forwarders
examples/integration/
Makefile
.github/workflows/ci.yml
AGENTS.md
```

**Structure Decision**: Keep the public facade at the module root. Use
role-specific existing or narrowly named new internal packages for mechanics.
Keep historical audit evidence in the feature directory and the operational
baseline beside the gate. The apidiff public-package allowlist is unchanged.

## Complexity Tracking

No constitution violations or unjustified complexity remain.

| Design choice | Why needed | Simpler alternative rejected because |
|---------------|------------|---------------------------------------|
| Type-aware six-profile inventory | The gate claims the complete externally selectable API and the project supports six build profiles. | Syntax-only AST scanning misses fields/interface members and includes unreachable hidden receivers; a host-only scan misses platform-only drift. |
| Permanent audit plus live baseline | Decision history must survive removal while the gate needs a current projection. | Deleting stale rows destroys the issue's required audit; one overloaded list gives historical and live rows conflicting validation rules. |
| Two-feature deprecation/removal lifecycle | A published minor is an external temporal boundary required by Principle XII. | One PR cannot both publish a notice and prove that a published release carried it. |

## Post-Design Constitution Re-Check

Re-evaluated after research, data-model, contracts, and quickstart generation.
All gates pass. The earlier Principle XII exception, plain-text stdout,
syntax-only inventory, disappearing audit rows, package/identifier conflation,
unexport-in-place disposition, unbenchmarked timing claim, and run-from-anywhere
claim have been removed. No runtime payload, supported API, ADR, dependency,
coverage floor, or performance budget is changed by the design.
