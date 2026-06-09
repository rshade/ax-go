# Phase 1 Data Model: Build-time Version Injection

**Feature**: `002-version-injection` | **Date**: 2026-06-08

This feature introduces no persisted data and no new envelope schema. The "data"
is the *version string* and the inputs that resolve it. The entities below map the
spec's Key Entities to concrete shapes and the resolution rules.

## Entities

### Resolved version (the output)

- **Type**: `string`.
- **Invariant**: NEVER empty; NEVER the bare strings `dev` or `unknown`
  (FR-004/FR-005).
- **Surfaces** (FR-007): the same value appears in all three —
  - `__schema.version` (`Schema.Version`, via `ax.WithVersion` → `WithSchemaVersion`),
  - the `ax.Error` envelope `version` (via `WithErrorVersion`),
  - the logger `version` **label** (via `ax.Labels{Version: ...}`).
- **Determinism**: constant for a given binary; identical across two builds from
  the same clean commit, modulo the dirty marker (FR-008/SC-005).

### Injected version variable (link-time input)

- **Shape**: a package-level `var version string` in the consumer's `main`
  package (the example uses exactly this). MUST be a `var`, not a `const`
  (FR-002) — the linker's `-X importpath.name=value` only rewrites string
  *variables*.
- **Default**: empty string (un-injected) — which is precisely why a fallback is
  required.
- **Set by**: `go build -ldflags "-X main.version=<value>"` (the Makefile
  `build-example` target supplies `<value>` from `git describe`). Bare
  placeholder values (`dev`, `unknown`, and `(devel)`) are treated as absent so
  the public guarantee never returns them.

### VCS build metadata (fallback input)

- **Source**: `runtime/debug.ReadBuildInfo() (*debug.BuildInfo, bool)` — reads the
  running binary's embedded info; no I/O.
- **Fields used**:
  - `info.Main.Version` — main-module version; `"(devel)"` for local `go build`,
    a real tag (e.g. `v1.2.3`) for `go install module@version`.
  - `info.Settings[]` where `Key == "vcs.revision"` — the commit hash.
  - `info.Settings[]` where `Key == "vcs.modified"` — `"true"` when the working
    tree was dirty at build time.
- **Availability**: `ok == false` when built without module/VCS context (rare);
  triggers the sentinel.

### Version-resolution helper (the transform)

- **Public**: `func ResolveVersion(injected string) string` — package `ax`, in
  `version.go`. Reads build info once, delegates to the pure resolver. Doc-comment
  is a contract (precedence + non-empty guarantee). Doc-coverage gated with a
  verified `ExampleResolveVersion`.
- **Unexported pure seam**: `func resolveVersionFrom(injected string, info
  *debug.BuildInfo, ok bool) string` — all branching lives here so it is
  table-testable with synthetic `*debug.BuildInfo`.

### Last-resort sentinel

- **Value**: `const versionUnknown = "0.0.0-unknown"` (unexported).
- **Constraints satisfied**: non-empty, visibly non-release (SemVer pre-release
  sorting below all real versions), not the bare `dev`/`unknown` strings.

## Resolution state table

Precedence is strictly ordered; the first matching row wins.

| # | `injected` | Build info (`ok`) | `Main.Version` | `vcs.revision` | `vcs.modified` | Result |
|---|------------|-------------------|----------------|----------------|----------------|--------|
| 1 | non-empty and not `dev`/`unknown`/`(devel)` | (any) | (any) | (any) | (any) | `injected` (e.g. `v1.2.3`, `5bf9b77-dirty`) |
| 2 | `""`       | true              | real (≠`(devel)`/`""`) | (any)  | (any)          | `Main.Version` (e.g. `v1.2.3`) |
| 3 | `""`       | true              | `(devel)`/`""` | present        | `"false"`/absent | `<revision>` |
| 4 | `""`       | true              | `(devel)`/`""` | present        | `"true"`       | `<revision>-dirty` |
| 5 | `""`       | true              | `(devel)`/`""` | absent         | (any)          | `0.0.0-unknown` |
| 6 | `""`       | false             | —              | —              | —              | `0.0.0-unknown` |

Mapping to build modes:

- Row 1 ← documented `make build-example` (or any `-ldflags -X` build).
- Row 2 ← `go install github.com/.../tool@v1.2.3`.
- Rows 3–4 ← `go run` / bare `go build` from a Git working tree (clean / dirty).
- Rows 5–6 ← built with no VCS context (e.g. extracted tarball) and no injection.

## No new envelope / schema shape

`Schema`, `Error`, and `Labels` are unchanged structs. This feature changes only
the *value* placed in their existing `version`/`Version` fields. Therefore the
root-package `__schema` and error-envelope golden files (which pass explicit fixed
versions in tests) require no regeneration.
