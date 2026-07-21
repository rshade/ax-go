# Quickstart: Public-Surface Audit and Gate

**Feature**: `015-internalize-helpers` | **Date**: 2026-07-19

Run these commands from the module root.

## Run the exact boundary check

```bash
make surface-check
```

Equivalent direct invocation:

```bash
go run ./internal/cmd/surfacecheck
```

From a nested directory:

```bash
make -C "$(git rev-parse --show-toplevel)" surface-check
```

A pass writes one minified JSON object to stdout and nothing to stderr:

```json
{"status":"pass","features_checked":94,"audit_records_checked":94,"profiles_checked":6}
```

Counts are illustrative. The command exits `0`.

## Bootstrap the two artifacts

Generate the current type-aware baseline candidate:

```bash
go run ./internal/cmd/surfacecheck -list > /tmp/baseline.json
```

Generate an audit-shaped seed:

```bash
go run ./internal/cmd/surfacecheck -audit-seed > /tmp/audit.json
```

Both modes are read-only. The audit seed intentionally leaves classification,
rationale, disposition, lifecycle, migration, and evidence fields incomplete.
It must not be committed until every record is reviewed.

Commit the approved artifacts at:

```text
specs/015-internalize-helpers/public-surface-audit.json
internal/cmd/surfacecheck/baseline.json
```

The audit is permanent history. The baseline is the current live projection.

## Review the audit before moving code

For every API Feature:

1. Confirm the canonical ID/signature across all six target profiles.
2. Classify it as `supported` or `implementation-leak`.
3. Write a one-line rationale.
4. For a leak, record:
   - repository and indexed downstream evidence with the search date;
   - earliest verified published presence;
   - supported replacement or removal reason;
   - cohesive `internal/<role>` target;
   - compatibility strategy;
   - `relocate-with-forwarder` or `deprecate-in-place`.
5. Resolve ambiguity to `supported`.

No internalization starts until the complete audit is reviewed.

## Internalize without breaking the API

For each approved leak:

1. Write a failing test for the compatibility and gate behavior.
2. Move mechanics into the approved cohesive `internal/` target when a root
   forwarder can preserve name, type, identity, semantics, streams, and payloads.
3. Leave the root export in place and delegate to the internal implementation.
4. Add a Go-recognized deprecation paragraph:

   ```go
   // Deprecated: Use Replacement. Removal is eligible only after a published
   // pre-v1 minor release carries this notice.
   ```

5. Migrate repository call sites to the supported replacement so SA1019 is
   clean.
6. Change the retained audit row to `deprecated`; do not remove its live
   baseline entry.

If forwarding would re-type a defined type or change behavior, deprecate it in
place and defer relocation to the follow-up removal feature.

## Read a failure

Failures write nothing to stdout and exactly one minified `ax.Error` envelope
to stderr.

Common error codes:

- `surface_drift` (exit `2`): source/profile/baseline/audit disagreement.
- `invalid_surface_artifact` (exit `2`): malformed, oversized, duplicate,
  unsorted, missing, or schema-invalid JSON/flags.
- `surface_permission` (exit `4`): permission denial.
- `surface_internal` (exit `1`): unexpected internal failure.

For drift:

- `added`: add a reviewed baseline entry and permanent audit row, or unexport
  before merge if it was accidental and never approved.
- `missing`: restore the export in feature 015. Removal belongs to the follow-up
  feature after publication.
- `signature-changed`: restore compatibility; feature 015 permits no re-type.
- `profile-divergent`: make the surface target-invariant.
- `audit-missing`: add and review the permanent decision row.
- `deprecation-missing`: restore the required Go-recognized notice.

## Publication and later removal

Feature 015 lands with a non-breaking `feat:` commit. A merge is not the notice
window: a real `0.MINOR.0` tag must publish the deprecation.

Only a follow-up Spec Kit feature may remove a forwarder. It must verify the
published notice tag, retain and transition the audit row, delete the live
baseline entry with the source export, apply `breaking-change-approved`, and
use `feat!:` / `BREAKING CHANGE:` so release-please produces the next minor.

### Follow-up removal feature (tracking issue pending)

The removal work is tracked as a follow-up Spec Kit feature (GitHub tracking
issue to be created; link it here once filed). Feature 015's audit carries
zero `deprecated` rows, so the follow-up activates only once a deprecation
exists. Its scope when activated:

1. Verify a real published `0.MINOR.0` tag carries the deprecation notices.
2. Record the published tag in each affected audit row's `deprecated_in`.
3. Transition audit rows `deprecated → removable`.
4. Delete the root forwarders and their live baseline entries.
5. Transition the rows to `removed` (rows are retained, never deleted).
6. Apply the `breaking-change-approved` PR label and land with a `feat!:` /
   `BREAKING CHANGE:` commit so release-please rides the break on the minor
   digit.

## Verification before hand-back

```bash
gofmt -s -l .
go test -race ./...
go vet ./...
golangci-lint run
make doc-coverage
make cover-check
make surface-check
make bench-check
```

Expected:

- `gofmt -s -l .` prints nothing.
- Existing `ax.Error` and `__schema` goldens are unchanged.
- API diff reports no incompatible change for feature 015.
- Coverage floors and performance budgets are not lowered.
- No ADR or `CHANGELOG.md` file is created or edited.
