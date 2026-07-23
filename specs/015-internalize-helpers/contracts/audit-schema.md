# Contract: Permanent Public-Surface Audit

**File**: `specs/015-internalize-helpers/public-surface-audit.json`  
**Schema version**: `1`

The audit is the permanent decision history required by FR-001–FR-005. It is
not the operational live baseline and no record is ever deleted.

## Top-Level Shape

```json
{"schema_version":1,"audited_at":"2026-07-19","records":[{"id":"field:Labels.Environment","kind":"field","owner":"Labels","name":"Environment","signature":"string","classification":"supported","rationale":"Low-cardinality logger label is part of the documented Labels contract.","disposition":"keep-public","internal_target":"","replacement":"","compatibility_strategy":"Keep the public selector unchanged.","lifecycle":"live","first_published":"","deprecated_in":"","removed_in":"","downstream_checked_at":"","downstream_evidence":[]}]}
```

## Fields

| Field | Type | Rules |
|-------|------|-------|
| `schema_version` | integer | Required; exactly `1`. |
| `audited_at` | string | Required RFC 3339 full-date (`YYYY-MM-DD`). |
| `records` | array | Required; sorted bytewise by `id`; no duplicates. |
| `records[].id` | string | Required canonical API Feature ID. |
| `records[].kind` | enum | `const`, `var`, `func`, `type`, `field`, `interface-method`, or `method`. |
| `records[].owner` | string | Empty for package scope; public root selector otherwise. |
| `records[].name` | string | Required exported name. |
| `records[].signature` | string | Required canonical Go type/signature. |
| `records[].classification` | enum | `supported` or `implementation-leak`. |
| `records[].rationale` | string | Required, non-empty, one line. |
| `records[].disposition` | enum | `keep-public`, `relocate-with-forwarder`, or `deprecate-in-place`. |
| `records[].internal_target` | string | Required for `relocate-with-forwarder`; empty or `internal/<role>`. |
| `records[].replacement` | string | Required for leaks; supported replacement or explicit no-replacement reason. |
| `records[].compatibility_strategy` | string | Required, non-empty for leaks. |
| `records[].lifecycle` | enum | `live`, `deprecated`, `removable`, or `removed`. |
| `records[].first_published` | string | Empty or SemVer tag such as `v0.3.0`. |
| `records[].deprecated_in` | string | Empty until a real published minor is verified. |
| `records[].removed_in` | string | Empty unless lifecycle is `removed`. |
| `records[].downstream_checked_at` | string | Empty for supported rows; full-date for leak decisions. |
| `records[].downstream_evidence` | array of string | Sorted, duplicate-free evidence lines. |

Unknown fields and trailing JSON values are rejected.

## Cross-Field Validation

- `id` equals the canonical form derived from `kind`, `owner`, and `name`:
  `<kind>:<name>` at package scope and `<kind>:<owner>.<name>` for members.
  `method` additionally admits the pointer-receiver spelling
  `method:*<owner>.<name>`, which `owner` alone cannot reconstruct.
- `owner` is non-empty for exactly `field`, `interface-method`, and `method`,
  and empty for `const`, `var`, `func`, and `type`.
- `supported` pairs only with `keep-public` and lifecycle `live`.
- `implementation-leak` pairs only with `relocate-with-forwarder` or
  `deprecate-in-place`.
- Feature 015 permits leak lifecycle `deprecated`; `removable` and `removed`
  are follow-up states.
- `relocate-with-forwarder` requires an `internal/<role>` target whose
  slash-separated segments are each a well-formed lowercase package name;
  empty segments, trailing slashes, and dangling `_`/`-` separators are
  rejected.
- `deprecated`, `removable`, and `removed` source declarations carry a valid
  `Deprecated:` paragraph while they remain exported.
- `removable` requires a verified non-empty `deprecated_in`.
- `removed` requires both `deprecated_in` and `removed_in`; the row remains in
  the audit but is absent from the live baseline and source.
- New identities are appended as new retained records. An old ID is never
  repurposed for a different feature.

## Completeness

At initial certification, audit IDs equal the compiler-visible feature IDs for
all six supported profiles. Later:

```text
live baseline IDs
  = audit IDs whose lifecycle is live | deprecated | removable
removed audit IDs
  ∩ live baseline IDs
  = empty
```

The gate validates these relationships on every run.

## Encoding and Bounds

- Strict UTF-8 JSON, minified, with one trailing newline.
- Maximum encoded size: 1 MiB.
- Records and evidence arrays use canonical bytewise ordering.
- No absolute paths, cache paths, timestamps finer than the audit/search date,
  or environment-dependent values.

## Change Protocol

- Initial audit: generated mechanically, classified manually, then reviewed
  before any internalization.
- Intentional addition: add the live baseline entry and a new audit record in
  the same PR.
- Feature-015 deprecation: retain the row, change lifecycle to `deprecated`,
  record the strategy/target/replacement, and keep the feature live.
- Publication: a later reviewed change may populate `deprecated_in` after the
  tag exists.
- Removal: only a follow-up Spec Kit feature may set `removed`, populate
  `removed_in`, and delete the live baseline entry. The audit row is retained.
