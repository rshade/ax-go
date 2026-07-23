# Feature Specification: Import-Isolated Logging Package

**Feature Branch**: `017-import-isolated-logging`

**Created**: 2026-07-22

**Status**: Draft

**Input**: User description: "Import-isolated `logging` public package backed by
`internal/logcore`, so a CLI consumer can take ax-go's zerolog logger, stream
separation, and trace-correlated trace_id/span_id fields without linking the
OTel SDK, OTLP exporter, gRPC, protobuf, MCP SDK, or Cobra. Root `ax` keeps
every symbol via type aliases (non-breaking). Loki stays in root `ax`
unchanged."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Small binary for a distributed CLI (Priority: P1)

A maintainer ships a desktop-shaped Go CLI as cross-compiled binaries for
Windows, macOS, and Linux. They want ax-go's structured logging, stream
separation, and trace-correlated log fields. Today the only way to get a
logger is through the root package, which links a telemetry export stack the
tool will never use, so every user downloads roughly 12 MB per platform.

They want to depend on logging alone and ship a few megabytes instead.

**Why this priority**: This is the entire reason the feature exists, and it is
the only story that delivers the size reduction. Shipped alone it is a
complete, valuable increment.

**Independent Test**: Build a program that obtains a logger and writes one
correlated log line, importing only the logging surface. Verify it produces
correct output, and measure the stripped binary.

**Acceptance Scenarios**:

1. **Given** a program importing only the logging surface, **When** it is
   built, **Then** the resulting binary excludes the telemetry export,
   remote-procedure-call, and command-framework dependency trees.
2. **Given** that same program, **When** its stripped binary is measured on
   64-bit Linux, **Then** it is under 3 MB.
3. **Given** an active trace context, **When** the program emits a log line,
   **Then** the line carries the correct trace and span identifiers.
4. **Given** no active trace context, **When** the program emits a log line,
   **Then** it is emitted without allocating additional memory for
   correlation.
5. **Given** the program writes a log line, **When** its output streams are
   inspected, **Then** the line appears on the diagnostic stream and the
   payload stream is untouched.

---

### User Story 2 - Existing adopters notice nothing (Priority: P2)

A team already builds on the root package. They upgrade to the release
containing this change and expect their code to compile untouched, behave
identically, and produce byte-identical log output. They have no interest in
the new surface and should not need to learn it exists.

**Why this priority**: Adoption of P1 is worthless if it costs existing
adopters a migration. This story is what makes the change safe to release,
but it delivers no new capability on its own.

**Independent Test**: Compile and run the existing test suite and the
integration example unchanged, and compare emitted log lines against the
pre-change output.

**Acceptance Scenarios**:

1. **Given** code calling any existing root-package logging symbol, **When**
   it is compiled against the new release, **Then** it compiles with no
   source change.
2. **Given** identical configuration, **When** a log line is emitted through
   the root package and through the new surface, **Then** the two lines are
   byte-identical.
3. **Given** the public API comparison tool runs on this change, **When** it
   evaluates the gated public surface, **Then** it reports no breaking
   change.
4. **Given** a consumer holds a logger obtained from the root package,
   **When** they pass it to a function expecting the new surface's logger
   type, **Then** it is accepted, because the two names denote one type.

---

### User Story 3 - Log shipping is untouched (Priority: P3)

An operator relies on ax-go's direct log-shipping addon, configured entirely
through environment variables. They expect this change to alter nothing:
same activation, same label discipline, same drain-on-shutdown behavior, same
failure semantics.

**Why this priority**: A regression here silently loses production logs — high
severity, but it is a preservation guarantee rather than new capability, and
it is verified largely by the existing suite.

**Independent Test**: Run the existing log-shipping test suite with no
substantive modification.

**Acceptance Scenarios**:

1. **Given** the log-shipping environment variables are set, **When** a
   logger is constructed through the root package, **Then** shipping
   activates exactly as before.
2. **Given** labels are configured, **When** log lines are shipped, **Then**
   only the sanctioned low-cardinality labels are promoted to index labels,
   and payload fields reusing a label key stay payload-only.
3. **Given** labels and shipping are configured in either order, **When** the
   logger is built, **Then** the resulting label promotion is the same.
4. **Given** buffered entries exist, **When** the drain operation is invoked
   at shutdown, **Then** entries are delivered under the documented deadline
   and failures never change the process exit code.

---

### Edge Cases

- **Drain on the isolated surface.** The drain operation is offered on both
  surfaces for symmetry, but the only destination that buffers anything lives
  in the root package and is unreachable from the isolated surface. On the
  isolated surface it therefore always succeeds without doing work. This must
  be documented plainly so callers do not conclude their logs were shipped.
- **Nil logger passed to drain.** Must succeed without error, as today.
- **Configuration order.** Constructing a logger with configuration applied in
  any order must yield identical behavior, including label promotion.
- **Derived loggers.** A logger derived by attaching further labels must carry
  its destinations forward, so a later drain still reaches buffered entries.
- **Declined build configurations.** The isolated surface never links the
  dependency trees that the optional build constraints decline, so its
  isolation guarantee must hold identically in all four supported build
  configurations rather than only the default.
- **Concurrent use.** Multiple goroutines emitting through one logger, and a
  concurrent drain, must remain race-free.
- **No trace context.** Emission must succeed and stay on the documented
  zero-allocation path.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A consumer MUST be able to obtain a fully functional,
  trace-correlated logger while depending only on the new logging surface.
- **FR-002**: The logging surface's dependency graph MUST exclude the root
  package, the telemetry lifecycle internals, the telemetry software
  development kit, all telemetry exporters, the remote-procedure-call stack
  and its instrumentation, and the standard library network-transport and
  transport-security packages.
- **FR-003**: The logging surface MUST offer logger construction, output
  destination configuration, minimum severity configuration, label
  attachment, label derivation from an existing logger, access to the
  underlying logging handle, and the drain operation.
- **FR-004**: Every logging symbol currently exported by the root package MUST
  remain exported there with unchanged documented behavior and unchanged
  signature.
- **FR-005**: The root package's logger and label types MUST denote the same
  types as the logging surface's, so values pass between the two surfaces
  without conversion.
- **FR-006**: Log lines emitted through either surface under identical
  configuration MUST be byte-identical.
- **FR-007**: When a span is active, every emitted line MUST carry the correct
  trace and span identifiers; when none is active, emission MUST remain on the
  documented zero-allocation path.
- **FR-008**: All log output MUST go to the diagnostic stream; the machine
  payload stream MUST remain reserved.
- **FR-009**: The log-shipping addon MUST remain in the root package with its
  activation, authentication, label allowlist, drain semantics, and failure
  behavior unchanged.
- **FR-010**: The core logger MUST NOT reference any log-shipping-specific
  identifier. Coordination between them MUST occur through a general-purpose
  extension point that a future destination with no label concept can satisfy
  without implementing label behavior.
- **FR-011**: No publicly reachable interface may allow an external consumer
  to select, register, or supply an alternative logging backend or output
  destination.
- **FR-012**: The drain operation offered on the logging surface MUST document
  that it performs no work for consumers of that surface alone, and that
  documented statement MUST be asserted by an automated test rather than left to
  a doc comment alone. Tooling gates that documentation exists, never that it is
  true.
- **FR-013**: The logging surface MUST be added to the gated public API
  surface, and every allowlist governing that gate MUST be updated
  consistently in the same change.
- **FR-014**: The isolation guarantee MUST hold in all four supported build
  configurations, verified explicitly rather than assumed.
- **FR-015**: An automated check MUST fail the build if a consumer built against
  an isolated surface exceeds a reviewed size ceiling, **or** if the size
  reduction it achieves against an equivalent consumer of the full surface falls
  below a reviewed minimum. A ceiling alone leaves the reduction claim
  unverified: absolute size drifts loudly with every toolchain, while the
  reduction ratio can decay silently because nothing re-measures the comparison.
- **FR-016**: Documentation MUST be updated to describe the new surface,
  including a runnable example, the compatibility notes, and the roadmap.
- **FR-017**: The integration example MUST continue exercising the root path
  including log shipping and drain, and MUST gain a second example exercising
  the logging surface in isolation.

### Key Entities

- **Logger**: The structured-logging handle a consumer obtains and emits
  through. Carries configured labels, a minimum severity, a primary output
  destination, and zero or more additional destinations. Enriches every line
  with trace correlation when a span is active. One type, reachable by two
  names.
- **Labels**: The small, fixed set of low-cardinality descriptors attached to
  every line and eligible for promotion to index labels by a shipping
  destination. Deliberately bounded to protect index performance.
- **Destination**: A write-through output that can drain buffered entries at
  shutdown. Internal-only; consumers cannot supply one. The shipping addon is
  the sole implementation.
- **Label sanctioning**: The separate, optional capability by which a
  destination is told which label pairs are eligible for promotion. Kept
  distinct from the destination concept so a destination without a label model
  need not implement it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A program using only the logging surface produces a stripped
  64-bit Linux binary under 3 MB, against a measured 12,017,929-byte baseline
  for the equivalent root-package program.
- **SC-002**: The size reduction for a default build is at least 75%, verified
  automatically on every run of the size gate against a committed equivalent
  consumer of the full surface. Measured expectation is 81%.
- **SC-003**: Existing adopters need zero source changes; the public API
  comparison reports no breaking change.

  **Implementation note (2026-07-22, measured).** SC-003 holds, but reaching it
  required correcting the gate rather than the code.

  `go-apidiff` keys a type's identity on its **declaring package**, so relocating
  `Logger`, `Labels`, and `Option` into `internal/logcore` — even with
  identity-preserving aliases left in root `ax` — produces nine raw
  `Incompatible changes` findings, including a `Flush` entry whose "before" and
  "after" renderings are textually identical.

  This is a known false-positive class, and ax-go has **already shipped it once**.
  The v0.1.0 → v0.2.0 release moved `Error`, `Mode`, `Envelope`, `Schema`, and the
  config/schema option types into the import-isolated public packages; it was
  released as a plain `feat:` and was a no-op for adopters. Running `go-apidiff`
  across that tag boundary today reports **37 findings of exactly this shape**:

  ```text
  - Error: changed from Error to github.com/rshade/ax-go/contract.Error
  - ParseConfigOption: changed from ParseConfigOption to .../config.Option
  - NewError: changed from func(...) *Error to func(...) *Error
  ```

  The apidiff gate landed afterwards (PR #82) and had never been reconciled
  against the project's own precedent, so it would have demanded a `feat!:` for a
  release the project already established was non-breaking.

  `internal/cmd/apidiff-verdict` therefore gained a narrow **type-relocation
  classifier**: a finding is excused only when the two renderings are textually
  identical, or when a bare type name gained a declaring package **inside this
  module** while keeping its name (allowing the established `LoggerOption` →
  `Option` prefix convention). Removals, renames, signature changes, member
  changes, and relocations outside the module all remain breaking. Excused
  findings are printed in their own report section — never silently dropped — and
  structural changes to the relocated types stay gated by `surfacecheck`, which
  inventories every field and interface method across 24 loads.

  Its acceptance test is the release history: the classifier must rule the shipped
  v0.1.0 → v0.2.0 diff non-breaking, and does.

  Verified for this feature: `apidiff-verdict` reports `public_breaking=false`;
  `examples/integration` compiles and passes **unchanged** under both
  `make build-example` and `make build-example-minimal`; `surfacecheck` reports
  zero added or removed root features. The release is a normal `feat:`.

- **SC-004**: Log output is byte-identical across both surfaces under
  identical configuration, verified automatically.
- **SC-005**: The existing log-shipping suite passes with no substantive
  modification.
- **SC-006**: Emission with no active span allocates no more than the
  pre-change baseline, verified by benchmark.
- **SC-007**: The isolation guarantee is verified in all four build
  configurations, and a size regression — whether an absolute breach or an
  eroded reduction — fails the build.
- **SC-008**: All existing quality gates — race-enabled tests, linting,
  documentation-example coverage, coverage floors, and the public surface
  gate — remain clean.

## Out of Scope

- Removing, relocating, or behaviorally changing the log-shipping addon. It
  stays where it is, exactly as shipped.
- Any conditional-compilation mechanism for this feature. The package boundary
  is the isolation mechanism.
- A pluggable or second logging backend, a publicly reachable
  destination-registration capability, or runtime backend selection.
- Deprecating or removing any existing root-package symbol.
- Isolating command execution, the telemetry lifecycle, or the transport
  helpers. Those are legitimately root-package concerns.
- Offering log shipping to consumers of the isolated surface.

## Assumptions

- Source inputs: GitHub issue #144 and the validated design at
  `specs/017-import-isolated-logging/design.md`, which carries the
  measurements, the export-visibility analysis, and the resolution of two
  contradictions in the issue text. No governing ADR: the relevant decisions
  were absorbed into `specs/007-loki-direct-push/research.md`, whose
  requirement that the destination extension point stay general-purpose is
  carried forward as FR-010.
- This work is stacked on the optional-telemetry-export feature
  (`016-optional-grpc-otlp`, PR #150). Its build-constraint matrix, its
  expanded public-surface gate, and its allowlist duplication are treated as
  the baseline. The sequencing question raised in issue #144 was resolved by
  re-measurement: because that feature's opt-out is a build constraint
  defaulting off, a default build is byte-identical to today, so this
  feature's benefit is undiminished for consumers who pass no constraints.
- The size ceiling is expressed for 64-bit Linux with symbol stripping.
  Absolute sizes vary by toolchain version, so the ceiling carries headroom
  and a documented adjustment procedure, consistent with existing gates.
- The logging surface is named to avoid colliding with the standard library's
  logging package name at consumer call sites.
- The governing constitution names a single logging constructor. Because this
  feature exposes that same constructor under a second name, the constitution
  was amended (`1.2.0 → 1.2.1`, a clarifying PATCH) to admit an
  identity-preserving alias while restating that a second implementation or
  backend stays forbidden. The amendment rides this feature's change set; the
  spec assumes it is in place. See the plan's Constitution Check.
- Consumers of the isolated surface are assumed not to need log shipping. If
  that changes, shipping is added as a separate feature rather than relocated
  by this one.
- The single-backend guardrail is enforced by the toolchain's internal-package
  import restriction. The residual possibility of a maintainer adding a second
  backend without a compiler complaint is accepted and held by review,
  consistent with how this guardrail is enforced elsewhere.
