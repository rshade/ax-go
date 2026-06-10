# Contract: Version Resolution & Injection

**Feature**: `002-version-injection` | **Date**: 2026-06-08

This feature exposes one new public symbol, one new build target, and a documented
recipe. The `__schema`, `ax.Error`, and logger shapes are unchanged — only the
*value* in their existing `version` fields changes.

## Exported surface (package `ax`, module root)

```go
// ResolveVersion returns a non-empty tool version, resolving in precedence order:
// the injected non-placeholder link-time value, then the Go toolchain's embedded
// build metadata (main-module version, else VCS revision with a "-dirty" suffix
// when the tree was modified), then the sentinel "0.0.0-unknown". The result is
// NEVER empty and is NEVER the bare strings "dev" or "unknown"; callers pass it
// to ax.WithVersion and ax.WithLoggerLabels so __schema.version, the ax.Error
// envelope, and the logger "version" label all agree.
func ResolveVersion(injected string) string
```

**Guarantees**:

- **Total & non-empty**: every call returns a non-empty string; it never errors
  and never panics (fails closed to the sentinel).
- **No context**: takes no `context.Context` — it performs no I/O and is not
  cancelable (reads in-binary build data only).
- **Pure-per-binary**: for a given binary, repeated calls with the same `injected`
  return the same value (deterministic).
- **Precedence**: as defined in `data-model.md` → "Resolution state table"
  (non-placeholder injected value → main-module version → VCS
  revision[-dirty] → `0.0.0-unknown`).

**Unexported seam (not part of the contract, named for the plan/tasks)**:
`resolveVersionFrom(injected string, info *debug.BuildInfo, ok bool) string` holds
the branching so it is table-testable with synthetic build info.

## Build-target contract (Makefile)

```make
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-unknown)

.PHONY: build-example
build-example:
    go build -ldflags "-X main.version=$(VERSION)" -o bin/ax-integration ./examples/integration
```

> The recipe line is indented with a real TAB in the actual Makefile (Make
> requires it); spaces are shown here only so this contract doc lints cleanly.

**Guarantees**:

- Produces a binary whose `__schema.version` is a real, non-empty VCS-derived
  value (FR-001/FR-003).
- SUCCEEDS with **zero tags** in the repo (today's state): `--always` degrades to
  the commit SHA, `--dirty` marks a modified tree (e.g. `5bf9b77-dirty`); the
  `|| echo` guard covers a non-Git checkout.
- `VERSION` is overridable (`make build-example VERSION=v1.2.3`) for release
  builds and reproducibility.
- The existing library `build` target (`go build ./...`) is unchanged and is NOT
  an injection site (FR-010).

## Version-surface contract (consumer wiring)

The consumer resolves once and feeds the single value to both options (FR-007):

```go
var version string // set via -ldflags "-X main.version=..."

func run(...) int {
    resolved := ax.ResolveVersion(version)
    // ... construct logger with ax.WithLoggerLabels(ax.Labels{Version: resolved}) ...
    return ax.Execute(ctx, root, ax.WithVersion(resolved) /* , ... */)
}
```

| Surface | Field | Fed by | Behavior |
|---------|-------|--------|----------|
| `__schema` | `version` | `ax.WithVersion(resolved)` → `WithSchemaVersion` | reports `resolved` |
| `ax.Error` envelope | `version` | `ax.WithVersion(resolved)` → `WithErrorVersion` | reports `resolved` |
| logger | `version` label | `ax.WithLoggerLabels(ax.Labels{Version: resolved})` | low-cardinality label, reports `resolved` |

**Invariant (SC-006)**: for a given build, all three values are byte-identical
because they derive from the one `resolved` string.

## Behavioral contract (maps to FR / SC)

| ID | Contract |
|----|----------|
| FR-001 / SC-001 | A `make build-example` binary reports non-empty `__schema.version`. |
| FR-002 | Version lives in a `var` (linker-writable), never a `const`. |
| FR-003 | Build path derives version even with zero tags (no build failure). |
| FR-004 / SC-003 | Resolution precedence holds; result never empty; un-injected build still real. |
| FR-005 | Result is never the bare `dev`/`unknown`; sentinel is `0.0.0-unknown`. |
| FR-006 / SC-004 | One public `ResolveVersion` call gives adopters the full behavior; zero build-metadata parsing on their side. |
| FR-007 / SC-006 | One resolved value feeds all three surfaces identically. |
| FR-008 / SC-005 | Same clean commit → byte-identical version, modulo dirty marker. |
| FR-009 | Example demonstrates the pattern; its test asserts non-empty `__schema.version`. |
| FR-010 | Makefile `build-example` is the injection site; `build` is not. |
| FR-011 | README + example README document the recipe; helper carries a verified `ExampleResolveVersion`. |

## Out of scope (delegated / deferred)

- A full release pipeline (release job, goreleaser, signed/checksummed artifacts)
  — the Makefile target is the documented injection site; release automation is a
  separate concern.
- Auto-injecting the version into the logger from inside `ax.Execute` — rejected
  in research D6 (couples logger to Execute; the resolve-once pattern is the fix).
- A `FuzzResolveVersion` — optional hardening (research D8), not a required
  deliverable (the helper is not a byte-level parser surface).
- Changing the `__schema`/`ax.Error` shapes or retiring ADR-0003 — out of scope
  (research D9).
