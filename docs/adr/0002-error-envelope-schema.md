# ADR-0002: JSON Error Envelope Schema

## Status

ACCEPTED — 2026-05-28.

## Context

When a command fails, the agent needs a predictable shape to parse and
act on. Today, every Go CLI invents its own error format. ax-go's value
comes from making this uniform across all tools that adopt the base.

The envelope is emitted to `stderr` (per the Golden Rule), not `stdout`.
`stdout` either emits a success payload or nothing.

## Decision Drivers

- Agents must be able to programmatically distinguish error categories
  without parsing prose.
- Errors must correlate to the trace context for downstream investigation.
- The envelope must be extensible so individual tools can add
  domain-specific context without forking the base.
- Schema versioning is needed so consumers can evolve safely.

## Considered Options

### A. Minimal — `{error_code, message}`

Pros: smallest possible surface.
Cons: no correlation to traces; no recovery hints; consumers re-invent
extensions ad hoc.

### B. Standard — `{error_code, message, actionable_fix, trace_id, tool, version}`

Pros: covers the common needs (categorization, recovery, correlation,
provenance) in six well-known fields.
Cons: slightly more verbose; `actionable_fix` is best-effort.

### C. Rich — Standard + `{schema_version, context: {...}, suggestions: [...]}`

Pros: maximally useful; `schema_version` supports evolution; `context`
holds tool-specific fields without polluting the top level.
Cons: largest payload; risk of agents misusing `suggestions` as ground
truth.

## Decision

Adopt **Option C** (rich envelope) as the canonical error format.

Required fields: `error_code`, `message`, `trace_id`, `tool`, `version`,
`schema_version`.

Optional fields: `actionable_fix`, `context`, `suggestions`.

`schema_version` follows SemVer; major bumps signal breaking shape
changes and require a superseding ADR.

## Consequences

- All adopting tools emit a consistent error shape to `stderr`.
- Agents can build a single error-handling path that works across every
  ax-go-built CLI.
- `trace_id` must be wired even when OTel is in no-op mode (use a
  zero-value valid hex string so consumer parsers do not break).
- The base package owns the JSON Schema document; consumers reference it
  rather than re-deriving.
- Changes to required fields require a `schema_version` major bump and a
  superseding ADR.
