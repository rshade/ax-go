# Feature Specification: Build-time Version Injection via -ldflags

**Feature Branch**: `002-version-injection`

**Created**: 2026-06-08

**Status**: Draft

**Input**: User description: "Build-time version injection via -ldflags" (GitHub issue #6)

## Clarifications

### Session 2026-06-08

- Q: When a binary is built WITHOUT the version injected (a bare `go build`,
  `go run`, or an adopter who forgot the `-ldflags` flag), what should
  `__schema.version` report? → A: A runtime **build-info fallback**. Version
  resolution follows a deterministic precedence: an explicitly injected
  (link-time) value wins; absent that, the VCS build metadata the toolchain
  embeds (`runtime/debug.ReadBuildInfo` — revision, modified flag, and the main
  module version) is used so even an un-injected build surfaces a real commit
  identifier; only when neither is available is a clearly-marked, documented
  last-resort sentinel used. The resolved value is NEVER empty and is never the
  bare strings `dev` or `unknown`.
- Q: Should ax-go expose a reusable version-resolution helper as public API, or
  stay docs + example only? → A: Expose a reusable **public helper** that
  applies the precedence and fallback, AND demonstrate it in the example and
  document it in the README. Adopters get the behavior with a single call rather
  than reimplementing build-metadata parsing. This is a governed public-API
  addition (recorded in `research.md` during planning, reconciled to the
  constitution).

## User Scenarios & Testing *(mandatory)*

The consumers of this feature are: (a) **adopters** — Go developers building
CLIs on top of `ax-go` who need a reliable, copyable way to stamp a real
version into their tool; (b) **LLM agents** — which read `__schema.version` to
identify exactly which build of a tool they are driving, for version-skew
detection, bug attribution, and reproducible runs; and (c) **`ax-go`
maintainers** building the `examples/integration` reference binary. Today
`__schema.version` ships empty because the version is held in a constant the
linker cannot override and no build wiring injects one — a direct violation of
the project mandate "Never ship `dev` or `unknown` to production agents."

### User Story 1 - A built binary reports a real version in `__schema` (Priority: P1)

A maintainer (or an adopter following the pattern) builds the reference binary
through the documented build path. Running the binary's `__schema` command shows
a real, VCS-derived version string instead of an empty field. An agent reading
that schema can now name the exact build it is talking to.

**Why this priority**: This is the entire reason the feature exists. It closes a
documented mandate violation — an empty `__schema.version` makes every
ax-go-based CLI unidentifiable to the agents that drive it. Every other story
refines or hardens this one.

**Independent Test**: Build the reference binary through the documented build
path, run its `__schema` command, and confirm the `version` field is non-empty
and equals the VCS-derived value for the source commit. This alone delivers the
core value: an identifiable build.

**Acceptance Scenarios**:

1. **Given** the reference binary built through the documented build path,
   **When** an agent runs its `__schema` command, **Then** the `version` field
   is non-empty and identifies the source commit (or nearest tag).
2. **Given** a build from a tagged commit, **When** `__schema` is invoked,
   **Then** the reported version reflects that tag.

---

### User Story 2 - Even an un-injected build reports a meaningful version (Priority: P2)

A developer runs the tool directly during iteration (`go run`), or an adopter
builds it without remembering the injection flag. Rather than reporting an empty
or placeholder version, the tool falls back to the VCS metadata the toolchain
embeds automatically, surfacing a real commit identifier. The version is never
empty and never the bare strings `dev` or `unknown`.

**Why this priority**: A guarantee that holds only on the "happy" build path is
fragile — the empty-version bug being fixed here originates exactly from a build
that did not inject anything. The fallback makes "non-empty, meaningful version"
a property of *every* build, not just the documented one. It depends on Story
1's version plumbing existing.

**Independent Test**: Run the binary built WITHOUT link-time injection (for
example via `go run` from a VCS working tree) and confirm the reported version
is non-empty and reflects VCS state (commit revision, with a marker when the
tree is modified), falling to the documented sentinel only when no VCS metadata
exists at all.

**Acceptance Scenarios**:

1. **Given** a binary built without link-time injection from a VCS working tree,
   **When** `__schema` is invoked, **Then** the `version` field reflects the
   commit revision and is non-empty.
2. **Given** a build from a modified (dirty) working tree, **When** `__schema`
   is invoked, **Then** the version carries a marker distinguishing it from a
   clean build.
3. **Given** a build with neither link-time injection nor any embedded VCS
   metadata, **When** `__schema` is invoked, **Then** the version is the
   documented last-resort sentinel — non-empty and visibly non-release, never
   the bare `dev` or `unknown` strings.

---

### User Story 3 - Adopters replicate the pattern with one helper call and one build flag (Priority: P3)

An adopter wiring a new CLI on `ax-go` follows the README recipe: they call the
reusable version-resolution helper, pass the result through the standard
execution path, and add the one documented build flag to their build command.
Their CLI now surfaces a correct version in `__schema` — without copying any
build-metadata parsing logic.

**Why this priority**: `ax-go`'s purpose is to make the AX pattern copyable
across the portfolio. A correct-but-unsharable fix would leave every downstream
CLI to reinvent (and get wrong) the same fallback parsing. The public helper and
documented recipe turn the fix into a reusable contract. It builds on Stories 1
and 2 having defined the resolution behavior.

**Independent Test**: Follow the README recipe in the reference example (or a
fresh CLI): call the public helper, build with the documented flag, and confirm
`__schema.version` is real — with no build-metadata parsing written by the
adopter.

**Acceptance Scenarios**:

1. **Given** the public version-resolution helper, **When** an adopter passes
   its result to the standard execution path and builds with the documented
   flag, **Then** `__schema.version` reports the injected version.
2. **Given** the README recipe, **When** an adopter follows it without writing
   build-metadata parsing, **Then** their CLI surfaces a real version.
3. **Given** the reference example, **When** its tests run, **Then** they assert
   `__schema.version` is non-empty and the helper carries a verified runnable
   example.

---

### Edge Cases

- **No tags exist yet**: The repository currently has zero git tags, so a
  tag-only describe would fail outright. The documented build path MUST derive a
  version that degrades gracefully to a commit identifier (with a dirty marker)
  rather than failing the build or producing an empty value.
- **Dirty (modified) working tree**: A build from uncommitted changes MUST carry
  a marker distinguishing it from a clean build of the same commit, so an agent
  can tell a reproducible release build from a local work-in-progress build.
- **Built with no VCS metadata at all**: For a binary built outside a VCS tree
  and without injection (for example, from a source archive lacking history),
  resolution MUST prefer the embedded main-module version when present and
  otherwise fall to the documented last-resort sentinel — always non-empty,
  always visibly non-release.
- **Un-injected `go run` / bare `go build` from a VCS tree**: Resolution MUST
  surface the toolchain-embedded commit revision, never an empty value.
- **Installed via the module toolchain** (`go install <module>@<version>`): The
  embedded main-module version (e.g., a release tag) MUST be surfaced when no
  link-time value was injected.
- **Adopter omits the build flag**: The helper's fallback MUST prevent an empty
  version; the result is a commit identifier or the documented sentinel, never
  empty.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The reference example MUST surface a real, non-empty version in
  `__schema.version` when built through the documented build path, closing the
  current empty-version defect.
- **FR-002**: The tool version MUST be injectable at build time via linker flags
  (`-ldflags "-X ..."`). This requires the version to be held in a writable
  package-level variable, not a constant (the linker cannot rewrite constants).
- **FR-003**: The documented build path MUST derive the injected version from
  VCS state in a way that SUCCEEDS even when no tags exist — degrading to a
  commit identifier with a dirty/modified marker rather than failing the build
  (e.g., `git describe --tags --always --dirty` semantics).
- **FR-004**: Version resolution MUST follow a deterministic precedence: (1) an
  explicitly injected link-time value; (2) absent that, the VCS build metadata
  embedded by the toolchain (`runtime/debug.ReadBuildInfo` — revision, modified
  flag, and main-module version); (3) only when neither is available, a
  clearly-marked documented sentinel. The resolved value MUST NEVER be empty.
- **FR-005**: The resolved version produced by the documented build path MUST
  NEVER be the bare strings `dev` or `unknown`. The last-resort sentinel — used
  only when there is neither injection nor embedded VCS metadata — MUST be a
  documented value that is non-empty and visibly distinguishable from a real
  release version.
- **FR-006**: `ax-go` MUST expose a reusable, public version-resolution
  capability so a consumer obtains the full precedence-and-fallback behavior
  (FR-004) with a single call, without reimplementing build-metadata parsing.
  This is a governed public-API addition.
- **FR-007**: The single resolved version MUST flow into every place `ax-go`
  surfaces a version for a CLI — `__schema.version`, the `ax.Error` envelope's
  `version` field, and the logger's `version` label — from one source of truth,
  so the three never disagree for a given build.
- **FR-008**: Given the same commit and the same build inputs, the documented
  build path MUST produce the same version string (determinism mandate), modulo
  the documented dirty/modified marker, which faithfully reflects real
  uncommitted-change state.
- **FR-009**: The `examples/integration` command MUST demonstrate the
  end-to-end pattern (injectable variable + helper call + the build target) and
  surface a real version in its `__schema` output; its tests MUST assert the
  version is non-empty.
- **FR-010**: The Makefile MUST provide a build target that builds the example
  binary WITH version injection. The existing library `build` target
  (`go build ./...` over the module) is not a binary build and MUST NOT be the
  injection site, because `-X main.version` has no effect on a library compile.
- **FR-011**: The README MUST document the injection recipe — both the build-flag
  invocation and the helper usage — so adopters can replicate it in their own
  CLIs. The documentation MUST be backed by verifiable artifacts (the runnable
  reference example and a verified runnable example on the public helper) rather
  than prose alone, per the docs-as-contract discipline.

### Key Entities *(include if feature involves data)*

- **Resolved version**: The human- and agent-facing identifier surfaced in
  `__schema.version`, the `ax.Error` envelope `version`, and the logger
  `version` label. Always non-empty; VCS-derived whenever possible.
- **Injected version variable**: The writable package-level value the linker
  overrides at build time via `-ldflags "-X ..."`.
- **VCS build metadata**: The revision, modified flag, build time, and
  main-module version the Go toolchain embeds automatically
  (`runtime/debug.ReadBuildInfo`); the fallback source when nothing was
  injected.
- **Version-resolution helper**: The reusable public `ax-go` capability that
  applies the FR-004 precedence and returns the final non-empty version.
- **Last-resort sentinel**: The documented, clearly non-release value used only
  when neither injection nor embedded VCS metadata exists; never the bare `dev`
  or `unknown` strings.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of reference-binary builds through the documented build path
  report a non-empty `__schema.version` — zero empty-version regressions.
- **SC-002**: A binary built through the documented path reports a version that
  uniquely identifies the source commit and indicates whether the tree was
  clean, verifiable against the VCS state captured at build time.
- **SC-003**: A binary built WITHOUT link-time injection still reports a
  non-empty version reflecting VCS state — never empty and never the bare `dev`
  or `unknown` strings — in 100% of cases where VCS metadata is available, and a
  documented non-empty sentinel otherwise.
- **SC-004**: An adopter can wire the pattern into a new CLI with a single
  helper call plus one documented build flag, writing zero lines of
  build-metadata parsing.
- **SC-005**: Two builds from the same clean commit produce byte-identical
  version strings (determinism), modulo the documented dirty marker.
- **SC-006**: For any given build, the version surfaced in `__schema.version`,
  the `ax.Error` envelope `version`, and the logger `version` label are
  byte-identical (single source of truth).

## Assumptions

- Source inputs: GitHub issue #6. No governing ADR is being retired by this
  feature: ADR-0003 (referenced by the issue) governs the `__schema`/MCP shape
  and stays frozen — it already establishes that `__schema` carries a `version`
  field; this feature only fills that field. The version-injection mandate
  itself originates in the project agent guidance ("Version injection at
  build"), not an ADR.
- This feature adds a public API symbol (the version-resolution helper). Per the
  constitution and agent guidance, a public-API addition is governed through the
  Spec Kit feature workflow: the decision is recorded in this feature's
  `research.md` during planning and, if cross-cutting, the constitution is
  amended. The helper's exact exported name and signature are a planning detail,
  not fixed by this spec.
- "Real version" means a VCS-derived identifier: a tag-based describe value when
  tags exist, otherwise a commit identifier (with a dirty marker for modified
  trees). Because the repository currently has zero tags, the documented build
  yields a commit-identifier-based version until the first release tag exists.
- The documented build path is the project Makefile (usable locally and from
  CI). A full release pipeline (a dedicated release job, goreleaser, signed
  artifacts, etc.) is out of scope for this feature; the Makefile target is the
  documented, reusable injection site.
- `ax-go` is a library; the only first-party runnable binary today is
  `examples/integration` (with the future `ax-go mcp-server` to come). The
  example is the reference demonstration, and the same pattern applies verbatim
  to future binaries.
- The exact spelling of the last-resort sentinel is a planning detail; the spec
  only constrains it to be non-empty, documented, visibly non-release, and
  never the bare `dev` or `unknown` strings.
