# Phase 0 Research: Cut v0.1.0 — Make ax-go a Consumable, Pinnable Release

**Feature**: `003-cut-v010-release` | **Date**: 2026-06-09

**Spec**: [spec.md](spec.md)

This feature is verification-heavy: the spec resolved its own open questions, so
Phase 0 confirms the repository state against the spec's claims and resolves the
one place where the spec's assumption conflicts with documented release-please
behavior (D1).

## Decisions

### D1 — First-release version requires a one-shot `Release-As: 0.1.0` footer (FR-010)

**Decision**: The commit that fixes the release-please workflow (Workstream C)
MUST carry a `Release-As: 0.1.0` git footer. The spec's Edge Case claim that the
bump settings alone yield `0.1.0` is **incorrect** and is superseded by this
decision.

**Rationale**: FR-010 mandates planning verify the proposed first version by
inspection. Verification findings:

- The commit history contains three qualifying `feat:` commits (`dd42c87`,
  `ea74c7d`, `4b2b85f`) — no corrective `feat:` commit is needed.
- However, release-please has a documented defect
  ([googleapis/release-please#2087](https://github.com/googleapis/release-please/issues/2087)):
  when the manifest version is exactly `"0.0.0"`, the `bump-minor-pre-major` and
  `bump-patch-for-minor-pre-major` options are **not respected** and the first
  release PR proposes **`1.0.0`**.
- Even absent that defect, the settings would propose `0.0.1` (with
  `bump-patch-for-minor-pre-major: true`, `feat:` bumps patch pre-1.0) — never
  `0.1.0`.
- The spec's own Workstream C description anticipates this ("the version
  override applied so the first release PR proposes `0.1.0`, not `0.0.1`") and
  Resolved Decision 1 designates `Release-As:` overrides for milestone cuts.

**Alternatives considered**:

- *No override (trust the bump settings)* — rejected: produces `1.0.0` per
  issue #2087, an accidental stability promise on an immutable tag.
- *Config-level `"release-as": "0.1.0"` in `release-please-config.json`* —
  rejected: pins **every** future release to 0.1.0 until removed, requiring a
  second cleanup PR; the commit footer is one-shot by design.
- *Seed the manifest at `"0.0.1"`* — rejected: works around #2087 but then
  `feat:` commits propose `0.0.2`, not `0.1.0`.

### D2 — Release workflow token: explicit `GITHUB_TOKEN` (FR-009)

**Decision**: Replace `token: ${{ secrets.RELEASE_PLEASE_TOKEN }}` in
`.github/workflows/release-please.yml` with `token: ${{ secrets.GITHUB_TOKEN }}`,
and delete the stale PAT-rationale comment block (it justifies a GoReleaser
trigger that does not exist and will not exist for v0.1.0).

**Rationale**: Spec Resolved Decision 3. The workflow already grants
`contents: write`, `issues: write`, and `pull-requests: write` — sufficient for
release-please to open the release PR and mint the tag. The known limitation
(GITHUB_TOKEN-created events don't trigger other workflows) is irrelevant: no
release-triggered workflow exists.

**Alternatives considered**:

- *Create/repair the `RELEASE_PLEASE_TOKEN` PAT* — rejected: operator burden
  (rotation, scoping) with zero benefit for a library whose release artifact is
  the git tag itself.
- *Omit `token:` entirely (action default)* — viable (the action defaults to
  `github.token`) but rejected in favor of the explicit reference, which
  documents the decision in-line.

### D3 — Workstream A is verified complete; remaining work is assertion, not implementation

**Decision**: Treat spec 002's deliverables as DONE. Workstream A reduces to
acceptance checks (no code changes expected).

**Rationale**: Verified in-repo on 2026-06-09:

- `grep -rn 'const version' examples/` → no matches (FR-001 / SC-002 already
  hold); `examples/integration/main.go` uses `var version string` +
  `ax.ResolveVersion(version)`.
- `Makefile` `build-example` target injects
  `-ldflags "-X main.version=$(VERSION)"` (FR-002 path exists).
- Public helper `ax.ResolveVersion` exists with table-driven tests
  (`TestResolveVersionFrom`) and a fuzz test (`FuzzResolveVersion`).
- README documents the recipe (writable `var version`, `make build-example`,
  manual `go build -ldflags` form, `VERSION=` override) at lines 188–230.

**Alternatives considered**: Re-implementing or re-specifying any spec-002
surface — rejected; the spec explicitly treats 002 as a prerequisite to verify,
not re-open.

### D4 — Success-envelope golden pins non-zero IDs via existing OTel + context seams (FR-004, FR-008)

**Decision**: The `Envelope[T]` golden test constructs its context with
`trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(...))`
using a fixed, valid, non-zero trace ID and span ID, plus
`ax.WithIdempotencyKey(ctx, <fixed UUID string>)`. `data` is a small fixed
struct (not a map — Constitution II). No harness normalization of any kind.

**Rationale**: FR-008 forbids scrubbing/normalization; the only compliant way to
get deterministic bytes is injection through seams that already exist.
Non-zero IDs are chosen over `context.Background()` zeros because they (a)
prove the injection seams actually populate the envelope (zeros are
indistinguishable from "seam silently broken"), (b) pin the field *ordering and
presence* of `span_id` and `idempotency_key`, which are `omitempty` and would
vanish from a zero-context fixture, leaving most of the `Metadata` shape
unlocked.

**Alternatives considered**:

- *`context.Background()` (zero trace/span, no key)* — rejected: `omitempty`
  drops `span_id`/`idempotency_key`, so the fixture would freeze only
  `trace_id` and leave most of the contract surface unverified.
- *Harness-side field scrubbing* — rejected outright by FR-008.
- *New public seam for ID injection* — rejected: spec Resolved Decision 4
  confirms no new seam is needed; OTel's `ContextWithSpanContext` is the
  existing, supported path (same dependency, no addition — FR-015).

### D5 — NDJSON line gets its own fixture and test, independent of the bounded fixture (FR-005, FR-006)

**Decision**: Two fixtures, two tests:

- `testdata/success_envelope.golden.json` — asserted against `WriteJSON` output
  of a `NewEnvelope` value.
- `testdata/ndjson_line.golden.json` — asserted against `WriteJSONLine` output
  built the same way (distinct fixed `data` value so the two fixtures are not
  byte-identical files).

**Rationale**: Today `WriteJSONLine` delegates to `WriteJSON`, so a single
fixture would *appear* to cover both — but the contracts are independent: if a
future change makes the streaming path diverge (framing, flushing, separators),
only an independent fixture catches it. Distinct payloads also prevent a
copy-paste fixture swap from passing silently.

**Alternatives considered**:

- *One shared fixture for both writers* — rejected: couples two public
  contracts; divergence in one writer could be masked.
- *A `.ndjson` extension for the line fixture* — considered cosmetic; kept
  `.golden.json` to match every existing fixture's naming
  (`<name>.golden.json`), since each NDJSON line is itself valid JSON.

### D6 — FR-007 fixture audit: PASSES, contingent on committing two untracked fixtures

**Decision**: Record the audit as complete; flag that
`testdata/config_patch_invalid.golden.json` and
`testdata/config_patch_hujson_invalid.golden.json` are currently **untracked**
and must be committed as part of this feature's hygiene gate.

**Audit table** (verified 2026-06-09):

| Contract | Fixture | Exercised by |
|---|---|---|
| `config_too_large` | `testdata/config_too_large.golden.json` | `config_test.go:472` |
| `config_max_bytes_invalid` | `testdata/config_max_bytes_invalid.golden.json` | `config_test.go:483` |
| `config_invalid` | `testdata/config_invalid.golden.json` | `config_test.go:494` |
| `config_option_invalid` | `testdata/config_option_invalid.golden.json` | `config_test.go:503` |
| `config_patch_invalid` | `testdata/config_patch_invalid.golden.json` (untracked) | `config_test.go:980` |
| `config_patch_hujson_invalid` | `testdata/config_patch_hujson_invalid.golden.json` (untracked) | `config_test.go:987` |
| `ax.Error` envelope shape | `testdata/error_envelope.golden.json` | `error_test.go:29` |
| `__schema` (default `--as=ax`) | `testdata/schema_ax.golden.json` | `schema_test.go:18` |
| `__schema --as=mcp` | `testdata/schema_mcp.golden.json` | `schema_test.go:47` |
| Success `Envelope[T]` | **MISSING** → `testdata/success_envelope.golden.json` | new test (this feature) |
| NDJSON line | **MISSING** → `testdata/ndjson_line.golden.json` | new test (this feature) |

**Rationale**: SC-003 demands 100% of v0.1.0-scope contracts be golden-locked;
the only gaps are the two success-path fixtures this feature exists to add.

### D7 — Golden tests follow strict test-first via the missing-fixture failure mode (Constitution VII)

**Decision**: Write each golden test before its fixture exists. The first run
fails at `assertGolden`'s `os.ReadFile` with a clear "read golden ... no such
file" error (the spec's Edge Case confirms this is the intended surfacing).
Then create the fixture with the exact expected bytes and watch the test pass.
No update-mode flag is added to `assertGolden` — the spec's Assumptions forbid
new test infrastructure.

**Rationale**: This is the same lifecycle the nine existing fixtures followed;
it satisfies "verify the test fails for the right reason" without inventing a
regeneration mechanism that could be run accidentally and silently bless drift.

**Alternatives considered**: An `-update` flag on `assertGolden` — rejected:
explicitly out of scope per spec Assumptions ("no new test infrastructure"),
and an update flag is a footgun on contracts that are frozen-by-intent.

### D8 — README status section replacement (FR-012)

**Decision**: Replace the "🚧 Implementation scaffold" status blockquote
(README.md line 9) with a release-status statement naming the current release
(v0.1.0) and linking to `CHANGELOG.md`. The change lands in the same PR as the
workflow fix, *before* the release PR is merged, so the tagged commit tree
already carries the corrected README.

**Rationale**: FR-012 binds at time-of-tagging. Since the release-please tag
points at the merge commit of the release PR, the README fix must precede it on
`main`.

### D9 — Governing ADRs: none; no ADR retirement task

**Decision**: Governing ADR(s) = **N/A**. No "final ADR-deletion task" will
appear in tasks.md.

**Rationale**: Spec Assumptions state no ADR is retired by this feature.
ADR-0003's `__schema.version` decision was already absorbed by
`specs/002-version-injection/research.md` (which records "Decision Records
Absorbed: None… ADR-0003 is referenced for context only"). The constitution's
ADR-absorption gate is therefore satisfied vacuously.

### D10 — Out-of-scope precondition: the working tree must be clean before implementation

**Decision**: Implementation (`/speckit-implement`) MUST NOT begin until the
repository's in-flight state is resolved: there are unresolved merge conflicts
(`UU example_test.go`, `UU internal/cmd/doccover/main.go`) and uncommitted
modifications on `main`. Resolving them is release hygiene, not feature work —
but FR-014 (`make ci` green) is unachievable until they are resolved, and the
two untracked patch fixtures (D6) must land.

**Rationale**: SC-006 requires `make ci` to pass on the commit that becomes the
tag; a conflicted tree cannot even build. Surfacing this in planning prevents a
broken "first task".

## Decision Records Absorbed

None. Governing ADR(s) = N/A (see D9). ADR-0003 remains frozen in `docs/adr/`
and is not deleted by this feature.

## Verification Summary (spec claims vs. repo reality)

| Spec claim | Verified result |
|---|---|
| `feat:` history qualifies for a minor-worthy first release | ✓ 3 `feat:` commits exist |
| Bump settings alone propose `0.1.0` (Edge Cases §1) | ✗ **False** — `1.0.0` per release-please #2087; `Release-As: 0.1.0` footer required (D1) |
| `Envelope[T]` has no `version` field | ✓ `json.go:18` — `data` + `meta{trace_id, span_id, idempotency_key, dry_run}` |
| Non-deterministic fields injectable via existing seams | ✓ OTel `ContextWithSpanContext` + `ax.WithIdempotencyKey` (D4) |
| `assertGolden` harness sufficient, no new infra | ✓ `golden_test.go:9` — read-and-compare only (D7) |
| Spec-002 deliverables complete | ✓ helper + Makefile target + README recipe + no `const version` (D3) |
| Workflow failure cause is the PAT secret | ✓ `release-please.yml` references `secrets.RELEASE_PLEASE_TOKEN` (D2) |
| No new dependencies needed | ✓ all techniques use existing deps (`go.opentelemetry.io/otel/trace` already in go.mod) |
