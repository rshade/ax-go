# Feature Specification: Hot-Path Benchmarks with `-benchmem`

**Feature Branch**: `011-hot-path-benchmarks`

**Created**: 2026-06-26

**Status**: Draft

**Input**: User description: "Hot-path benchmarks with -benchmem. ADR-0009 claims zerolog gives a zero/near-zero allocation hot path. AGENTS.md forbids asserting performance claims without a benchmark. No `testing.B` exists. Acceptance: `BenchmarkLogger*` with `-benchmem` covering the logging hot path (incl. the tracing hook); allocation profile documented; claim backed or revised."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Substantiate the logger allocation claim (Priority: P1)

A maintainer reading the logger's accepted design decision sees a claim of a
"zero or near-zero allocation hot path." Today that claim is asserted with no
measurement behind it, which the project's own testing discipline forbids. The
maintainer needs a runnable benchmark that reports the actual per-operation
allocation count and bytes for the common logging call so the claim is either
proven or corrected to match reality.

**Why this priority**: This is the core deliverable. Without a measured
allocation profile the performance claim is unsubstantiated and violates the
project mandate against asserting performance without a benchmark. Everything
else in this feature builds on having this number.

**Independent Test**: Run the logger benchmark suite with allocation reporting
enabled and observe a per-operation allocation count and byte count for the
standard "log one line at an enabled level" path. Delivers value on its own:
the project now has a measured number where it previously had only a claim.

**Acceptance Scenarios**:

1. **Given** the logger benchmark suite, **When** a maintainer runs it with
   allocation reporting enabled, **Then** each benchmark reports operations per
   second, bytes allocated per operation, and allocations per operation for the
   measured logging path.
2. **Given** the measured allocation numbers, **When** a maintainer compares
   them to the documented design claim, **Then** the claim is confirmed as
   accurate or the wording is revised to match the measurement, with the
   evidence recorded.
3. **Given** a logging call at a level below the configured threshold (a
   filtered/no-op line), **When** the benchmark runs that path, **Then** the
   reported allocation count reflects the disabled-level fast path distinctly
   from the enabled-level path.

---

### User Story 2 - Isolate the tracing-hook cost (Priority: P2)

The logger attaches a tracing hook that runs on every emitted line and injects
trace and span identifiers. A maintainer needs to know how much of the hot-path
cost comes from that hook, and specifically whether the cost changes when a
trace context is active versus absent, so future changes to trace correlation
can be evaluated against a known baseline.

**Why this priority**: The acceptance criteria explicitly require the tracing
hook to be covered. The hook behaves differently with and without an active
span (identifier formatting differs), so a single number is misleading; the
two paths must be measurable separately. This refines the P1 number rather than
standing fully alone, hence P2.

**Independent Test**: Run the benchmark variants that exercise (a) logging with
no active trace context and (b) logging with an active trace context, and
confirm each reports its own allocation profile, making the hook's marginal
cost visible.

**Acceptance Scenarios**:

1. **Given** a logger with the tracing hook active and no trace context on the
   call, **When** the benchmark emits a line, **Then** the allocation profile
   for the no-context path is reported.
2. **Given** a logger with the tracing hook active and a live trace context on
   the call, **When** the benchmark emits a line, **Then** the allocation
   profile for the active-context path is reported and is distinguishable from
   the no-context path.

---

### User Story 3 - Cover representative field-shape variations (Priority: P3)

A maintainer wants the benchmark to reflect realistic usage, not just the
emptiest possible call. Real log lines carry structured fields and sometimes
low-cardinality labels. The maintainer needs benchmark variants that cover a
bare message, a message with a few typed payload fields, and a logger
configured with labels, so the allocation profile is representative of how the
logger is actually used.

**Why this priority**: Adds fidelity and guards against an unrepresentative
"best case only" number, but the headline claim can be substantiated by P1/P2
alone. This is valuable hardening rather than the minimum viable deliverable.

**Independent Test**: Run the field-shape benchmark variants and confirm each
(bare message, message with typed fields, logger with labels) reports its own
allocation profile.

**Acceptance Scenarios**:

1. **Given** a logging call carrying several typed payload fields, **When** the
   benchmark runs, **Then** its allocation profile is reported separately from
   the bare-message path.
2. **Given** a logger constructed with low-cardinality labels, **When** the
   benchmark emits a line, **Then** its allocation profile is reported.

---

### Edge Cases

- **Disabled-level fast path**: a call at a level below the configured
  threshold should be measured as its own path; it is expected to be cheaper
  than an emitted line, and conflating it with the enabled path would hide a
  meaningful difference.
- **Active vs. absent trace context**: identifier formatting differs between
  the two; the benchmark must not average them into a single misleading figure.
- **Output sink choice**: the benchmark must direct emitted output to a discard
  sink so that the operating-system write cost and console formatting do not
  dominate or distort the measured allocation profile.
- **Claim is contradicted**: if the measurement shows the hot path allocates
  more than "near-zero," the outcome is to revise the documented claim to match
  the evidence, not to suppress or discard the measurement.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The project MUST provide a runnable benchmark suite, named with a
  `BenchmarkLogger` prefix, that exercises the logger's emit path.
- **FR-002**: The benchmark suite MUST report per-operation memory allocation
  data (bytes per operation and allocations per operation) when run with
  allocation reporting enabled.
- **FR-003**: The benchmark suite MUST cover the standard enabled-level emit
  path (the common "log one line" case) as its primary measured scenario.
- **FR-004**: The benchmark suite MUST cover the tracing hook that runs on
  every emitted line, including a variant with an active trace context and a
  variant with no trace context, reported separately.
- **FR-005**: The benchmark suite MUST cover the disabled-level (filtered) path
  separately from the enabled-level path.
- **FR-006**: The benchmark suite MUST include at least one variant carrying
  representative typed payload fields and at least one variant on a logger
  configured with low-cardinality labels.
- **FR-007**: The benchmark suite MUST direct logger output to a discard sink so
  that sink write cost does not distort the measured allocation profile.
- **FR-008**: The benchmark suite MUST be deterministic and self-contained — it
  MUST NOT require network access, external services, or environment-specific
  configuration to run.
- **FR-009**: The measured allocation profile MUST be recorded in the feature's
  documentation so the numbers and the conditions under which they were taken
  are discoverable without re-running the benchmark.
- **FR-010**: The previously unsubstantiated allocation claim MUST be
  reconciled with the measurement — either confirmed as accurate or revised to
  match the evidence — and the reconciliation MUST be recorded.
- **FR-011**: The benchmark suite MUST follow the repository's existing
  benchmark conventions (naming, structure, and per-benchmark documentation of
  what each variant substantiates) so it is consistent with the existing
  benchmark in the codebase.

### Key Entities *(include if feature involves data)*

- **Logging hot path**: the per-line emit operation a caller performs at runtime
  (acquire an event at a level, optionally attach typed fields, finalize with a
  message). This is the unit each benchmark measures.
- **Tracing hook**: the always-on per-line step that injects trace and span
  identifiers; its cost depends on whether a trace context is active.
- **Allocation profile**: the measured per-operation result set — operations per
  second, bytes per operation, allocations per operation — for a given variant.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A maintainer can obtain a per-operation allocation count and byte
  count for the standard logging path in a single command run, with no manual
  setup beyond invoking the benchmark.
- **SC-002**: The benchmark suite reports distinct allocation profiles for at
  least these paths: enabled-level emit, disabled-level (filtered),
  emit-with-active-trace-context, and emit-with-no-trace-context.
- **SC-003**: The documented allocation claim and the measured numbers agree —
  either the claim already matched the measurement, or it has been revised so
  that no unmeasured performance assertion remains.
- **SC-004**: The recorded allocation profile states the exact conditions of
  measurement (which path, field shape, and trace-context state) so a future
  reader can reproduce the same numbers.
- **SC-005**: The benchmark suite completes successfully in a standard local run
  and in continuous integration without requiring network access or external
  services.

## Assumptions

- Source inputs: GitHub issue #11 and governing ADR `docs/adr/0009-logger-zerolog.md`
  (the source of the "zero or near-zero allocation hot path" claim). The ADR's
  decisions are absorbed into `research.md` during planning and the ADR is
  retired as the feature's final task; ADR detail is intentionally kept out of
  this user-facing spec body.
- The benchmark is added as a `testing.B` suite alongside the existing
  benchmark in the repository and follows the same idiom already established
  there.
- No automated CI performance *gate* (build-failing allocation threshold) is in
  scope. The existing benchmark in the repository is not gated, and the
  acceptance criteria call for the profile to be *documented* and the claim
  *backed or revised*, not enforced by a numeric pass/fail bar. A regression
  gate, if desired, is a separate follow-up.
- The allocation profile is recorded in the feature's planning documentation
  (and reflected in per-benchmark doc comments), consistent with how the
  existing benchmark documents what it substantiates. Whether the public design
  claim lives in the ADR being retired or in another doc is resolved during
  planning.
- "Near-zero" is interpreted as a small, bounded, documented allocation count
  rather than strictly zero; the active-trace-context path is expected to incur
  identifier-formatting allocations and that is an acceptable, documented
  outcome rather than a defect to fix in this feature.
- The benchmark measures the primary single-writer emit path. The multi-sink
  fan-out path is out of scope for this feature unless trivially included.
