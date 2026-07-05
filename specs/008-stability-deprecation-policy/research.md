# Phase 0 Research: Stability + Deprecation Policy

**Feature**: `008-stability-deprecation-policy` | **Date**: 2026-06-16

This document resolves every open decision behind the two new constitution principles, records
the retroactive verdict the issue asks for, and confirms the existing tooling state. There are
**no governing ADRs to absorb** (see Decision Records note below), so there is no "Decision
Records Absorbed" section and no ADR-retirement task.

## Decision Records note (no ADRs absorbed)

GitHub issue #17 requested creating `docs/adr/0013-stability-semver-policy.md` and
`docs/adr/0014-deprecation-policy.md`. The constitution (v1.1.0, §Governance) freezes ADRs:
*"They MUST NOT be created or edited going forward."* Those two ADRs therefore do not exist
and will not be created (verified absent). Their intended decisions are instead recorded here
(Phase 0) and elevated to constitution principles by amendment. **No existing ADR governs this
feature**, so the ADR-absorption gate is not triggered. The structured-logger decision is the
*subject* of the retroactive evaluation in Decision 5 below — it is read, not absorbed or
deleted by this feature.

Structured-logger retirement was explicitly OUT OF SCOPE for this feature. This feature only
*applies the new policy to* the historical change it records, and does not absorb or delete
the logger decision. The later absorption and retirement are recorded in
`../011-hot-path-benchmarks/research.md` per the §Governance feature workflow. A reviewer of
this feature should NOT expect a `docs/adr/` deletion in this PR (consistent with SC-004:
zero new ADR files, and — here — zero ADR deletions).

---

## Decision 1 — Pre-v1.0 SemVer contract

**Decision**: **Pragmatic pre-v1.0.** While the module is in `0.x`:

- A `0.MINOR.0` bump (minor digit increments) MAY contain breaking changes.
- A `0.x.PATCH` release MUST stay backward-compatible — bug fixes only, no breaking changes,
  no new exported surface that would force a minor under stable SemVer.
- Breaking changes never auto-promote to `1.0.0` while in `0.x`; promotion to v1 is a
  deliberate, separate decision.

**Rationale**: ax-go is young and its surface is still settling, so a strict
"every break is a major bump" rule would either freeze the API prematurely or churn the major
digit. Pragmatic pre-v1.0 matches the widely understood Go community reading of `0.x` while
still giving consumers a hard guarantee they can rely on: **patch releases are always safe to
take.** That single guarantee is what makes the policy actionable.

**Alternatives considered**:

- *Strict SemVer even in `0.x`* (treat every break as needing a major bump) — rejected:
  pins the API too early and conflicts with `bump-minor-pre-major: true` (which intentionally
  keeps breaks on the minor digit pre-v1).
- *Anything-goes `0.x`* (no guarantee at any level) — rejected: leaves consumers with no safe
  upgrade path at all, defeating the purpose of publishing a policy.

## Decision 2 — Definition of "breaking change"

**Decision**: A change is breaking if it breaks a consumer who depends only on the **public**
surface. Two surfaces are in scope:

1. **Go API surface** (exported identifiers of package `ax`): removing, renaming, or
   re-typing an exported symbol; changing a function signature; changing the meaning/semantics
   of an existing exported symbol; tightening accepted input or loosening guaranteed output in
   an incompatible way. Adding a new exported symbol is NON-breaking.
2. **Machine-payload output shapes** (`ax.Error` envelope, `__schema` output): **additive-
   tolerant** — *adding* a new field is NON-breaking (consumers MUST tolerate unknown fields);
   *removing, renaming, re-typing, or changing the semantics* of an existing field IS
   breaking.

**Scope by package kind**:

- `internal/` packages are **exempt** — the Go toolchain already blocks external import, so
  there is no external consumer to break.
- The root package `ax` is **governed** by the policy.
- Future experimental packages (e.g., a hypothetical `ax/x/...`) are treated as pre-v1.0
  regardless of the root version and documented separately if/when introduced.

**Rationale**: ax-go's contract is explicitly broader than its Go types — `__schema` and the
`ax.Error` envelope are declared public API (Constitution Principle III) and are guarded by
golden-file tests. A policy that only covered Go types would let a "non-code" envelope change
silently break every agent diffing output. The additive-tolerant rule is the standard
forward-compatibility contract for machine payloads and is exactly why the constitution already
tells consumers to tolerate unknown fields.

**Alternatives considered**:

- *Go-types-only definition* — rejected: ignores the machine-payload contract that agents
  actually depend on.
- *Strict payload (any field addition is breaking)* — rejected: would make every additive
  envelope improvement a breaking change, punishing the exact evolution the additive-tolerant
  contract is designed to allow.

## Decision 3 — Deprecation notice window

**Decision**: **One minor release.** A symbol MUST carry a `//Deprecated:` comment in **at
least one published `0.MINOR.0` release** before it may be removed in a later release. The
`//Deprecated:` comment MUST follow the Go convention (`//Deprecated:` paragraph in the doc
comment) and MUST include a migration note: what to use instead, or — when no replacement
exists — the reason for removal.

**Rationale**: One published minor release gives every consumer at least one upgrade cycle in
which `staticcheck SA1019` flags their call sites before the symbol disappears, while keeping
the pre-v1.0 surface from accumulating zombie identifiers indefinitely. It is the lightest
window that still guarantees a machine-visible warning before breakage.

**Alternatives considered**:

- *Two minor releases* — rejected: heavier than necessary for a pre-v1.0 library with no
  current deprecations; can be revisited at v1.
- *One major release* — rejected: incoherent pre-v1.0, where breaks ride the minor digit and
  the major stays `0`.
- *Deprecate-and-remove in the same release* — rejected: defeats the purpose; gives consumers
  no warning window.

## Decision 4 — release-please enforcement of the pre-v1.0 contract

**Decision**: `release-please-config.json` MUST operationalize Decision 1:

- `bump-minor-pre-major: true` (unchanged) — breaking changes bump the **minor** digit and
  never auto-promote to `1.0.0`.
- `bump-patch-for-minor-pre-major: false` (changed from `true`) — `feat:` commits bump the
  **minor** digit, NOT the patch digit, so patch releases stay bug-fixes-only and Decision 1's
  patch guarantee holds.

Net effect: `fix:` → patch; `feat:` → minor; `feat!:` / `BREAKING CHANGE:` → minor (never
auto-1.0.0). This change is **already staged** in the working tree (verified via
`git diff release-please-config.json`).

**Rationale**: A policy that lives only in prose drifts from what the tooling actually does.
With `bump-patch-for-minor-pre-major: true` (the prior value), a `feat:` commit would have
been folded into a *patch* release — directly contradicting the "patch = bug-fixes-only"
guarantee. Flipping it to `false` makes the release automation enforce the documented contract
instead of relying on reviewer discipline.

**Alternatives considered**:

- *Leave `bump-patch-for-minor-pre-major: true` and rely on review* — rejected: tooling and
  policy would disagree (SC-007 would fail); the guarantee would be unenforceable.

## Decision 5 — Retroactive verdict: `*zerolog.Logger → ax.Logger` (structured-logger revision)

**Question (from issue #17 / User Story 3)**: Under the policy in Decisions 1–2, was the change
from a concrete `*zerolog.Logger` return to the `ax.Logger` **interface** return (the
structured-logger revision) a breaking change requiring a major bump?

**Verdict**: **Breaking under the Go-API-surface rule (Decision 2.1) — but NOT requiring a
major bump, because the change occurred pre-v1.0 where breaks ride the minor digit
(Decision 1).**

**Reasoning**:

- Changing an exported constructor's return type from a concrete `*zerolog.Logger` to an
  interface `ax.Logger` is a **re-typing of an exported symbol's signature**. Any consumer
  whose code named the concrete type — `var l *zerolog.Logger = ax.NewLogger(ctx)`, or who
  called a `*zerolog.Logger` method not present on `ax.Logger` — fails to compile against the
  new signature. By Decision 2.1 (re-typing an exported symbol IS breaking) this is a breaking
  change to the Go API surface.
- It is **not** rescued by being "just an interface": narrowing a concrete return to an
  interface removes capability from the caller's static view, which is the classic
  source-incompatible direction.
- However, the change landed while the module is in `0.x`. Under Decision 1 (pragmatic
  pre-v1.0), a breaking change is permitted in a `0.MINOR.0` bump and **must not** force or
  auto-promote a `1.0.0` major. So the correct version response is a **minor** bump, not a
  major one.

**Consequence for version history**: No retrospective **major** bump is owed. The change is
correctly absorbed by a pre-v1.0 minor bump. Going forward, once the `ax.Logger` interface is
itself part of a published surface, *further* changes to it (e.g., adding a method, which
breaks external implementers) are governed by the same rules — and the constitution's existing
structured-logger guardrail (no second logger backend, interface is a migration seam only)
limits how often that surface churns.

## Decision 6 — Confirmation: `staticcheck SA1019` already enabled

**Finding (confirms FR-006 / SC-002)**: `staticcheck SA1019` (the deprecation check that
reports use of `//Deprecated:` symbols) is **already enabled** and requires **no config
change**.

**Evidence** (`.golangci.yml`, `linters.settings.staticcheck.checks`):

```yaml
staticcheck:
  checks:
    - all
    - -ST1000   # package comment
    - -ST1016   # receiver-name consistency
    - -QF1008   # embedded-field selector
```

The list is `["all", "-ST1000", "-ST1016", "-QF1008"]`. It enables `all` and excludes only
three style/quickfix checks — **SA1019 is not excluded**, so it is active. Therefore any
deprecated symbol added in the future is automatically surfaced at every call site by
`golangci-lint run`, satisfying the Deprecation Lifecycle principle's tooling requirement with
zero additional configuration. (`staticcheck` is also listed under `linters` as enabled.)

**No new deprecations exist** in ax-go today (verified: no `//Deprecated:` comments in the
public surface), so the principle introduces the mechanism for future use; the SA1019 behavior
is confirmed by inspection rather than by adding a throwaway deprecated symbol.

## Decision 7 — Constitution version bump

**Decision**: **1.1.0 → 1.2.0 (MINOR)** with a Sync Impact Report prepended to the
constitution.

**Rationale**: Per the constitution's own versioning policy, MINOR = "a new principle/section
or materially expanded guidance." This amendment adds **two new principles** (Stability &
SemVer; Deprecation Lifecycle) and removes/redefines nothing, so MAJOR is not warranted and
PATCH is insufficient (FR-007, SC-005). The Sync Impact Report must name both added principles,
the bump rationale, the derived docs reconciled in the same change (`AGENTS.md`, README), and
confirm the plan/spec/tasks templates need no change.

---

## Summary of resolved unknowns

| # | Topic | Resolution |
|---|-------|-----------|
| 1 | Pre-v1.0 SemVer contract | Pragmatic pre-v1.0: `0.MINOR.0` may break; `0.x.PATCH` bug-fix-only |
| 2 | "Breaking" definition | Go API + machine-payload (additive-tolerant); `internal/` exempt |
| 3 | Deprecation notice window | ≥1 published `0.MINOR.0` release with `//Deprecated:` before removal |
| 4 | release-please flags | `bump-minor-pre-major: true`, `bump-patch-for-minor-pre-major: false` (staged) |
| 5 | `ax.Logger` retroactive verdict | Breaking (re-type), but only a minor bump owed pre-v1.0 — no major |
| 6 | SA1019 status | Already enabled (`checks: [all]`, SA1019 not excluded) — no config change |
| 7 | Constitution version bump | 1.1.0 → 1.2.0 (MINOR) + Sync Impact Report |

All NEEDS CLARIFICATION items are resolved. Ready for Phase 1.
