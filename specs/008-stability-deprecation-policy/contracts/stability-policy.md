# Contract: Stability & Deprecation Policy

**Feature**: `008-stability-deprecation-policy` | **Date**: 2026-06-16

This is the external interface this feature delivers. ax-go's "interface to other systems" here
is not a Go API or an HTTP endpoint — it is the **policy contract** that consumers and
contributors rely on: a deterministic answer to "is this change breaking, and what version bump
does it require?" plus the deprecation lifecycle. The tables below are the normative,
testable form of the two constitution principles; the prose in the constitution MUST match
them.

## Contract A — Breaking-change classification

Given a change, classify it. The verdict is a pure function of (change kind, surface).

**Terminology**: the **surface** for machine-payload classification is the `ax.Error`
envelope and `__schema` output; a **field** is one element of that surface. The
classification below operates per field; the additive-tolerant rule is a property of the
surface as a whole.

| Surface | `add` | `remove` | `rename` | `re-type` | `semantic change` |
|---------|-------|----------|----------|-----------|-------------------|
| **Go exported symbol** (`ax.*`) | non-breaking | **breaking** | **breaking** | **breaking** | **breaking** |
| **Machine-payload field** (`ax.Error`, `__schema`) | non-breaking¹ | **breaking** | **breaking** | **breaking** | **breaking** |
| **`internal/` symbol** | non-breaking | non-breaking | non-breaking | non-breaking | non-breaking |

¹ Additive-tolerant: consumers MUST tolerate unknown fields, so adding a field is non-breaking
by contract.

### Implied version bump (pre-v1.0, `0.x`)

| Verdict | Bump | Notes |
|---------|------|-------|
| `non-breaking` feature/addition (`feat:`) | **minor** (`0.x.0`) | `bump-patch-for-minor-pre-major: false` |
| `non-breaking` bug fix (`fix:`) | **patch** (`0.0.x`) | patch releases are bug-fixes-only |
| `breaking` (any of the **breaking** cells above) | **minor** (`0.x.0`) | `bump-minor-pre-major: true`; NEVER auto-`1.0.0` |

**Invariants**:

- INV-1: Any `breaking` change pre-v1.0 maps to a **minor** bump — never major, never patch.
- INV-2: A `0.x.PATCH` release contains bug fixes only; it MUST NOT include any cell marked
  **breaking** and MUST NOT include a `feat:`.
- INV-3: `internal/` is always non-breaking for external consumers (Go toolchain blocks import).
- INV-4: Promotion to `1.0.0` is a deliberate, separate decision — breaking changes do not
  trigger it automatically while in `0.x`.

### Worked example (the issue's test case)

`ax.NewLogger` return type `*zerolog.Logger` → `ax.Logger` (interface): surface =
*Go exported symbol*, change kind = *re-type* → **breaking**. Pre-v1.0 ⇒ **minor** bump
(INV-1). No major bump owed. (Full reasoning: `research.md` Decision 5.)

## Contract B — Deprecation lifecycle

A symbol moves through these states; each transition has a precondition.

```text
live ──(add //Deprecated: comment, publish in a 0.MINOR.0)──▶ deprecated
deprecated ──(≥1 published 0.MINOR.0 elapsed with the comment)──▶ removable
removable ──(remove symbol; rides a minor bump, INV-1)──▶ removed
```

| Transition | Precondition | Tooling check |
|------------|--------------|---------------|
| `live → deprecated` | Doc comment carries a `//Deprecated:` paragraph **with a migration note** (replacement, or removal reason if none) | `staticcheck SA1019` begins reporting at call sites |
| `deprecated → removable` | The `//Deprecated:` comment has shipped in **at least one published `0.MINOR.0` release** | release history |
| `removable → removed` | Removal is a **breaking** change → **minor** bump pre-v1.0 | release-please (`feat!:`/`BREAKING CHANGE:` → minor) |

### Deprecation comment format (Go convention)

```go
// OldFunction does X.
//
// Deprecated: Use NewFunction instead. OldFunction will be removed in a future
// release; it cannot represent the new Y parameter.
func OldFunction() { /* ... */ }
```

**Rules**:

- DEP-1: The `//Deprecated:` paragraph MUST be its own paragraph in the doc comment (Go
  convention) so `staticcheck SA1019` and `go doc` recognize it.
- DEP-2: The note MUST state the replacement, or — when none exists — the reason for removal.
- DEP-3: Removal before the window (INV: ≥1 published minor) MUST be rejected in review with a
  pointer to the Deprecation Lifecycle principle.

## Contract C — Tooling enforcement (confirmation, not change)

| Guarantee | Mechanism | Status |
|-----------|-----------|--------|
| Deprecated symbols are flagged at every call site | `staticcheck SA1019` via `golangci-lint run` | **already enabled** — `.golangci.yml` `checks: [all]` excludes only `-ST1000`, `-ST1016`, `-QF1008`; SA1019 is active. No config change. |
| `feat:` → minor, `fix:` → patch, breaking → minor (no auto-`1.0.0`) | `release-please-config.json` flags | `bump-minor-pre-major: true`, `bump-patch-for-minor-pre-major: false` (staged) |

## Verification (how this contract is checked)

This is a documentary contract; "tests" are inspections, not Go tests:

- SC-002: a deprecated symbol is reported by `golangci-lint run` (SA1019) — confirmed by config
  inspection (research.md Decision 6); no throwaway code added.
- SC-004: zero new files under `docs/adr/`.
- SC-007: `release-please-config.json` flags match Contract A's bump table.
- SC-001/SC-005/SC-006: the constitution prose and README match Contracts A & B; Sync Impact
  Report present; derived docs reconciled in the same PR.
