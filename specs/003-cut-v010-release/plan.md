# Implementation Plan: Cut v0.1.0 — Make ax-go a Consumable, Pinnable Release

**Branch**: `003-cut-v010-release` | **Date**: 2026-06-09 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/003-cut-v010-release/spec.md`

## Summary

Mint the first pinnable release of ax-go by closing three gaps before the
immutable `v0.1.0` tag: (A) confirm spec-002's version-injection deliverables
are complete end-to-end in the integration example (verification only — all
deliverables are already in place, see research D3); (B) freeze the last two
unlocked public output contracts — the success `Envelope[T]` JSON shape and the
NDJSON line shape — with golden fixtures pinned via existing context-injection
seams (research D4–D7); and (C) fix the release-please workflow by switching to
`GITHUB_TOKEN` and land a one-shot `Release-As: 0.1.0` commit footer, which
research proved is mandatory (release-please proposes `1.0.0` from a `0.0.0`
manifest — issue #2087, research D1). The release artifact is the git tag;
`CHANGELOG.md` is generated exclusively by release-please.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/rshade/ax-go`, public package `ax` at module root)

**Primary Dependencies**: existing only — `spf13/cobra`, `rs/zerolog`, `go.opentelemetry.io/otel/trace` (fixed `SpanContext` injection in tests), `tailscale/hujson`, `google/uuid`. **Zero new dependencies** (FR-015).

**Storage**: N/A — golden fixtures are flat files under `testdata/`

**Testing**: `go test -race ./...`; table-driven tests + `assertGolden` byte-comparison harness (`golden_test.go`); `make ci` = test + vet + `golangci-lint run` + `make doc-coverage`

**Target Platform**: any Go platform (library); CI on `ubuntu-latest`; release automation via `googleapis/release-please-action@v5`

**Project Type**: single Go library + GitHub Actions release pipeline

**Performance Goals**: N/A — no allocation/hot-path claims are made, so no benchmarks are required (Constitution VII)

**Constraints**: byte-identical golden outputs with zero harness normalization (FR-008); `stdout`/`stderr` separation inviolate in all test code (FR-016); `CHANGELOG.md` never hand-edited (FR-011); release PR must propose exactly `0.1.0` (FR-010 + research D1)

**Scale/Scope**: 2 new golden tests + 2 fixtures; 1 workflow file edit; 1 README status edit; commit 2 untracked fixtures; 1 fixture-coverage audit (complete — research D6); release execution + post-tag verification (FR-013/SC-001/SC-005)

**Governing ADR(s)**: N/A — no ADR is retired by this feature; ADR-0003 remains frozen and was already absorbed by `specs/002-version-injection/research.md` (research D9)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Status | Evidence |
|---|---|---|---|
| I | Stream Separation | ✅ PASS | New tests write to `bytes.Buffer`; nothing touches `os.Stdout`. FR-016 restates the gate for all new code. |
| II | Deterministic Output & Exit Codes | ✅ PASS | The feature's purpose is locking determinism. Fixture `data` uses a struct, never a map; non-deterministic fields pinned by injection (D4). |
| III | Machine Discoverability via `__schema` | ✅ PASS | Both `__schema` formats already golden-locked (`schema_ax`/`schema_mcp`); FR-007 audit confirms coverage (D6). |
| IV | Agent-Safety Primitives | ✅ PASS | Idempotency-key surfacing in the envelope is part of the frozen fixture (D4); no primitive behavior changes. |
| V | Asymmetric JSON I/O | ✅ PASS | No I/O behavior changes; fixtures assert the strict-minified-JSON write side. |
| VI | ADR-Governed Scope | ✅ PASS | No new ADRs; change flows through Spec Kit; no public API change — fixtures freeze existing shapes only. |
| VII | Test-First Discipline | ✅ PASS | Golden tests land before fixtures; first run fails at `os.ReadFile` for the right reason (D7). No doc-coverage change: no new exported symbols. |
| VIII | Observability & ID Discipline | ✅ PASS | Trace/span pinning uses the OTel seam (`ContextWithSpanContext`); no ID-class mixing. |
| IX | Security & Resource Safety | ✅ PASS | Workflow drops the PAT secret reference in favor of scoped `GITHUB_TOKEN` (least privilege); no parsing/input surface added. |
| X | Idiomatic Go & Dependency Minimalism | ✅ PASS | Zero new deps (FR-015); version injected at build via `-ldflags` (verified, D3); test-only changes to Go code. |

**ADR absorption gate (Constitution §Governance)**: Governing ADR(s) = N/A →
gate satisfied vacuously. `research.md` carries the "Decision Records Absorbed:
None" section; tasks.md requires no ADR-retirement task (D9).

**Post-Phase-1 re-check (2026-06-09)**: design artifacts introduce no new
violations — contracts freeze existing wire shapes verbatim; no Complexity
Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/003-cut-v010-release/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output; holds "Decision Records Absorbed: None" (D9)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   ├── success-envelope.md
│   └── ndjson-line.md
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
json.go                          # Envelope[T], Metadata, WriteJSON, WriteJSONLine — shapes frozen, NOT modified
json_test.go                     # + TestEnvelopeGolden, TestWriteJSONLineGolden (new golden tests)
golden_test.go                   # assertGolden harness — unchanged (D7: no update mode)
testdata/
├── success_envelope.golden.json # NEW — frozen Envelope[T] success shape (FR-004)
├── ndjson_line.golden.json      # NEW — frozen NDJSON line shape (FR-005)
├── config_patch_invalid.golden.json         # untracked → committed (D6)
├── config_patch_hujson_invalid.golden.json  # untracked → committed (D6)
└── *.golden.json                # 7 existing fixtures — audit-confirmed, unchanged
.github/workflows/
└── release-please.yml           # token: RELEASE_PLEASE_TOKEN → GITHUB_TOKEN (FR-009, D2)
release-please-config.json       # unchanged — bump policy verified (D1)
.release-please-manifest.json    # unchanged — "0.0.0"; Release-As footer handles first version (D1)
README.md                        # status blockquote: scaffold → release + changelog link (FR-012, D8)
Makefile                         # unchanged — build-example target verified complete (D3)
examples/integration/            # unchanged — version wiring verified complete (D3)
```

**Structure Decision**: Single-project Go library layout (constitution-mandated:
public package `ax` at module root, no `pkg/`/`src/`). All Go changes are
test-only additions to `json_test.go` plus fixture files; the remaining
changes are CI workflow, README, and release-process execution.

## Workstream Sequencing

```text
B (freeze contracts)  ──┐
A (verify spec-002)   ──┼──►  hygiene gate (D10: clean tree, make ci green)
C1 (workflow+README fix, Release-As footer) ──┘
                                   │
                                   ▼
                     C2: release PR opens → verify it proposes 0.1.0 → merge
                                   │
                                   ▼
                     C3: post-tag verification (FR-013, SC-001, SC-005)
```

- **B before the tag** is the hard ordering constraint (US2 priority
  rationale: the module proxy caches the tag immutably).
- **C1's commit carries the `Release-As: 0.1.0` footer** (D1) so the very next
  release-please run on `main` proposes the correct version.
- **C3 is post-merge verification only** — `go get github.com/rshade/ax-go@v0.1.0`
  resolution, changelog section presence, and a tag-built example reporting
  `v0.1.0` everywhere (SC-005).

## Pre-Implementation Precondition (research D10)

The working tree currently holds unresolved merge conflicts
(`example_test.go`, `internal/cmd/doccover/main.go`) and uncommitted
modifications. These MUST be resolved before any task in this feature executes:
FR-014 / SC-006 (`make ci` green on the tagged commit) is unachievable on a
conflicted tree. This is release hygiene that gates the feature, not feature
work itself.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations — table intentionally empty.
