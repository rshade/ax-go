# Feature Specification: Independently Opt-Out OTLP Export and gRPC Dial Adapter

**Feature Branch**: `016-optional-grpc-otlp`

**Created**: 2026-07-22

**Status**: Draft

**Input**: GitHub issue #143 — "feat(telemetry): make OTLP export and the gRPC dial adapter independently opt-out so root-facade consumers can drop the gRPC/protobuf tree"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ship a slim binary without giving up tracing (Priority: P1)

A maintainer of a long-running daemon distributes cross-compiled desktop binaries. They adopted ax-go for its tracing API, trace-correlated logging, and W3C context propagation, and they use the root facade because that is the only place live tracing exists. Today their shipped artifact carries an entire gRPC/protobuf/gateway dependency tree they never call. They want to declare, at build time, that they decline both trace export and the outbound gRPC dial helper — and get a dramatically smaller artifact that still produces trace-correlated logs and still honours inbound trace context.

**Why this priority**: This is the entire value of the feature. Every other story exists to make this one safe or to keep it from rotting. The measured payoff — a ~63% reduction in stripped binary size — only materialises when both declines are applied together, so this story is indivisible.

**Independent Test**: Build a fixture consumer that calls the facade's execution entry point, logger constructor, and schema command, with both declines applied. Verify the resulting artifact links zero packages from the gRPC, protobuf, OTLP-proto, and gRPC-gateway trees; verify it is materially smaller than the default build; and verify that inbound W3C trace context is still extracted, a recording span still wraps execution, and log lines still carry trace and span identifiers.

**Acceptance Scenarios**:

1. **Given** a consumer of the root facade, **When** it builds with both the export decline and the gRPC-adapter decline applied, **Then** the resulting dependency set contains zero gRPC, protobuf, OTLP-proto, and gRPC-gateway packages.
2. **Given** that same build, **When** the program runs with inbound W3C trace context supplied by its caller, **Then** the context is extracted, a recording root span wraps the command, and every log line emitted inside that span carries the correlated trace and span identifiers.
3. **Given** that same build, **When** a trace-export endpoint is configured in the environment, **Then** the command still succeeds with its normal exit code and emits exactly one diagnostic on the error stream noting that export is unavailable in this build.
4. **Given** that same build, **When** the machine payload is inspected, **Then** the schema description and the error envelope are byte-identical to the default build's.

---

### User Story 2 - Existing consumers observe no change whatsoever (Priority: P2)

Every consumer that does not opt out must be completely unaffected: same exported API, same tracing behaviour, same machine payloads, same export path. Adopting this feature must not force anyone to change a line of code, take a breaking release, or re-verify their observability pipeline.

**Why this priority**: The feature is only acceptable if it is purely additive. A default build whose exported surface shrank would be a breaking change under the project's stability policy and would force a breaking release — which is explicitly not wanted here.

**Independent Test**: With no declines applied, run the full existing test suite, the public-API difference gate, and the golden-file comparisons. All must pass unchanged and without any breaking-change approval.

**Acceptance Scenarios**:

1. **Given** no declines are applied, **When** the public API difference gate runs against the base branch, **Then** it reports no incompatible change and requires no breaking-change approval.
2. **Given** no declines are applied, **When** trace export, debug span output, W3C context extraction, root-span creation, and log trace-correlation are exercised, **Then** each behaves exactly as it does before this feature.
3. **Given** no declines are applied, **When** the schema and error-envelope golden files are compared, **Then** they are byte-identical to the pre-feature goldens.

---

### User Story 3 - Each decline is independently selectable (Priority: P3)

A consumer may want to decline only trace export (they scrape logs instead) or only the gRPC dial helper (they use a hand-rolled gRPC client). Both single declines, the combination, and the default must all be valid, buildable configurations.

**Why this priority**: Independent selection is a correctness and design-hygiene requirement rather than a size win — the measurements show each decline alone recovers almost nothing. It matters because coupling the two knobs would make the feature a single blunt switch and would tie an outbound-transport helper to an observability-export decision that has nothing to do with it.

**Independent Test**: Build the fixture consumer in all four configurations (neither decline, export decline only, adapter decline only, both) and confirm each compiles and runs successfully.

**Acceptance Scenarios**:

1. **Given** only the export decline, **When** the consumer builds, **Then** it compiles successfully and the outbound gRPC dial helper remains available.
2. **Given** only the gRPC-adapter decline, **When** the consumer builds, **Then** it compiles successfully and trace export to a configured endpoint still works.
3. **Given** any of the four configurations, **When** the consumer runs, **Then** stream separation, exit-code mapping, dry-run suppression, and idempotency-key surfacing behave identically.

---

### User Story 4 - The reduced configurations cannot silently rot (Priority: P3)

A configuration that nothing builds and nothing checks will break within a release or two — most likely by an unrelated change reintroducing a forbidden dependency through a new import. The reduced configurations need the same automated enforcement the project already applies to its thin contract packages.

**Why this priority**: Prevents regression rather than delivering new capability, but without it the guarantee in User Story 1 has a short shelf life.

**Independent Test**: Introduce a change that reintroduces a forbidden dependency into a declined configuration and confirm continuous integration fails with a message naming the offending dependency and the configuration it appeared in.

**Acceptance Scenarios**:

1. **Given** a change that reintroduces a gRPC, protobuf, OTLP-proto, or gRPC-gateway dependency under both declines, **When** the automated checks run, **Then** they fail and name the offending dependency.
2. **Given** any of the four configurations across every supported operating-system/architecture profile, **When** the cross-compilation checks run, **Then** every combination builds successfully.
3. **Given** a change that adds, removes, or alters an exported identifier in any single configuration, **When** the surface inventory gate runs against its committed baseline, **Then** it fails and names both the identifier and the configuration it changed in.
4. **Given** an unchanged commit, **When** the surface inventory gate is run locally and in continuous integration, **Then** both produce identical results.

---

### Edge Cases

- **Export endpoint configured but export declined**: The command must succeed, not error. Exactly one diagnostic is emitted on the error stream, and the tracing provider returned is still a recording one so log correlation survives. This preserves the existing fail-open contract — a misconfigured or unreachable observability backend never fails a command.
- **Debug span output requested while export is declined**: Local debug span emission is independent of network export and must continue to work in every configuration.
- **Repeated command invocations with export declined**: The diagnostic is emitted once per process, not once per span or once per command, matching the existing one-time diagnostic behaviour.
- **A declined configuration used with an execution path that would have dialled gRPC**: The outbound gRPC dial helper is not part of that build; a consumer that calls it while declining it gets a build-time failure, not a runtime surprise. The declined build additionally carries a discoverable explanation of the absence so the developer is not left with only a bare "undefined" message to interpret.
- **The declined configuration's exported surface drifts unnoticed**: An identifier present only in the default configuration, or newly absent from a declined one, is caught by the surface inventory gate against its committed baseline rather than discovered by a consumer whose build broke.
- **Shutdown and flush semantics under a declined export**: Shutdown must still complete within the existing budget and must not block waiting on an exporter that does not exist.
- **A new dependency added later that transitively reintroduces the forbidden trees**: Caught by the automated dependency-boundary check, not discovered by a consumer measuring their binary.

## Requirements *(mandatory)*

### Functional Requirements

#### Opt-out mechanism

- **FR-001**: The system MUST provide a build-time way for a consumer to decline trace export, and a separate build-time way to decline the instrumented outbound gRPC dial helper.
- **FR-002**: The two declines MUST be independently selectable and MUST also be valid in combination, yielding exactly four supported configurations.
- **FR-003**: Both declines MUST default to *off* — that is, a consumer who does nothing gets today's full behaviour, and the reduced configuration is always an explicit consumer choice.
- **FR-004**: Declining MUST NOT require the consumer to change source code, adopt a different import path, or migrate away from any exported identifier they use today.

#### Default-build preservation

- **FR-005**: With no declines applied, the set of exported identifiers in the public packages MUST be byte-identical to the pre-feature surface, requiring no breaking-change approval and no breaking release.
- **FR-006**: With no declines applied, trace export, debug span output, W3C trace-context extraction, root-span creation around command execution, and trace/span correlation on log lines MUST behave exactly as they do today.
- **FR-007**: With no declines applied, the schema description output and the error-envelope output MUST be byte-identical to their existing golden fixtures.

#### Behaviour under the declines

- **FR-008**: Under the export decline, a configured trace-export endpoint MUST NOT cause an error. The system MUST emit exactly one diagnostic on the error stream **per telemetry start** indicating export is unavailable, and the command MUST complete with the exit code it would otherwise have produced. Command execution starts telemetry once, so for every real invocation this is one diagnostic per process. *(Precision fix — see research.md D4: the existing once-guard is per-start instance state; a process-global guard would require mutable package-level state, which the constitution forbids.)*
- **FR-009**: Under any combination of declines, telemetry start-up MUST still return a usable recording tracer provider, so tracing degrades to *no export* and never to *no tracing*.
- **FR-010**: Under any combination of declines, W3C trace-context extraction and propagation, root-span creation, and trace/span correlation on log lines MUST behave identically to the default build.
- **FR-011**: Under any combination of declines, local debug span output MUST remain available.
- **FR-012**: Under any combination of declines, the machine payloads — schema description and error envelope — MUST be byte-identical to the default build's, and stream separation, exit-code mapping, dry-run suppression, and idempotency-key behaviour MUST be unchanged.
- **FR-013**: The instrumented HTTP client helpers MUST remain available and unchanged in every configuration; they are not gated by either decline.
- **FR-014**: Telemetry shutdown MUST complete within the existing shutdown budget in every configuration and MUST NOT wait on an exporter that is not present.

#### Enforcement

- **FR-015**: With both declines applied, a root-facade consumer's resolved dependency set MUST contain zero packages from the gRPC, protobuf, OTLP-protocol, and gRPC-gateway dependency trees.
- **FR-016**: The guarantee in FR-015 MUST be enforced automatically in continuous integration, reusing the project's existing forbidden-import assertion mechanism rather than introducing a parallel one.
- **FR-017**: All four configurations MUST build successfully across every operating-system/architecture profile already covered by the cross-compilation checks.
- **FR-018**: The project MUST gain a public-surface inventory gate that enumerates the exported surface of the public packages across all four configurations and every operating-system/architecture profile, compares the result against a committed baseline, and fails when the surface drifts from that baseline.
- **FR-019**: The surface inventory gate MUST record, per exported identifier, which configurations it is present in — so an identifier that exists only in the default configuration is visibly and deliberately recorded as such rather than silently unexamined.
- **FR-020**: The surface inventory gate MUST be runnable locally through the same entry point continuous integration uses, so a contributor can reproduce a failure without pushing.
- **FR-021**: Behaviour-parity assertions covering W3C extraction, root-span creation, and log trace/span correlation MUST execute under every configuration, not only the default one.

#### Declined-identifier discoverability

- **FR-022**: Under the gRPC-adapter decline, the outbound dial helper MUST be absent from the build, so that a consumer calling it fails at build time rather than at run time.
- **FR-023**: The declined configuration MUST carry a discoverable, documented explanation of the identifier's absence — reachable by a developer inspecting the package rather than only by reading release notes — naming the decline responsible and what to do to restore the identifier.
- **FR-024**: The mechanism satisfying FR-023 MUST NOT reference any type from the forbidden dependency trees, so it cannot reintroduce the dependencies the decline exists to remove.

#### Documentation

- **FR-025**: The public documentation MUST describe both declines, what each removes, and the fact that the size benefit only materialises when both are applied.
- **FR-026**: The public documentation MUST state plainly that the thin contract packages provide **no live tracing** — their trace-identifier accessor reads a value previously stored in the context and does not resolve an active span — so readers do not mistake them for a tracing-capable escape hatch.
- **FR-027**: The runnable integration example MUST include a verified build under the declined configuration.
- **FR-028**: The measured binary-size reduction MUST be recorded during planning, together with a portable command that reproduces the measurement.

### Key Entities

- **Configuration**: One of four build-time states of an ax-go consumer, formed by the independent presence or absence of the export decline and the gRPC-adapter decline. Each has a defined exported surface, a defined dependency set, and a defined runtime behaviour — and all four are supported.
- **Forbidden dependency tree**: The four package families that must be absent under both declines — gRPC, protobuf, the OTLP protocol definitions, and the gRPC gateway runtime. These are the unit the automated boundary check asserts against.
- **Tracing capability tier**: The distinction the documentation must make explicit — thin contract packages carry *no live tracing*; the root facade carries live tracing; the root facade under the export decline carries live tracing *without* network export.
- **Surface baseline**: The committed record of which exported identifiers exist in which configuration, across every operating-system/architecture profile. It is the artifact the surface inventory gate compares against, and the place where "this identifier exists only in the default configuration" is stated deliberately rather than discovered accidentally.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A root-facade consumer applying both declines produces a stripped artifact at least 55% smaller than the same consumer built with defaults, on at least two distinct operating-system/architecture profiles. *(Reference measurement from issue #143: 63.3% on linux/amd64 and 62.4% on windows/amd64.)*
- **SC-002**: A root-facade consumer applying both declines resolves to zero packages from each of the four forbidden dependency trees — an exact count of zero, not a reduction.
- **SC-003**: A root-facade consumer applying both declines still produces trace-correlated log output: 100% of log lines emitted inside an active span carry both a trace identifier and a span identifier, matching the default build.
- **SC-004**: The default configuration passes the public-API difference gate with zero incompatible changes and without any breaking-change approval.
- **SC-005**: The schema description and error-envelope outputs are byte-identical across all four configurations and identical to the pre-feature golden fixtures.
- **SC-006**: With export declined and an export endpoint configured, the command exits with its normal exit code and emits exactly one diagnostic per telemetry start — verified by counting diagnostics emitted across a command execution, and by asserting repeated telemetry starts each emit exactly one.
- **SC-007**: All four configurations build successfully across 100% of the operating-system/architecture profiles in the cross-compilation matrix.
- **SC-008**: A deliberately introduced forbidden dependency under the declined configuration causes continuous integration to fail with a message naming the offending dependency.
- **SC-009**: The surface inventory gate enumerates 100% of the exported identifiers of the public packages across all four configurations and every operating-system/architecture profile, and a deliberately introduced surface change in any single configuration causes it to fail, naming the identifier and the configuration.
- **SC-010**: The surface inventory gate produces identical results when run locally and in continuous integration, on the same commit.
- **SC-011**: A developer who calls the declined outbound dial helper can determine the cause — which decline removed it and how to restore it — from the package's own documentation, without consulting release notes or the issue tracker.
- **SC-012**: Test coverage meets the project's existing per-package and repository-wide floors, with no floor lowered to accommodate this feature, and the new surface inventory gate carries its own coverage floor.

## Assumptions

- **Source inputs**: GitHub issue #143. No governing ADR — the ADR log is frozen and this feature routes through the Spec Kit workflow. Prior related features: `specs/004-real-otel-export/` (delivered the export path this makes declinable) and `specs/010-import-isolated-contracts/` (solved the thin-consumer case this leaves open).
- **Verified against the repository at `741a8d4`**: a root-facade consumer resolves 410 total packages, of which 68 are gRPC, 36 are protobuf, 4 are OTLP-proto, and 3 are gRPC-gateway — matching the issue's figures and the reproduction reference in `quickstart.md`. (The bare `ax` package alone resolves 409, differing by the consumer's own `main` package; all four artifacts quote the consumer measurement.) There are currently **zero** `//go:build` lines anywhere in the repository, production or test.
- **Discrepancy — surface-gate tooling does not exist**: Issue #143 describes *extending* `internal/cmd/surfacecheck/inventory.go` and `baseline.json`, and a `make surface-check` target. None of these exist in this repository — `internal/cmd/` contains only `apidiff-verdict`, `benchcheck`, `covercheck`, and `doccover`, and the `Makefile` has no `surface-check` target. **Resolved (see Resolved Clarifications):** this feature builds that gate rather than extending one. FR-018 through FR-020 are therefore net-new deliverables, and the feature's effort estimate should account for a new continuous-integration gate alongside the new build mechanism.
- **The tracing SDK stays unconditional.** It is part of the public surface and contributes zero gRPC packages, so it is never gated.
- **The instrumented HTTP client stays unconditional.** Its instrumentation contributes zero gRPC packages, so gating it would cost API surface for no benefit.
- **No first-party consumer depends on the outbound gRPC dial helper.** The issue reports that no repository across the portfolio imports it. It nonetheless remains available by default; only an explicit decline removes it.
- **Both declines land together.** Because each alone recovers a rounding error, a phased delivery shipping one first would measure as a failure. If the work is staged, the first stage is explicitly framed as preparation with no expected size win.
- **Reduced-configuration test execution**: it is assumed the existing test suites for the affected packages run under the declined configurations, not merely compile.
- **The number of independent declines is exactly two.** No third knob is introduced, and the mechanism is wired into the project's existing gates rather than standing up new ones.

## Out of Scope

- Reverting, weakening, or making optional the default-on tracing and span-lifecycle behaviour delivered by the earlier export feature.
- Making the tracing SDK itself optional.
- Making the instrumented HTTP client optional.
- Adding a gRPC-transport trace exporter — it remains rejected.
- Any runtime-selectable exporter registry or pluggable-backend surface; backend selection at runtime is ruled out by the project's governance.
- Moving live tracing into the thin contract packages, which would breach the import-isolation guarantee those packages exist to provide.
- Trimming the structured-logging, CLI-framework, or MCP dependencies.

## Resolved Clarifications

### Question 1: Scope of the surface-gate requirement — **RESOLVED 2026-07-22**

**Question**: Issue #143 directs this feature to extend `internal/cmd/surfacecheck/inventory.go`, `baseline.json`, and a `make surface-check` target — none of which exist in this repository. What should the feature deliver?

**Decision**: **Build the full gate.** This feature creates a real public-surface inventory gate covering all four configurations across every operating-system/architecture profile, with a committed baseline and a local entry point matching the one continuous integration uses.

**Rationale**: The declines change the exported surface of a declined build. Without an inventory that understands configurations, that surface is examined by nothing — the public-API difference gate only ever sees the default configuration. Deferring the gate would ship the size win alongside a permanent blind spot in exactly the area the feature disturbs.

**Consequences**: This is net-new tooling, not an extension, and it materially raises the feature's effort. Encoded as FR-018 through FR-020, SC-009, SC-010, and the coverage clause in SC-012. Planning should treat the gate as a distinct workstream that can proceed in parallel with the opt-out mechanism.

---

### Question 2: Fate of the outbound gRPC dial helper under its decline — **RESOLVED 2026-07-22**

**Question**: When a consumer applies the gRPC-adapter decline but still calls the outbound dial helper, what should happen?

**Decision**: **Absent, with a guiding explanation.** The identifier is removed from the declined build so the failure occurs at build time, and the declined build additionally carries a discoverable, documented explanation naming the decline responsible and how to restore the identifier.

**Rationale**: Absence alone is idiomatic but yields only a bare "undefined" message, which does not tell a developer *why* the identifier vanished — particularly a developer who inherited the build configuration rather than choosing it. Keeping the identifier present with a runtime failure was rejected outright: retaining its signature retains the gRPC types it references, and therefore the entire dependency tree the decline exists to remove.

**Consequences**: Encoded as FR-022 through FR-024 and SC-011. FR-024 is the important constraint — whatever mechanism carries the explanation must not itself reference the forbidden dependency trees. Planning should confirm how much of the guidance can surface at build time versus in package documentation, and should not promise a customised build-time diagnostic the toolchain cannot actually produce.
