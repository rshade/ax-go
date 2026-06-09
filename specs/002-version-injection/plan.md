# Implementation Plan: Build-time Version Injection via -ldflags

**Branch**: `002-version-injection` | **Date**: 2026-06-08 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-version-injection/spec.md`

## Summary

`__schema.version` ships empty because the reference example holds its version in
a `const` (which the linker cannot rewrite) and no build wiring injects a value —
a direct violation of constitution Principle X ("Version MUST be injected at
build via `-ldflags`; never ship `dev` or `unknown`"). This feature makes a real,
non-empty, VCS-derived version flow into every version surface (`__schema.version`,
the `ax.Error` envelope `version`, and the logger `version` label) for every
build.

Technical approach: add one small, pure, public helper `ax.ResolveVersion(injected
string) string` that applies a deterministic precedence — injected link-time value
→ Go toolchain build metadata (`runtime/debug.ReadBuildInfo`: main-module version,
then VCS revision + dirty marker) → a documented non-empty sentinel
(`0.0.0-unknown`). A new Makefile `build-example` target injects
`git describe --tags --always --dirty` into the example's `main.version` variable;
the example flips its `const` to a `var`, calls `ax.ResolveVersion`, and feeds the
single resolved value to both `ax.WithVersion` and the logger labels. README and
the example README document the recipe so adopters replicate it with one helper
call plus one build flag.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/rshade/ax-go`, package `ax`)

**Primary Dependencies**: standard library only — `runtime/debug` for build-info
fallback. NO new third-party dependency (Principle X dependency-minimalism). Cobra
and the existing `WithVersion`/`WithSchemaVersion`/`Labels.Version` plumbing are
reused unchanged.

**Storage**: N/A

**Testing**: `go test -race ./...`; table-driven tests for the resolver over a
pure, build-info-as-parameter seam; a verified `ExampleResolveVersion` (doc-
coverage gated); the `examples/integration` test asserts `__schema.version` is
non-empty. No numeric performance claim → no benchmark required.

**Target Platform**: cross-platform Go CLIs built on ax-go (the helper reads the
*running binary's* build info, so it is platform-agnostic).

**Project Type**: single Go library at module root + one reference example binary
(`examples/integration`). No new `cmd/` binary is created.

**Performance Goals**: N/A — version is resolved once at process start, never on a
hot path. The only timing-shaped requirement is determinism (SC-005): a given
binary always reports the same version.

**Constraints**: resolved version MUST be non-empty and MUST NEVER be the bare
strings `dev`/`unknown` (FR-004/FR-005); deterministic per build (FR-008); no
`panic` (Principle IX) — the resolver fails closed to the sentinel; no
`context.Context` parameter because the helper performs no I/O and is not
cancelable (`ReadBuildInfo` reads in-binary data); a single source of truth feeds
all three version surfaces (FR-007).

**Scale/Scope**: small (effort/small). ~1 new public function in 1 new file
(`version.go`), 1 new test file, a one-line `requiredSymbols()` edit, the example
wiring, a Makefile target, and two docs sections.

**Governing ADR(s)**: N/A. Issue #6 references ADR-0003, but ADR-0003 governs the
`__schema` *output format* (the broader discoverability contract), which this
feature does NOT change — it only populates the already-specified `version` field.
The governing decision for build-time version injection is constitution Principle
X, which is already ratified, so there is no ADR to absorb or retire in this
feature. ADR-0003 stays frozen and untouched.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Verdict | Notes |
|-----------|---------|-------|
| I. Stream Separation | ✅ PASS | `version` is part of the `__schema` machine payload on `stdout` and of the `ax.Error` envelope / logs on `stderr` — exactly where each already goes. No new stream behavior. |
| II. Determinism & Exit Codes | ✅ PASS | A given binary always reports the same version (SC-005); the value is build identity, not runtime non-determinism, so it is constant within a binary — consistent with "byte-identical stdout per build". No exit-code change. |
| III. `__schema` Discoverability | ✅ PASS | Fills the existing `Schema.Version` field; the schema *shape* is unchanged, so root-package golden tests (which pass explicit fixed versions) are unaffected. The example test asserts non-empty rather than golden-pinning a dynamic VCS value. |
| IV. Agent-Safety Primitives | ✅ PASS | Untouched. |
| V. Asymmetric JSON I/O | ✅ PASS | Untouched. |
| VI. Library, Not Application | ✅ PASS | The helper is a cross-cutting AX identity primitive (in scope); no state, no `init()`, no new CLI framework, no new `cmd/` binary. The public-API addition is specified through this Spec Kit feature (not a new ADR), satisfying the Principle VI change-governance rule. |
| VII. Test-First Discipline | ✅ PASS (by plan) | Tests land first (table-driven resolver + `ExampleResolveVersion` + example non-empty assertion); doc comment on the new exported symbol (godoclint `require-doc`); added to `doccover` `requiredSymbols()` and shipped *with* its example so no baseline entry is needed; `-race`, `go vet`, `golangci-lint`, `make doc-coverage` all clean. |
| VIII. Observability & ID Discipline | ✅ PASS | `version` is a low-cardinality **label** (already so in `logger.go`) — the cardinality split is preserved. No ID semantics involved. |
| IX. Security & Resource Safety | ✅ PASS | No `panic` — resolver fails closed to the sentinel; no unbounded read; version is build/VCS-derived (trusted) and flows through zerolog field methods as a label, never `.Msg(fmt.Sprintf(...))`. |
| X. Idiomatic Go & Dependency Minimalism | ✅ PASS (directly implements) | This feature *is* Principle X's "Version MUST be injected at build via `-ldflags`". Stdlib-only; no mutable package-level state beyond the conventional injectable `var version` in the consumer's `main` (not in package `ax`); no `context` needed (no I/O). |

**ADR absorption gate (Constitution §Governance)**: Governing ADR(s) = N/A, so no
"Decision Records Absorbed" section and no ADR-retirement task are required. ADR-0003
is referenced for context only and is not deleted by this feature.

**Result**: All gates pass. No entries in Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/002-version-injection/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 output — resolution-precedence & sentinel decisions
├── data-model.md        # Phase 1 output — version entities & resolution state table
├── quickstart.md        # Phase 1 output — build-with-injection + adopter recipe
├── contracts/
│   └── version-api.md   # Phase 1 output — ResolveVersion + build-target + surfaces contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
# Module github.com/rshade/ax-go, public package `ax` at module root
version.go                 # NEW — public ResolveVersion(injected) + unexported pure
                           #       resolveVersionFrom(injected, *debug.BuildInfo, ok) seam
                           #       + sentinel const versionUnknown = "0.0.0-unknown"
version_test.go            # NEW — table-driven resolver tests (synthetic BuildInfo) +
                           #       ExampleResolveVersion (verified, // Output:)
execute.go                 # UNCHANGED — WithVersion already threads version → schema + envelope
schema.go                  # UNCHANGED — Schema.Version already wired
logger.go                  # UNCHANGED — Labels.Version already a low-cardinality label

internal/cmd/doccover/
└── main.go                # EDIT — add "ResolveVersion" to requiredSymbols() (entry points)
                           #        (baseline.txt unchanged: ships with its example)

examples/integration/
├── main.go                # EDIT — `const version` → `var version`; resolve via
                           #        ax.ResolveVersion(version); feed ONE value to
                           #        ax.WithVersion(...) AND ax.WithLoggerLabels(Version:...)
├── main_test.go           # EDIT — assert __schema.version is non-empty
└── README.md              # EDIT — document the `make build-example` / -ldflags build

Makefile                   # EDIT — VERSION (git describe --tags --always --dirty),
                           #        LDFLAGS, and a `build-example` target
README.md                  # EDIT — "Build-time version injection" adopter section
CLAUDE.md                  # EDIT (Phase 1 step 3) — SPECKIT markers → point to this plan
```

**Structure Decision**: Single Go library at the module root with a reference
example binary — the existing repository layout. The only new files are
`version.go` and `version_test.go` at the root (public package `ax`); everything
else is an edit to existing wiring, the Makefile, and docs. No new package and no
new `cmd/` binary, consistent with Principle VI (library, not application) and the
effort/small scope.

## Complexity Tracking

> No Constitution Check violations. No justifications required.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| (none)    | —          | —                                    |
