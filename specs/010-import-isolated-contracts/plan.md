# Implementation Plan: Import-Isolated Contracts

**Branch**: `010-import-isolated-contracts` | **Date**: 2026-06-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/010-import-isolated-contracts/spec.md`

## Summary

Create narrow public contract packages so thin consumers can reuse ax-go's configuration, schema, machine-envelope, mode, exit-code, and ID contracts without importing the root runtime facade. The root `ax` package remains the ergonomic compatibility surface: existing public symbols stay available, delegate to the new package boundaries where practical, and preserve current machine-readable output shapes. Import-isolation tests become the enforcement mechanism for keeping contract surfaces free of execute, telemetry exporter, HTTP/gRPC instrumentation, logger, and Loki dependencies.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: Existing dependency set only. New public package boundaries reuse stdlib, `github.com/tailscale/hujson`, `github.com/spf13/cobra`, `github.com/spf13/pflag`, and `github.com/google/uuid`; no new dependency is planned.

**Storage**: N/A. This is an import/API boundary feature with no persistent state.

**Testing**: `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `make doc-coverage`; focused tests include import-isolation checks and existing golden tests for `ax.Error` and `__schema`.

**Target Platform**: Go library consumers on the same platforms already supported by the module and CI.

**Project Type**: Go library with root facade package plus narrowly scoped public subpackages.

**Performance Goals**: Thin consumers that import only contract surfaces avoid runtime adapter dependency graphs; package-level import checks complete within the normal test suite budget.

**Constraints**:
- Root `ax` public symbols covered by this feature remain available and behavior-compatible in this phase.
- Contract surfaces must not import root `github.com/rshade/ax-go`.
- Contract surfaces must not import `github.com/rshade/ax-go/internal/telemetry`, `go.opentelemetry.io/otel/exporters/*`, `go.opentelemetry.io/otel/sdk`, `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`, `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc`, `google.golang.org/grpc`, `github.com/rs/zerolog`, or Loki implementation code.
- No public deprecations or removals in this phase.
- No new ADRs; legacy ADR decisions are absorbed into `research.md`.

**Scale/Scope**: Four new public import surfaces plus root facade wrappers, documentation updates, examples, and import-isolation tests.

**Governing ADR(s)**: `docs/adr/0001-agent-mode-trigger.md`, `docs/adr/0002-error-envelope-schema.md`, `docs/adr/0003-schema-output-format.md`, `docs/adr/0007-id-strategy.md`, `docs/adr/0012-directory-layout.md` — decisions absorbed into `research.md`; retirement tasks must be final tasks in `tasks.md`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | PASS | Contract packages expose payload/error helpers but must not write logs, prompts, or progress to stdout/stderr during import. JSON writers retain payload-only behavior. |
| II. Deterministic Output & Exit Codes | PASS | Existing envelope and exit-code contracts remain stable and guarded by golden tests; new package surfaces use structs for public shapes. |
| III. Machine Discoverability via `__schema` | PASS | Schema contract is explicitly preserved and import-isolated; golden tests continue to guard output shape. |
| IV. Agent-Safety Primitives | PASS | Mode, dry-run metadata, and idempotency-key contracts move behind import-isolated surfaces while root behavior remains compatible. |
| V. Asymmetric JSON I/O | PASS | Config package keeps bounded Hujson reads and comment-preserving patch behavior; output helpers emit strict JSON/NDJSON only. |
| VI. ADR-Governed Scope | PASS | This is a Spec Kit feature for public API boundaries; no domain commands, auth, orchestration, persistence, or natural-language behavior are added. |
| VII. Test-First Discipline | PASS | Tasks must start with failing import-isolation, compatibility, doc-example, and golden-preservation tests before implementation. |
| VIII. Observability & ID Discipline | PASS | Runtime telemetry remains in root/runtime surfaces; ID helpers retain UUID v4/v7 policy and do not mix with trace/span IDs. |
| IX. Security & Resource Safety | PASS | No unbounded reads, network side effects, TLS changes, or logging behavior are introduced by contract imports. |
| X. Idiomatic Go & Dependency Minimalism | PASS | Uses existing dependencies only; public subpackages are justified by a consumer-driven import-isolation need and documented through this feature. |
| XI. Stability & SemVer | PASS | New public surfaces are additive; root removals/deprecations are out of scope. Release is a pre-v1 minor feature. |
| XII. Deprecation Lifecycle | PASS | No deprecations in this phase; future cleanup must use the published lifecycle. |

**ADR absorption gate (Constitution §Governance)**: PASS for planning. `research.md` contains a "Decision Records Absorbed" section for all governing ADRs. `tasks.md` MUST include final ADR-retirement tasks and MUST NOT delete ADRs before those decisions are present in `research.md`.

**Post-design re-check**: PASS. Phase 1 artifacts keep the feature additive, preserve root compatibility, and document boundary tests as acceptance contracts.

## Project Structure

### Documentation (this feature)

```text
specs/010-import-isolated-contracts/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── import-isolation.md
│   └── public-packages.md
└── tasks.md
```

### Source Code (repository root)

```text
contract/
├── context.go
├── error.go
├── exit.go
├── json.go
├── mode.go
└── doc.go

config/
├── config.go
├── config_test.go
├── fuzz_test.go
├── example_test.go
└── import_isolation_test.go

schema/
├── schema.go
├── schema_test.go
├── example_test.go
└── import_isolation_test.go

id/
├── id.go
├── id_test.go
├── fuzz_test.go
├── example_test.go
└── import_isolation_test.go

internal/
├── config/
├── schema/
├── mcp/
└── telemetry/

testdata/
├── error_envelope.golden.json
└── schema_*.golden.json

README.md
AGENTS.md
examples/integration/
```

**Structure Decision**: Public subpackages are intentionally limited to contract surfaces with thin-consumer reuse value. Root `ax` remains the primary full-runtime package and delegates to the new packages where compatible. Runtime adapters stay in root/internal boundaries: `Execute`, telemetry startup/export, logger/Loki, and HTTP/gRPC helpers are not split in this feature.

## Complexity Tracking

No constitution violations. Public subpackages are justified by this Spec Kit feature, absorb the conflicting legacy directory-layout ADR decision into `research.md`, and remain narrowly scoped to reusable machine contracts.
