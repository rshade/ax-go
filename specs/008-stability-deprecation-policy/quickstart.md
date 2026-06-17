# Quickstart: Stability + Deprecation Policy

**Feature**: `008-stability-deprecation-policy` | **Date**: 2026-06-16

Two short guides: one for a **consumer** deciding whether ax-go is safe to depend on, and one
for a **maintainer** deprecating a symbol. Both are answerable from the constitution + README
alone (that is success criterion SC-001).

## For a consumer: "Is this safe to depend on?"

1. **What tier is ax-go in?** Pre-v1.0 (`0.x`). See the README `Status` section.
2. **Can a `0.x.0` bump break my build?** Yes — a minor bump (`0.MINOR.0`) MAY contain breaking
   changes.
3. **Is a `0.0.x` (patch) upgrade always safe?** Yes — patch releases are bug-fixes-only and
   stay backward-compatible. This is the one hard guarantee pre-v1.0.
4. **Will it ever jump to `1.0.0` on me unexpectedly?** No — breaking changes ride the minor
   digit and never auto-promote to `1.0.0`.
5. **Full policy**: constitution → "Stability & SemVer" principle.

## For a maintainer: deprecate `ax.OldThing` → `ax.NewThing`

1. **Add the deprecation comment** (Go convention — its own paragraph, with a migration note):

   ```go
   // OldThing does X.
   //
   // Deprecated: Use NewThing instead. OldThing will be removed in a future
   // release; it cannot represent the new Y parameter.
   func OldThing() { /* ... */ }
   ```

2. **Verify the tooling flags it.** Run `golangci-lint run`. `staticcheck SA1019` reports the
   deprecation at every call site — no config change needed (it is already enabled).
3. **Ship it.** The deprecation must appear in **at least one published `0.MINOR.0` release**
   before removal. Commit as a `feat:` (it changes the public surface → minor bump).
4. **Wait the window.** Do not remove `OldThing` until that minor release is published.
   Removing earlier is rejected in review (Deprecation Lifecycle principle).
5. **Remove it.** Removal is a breaking change → still a **minor** bump pre-v1.0. Commit as
   `feat!:` / `BREAKING CHANGE:`. release-please bumps the minor; it will not auto-promote to
   `1.0.0`.

## Classify any change in 5 seconds

| You are… | …on this surface | Verdict | Pre-v1.0 bump |
|----------|------------------|---------|---------------|
| adding a new exported symbol or payload field | `ax.*` / `ax.Error` / `__schema` | non-breaking | minor (`feat:`) |
| fixing a bug, no surface change | anywhere public | non-breaking | patch (`fix:`) |
| removing / renaming / re-typing / changing semantics | `ax.*` exported | **breaking** | minor (`feat!:`) |
| removing / renaming / re-typing an existing payload field | `ax.Error` / `__schema` | **breaking** | minor (`feat!:`) |
| anything | `internal/` | non-breaking (no external consumer) | by the change kind |

Full decision tables: `contracts/stability-policy.md`.

## What this feature changed (for reviewers)

- `.specify/memory/constitution.md`: +2 principles (Stability & SemVer; Deprecation Lifecycle),
  +Sync Impact Report, version `1.1.0 → 1.2.0`.
- `AGENTS.md`: Accepted Architecture references both new principles.
- `README.md`: `Status` section documents the pre-v1.0 tier + links the policy.
- `release-please-config.json`: `bump-patch-for-minor-pre-major: false` (already staged).
- **No** Go source, **no** new ADR files, **no** `.golangci.yml` change (SA1019 already on).
