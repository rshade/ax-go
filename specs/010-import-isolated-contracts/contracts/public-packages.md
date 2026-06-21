# Contract: Public Package Surfaces

**Feature branch**: `010-import-isolated-contracts`
**Audience**: downstream Go consumers and ax-go maintainers

## `github.com/rshade/ax-go/contract`

Owns shared machine contracts that do not require runtime CLI execution.

**Must expose**

- Deterministic exit-code constants.
- `Mode`, mode parsing, mode resolution, and context helpers for resolved mode.
- Dry-run and idempotency-key context helpers.
- `Metadata` and `Envelope[T]` success payload shapes.
- Strict JSON and NDJSON line writers.
- `Error`, error options, schema version, JSON error writer, and error-to-exit-code mapping.
- `ZeroTraceID` and `ZeroSpanID` constants for valid no-active-span metadata.

**Must not expose**

- Runtime command execution.
- OpenTelemetry extraction, exporters, SDK setup, or shutdown.
- Logger, Loki, HTTP, or gRPC helpers.

## `github.com/rshade/ax-go/config`

Owns bounded Hujson reads and comment-preserving patch behavior.

**Must expose**

- Default and ceiling byte-limit constants.
- Parse and parse-file operations.
- Patch and patch-file operations.
- Per-call max-byte option.
- Standard error classifications compatible with existing config error codes.

**Compatibility requirement**

Root `ax.ParseConfig`, `ax.ParseConfigFile`, `ax.PatchConfig`, `ax.PatchConfigFile`, `ax.WithMaxConfigBytes`, and related constants remain available with the same documented behavior.

## `github.com/rshade/ax-go/schema`

Owns machine discoverability schema shapes and builders.

**Must expose**

- Ax-native schema shape.
- Error-envelope schema metadata shape.
- Command and flag schema shapes.
- MCP-compatible schema shape.
- Builders for ax-native and MCP-compatible schema output.
- Reserved schema command builder compatible with current root behavior.

**Compatibility requirement**

Root `ax.Schema`, `ax.CommandSchema`, `ax.FlagSchema`, `ax.BuildSchema`, `ax.BuildMCPSchema`, `ax.NewSchemaCommand`, and schema options remain available with the same documented behavior.

## `github.com/rshade/ax-go/id`

Owns non-observability identifier helpers.

**Must expose**

- UUID v4 idempotency-key generation.
- UUID v7 entity/resource ID generation.

**Must not expose**

- W3C trace/span ID helpers.
- Any function that encourages using trace IDs as entity IDs or entity IDs as trace IDs.

**Compatibility requirement**

Root `ax.NewIdempotencyKey` and `ax.NewEntityID` remain available with the same documented behavior.

## Root `github.com/rshade/ax-go`

Remains the primary full-runtime facade.

**Continues to own**

- `Execute` and Cobra execution integration.
- Root span creation and telemetry shutdown behavior.
- Logger/Loki integration.
- HTTP/gRPC instrumented client helpers.
- Trace/span extraction from active OpenTelemetry context.

**Compatibility requirement**

Existing root API users should not be forced to change imports in this feature.
