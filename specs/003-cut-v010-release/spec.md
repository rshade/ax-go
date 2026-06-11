# Feature Specification: Cut v0.1.0 — Make ax-go a Consumable, Pinnable Release

**Feature Branch**: `003-cut-v010-release`

**Created**: 2026-06-09

**Status**: Draft

**Input**: User description: "Cut v0.1.0 — make ax-go a consumable, pinnable release" (GitHub issues #14, #6, #3)

## Problem Context

ax-go has earned its v0.1.0 by its own roadmap definition — the sole
"Immediate Focus" item (#1, bounded config reads) merged in `ea74c7d` — but no
release tag exists. Downstream consumers cannot pin a version; they would
depend on a pseudo-version commit hash with no changelog, no contract snapshot,
and no upgrade signal.

Three concrete gaps block an honest tag, addressed by three workstreams in this
feature:

- **Workstream A** (issue #6): The integration example's version string must
  be derived from VCS at build time — not hardcoded — to close the "never ship
  `dev`/`unknown`" safeguard. The public version-resolution helper and build
  tooling wiring are governed by `specs/002-version-injection`; this workstream
  confirms those deliverables are complete and the example reports a real version.
- **Workstream B** (issue #3, narrowed): Two public output contracts — the
  success `Envelope[T]` JSON shape and the NDJSON line shape — have no golden
  fixture. Once v0.1.0 is tagged and the module proxy caches it immutably, these
  shapes must be locked before the tag is minted.
- **Workstream C** (issue #14): The release-please pipeline has failed on every
  push to `main`; the root cause is a missing or invalid secret. The workflow
  must be fixed and the version override applied so the first release PR proposes
  `0.1.0`, not `0.0.1`, before any tag is minted.

## Resolved Decisions

The following open questions from the user description are resolved here and
need not be re-raised during planning:

1. **Versioning cadence**: Keep `bump-patch-for-minor-pre-major: true`. Minor
   bumps (e.g., v0.2.0) signal deliberate roadmap milestones; `feat:` commits
   accumulate as patch releases within a minor. `Release-As:` overrides handle
   milestone cuts. This is already set in `release-please-config.json`.
2. **Version fallback semantics**: The precedence chain (ldflags → build-info
   → documented sentinel) is governed by `specs/002-version-injection`. The
   "never ship `dev`/`unknown`" safeguard binds *release artifacts*, not
   development builds. A `(devel)` build-info value is acceptable and honest for
   in-development builds.
3. **Token strategy**: Switch to `GITHUB_TOKEN`. No GoReleaser workflow is
   planned for v0.1.0; the PAT requirement adds operator burden without benefit.
   If a release-triggered downstream workflow is needed in future, the token can
   be upgraded at that time.
4. **Version seams in goldens**: The success `Envelope[T]` type holds no
   `version` field — only `trace_id`, `span_id`, and `idempotency_key`. All
   three are injectable via existing context seams. No new public seam is needed
   for Workstream B.

## User Scenarios & Testing *(mandatory)*

The actors in this feature are: **(a) downstream Go module consumers** — teams
building CLIs on ax-go who need a stable, importable release to pin in their
`go.mod`; **(b) LLM agents** that drive ax-go-based CLIs and read `__schema`
to identify exactly which build they are talking to; **(c) the ax-go
maintainer** who needs a reproducible release flow that runs without manual
intervention; and **(d) API consumers** whose contract expectations must survive
across releases without surprise breakage.

### User Story 1 - A downstream consumer can pin ax-go at a stable release (Priority: P1)

A developer at `gh-aw-fleet` (or any other rshade-portfolio project) wants to
add ax-go as a dependency with a real, go-module-proxy-resolvable version —
not a pseudo-version hash. They run a standard module pin command and get a
tagged release with a corresponding changelog.

**Why this priority**: Until this story is complete, none of the downstream
portfolio projects can safely depend on ax-go. This is the gate that all other
workstreams flow toward — if the tag is never minted, the rest is moot.

**Independent Test**: Run a standard module resolution command from an external
directory, specifying `v0.1.0`. Confirm the module proxy returns the release,
the `go.sum` entry resolves, and the `CHANGELOG.md` contains a generated
section for the release. This delivers the core consumer value independently of
any other story.

**Acceptance Scenarios**:

1. **Given** the release-please PR for v0.1.0 has been merged into `main`,
   **When** a consumer resolves the module at `v0.1.0`, **Then** the Go module
   proxy returns the release and the version is resolvable without errors.
2. **Given** the tag `v0.1.0` exists on `main`, **When** a consumer inspects
   `CHANGELOG.md`, **Then** it contains a generated section for `0.1.0` derived
   from the conventional-commit history — no manually authored changelog content.
3. **Given** the release-please workflow run for the release PR merge, **When**
   the workflow completes, **Then** it is green (no failures).

---

### User Story 2 - Public output contracts are frozen before the immutable tag (Priority: P2)

An API consumer (a developer or agent) relies on the JSON shapes that ax-go
emits on `stdout`. They need confidence that the `Envelope[T]` success shape
and the NDJSON streaming line shape are stable-by-contract — so that a minor
version bump never silently breaks a consumer's JSON parser.

**Why this priority**: Once v0.1.0 is tagged, the module proxy caches it
immutably. A wire-shape defect discovered after tagging requires a new release
to fix. The golden fixtures must land in the same release, not after it. This
story depends only on being able to construct a fixed-input envelope output —
independent of the tag existing yet.

**Independent Test**: Write two golden-file tests that (a) produce a
`Envelope[T]` JSON output with all non-deterministic fields held fixed via
context injection, and (b) produce a NDJSON line output the same way. Confirm
both tests fail on any byte-level change to the output. The tests pass in CI
with no tag required.

**Acceptance Scenarios**:

1. **Given** a fixed set of context values (known trace ID, span ID, idempotency
   key), **When** `NewEnvelope` and `WriteJSON` produce a success payload,
   **Then** the output matches the golden fixture byte-for-byte.
2. **Given** the same fixed context values, **When** `WriteJSONLine` produces
   an NDJSON line, **Then** the output matches the NDJSON golden fixture
   byte-for-byte.
3. **Given** any byte-level change to the `Envelope[T]` or `Metadata` output
   shape, **When** the golden-file tests run, **Then** they fail — so no shape
   drift goes undetected.
4. **Given** the existing golden fixtures in `testdata/`, **When** this feature
   is complete, **Then** every public `error_code` defined in specs/001 has a
   corresponding fixture and both `__schema` formats are covered.

---

### User Story 3 - The integration example reports its real built version everywhere (Priority: P3)

A maintainer or an adopter following the pattern builds the integration example.
Every version-bearing output — `__schema.version`, the `ax.Error` envelope's
`version` field, and the logger's `version` label — reflects the actual VCS
state of the build. No hardcoded constant exists in the example.

**Why this priority**: This is the concrete behavioral demonstration that version
injection works end-to-end. It is the most visible signal that the "never ship
`dev`/`unknown`" safeguard is upheld, and it validates that `specs/002`'s
deliverables are complete. It depends on Workstream A from spec 002 and the
Makefile build target being wired.

**Independent Test**: Build the integration example through the documented build
path and run its `__schema` command. Confirm the `version` field is non-empty,
reflects the current VCS state, and is byte-identical to what the `ax.Error`
envelope and logger emit. Also confirm `grep -rn 'const version' examples/`
returns nothing.

**Acceptance Scenarios**:

1. **Given** the integration example built through the documented Makefile
   target, **When** `__schema` is invoked, **Then** the `version` field is
   non-empty and reflects the source commit (or nearest tag).
2. **Given** the same build, **When** an error is emitted to stderr, **Then**
   the `ax.Error` envelope's `version` field is byte-identical to `__schema.version`.
3. **Given** the example source, **When** `grep -rn 'const version' examples/`
   runs, **Then** it returns no matches — the hardcoded constant is gone.

---

### User Story 4 - The release pipeline runs automatically on future `main` merges (Priority: P4)

After the v0.1.0 release is cut, the maintainer merges a conventional-commit
feature branch. The release-please pipeline picks it up automatically, opens a
release PR, and the next release flows through the same path — no operator
intervention required beyond the initial workflow fix.

**Why this priority**: A one-time release cut with a broken pipeline is not
sustainable. The pipeline fix must survive the first use and be reproducible for
v0.1.1 and beyond. It depends on Story 1 having established the flow works for
v0.1.0.

**Independent Test**: Confirm the release-please workflow run for the v0.1.0
release PR merge is green. Then verify the workflow configuration is correct for
future runs (no missing secrets, no broken token reference).

**Acceptance Scenarios**:

1. **Given** the `release-please.yml` workflow updated to use `GITHUB_TOKEN`,
   **When** a push lands on `main`, **Then** the workflow run does not fail in
   the first 7–9 seconds (the symptom of the current breakage).
2. **Given** a conventional-commit `feat:` push to `main` after v0.1.0,
   **When** release-please runs, **Then** it opens a release PR proposing a
   version consistent with the configured bump policy.

---

### Edge Cases

- What happens when the release-please manifest starts at `0.0.0` and the first
  release commit history contains only `feat:` commits? Research D1 proved the
  bump settings alone do NOT produce `0.1.0`: release-please proposes `1.0.0`
  from a `0.0.0` manifest (issue #2087), and even absent that defect the
  settings would propose `0.0.1`. A one-shot `Release-As: 0.1.0` commit footer
  is therefore REQUIRED (FR-010); the release-PR version is verified before
  merge as the last gate before tag immutability.
- How does the system handle a dirty (modified) working tree when building the
  example for golden-fixture generation? The fixture must use a fully
  deterministic context (injected IDs, no real tracing) — the build's VCS state
  affects the version field of `__schema` but not the `Envelope[T]` golden
  fixture, which has no version field.
- What if the `RELEASE_PLEASE_TOKEN` secret was already set but incorrect?
  Switching to `GITHUB_TOKEN` removes that dependency entirely; the fix is
  unconditional.
- What happens if `make ci` is run with no prior golden fixtures in `testdata/`
  for the success envelope? The missing fixture path causes the test to fail at
  `os.ReadFile`, surfacing a clear error — not a silent pass.

## Requirements *(mandatory)*

### Functional Requirements

**Workstream A — Version Injection Completion (spec 002 gate)**

- **FR-001**: The integration example MUST NOT contain a hardcoded `const version`
  declaration. The version MUST be derived at build time via the pattern governed
  by `specs/002-version-injection`.
- **FR-002**: The Makefile `build-example` target MUST produce a binary that
  reports a non-empty, VCS-derived version in `__schema.version`, the `ax.Error`
  envelope's `version` field, and the logger's `version` label — all
  byte-identical for a given build.
- **FR-003**: The `specs/002-version-injection` deliverables (public
  version-resolution helper, Makefile target, README recipe) MUST be verified
  complete before v0.1.0 is tagged.

**Workstream B — Golden Fixtures for Success Contracts**

- **FR-004**: `testdata/` MUST contain a golden fixture for the success
  `Envelope[T]` JSON shape. The fixture MUST be generated with all
  non-deterministic fields (`trace_id`, `span_id`, `idempotency_key`) held
  fixed via context injection — using the same technique as the existing
  `error_envelope.golden.json`.
- **FR-005**: `testdata/` MUST contain a golden fixture for the NDJSON streaming
  line shape produced by `WriteJSONLine`. The fixture MUST be generated with the
  same fixed-context approach.
- **FR-006**: Both new golden-fixture tests MUST fail on any byte-level change
  to the corresponding output shape and MUST be included in the standard `make
  test` / `go test -race ./...` run.
- **FR-007**: A fixture audit MUST confirm: (a) every public `error_code` defined
  in specs/001 has a `testdata/` fixture exercised by a golden-file test; (b)
  both `__schema` output formats (`--as=ax` default and `--as=mcp`) are covered
  by fixtures in `testdata/`.
- **FR-008**: Non-deterministic fields MUST be pinned by injection through
  existing context/option seams — no normalization layer, no redaction, no
  field-scrubbing in the test harness.

**Workstream C — Release Pipeline Fix and v0.1.0 Tag**

- **FR-009**: The `release-please.yml` workflow MUST be updated to use
  `GITHUB_TOKEN` instead of `secrets.RELEASE_PLEASE_TOKEN`. The PAT secret
  reference MUST be removed.
- **FR-010**: The first release PR MUST propose exactly `0.1.0`. Planning
  verified (research D1) that the `.release-please-manifest.json` starting
  point (`"0.0.0"`) combined with the existing bump settings does NOT achieve
  this on its own: release-please does not respect `bump-minor-pre-major` /
  `bump-patch-for-minor-pre-major` when the manifest version is exactly
  `0.0.0` and proposes `1.0.0`
  ([release-please#2087](https://github.com/googleapis/release-please/issues/2087));
  even absent that defect, the settings would propose `0.0.1`. Therefore the
  commit carrying the workflow fix MUST include a one-shot `Release-As: 0.1.0`
  git footer. The commit log contains qualifying `feat:` commits (`dd42c87`,
  `ea74c7d`, `4b2b85f`); no corrective `feat:` commit is needed.
- **FR-011**: `CHANGELOG.md` MUST be generated by release-please from
  conventional-commit history. It MUST NOT be hand-authored. The generated
  section MUST use the `changelog-sections` mapping in `release-please-config.json`.
- **FR-012**: The README status section MUST NOT say "Implementation scaffold"
  at the time of tagging. It MUST state the current release and link to the
  changelog.
- **FR-013**: After the release PR is merged, `go get github.com/rshade/ax-go@v0.1.0`
  MUST resolve via the module proxy, and an integration example binary built
  from the tag MUST report `v0.1.0`.

**Cross-cutting**

- **FR-014**: `make ci` (or the equivalent `go test -race ./...` + `golangci-lint
  run` + `make doc-coverage`) MUST pass cleanly with all new golden fixtures in
  place before any release PR is merged.
- **FR-015**: No new third-party dependencies MAY be introduced by any workstream.
- **FR-016**: Stream separation MUST remain inviolate in all new test and example
  code: `stdout` carries only the final machine payload; `stderr` carries
  everything else.

### Key Entities *(include if feature involves data)*

- **Success Envelope** (`Envelope[T]`): The standard bounded JSON payload shape
  wrapping any command's `data` output alongside `meta` fields. Its wire shape
  is the primary output contract for all ax-go-based CLIs.
- **NDJSON Line**: A single newline-delimited JSON record emitted by streaming
  commands. Wire-identical to a success envelope serialized as one line.
- **Release Tag**: The git tag `v0.1.0` created by release-please on `main`,
  representing the first stable, pinnable ax-go release.
- **Release PR**: The pull request release-please opens (and auto-merges, or
  prompts the maintainer to merge) containing the version bump and generated
  `CHANGELOG.md` section.
- **Golden Fixture**: A file in `testdata/` whose byte content is the canonical
  expected output of a public output contract; any drift causes CI to fail.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: `go get github.com/rshade/ax-go@v0.1.0` resolves in under 30
  seconds from a fresh module cache — confirming the tag and module proxy
  entry exist.
- **SC-002**: Zero hardcoded version constants remain in `examples/` —
  `grep -rn 'const version' examples/` returns no output.
- **SC-003**: 100% of public output contracts defined in the v0.1.0 scope (success
  envelope, NDJSON line, error envelope, `__schema` in both formats, all
  specs/001 error codes) have corresponding golden fixtures that fail CI on any
  byte-level drift.
- **SC-004**: The release-please workflow run for the v0.1.0 release PR merge
  completes green — zero failures.
- **SC-005**: An integration example binary built from the `v0.1.0` tag via the
  documented build path reports `v0.1.0` in `__schema.version`, the `ax.Error`
  envelope `version` field, and the logger `version` label — all byte-identical.
- **SC-006**: `make ci` passes cleanly on the commit that becomes the v0.1.0
  tag — zero lint, test, vet, or doc-coverage failures.
- **SC-007**: The `CHANGELOG.md` generated by release-please contains a section
  for `0.1.0` populated exclusively from conventional-commit history — zero
  manually authored lines.

## Assumptions

- Source inputs: GitHub issues #14, #6, and #3. The version-injection
  implementation details (public helper, Makefile target, build-metadata
  fallback chain) are governed by `specs/002-version-injection`; this feature
  treats spec 002 deliverables as a prerequisite and verifies them complete, but
  does not re-specify them.
- No governing ADR is being retired by this feature. ADR-0003 (which established
  `__schema` carries a `version` field) remains frozen; its decisions have
  already been absorbed into `specs/002-version-injection/research.md`.
- The release-please manifest starting at `"0.0.0"` does NOT produce a `0.1.0`
  proposal on its own — planning verified this assumption false (research D1,
  release-please #2087). A one-shot `Release-As: 0.1.0` commit footer on the
  workflow-fix commit produces the correct first version (FR-010).
- Switching to `GITHUB_TOKEN` removes the only known failure cause for the
  release-please workflow. If the workflow fails for an unrelated reason after
  this change, that is a new defect, not in scope for this feature.
- The success `Envelope[T]` type has no `version` field; only `trace_id`,
  `span_id`, and `idempotency_key` are non-deterministic and must be pinned by
  context injection in the golden-fixture tests.
- All new golden fixtures are written once (update mode) and then locked (assert
  mode) using the existing `assertGolden` test harness — no new test
  infrastructure is needed.
- GoReleaser and binary distribution artifacts are out of scope. ax-go is a
  library; the git tag is the release artifact.
- Any change to the `Envelope[T]`, `Metadata`, or other envelope shapes is out
  of scope. This feature freezes what exists; it does not redesign it.
