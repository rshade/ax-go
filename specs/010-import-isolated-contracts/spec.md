# Feature Specification: Import-Isolated Contracts

**Feature Branch**: `010-import-isolated-contracts`

**Created**: 2026-06-21

**Status**: Draft

**Input**: User description: "Create import-isolated public contract packages for ax-go so thin consumers can reuse config parsing, schema output, shared machine contracts, and ID helpers without importing the root runtime package or dragging telemetry, execute, HTTP, gRPC, logger, or Loki dependencies. Keep the root ax package as the ergonomic facade, avoid removing existing root APIs in this phase, update documentation, and add import-isolation tests."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Thin Consumer Imports Contracts (Priority: P1)

A maintainer of a thin orchestrator needs to reuse ax-go's stable configuration, schema, envelope, mode, exit-code, and identifier contracts without taking on runtime orchestration or observability dependencies.

**Why this priority**: This is the motivating failure mode: importing the root package for a small contract need makes thin binaries heavier and couples them to unrelated runtime behavior.

**Independent Test**: Can be fully tested by creating a minimal consumer that uses only the public contract surfaces and verifying it compiles and resolves no forbidden runtime dependencies.

**Acceptance Scenarios**:

1. **Given** a thin consumer that only needs config parsing and schema data, **When** it imports the relevant ax-go contract surfaces, **Then** it can build without importing telemetry, execute, HTTP, gRPC, logger, or Loki runtime adapters.
2. **Given** a thin consumer that only needs shared envelopes, exit codes, mode resolution, or ID helpers, **When** it imports the relevant ax-go contract surfaces, **Then** it receives the same public contract semantics as the root ax package.

---

### User Story 2 - Existing Root Package Users Remain Compatible (Priority: P2)

An existing CLI maintainer using the root ax package needs their current imports, behavior, and machine-readable output contracts to continue working during this modularization release.

**Why this priority**: The feature should reduce coupling for new thin consumers without forcing immediate migration or creating avoidable churn for existing users.

**Independent Test**: Can be fully tested by running the existing public examples and contract tests against the root ax package and confirming output shapes remain unchanged except for documented additive behavior.

**Acceptance Scenarios**:

1. **Given** an existing CLI using root-package config, schema, envelope, error, mode, ID, execute, logger, telemetry, or transport helpers, **When** it updates to this release, **Then** the existing public symbols remain available with the same documented behavior.
2. **Given** existing golden files for error envelopes and schema output, **When** the suite is run after modularization, **Then** those machine contracts remain stable unless the change is explicitly additive and documented.

---

### User Story 3 - Maintainers Can Enforce Package Boundaries (Priority: P3)

An ax-go maintainer needs repeatable checks and documentation that make the new boundaries clear and prevent future changes from accidentally pulling runtime dependencies back into thin contract surfaces.

**Why this priority**: Import isolation is only valuable if it is continuously enforced and easy for future contributors to understand.

**Independent Test**: Can be fully tested by running boundary checks for each contract surface and reviewing documentation that explains when to use the root package versus the isolated surfaces.

**Acceptance Scenarios**:

1. **Given** any public contract surface, **When** its dependency boundary is inspected, **Then** forbidden runtime adapters are absent.
2. **Given** a maintainer deciding what to import, **When** they read the documentation, **Then** they can identify the correct public surface for a thin consumer and the root package for full CLI integration.

### Edge Cases

- A contract surface shares names or behavior with existing root-package symbols; the root behavior must remain the compatibility source for existing users.
- A future change accidentally imports telemetry, logger, Loki, HTTP, gRPC, or execute behavior into a contract surface; boundary checks must fail.
- A consumer needs full command execution, telemetry, or logging behavior; documentation must direct that consumer to the root package instead of the isolated contract surfaces.
- A machine-contract shape needs to evolve while this feature is in progress; the change must follow the additive-tolerant stability rules and remain covered by golden tests.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide independently importable public contract surfaces for bounded configuration reads and patches, schema and MCP-compatible schema data, shared success and error envelopes, mode and exit-code contracts, metadata propagation, and idempotency/resource ID helpers.
- **FR-002**: Each public contract surface MUST be usable by thin consumers without importing runtime orchestration, telemetry export, HTTP transport instrumentation, gRPC transport instrumentation, logger, or Loki behavior.
- **FR-003**: Existing root-package public symbols for the covered contracts MUST remain available in this phase with unchanged documented behavior.
- **FR-004**: Existing root-package machine-readable output shapes MUST remain stable, with only additive changes allowed unless a later feature explicitly schedules a breaking minor release.
- **FR-005**: The system MUST include automated boundary checks that fail when any contract surface imports a forbidden runtime adapter.
- **FR-006**: Documentation MUST explain when consumers should use isolated contract surfaces and when they should use the root package.
- **FR-007**: Examples or equivalent verified usage MUST demonstrate the primary contract surfaces in isolation from the root runtime package.
- **FR-008**: The feature MUST avoid public deprecations or removals in this phase; any future cleanup must follow the published deprecation lifecycle.
- **FR-009**: Importing a contract surface MUST NOT initialize observability, start goroutines, perform network work, persist state, read environment configuration for runtime behavior, or write to stdout or stderr.
- **FR-010**: The feature MUST NOT add domain commands, application orchestration, persistent memory, authentication flows, or natural-language intent handling.

### Key Entities

- **Contract Surface**: A public, narrowly scoped group of ax-go contracts that can be imported independently by consumers that do not need full CLI runtime behavior.
- **Thin Consumer**: A downstream project or binary that needs stable ax-go contracts but not root-package execution, telemetry, logging, or transport adapters.
- **Root Facade**: The existing root ax package surface that remains the ergonomic entry point for full CLI integration and compatibility with current users.
- **Boundary Check**: A repeatable verification that a contract surface does not depend on forbidden runtime adapters.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A minimal thin consumer can use the isolated configuration and schema contract surfaces and complete a successful build without any forbidden runtime adapter dependencies.
- **SC-002**: A minimal thin consumer can use the shared envelope, mode, exit-code, metadata, and ID contracts and complete a successful build without importing the root runtime package.
- **SC-003**: Boundary checks cover 100% of the newly public contract surfaces and fail when a forbidden runtime adapter dependency is introduced.
- **SC-004**: Existing root-package public contract tests and golden files continue to pass after the feature lands.
- **SC-005**: Documentation lets a maintainer determine the correct import surface for the three primary use cases--thin config/schema consumer, thin machine-contract consumer, and full CLI runtime consumer--in under five minutes.

## Assumptions

- Source inputs: current maintainer discussion and governing ADRs `docs/adr/0001-agent-mode-trigger.md`, `docs/adr/0002-error-envelope-schema.md`, `docs/adr/0003-schema-output-format.md`, `docs/adr/0007-id-strategy.md`, and `docs/adr/0012-directory-layout.md`. Any governing ADR decisions are absorbed into `research.md` during planning and retired as final tasks where this feature supersedes them.
- The first downstream driver is a thin orchestrator that needs ax-go config, schema, and machine-contract reuse without the full CLI runtime surface.
- Existing root-package API compatibility is required for this phase; removals and deprecations are intentionally out of scope.
- The new public contract surfaces are governed by the repository's pre-v1 stability policy rather than treated as private internals.
- Final naming of public contract surfaces is confirmed during planning, but the user-visible categories listed in FR-001 are in scope.
