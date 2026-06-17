# Phase 1 Data Model: Stability + Deprecation Policy

**Feature**: `008-stability-deprecation-policy` | **Date**: 2026-06-16

This feature ships no runtime data structures. The "entities" here are governance artifacts —
the documents and config the feature creates or edits, plus the conceptual model of how a
change is classified. Each entity lists its fields, the validation rules drawn from the
requirements, and its lifecycle/state transitions where applicable.

## Entity: Stability & SemVer Principle

- **Location**: new section in `.specify/memory/constitution.md` (§Core Principles).
- **Fields / required content**:
  - Pre-v1.0 contract statement (pragmatic pre-v1.0: `0.MINOR.0` MAY break; `0.x.PATCH`
    bug-fix-only; never auto-promote to `1.0.0`).
  - Breaking-change definition over **two surfaces**: Go API surface and machine-payload
    output shapes (additive-tolerant).
  - Stability tier per package kind: `internal/` exempt, root `ax` governed, experimental
    packages noted separately.
  - Pointer relationship to release tooling (Decision 4).
- **Validation rules** (from FR-001, FR-007):
  - MUST define breaking for BOTH Go API and machine-payload.
  - Machine-payload rule MUST be additive-tolerant (add = non-breaking; remove/rename/re-type/
    semantic-change = breaking).
  - MUST classify package kinds.
  - Adding this principle is a MINOR constitution bump.
- **State**: absent → present (added by the amendment).

## Entity: Deprecation Lifecycle Principle

- **Location**: new section in `.specify/memory/constitution.md` (§Core Principles).
- **Fields / required content**:
  - `//Deprecated:` comment format (Go convention).
  - Minimum notice window: ≥1 published `0.MINOR.0` release carrying the comment before
    removal.
  - Required migration-note content (replacement, or removal reason when none).
  - Tooling expectation: `staticcheck SA1019` (already enabled — confirm + document).
- **Validation rules** (from FR-002):
  - MUST state the comment format, the window, the migration-note requirement, and the tooling.
- **State**: absent → present (added by the amendment).
- **Lifecycle of a deprecated symbol** (the state machine this principle governs):
  `live` → `deprecated` (`//Deprecated:` added, published in a `0.MINOR.0`) → `removable`
  (after ≥1 published minor with the comment) → `removed` (breaking change, rides a minor bump
  pre-v1.0).

## Entity: Sync Impact Report

- **Location**: HTML-comment block at the top of `.specify/memory/constitution.md`.
- **Fields / required content** (from SC-005, §Governance amendment procedure):
  - Version change line: `1.1.0 → 1.2.0 (MINOR)`.
  - Bump rationale: two new principles added; nothing removed/redefined.
  - Added principles named: Stability & SemVer; Deprecation Lifecycle.
  - Templates reviewed: plan/spec/tasks — no change required (asserted).
  - Derived docs reconciled: `AGENTS.md`, README — in the same change.
- **Validation rules**: MUST be present whenever the constitution is versioned; MUST name both
  new principles and the correct bump.
- **State**: prepended/updated as part of the amendment.

## Entity: README Status Section

- **Location**: `README.md`, the `> **Status: …**` blockquote (currently lines 9–15).
- **Fields / required content** (from FR-003, SC-001):
  - Current stability tier (pre-v1.0).
  - SemVer contract for that tier (patch = bug-fix-only safe upgrade; minor may break).
  - Pointer/reference to the constitution's Stability principle for the full policy.
- **Validation rules**: a second reader can answer "what is the stability guarantee?" from the
  README + constitution alone (SC-001).
- **State**: updated in the same PR as the amendment (SC-006).

## Entity: AGENTS.md Reference

- **Location**: `AGENTS.md`, "Accepted Architecture" section.
- **Fields / required content** (from FR-004):
  - Reference to BOTH new constitution principles (replacing the issue's original request to
    reference ADR-0013 / ADR-0014).
- **Validation rules**: references the constitution principles, NOT any ADR (SC-004 — zero new
  ADR files).
- **State**: updated in the same PR (SC-006).

## Entity: release-please-config.json Flags

- **Location**: `release-please-config.json`, `packages["."]`.
- **Fields**:
  - `bump-minor-pre-major: true` (unchanged).
  - `bump-patch-for-minor-pre-major: false` (changed from `true` — already staged).
- **Validation rules** (from FR-008, SC-007): `feat:` → minor, `fix:` → patch, breaking →
  minor without auto-1.0.0. Directly inspectable.
- **State**: already edited in the working tree; the feature's job is to keep it consistent
  with the documented contract and verify it.

## Classification model (conceptual)

The decision a contributor actually runs, expressed as inputs → output. Encoded as a decision
table in `contracts/stability-policy.md`.

- **Inputs**: change kind (`add` | `remove` | `rename` | `re-type` | `semantic-change`),
  surface (`go-exported` | `machine-payload-field` | `internal`).
- **Output**: `breaking` | `non-breaking`, and the implied version bump pre-v1.0
  (`minor` | `patch`).
- **Invariant**: any `breaking=true` pre-v1.0 maps to a **minor** bump (never major, never
  patch); any `internal` surface is always `non-breaking` for external consumers.
