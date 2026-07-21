# Feature Specification: Certify and Internalize the Public Boundary Before v1.0

**Feature Branch**: `015-internalize-helpers`

**Created**: 2026-07-19

**Status**: Draft

**Input**: GitHub issue #18, "internal/: identify and migrate non-public helpers
before v1.0"

## Clarifications

### Session 2026-07-19

- Q: May identifiers classified as leaked helpers be removed immediately? →
  A: No. Constitution Principle XII applies to every exported symbol. Feature
  015 relocates non-public mechanics behind compatibility-preserving root
  forwarders, adds Go-recognized `// Deprecated:` migration notices, and ships
  those notices in a real `0.MINOR.0` release. A follow-up Spec Kit feature may
  remove a forwarder only after at least one published minor carried its notice.
- Q: How is the boundary protected from silent regression? → A: A new
  committed-baseline gate inventories the complete compiler-visible root API
  and fails CI when the live surface differs from its reviewed baseline.
- Q: Is the historical classification audit also the live baseline? → A: No.
  The audit is a permanent decision record and never deletes rows. The live
  baseline is the current approved compiler-visible surface. The gate
  cross-validates both artifacts so their distinct lifecycles cannot drift.
- Q: What does the public-package allowlist decide? → A: It decides which
  packages are governed public packages. It does not classify identifiers
  within package `ax`; identifier intent, documented contract, and downstream
  evidence determine that classification.
- Q: Does "relocate under internal" allow merely lower-casing a helper in the
  root package? → A: No. Clear non-public mechanics move to cohesive
  role-specific `internal/` packages. Temporary root forwarders are public
  compatibility seams, not the implementation's permanent home.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Permanent certification of the current surface (Priority: P1)

As the ax-go maintainer preparing for v1.0, I need a committed record of every
compiler-visible API feature exposed by the root `ax` package, together with the
decision to support it or retire it, so the candidate v1 surface is reviewable
before any migration begins.

**Why this priority**: Classification is the approval boundary for every later
code and release change.

**Independent Test**: Generate the surface inventory for every supported
GOOS/GOARCH profile, confirm the profiles expose the same canonical feature set,
and verify that every feature maps to exactly one retained audit decision.

**Acceptance Scenarios**:

1. **Given** the root package, **When** the audit is generated, **Then** it
   includes package declarations, exported fields, complete interface methods,
   value and pointer method sets, promoted selectors, aliases, embedding, and
   only genuinely reachable hidden concrete types.
2. **Given** an intentional facade alias or wrapper, **When** it is classified,
   **Then** it is supported public API unless explicit contract evidence says
   otherwise.
3. **Given** a clear implementation leak, **When** it is classified, **Then**
   the permanent record includes its rationale, internal target, compatibility
   strategy, replacement, lifecycle, and downstream-search evidence.
4. **Given** an ambiguous identifier, **When** evidence is insufficient,
   **Then** it remains supported public API.

---

### User Story 2 - Non-public mechanics internalized without an early break (Priority: P2)

As a downstream CLI author, I want accidental helpers to move behind a stable
compatibility seam before eventual removal so I receive a machine-visible
migration warning without an unannounced break.

**Why this priority**: It enacts the approved audit while satisfying the
constitution's deprecation lifecycle.

**Independent Test**: For every audit-approved leak, verify that its mechanics
live in a cohesive `internal/` package, the root declaration retains the same
name/type/semantics, its doc comment contains a valid `Deprecated:` paragraph,
all in-repo call sites use the replacement, and `go-apidiff` reports no
incompatible public change.

**Acceptance Scenarios**:

1. **Given** a function or value that can be forwarded without changing its
   contract, **When** it is internalized, **Then** the implementation moves
   under `internal/` and the root export delegates to it unchanged.
2. **Given** a type whose relocation would change identity or semantics,
   **When** compatibility cannot be proved, **Then** it is deprecated in place
   and relocation is deferred to the follow-up removal feature rather than
   forcing a break.
3. **Given** a deprecated export, **When** documentation and static analysis
   run, **Then** its `Deprecated:` paragraph names the replacement or reason,
   and no repository call site triggers SA1019.
4. **Given** this feature's PR, **When** API diff runs, **Then** it contains no
   removal, rename, re-type, or semantic break.

---

### User Story 3 - Boundary changes remain explicit and machine-checkable (Priority: P3)

As the maintainer, I want new, removed, or signature-changed API features to
fail a deterministic local and CI gate until both the live baseline and the
permanent audit carry an explicit reviewed decision.

**Why this priority**: A boundary is durable only when future changes cannot
silently bypass it.

**Independent Test**: Add a package declaration, exported field, interface
method, promoted selector, or platform-specific export and confirm the gate
returns validation exit `2`, leaves stdout empty, and writes exactly one
minified `ax.Error` envelope to stderr.

**Acceptance Scenarios**:

1. **Given** unchanged source and artifacts, **When** the gate runs from the
   module root, **Then** stdout contains one minified strict-JSON pass result,
   stderr is empty, and the exit code is `0`.
2. **Given** live-surface drift, **When** the gate runs, **Then** stdout is
   empty, stderr contains one deterministic `ax.Error`, and the exit code is
   `2`.
3. **Given** an audit record in `deprecated` state, **When** the gate runs,
   **Then** the corresponding source declaration exists and carries a
   Go-recognized deprecation paragraph.
4. **Given** a later follow-up removal, **When** its audit row transitions to
   `removed`, **Then** the row remains in history while the live baseline and
   source no longer contain the feature.

### Edge Cases

- Direct and promoted exported struct fields are independently visible API
  features and must be inventoried.
- Embedded interface methods are inventoried through the public selector by
  which consumers call them.
- Value and pointer-only methods are distinct when their selector sets differ.
- An exported method on an unexported receiver is included only when an exported
  root declaration exposes that concrete type. Returning an interface exposes
  the interface's methods, not every method of its hidden implementation.
- Type aliases remain root declarations; their externally selectable members
  are attributed to the root alias for review.
- Ambiguous or shadowed promoted members follow the Go type checker's selector
  rules rather than a syntax-only approximation.
- The six supported profiles (`linux`, `darwin`, `windows` × `amd64`, `arm64`)
  must expose an invariant surface. Profile drift is validation failure.
- `_test.go` declarations are not importable production API and are excluded.
- A known downstream use favors `supported`; absence in an indexed search is
  evidence only and never proof that early removal is safe.
- An identifier that has never appeared in a published tag is still not removed
  by this feature because Principle XII defines no unpublished-symbol exception.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The feature MUST commit
  `specs/015-internalize-helpers/public-surface-audit.json`, retaining one
  decision record for every compiler-visible API feature of the root `ax`
  package at the audit point.
- **FR-002**: The inventory MUST be type-aware and include exported package
  declarations, direct and promoted fields, complete interface method sets,
  value and pointer method sets, aliases, embedding, and reachable hidden
  concrete types according to Go selector rules.
- **FR-003**: The inventory MUST produce the same canonical feature set for all
  six supported GOOS/GOARCH profiles; a profile-specific difference MUST fail
  closed as validation error exit `2`.
- **FR-004**: The public-package allowlist MUST define the packages governed by
  API policy, while each root API feature MUST be classified by intended
  contract and evidence as `supported` or `implementation-leak`.
- **FR-005**: Every `implementation-leak` record MUST retain a one-line
  rationale, cohesive `internal/` target, compatibility strategy, replacement
  or removal reason, downstream-search evidence, and lifecycle state.
- **FR-006**: Clear non-public mechanics MUST move to cohesive role-specific
  `internal/` packages when a compatibility-preserving root forwarder can keep
  the current Go contract byte-for-byte/type-for-type equivalent. A generic
  `internal/helpers` package is forbidden.
- **FR-007**: Every exported retirement candidate MUST remain exported in this
  feature with unchanged name, type, and observable semantics and MUST gain a
  Go-recognized `// Deprecated:` paragraph. If safe forwarding cannot be
  proved, implementation relocation is deferred rather than breaking the API.
- **FR-008**: Feature 015 MUST NOT remove, rename, re-type, or semantically
  change an exported identifier. It MUST ship deprecation notices in a
  non-breaking `feat:` release that becomes a published `0.MINOR.0`.
- **FR-009**: Removal MUST occur only in a follow-up Spec Kit feature after at
  least one published minor carried the notice; that removal uses
  `breaking-change-approved` and a `feat!:` / `BREAKING CHANGE:` commit.
- **FR-010**: Supported public features and existing machine-payload contracts
  MUST remain unchanged; the error-envelope and `__schema` golden files MUST
  pass without modification.
- **FR-011**: `go build ./...`, `go test -race ./...`, `go vet ./...`,
  `golangci-lint run`, `make doc-coverage`, `make cover-check`, and the existing
  benchmark budget MUST remain green. The integration example MUST use only
  supported public API and internal test support.
- **FR-012**: A committed live baseline at
  `internal/cmd/surfacecheck/baseline.json` and a new
  `internal/cmd/surfacecheck` gate MUST detect additions, removals, and
  signature/member changes across the complete root surface and cross-validate
  active audit records.
- **FR-013**: Successful gate output MUST be one minified strict-JSON struct on
  stdout with empty stderr. Every failure MUST leave stdout empty, write exactly
  one minified `ax.Error` envelope to stderr, and use deterministic exit codes:
  `2` for drift or invalid repository input, `1` for unexpected internal errors,
  and `4` for permission failure.
- **FR-014**: The boundary decision MUST be recorded in `research.md`, anchored
  to Constitution Principles X–XII, and MUST NOT create or edit an ADR.

### Key Entities

- **API Feature**: One compiler-visible selector or package declaration with a
  stable canonical ID and signature.
- **Audit Record**: Permanent decision history for an API Feature, including
  classification, rationale, disposition, lifecycle, release evidence, and
  downstream evidence. Records are never deleted.
- **Live Surface Baseline**: The current approved canonical feature IDs and
  signatures consumed by the gate.
- **Lifecycle**: `live` → `deprecated` → `removable` → `removed`; only a
  published minor can make `deprecated` eligible for `removable`.
- **Supported Package Allowlist**: The package-level scope guarded by
  `apidiff-verdict`; distinct from identifier-level classification.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of compiler-visible root API features across all supported
  profiles have exactly one retained audit decision.
- **SC-002**: 100% of approved implementation leaks have an internal target,
  compatibility strategy, migration note, and downstream evidence.
- **SC-003**: Feature 015 removes zero exported symbols and introduces zero
  incompatible `go-apidiff` findings.
- **SC-004**: Every deprecated record maps to a present source declaration with
  a valid `Deprecated:` paragraph; repository call sites produce zero SA1019
  findings.
- **SC-005**: The live baseline and current source match exactly; a change to a
  declaration, field, interface method, promoted selector, method set, alias, or
  supported-target profile fails CI until explicitly reviewed.
- **SC-006**: Gate stdout/stderr and exit behavior are byte-deterministic and
  conform to FR-013 for every tested outcome.
- **SC-007**: Existing machine-payload goldens pass unchanged and all required
  quality gates remain green without lowering coverage floors or performance
  budgets.
- **SC-008**: Zero ADR files are created or modified; the permanent audit and
  research record remain discoverable after later removals.

## Assumptions

- GitHub issue #18 is the source input. Its ADR-0012 reference is stale:
  ADR-0012 never existed, ADRs are frozen, and Constitution Principle X now
  governs the layout decision.
- Root package `ax` remains the supported facade. The contract packages
  (`config`, `contract`, `id`, `mcp`, `schema`) remain public and apidiff-gated,
  but their identifiers are outside this root-only audit.
- Classification is conservative. Ambiguity resolves to `supported`.
- Feature 015 is the notice/internalization feature, not the removal feature.
  Publication is an external release milestone and cannot be simulated inside
  one implementation PR.
- No new module dependency is required; compiler export data and type traversal
  are available through the standard library and the Go toolchain.
