# Phase 0 Research: Build-time Version Injection via -ldflags

**Feature**: `002-version-injection` | **Date**: 2026-06-08

The spec's two open decisions were resolved interactively at `/speckit-specify`
time (see spec.md → Clarifications): the un-injected fallback is a runtime
build-info fallback, and the reuse surface is a public helper plus example plus
docs. Phase 0 turns those into concrete, testable design decisions. There are no
remaining NEEDS CLARIFICATION markers.

## Decisions

### D1 — Resolution precedence with a pure, testable seam

**Decision**: Expose `ax.ResolveVersion(injected string) string`. It reads the
running binary's build info once via `runtime/debug.ReadBuildInfo()` and delegates
all logic to an unexported pure function
`resolveVersionFrom(injected string, info *debug.BuildInfo, ok bool) string`.
Precedence (FR-004):

1. `injected` is non-empty and not a bare placeholder (`dev`, `unknown`, or
   `(devel)`) → return `injected` (the link-time `-X` value wins).
2. else if `ok` (build info present):
   a. main-module version is a real release version → return it;
   b. else a VCS revision is present → return `revision[ + dirty marker ]`.
3. else → return the sentinel (D3).

**Rationale**: Keeping `ResolveVersion` a thin shell over a pure function makes
every branch table-testable with a synthetic `*debug.BuildInfo`, without mocking
the toolchain — exactly the "pure transformation function / don't mock what you
don't own" guidance. `ReadBuildInfo` returns the *main program's* build info
regardless of the calling package, so an adopter calling `ax.ResolveVersion` from
their `main` gets *their* binary's VCS data, not ax-go's.

**Alternatives considered**: (a) A `func() (*debug.BuildInfo, bool)` field/closure
seam on a struct — heavier than needed for one pure function. (b) Reading build
info inside the exported function with no seam — untestable fallback branches.
(c) Resolving entirely in the Makefile and passing a non-empty value always —
leaves `go run`/bare `go build` (the exact origin of the empty-version bug)
unprotected, violating FR-004/SC-003.

### D2 — Build-info fallback details (module version, revision, dirty marker)

**Decision**:

- **Main-module version** = `info.Main.Version`, used only when it is non-empty
  AND not the Go placeholder `"(devel)"`. This surfaces a real tag for
  `go install <module>@<version>` builds.
- **VCS revision** = the `vcs.revision` value from `info.Settings`; **dirty** =
  `vcs.modified == "true"`. The returned string is the revision with a `-dirty`
  suffix appended when modified, e.g. `9fceb02...` or `9fceb02...-dirty`.
- The `-dirty` suffix is chosen to match the Makefile's
  `git describe --tags --always --dirty` output (D4), so the injected path and the
  fallback path read consistently to an agent.

**Rationale**: This covers every realistic build mode with a real identifier:
`go install pkg@v1.2.3` → `v1.2.3`; `go build`/`go run` from a Git tree →
`revision[-dirty]`; nothing available → sentinel. The full (untruncated) revision
is used to stay deterministic and unambiguous; truncation is a cosmetic choice the
injected `git describe` path already makes and is not needed for correctness.

**Alternatives considered**: Truncating the revision to a short SHA in the
fallback — rejected as needless logic that risks collisions and adds a branch to
test; the injected `git describe --always` path already yields a short SHA when a
human wants brevity.

### D3 — Sentinel value: `0.0.0-unknown`

**Decision**: The last-resort sentinel (no injection, no build info at all) is the
constant `versionUnknown = "0.0.0-unknown"`.

**Rationale**: It is non-empty, parses as a SemVer pre-release so it sorts *below*
any real release, is visibly non-production, and is NOT the bare string `unknown`
or `dev` that FR-005 / Principle X forbid. It only appears in pathological builds
(no `-ldflags`, no VCS metadata — e.g. an extracted source tarball), never on the
documented build path.

**Alternatives considered**: `dev`, `unknown`, `""` — all forbidden or empty.
`v0.0.0` alone — indistinguishable from a real zero release; the `-unknown`
pre-release tag makes the cause explicit.

### D4 — Makefile injects `git describe --tags --always --dirty`

**Decision**: Add `VERSION ?= $(shell git describe --tags --always --dirty
2>/dev/null || echo 0.0.0-unknown)` and a `build-example` target that runs
`go build -ldflags "-X main.version=$(VERSION)" -o bin/ax-integration
./examples/integration`. The existing `build` target (`go build ./...`, a library
compile) is left as-is — `-X main.version` has no effect there (FR-010).

**Rationale**: `--tags --always --dirty` is the one invocation that satisfies
FR-003 in this repo's current state: with **zero tags** (verified: `git tag`
lists nothing), a tags-only `git describe` *fails*, but `--always` degrades to the
commit SHA and `--dirty` marks a modified tree — e.g. today it yields
`5bf9b77-dirty`. The `|| echo 0.0.0-unknown` guard keeps the build working outside
a Git checkout. Once the first release tag exists, the same target yields
`v1.2.3` / `v1.2.3-4-g<sha>` with no change.

**Alternatives considered**: `git describe --tags` (fails with no tags — breaks
the build today); embedding only the build-info VCS data and no `-ldflags`
(works, but the issue and Principle X explicitly require the `-ldflags "-X"`
recipe, and `git describe` gives richer tag-aware output than `ReadBuildInfo`).

### D5 — No `context.Context`; fail closed, never panic

**Decision**: `ResolveVersion` takes no context and returns no error. It cannot
fail: every path returns a non-empty string, and the sentinel is the floor.

**Rationale**: The constitution's context-first mandate (Principle X) applies to
functions that do I/O, make outbound calls, run goroutines, or are cancelable.
`ReadBuildInfo` reads data embedded in the binary — no syscall, no cancelation
point — so a context parameter would be ceremony with no semantics. Returning an
error for "no version" is rejected because FR-004 guarantees a non-empty result;
the caller has nothing actionable to do, and a non-erroring signature keeps the
adopter recipe a single expression (SC-004). No `panic` (Principle IX): the
sentinel is the closed-fail value.

### D6 — One resolved value feeds all three version surfaces (FR-007)

**Decision**: The consumer resolves once and passes the same string to both
`ax.WithVersion(resolved)` (which already threads it into `Schema.Version` and the
`ax.Error` envelope `version` via `execute.go`) and
`ax.WithLoggerLabels(ax.Labels{Version: resolved})`. The reference example is
updated to do exactly this; the README recipe shows it.

**Rationale**: The version flows from `WithVersion` → `cfg.version` →
`ensureSchemaCommand`/`WithSchemaVersion` (schema) and → `WithErrorVersion`
(envelope) — two of the three surfaces already share one source. The logger label
is set independently by the consumer (`logger.go` reads `Labels.Version`), so
ax-go cannot *force* agreement; the contract is satisfied by the documented
resolve-once pattern, which the example demonstrates and its test guards. No
change to `execute.go`/`schema.go`/`logger.go` is needed — only correct usage.

**Alternatives considered**: Auto-injecting the resolved version into the logger
from inside `ax.Execute` — rejected: it would couple logger construction to
Execute, the logger is constructed by the consumer with its own labels, and the
constitution warns against widening the logger seam (Principle VI / ADR-0009
boundary). Documentation + example is the proportional fix.

### D7 — Public API shape, naming, and doc-coverage gating

**Decision**: One new exported symbol — `func ResolveVersion(injected string)
string` — in a new `version.go`. Add `"ResolveVersion"` to `doccover`'s
`requiredSymbols()` under "entry points", and ship a verified
`ExampleResolveVersion` so no `baseline.txt` line is needed. Carry a contract doc
comment (inputs, precedence, the non-empty/never-`dev` guarantee).

**Rationale**: It is a top-level entry point an agent/adopter calls directly, so
Principle VII puts it on the gated primary-API surface. The name mirrors the
existing `WithVersion`/`WithSchemaVersion` vocabulary and reads as a verb at the
call site: `ax.WithVersion(ax.ResolveVersion(version))`. Naming an exported
identifier is itself a governed public-API decision (Principle VI) — recorded
here.

**Alternatives considered**: `Version(injected)` (noun/verb ambiguity, collides
conceptually with the `version` field); a `WithResolvedVersion` Execute option
that internally reads build info (hides the seam, can't also feed the logger
label, and reads build info on every Execute rather than once at startup);
returning a struct of `{Source, Value}` (over-engineered for effort/small — the
string is the contract).

### D8 — Testing strategy

**Decision**:

- **Table-driven** tests on `resolveVersionFrom` covering: injected wins; module
  version used (skipping `(devel)`); revision used; revision + `-dirty`; no info →
  sentinel; `(devel)` main version with no VCS → sentinel.
- **`ExampleResolveVersion`** with `// Output:` — verified by `go test`,
  doc-coverage gated, and demonstrating the injected-value path (deterministic
  output, no build-info dependence).
- **Integration**: `examples/integration` test asserts `__schema.version` is
  non-empty (it cannot golden-pin a dynamic VCS value).
- **Race**: `go test -race ./...` as always.

**Rationale**: The pure seam (D1) makes the fallback branches deterministically
testable without controlling the toolchain. The example asserts the observable
end-state. SC-005 (per-build determinism) is covered by the example's fixed
`// Output:` for the injected path; the dynamic fallback is not golden-pinnable by
nature, which is why FR-008 scopes determinism to "same commit + same build
inputs, modulo the dirty marker".

**Optional hardening (not required, may be deferred)**: a `FuzzResolveVersion`
over the `injected` and synthetic revision strings to assert it never panics and
never returns empty. Listed as optional because `ResolveVersion` is not a
byte-level parser surface (its input is a structured `*debug.BuildInfo`), so the
AGENTS.md "fuzz every parser surface" rule does not strictly bind it.

### D9 — No governing ADR to absorb

**Decision**: This feature retires no ADR. Governing ADR(s) = N/A.

**Rationale**: Issue #6 cites ADR-0003, but ADR-0003 ("`__schema` Output Format")
governs the *shape* of the discoverability surface, which this feature does not
modify — it only fills the already-specified `version` field. The decision that
mandates build-time injection is constitution **Principle X** ("Version MUST be
injected at build via `-ldflags`…"), already ratified at v1.1.0. Because no ADR
*governs* this feature's contract, there is nothing to transcribe into a "Decision
Records Absorbed" section and no ADR-retirement task. ADR-0003 remains frozen and
untouched (it will be absorbed by a future feature that actually changes the
`__schema` format).

## Decision Records Absorbed

None. Governing ADR(s) = N/A (see D9). ADR-0003 is referenced for context only and
is not retired by this feature.
