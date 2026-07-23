# Phase 1 Data Model: Certify and Internalize the Public Boundary Before v1.0

**Feature**: `015-internalize-helpers` | **Date**: 2026-07-19

## Entity: Target Profile

One supported compiler selection used to verify that the root API is
platform-invariant.

| Field | Type | Rules |
|-------|------|-------|
| `goos` | enum | `linux`, `darwin`, or `windows` |
| `goarch` | enum | `amd64` or `arm64` |

The required Cartesian product contains six profiles. The host process remains
host-built; it passes profile values only to child `go list` commands.

## Entity: API Feature

One compiler-visible package declaration or selector exposed by root package
`ax`.

| Field | Type | Meaning |
|-------|------|---------|
| `id` | string | Canonical public-selector identity, such as `func:Execute`, `field:Labels.Environment`, or `method:*Telemetry.Shutdown`. |
| `kind` | enum | `const`, `var`, `func`, `type`, `field`, `interface-method`, or `method`. |
| `owner` | string | Empty for package-scope objects; otherwise the root public type/alias selector. |
| `name` | string | Exported declaration or selector name. |
| `signature` | string | Canonical Go type/signature used to detect re-types and member changes. |
| `access` | enum | Review metadata: `direct`, `promoted`, or `alias`; not part of identity. |
| `profiles` | target set | Must contain all six supported profiles after invariance validation. |

### Identity and reachability invariants

- `id` is unique and entries sort bytewise by it.
- Package declarations are roots.
- Fields and methods are attributed to the root selector consumers use.
- Pointer-only methods use a `*` owner marker; value-set methods do not.
- Promoted selectors follow `go/types` ambiguity and shallowest-depth rules.
- Hidden concrete types are traversed only when an exported declaration exposes
  that concrete type. Interface results expose their interface methods only.
- Absolute paths, compiler-cache paths, source order, and implementation origin
  never enter identity or signature.

## Entity: Audit Record

The permanent decision for one API Feature at the feature-015 audit point.
Records live in `public-surface-audit.json` and are never deleted.

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `id` | string | always | Matches one canonical API Feature identity. |
| `kind`, `owner`, `name`, `signature` | API metadata | always | Pins the audited starting feature. |
| `classification` | enum | always | `supported` or `implementation-leak`. |
| `rationale` | string | always | One line explaining the decision; mandatory and non-empty for every row. |
| `disposition` | enum | always | `keep-public`, `relocate-with-forwarder`, or `deprecate-in-place`. |
| `internal_target` | string | leaks | Cohesive `internal/<role>` target; empty only for supported rows or compatibility-blocked in-place deprecation. |
| `replacement` | string | leaks | Supported replacement selector, or an explicit no-replacement removal reason. |
| `compatibility_strategy` | string | leaks | How name, type, identity, and semantics remain unchanged during feature 015. |
| `lifecycle` | enum | always | `live`, `deprecated`, `removable`, or `removed`. |
| `first_published` | string | optional | Earliest verified tag exposing the feature. |
| `deprecated_in` | string | after publication | Published `0.MINOR.0` tag carrying the notice. |
| `removed_in` | string | removed only | Published minor carrying the removal. |
| `downstream_checked_at` | RFC 3339 date | leaks | Date of the evidence search. |
| `downstream_evidence` | array of string | leaks | Repository and indexed downstream evidence; sorted and retained. |

### Classification/disposition rules

| Classification | Allowed disposition | Feature-015 source state |
|----------------|---------------------|--------------------------|
| `supported` | `keep-public` | Present, unchanged, lifecycle `live`. |
| `implementation-leak` | `relocate-with-forwarder` | Present as deprecated root forwarder; mechanics under `internal/`. |
| `implementation-leak` | `deprecate-in-place` | Present and deprecated; relocation waits for later removal because forwarding could not preserve compatibility. |

Ambiguity resolves to `supported`. `unexport-in-place` is not a disposition.

### Lifecycle state machine

```text
live
  └─ feature 015 adds a valid notice ─> deprecated
       └─ a real 0.MINOR.0 publishes it ─> removable
            └─ follow-up breaking feature removes it ─> removed
```

- Feature 015 may perform only `live → deprecated`.
- `deprecated → removable` requires a non-empty, verified `deprecated_in`.
- `removable → removed` occurs only in a follow-up Spec Kit feature.
- A removed row remains in the audit and gains `removed_in`.

## Entity: Audit Document

The permanent decision artifact at
`specs/015-internalize-helpers/public-surface-audit.json`.

| Attribute | Rule |
|-----------|------|
| Format | Strict minified-or-indented JSON object with `schema_version` and sorted `records`; committed formatting is two-space indented for review. |
| Completeness | At creation, exactly one record for every canonical API Feature across all profiles. |
| Ordering | `records` sorted bytewise by `id`; evidence arrays sorted bytewise. |
| Read bound | Maximum 1 MiB; oversize is validation exit `2`. |
| History | Rows are never deleted or identity-reused. |
| Cross-validation | Active rows (`live`, `deprecated`, `removable`) map to the live baseline; `removed` rows must not. |

## Entity: Live Baseline Entry

One current approved compiler-visible API feature in
`internal/cmd/surfacecheck/baseline.json`.

| Field | Type | Rules |
|-------|------|-------|
| `id` | string | Unique canonical ID. |
| `signature` | string | Exact canonical signature. |

The baseline intentionally omits classification and historical fields. Its
entries are sorted by ID and always mirror current source.

## Entity: Live Baseline Document

| Attribute | Rule |
|-----------|------|
| Format | Strict JSON object with `schema_version` and sorted `features`. |
| Completeness | Exact current surface for all six invariant profiles. |
| Read bound | Maximum 1 MiB. |
| Updates | Changed in the same reviewed PR as an intentional live API change. |

## Entity: Drift Item

One deterministic validation difference.

| Field | Meaning |
|-------|---------|
| `id` | Canonical feature ID. |
| `drift` | `added`, `missing`, `signature-changed`, `profile-divergent`, `audit-missing`, `audit-state-invalid`, or `deprecation-missing`. |
| `expected` | Expected signature/state when applicable. |
| `actual` | Actual signature/state when applicable. |

Items sort by `(id, drift, expected, actual)`. The gate reports every detected
item in a deterministic error envelope.

## Entity: Gate Success

Struct emitted as minified strict JSON on stdout.

| Field | Type | Meaning |
|-------|------|---------|
| `status` | string | Always `pass`. |
| `features_checked` | integer | Current live feature count. |
| `audit_records_checked` | integer | Retained audit row count. |
| `profiles_checked` | integer | Always `6` for the default contract. |

## Entity: Gate Failure

Exactly one `ax.Error` envelope on stderr. Stable error codes distinguish
`surface_drift`, `invalid_surface_artifact`, `surface_permission`, and
`surface_internal`. Sorted drift descriptions are carried in deterministic
suggestions/context.

| Outcome | Exit |
|---------|------|
| Success | `0` |
| Drift or invalid repository input | `2` |
| Unexpected internal failure | `1` |
| Permission failure | `4` |

## Relationships

```text
Target Profile 6 ─── * API Feature observation
API Feature    1 ─── 1 Live Baseline Entry
API Feature    1 ─── 1 active Audit Record
Audit Document 1 ─── * Audit Record (including removed history)
Gate Failure   1 ─── * Drift Item
```
