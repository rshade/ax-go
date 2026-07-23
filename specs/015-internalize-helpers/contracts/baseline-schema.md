# Contract: Live Public-Surface Baseline

**File**: `internal/cmd/surfacecheck/baseline.json`  
**Schema version**: `1`

The baseline is the exact current compiler-visible root `ax` surface. Historical
classification and removal evidence live in `public-surface-audit.json`.

## Shape

```json
{"schema_version":1,"features":[{"id":"field:Labels.Environment","signature":"string"},{"id":"func:Execute","signature":"func(context.Context, *cobra.Command, ...ExecuteOption) int"}]}
```

| Field | Type | Rules |
|-------|------|-------|
| `schema_version` | integer | Required; exactly `1`. |
| `features` | array | Required; sorted bytewise by `id`; no duplicates. |
| `features[].id` | string | Required canonical API Feature ID. |
| `features[].signature` | string | Required canonical Go type/signature. |

Unknown fields, missing fields, empty IDs/signatures, duplicate IDs, unsorted
records, and trailing JSON values are invalid repository input.

## Comparison

For the type-aware inventory common to all six target profiles:

- source ID absent from baseline → `added`;
- baseline ID absent from source → `missing`;
- same ID with different signature → `signature-changed`;
- target profiles yield different ID/signature sets → `profile-divergent`;
- active audit row absent for a baseline ID → `audit-missing`;
- removed audit row present in baseline → `audit-state-invalid`;
- deprecated active row whose source lacks a valid notice →
  `deprecation-missing`.

All are repository validation failures: stdout is empty, stderr receives one
`ax.Error`, and exit code is `2`.

## Determinism and Bounds

- Strict UTF-8 JSON, minified, with one trailing newline.
- Maximum encoded size: 1 MiB.
- Features sort bytewise by `id`.
- Signatures leave root types unqualified and qualify every external type by
  its full import path, so same-named packages remain distinct.
- Absolute source/cache paths and implementation origin are forbidden.

## Change Protocol

- Intentional current-surface change updates this file in the same reviewed PR.
- Intentional additions also add a permanent audit record.
- Feature 015 deprecations do not remove baseline entries.
- A follow-up removal deletes the live entry only after the retained audit row
  has verified deprecation-release evidence and transitions to `removed`.
- Default check mode never writes either artifact. Inventory mode prints a
  candidate minified baseline to stdout for manual review.
