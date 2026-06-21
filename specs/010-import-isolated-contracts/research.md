# Research: Import-Isolated Contracts

**Feature branch**: `010-import-isolated-contracts`
**Date**: 2026-06-21

## Decision 1: Add Narrow Public Contract Packages

**Decision**: Add four public import surfaces:

- `github.com/rshade/ax-go/contract` for machine-envelope, error, exit-code, mode, metadata, and JSON writer contracts.
- `github.com/rshade/ax-go/config` for bounded Hujson reads and comment-preserving patches.
- `github.com/rshade/ax-go/schema` for ax-native and MCP-compatible schema shapes/builders.
- `github.com/rshade/ax-go/id` for UUID v4 idempotency keys and UUID v7 entity/resource IDs.

**Rationale**: These are the surfaces thin consumers need without the root runtime facade. They match existing cohesive contract areas and keep package count low.

**Alternatives considered**:

- Only `config` and `schema`: rejected because shared envelopes, mode, exit codes, and IDs would still force root imports or duplicated downstream contracts.
- Split every subsystem: rejected because `execute`, telemetry, logger/Loki, and HTTP/gRPC are runtime adapters, not thin reusable contracts.
- Keep root-only API: rejected because it reproduces the initial thin-consumer import failure.

## Decision 2: Keep Root `ax` as Compatibility Facade

**Decision**: Root `ax` symbols remain available in this phase and delegate to the new contract packages where practical. No root public symbol is deprecated or removed.

**Rationale**: This is an additive pre-v1 minor feature. Current users should not migrate under pressure, and existing golden contracts must continue to pass.

**Alternatives considered**:

- Move symbols and deprecate root immediately: rejected because the feature's goal is import isolation, not churn.
- Remove root wrappers in the same release: rejected by the stability and deprecation lifecycle.
- Maintain independent root implementations forever: rejected because it invites contract drift.

## Decision 3: Keep Contract Package Runtime-Free

**Decision**: `contract` uses only standard-library dependencies. Trace/span IDs are represented as data (`Metadata`, `ZeroTraceID`, `ZeroSpanID`, explicit options), while root `ax` remains responsible for extracting active OpenTelemetry span context.

**Rationale**: This maximizes thin-consumer value and avoids ambiguity around "telemetry-free" imports. Consumers that need the full runtime trace extraction continue to use root `ax`; consumers that only need shapes can provide metadata explicitly.

**Alternatives considered**:

- Let `contract` import OpenTelemetry trace API: rejected because even API-only telemetry dependencies weaken the import-isolation promise for the thinnest consumers.
- Drop trace fields from contract envelopes: rejected because trace fields are part of the stable machine contracts.
- Require every consumer to use root `ax` for envelopes: rejected because this is the coupling the feature is meant to remove.

## Decision 4: Config Package Returns Standard Contract Errors

**Decision**: `config` exposes bounded parse/patch operations and classifies validation failures using the shared contract error shape and existing error codes. Root `ax` preserves current trace-aware error behavior through wrapper normalization.

**Rationale**: Thin consumers need error-code semantics, while existing root consumers need active trace IDs in envelopes when spans exist.

**Alternatives considered**:

- Return only raw package-specific errors from `config`: rejected because consumers would need to recreate ax-go error classification.
- Import root `ax` from `config`: rejected because it creates an import cycle and destroys isolation.
- Make config errors unclassified: rejected because current config error codes are public contract behavior.

## Decision 5: Schema Package Owns Discoverability Shapes

**Decision**: `schema` exposes ax-native schema types, MCP-compatible adapter types, and command-tree builders. It may depend on Cobra/pflag because command reflection is inherently tied to the accepted CLI framework, but it must not depend on root execution, telemetry, logging, or transport helpers.

**Rationale**: Orchestrators need command schema data more often than full command execution. Cobra is a bounded and already accepted dependency.

**Alternatives considered**:

- Keep schema only in root: rejected because schema is one of the main thin-consumer needs.
- Move only data structs and keep builders in root: rejected because consumers would still import root to produce data.
- Remove MCP adapter from the isolated package: rejected because MCP compatibility is part of the discoverability contract.

## Decision 6: Import-Isolation Tests Are Contract Tests

**Decision**: Add tests that inspect dependency graphs for `contract`, `config`, `schema`, and `id`. Tests fail if any forbidden runtime dependency appears.

**Rationale**: Import isolation is not visible from normal behavior tests. It needs a guardrail that fails during routine test runs.

**Alternatives considered**:

- Rely on code review: rejected because accidental imports are easy to miss.
- Check only `go.mod`: rejected because transitive package imports, not module presence, are what affect consumers.
- Keep a manually maintained dependency list in documentation only: rejected because docs drift.

## Decision 7: Documentation Shows Import Choice by Use Case

**Decision**: Update README and examples to show when to import `contract`, `config`, `schema`, `id`, and when to stay on root `ax`.

**Rationale**: Multiple public packages increase choice. Documentation must make the choice fast for maintainers and agents.

**Alternatives considered**:

- Rely on godoc only: rejected because the repository README currently states the public import path remains one package and must be reconciled.
- Document only the new packages: rejected because root remains the full-runtime facade and should remain the default for full CLI integration.

## Decision Records Absorbed

### ADR-0001: Agent-Mode Trigger

**Decision absorbed**: Mode resolution precedence is `--format` flag, then `AGENT_MODE`, then TTY detection. The resolved mode is carried in context.

**Considered alternatives absorbed**: flag-only, env-only, TTY-only, and hybrid precedence.

**Consequences absorbed**: Schema output must document the mode-detection rule, and deeper code must read the resolved mode from context rather than re-detecting.

**Feature application**: Mode values, parsing, resolution, and context helpers move into the isolated `contract` surface while root `ax` continues to expose the current API.

### ADR-0002: JSON Error Envelope Schema

**Decision absorbed**: Error envelopes use the rich schema with required `error_code`, `message`, `trace_id`, `tool`, `version`, and `schema_version`, plus optional `actionable_fix`, `context`, and `suggestions`.

**Considered alternatives absorbed**: minimal `{error_code,message}`, standard six-field envelope, and rich versioned envelope.

**Consequences absorbed**: Errors are emitted to stderr, include a valid trace ID field, and evolve under schema versioning.

**Feature application**: The envelope shape moves into `contract`. Root `ax.NewError(ctx, ...)` remains trace-aware and delegates shape construction without changing existing JSON output.

### ADR-0003: `__schema` Output Format

**Decision absorbed**: Primary schema output is ax-native reflective JSON, with `__schema --as=mcp` producing MCP-compatible output.

**Considered alternatives absorbed**: ax-native only, OpenAPI, MCP-only, and ax-native plus MCP adapter.

**Consequences absorbed**: The base owns schema shapes and MCP adapter behavior; command examples remain important schema data.

**Feature application**: Schema data types and builders move into `schema`; root `ax.BuildSchema`, `ax.BuildMCPSchema`, and `ax.NewSchemaCommand` remain compatibility wrappers.

### ADR-0007: ID Strategy Across the Library

**Decision absorbed**: Idempotency keys auto-generated by ax-go use UUID v4; entity/resource IDs use UUID v7; observability IDs remain W3C trace/span IDs and must not be reused as resource IDs.

**Considered alternatives absorbed**: UUID v7 and ULID for entity IDs, plus a single UUID dependency for v4/v7.

**Consequences absorbed**: `github.com/google/uuid` remains the canonical dependency; trace IDs and resource IDs stay distinct.

**Feature application**: ID helpers move into the isolated `id` surface while root `ax.NewIdempotencyKey` and `ax.NewEntityID` remain available.

### ADR-0012: Directory Layout

**Decision absorbed**: Root `ax` remains the primary public package; `internal/` protects private mechanics; `cmd/` is reserved for real commands; no `pkg/` or `src/`.

**Considered alternatives absorbed**: root plus internal, `pkg/ax`, broad public subpackages, and all-root implementation.

**Consequences absorbed**: Directories are public API; new public subpackages require explicit governance.

**Feature application**: This Spec Kit feature supersedes the blanket "no public subpackages" stance for the narrow contract surfaces listed above. It keeps the root facade, avoids `pkg/` and `src/`, and documents/guards every new public package.
