# ADR-0003: `__schema` Output Format

## Status

ACCEPTED — 2026-05-28.

## Context

The `__schema` command is the discoverability surface: when an LLM agent
encounters an ax-go CLI it has never seen, it runs `__schema` to learn
what commands exist, what flags they accept, what types are expected, and
what the output envelope looks like.

Output format choice affects: agent grounding quality (examples help),
toolchain interoperability (MCP, OpenAPI), and implementation cost
(reflection vs. hand-curated).

## Decision Drivers

- LLMs ground on examples far better than on type signatures alone.
- User preference: CLI is the primary surface, but MCP is the de facto
  standard agents reach for — cheap MCP composability is valuable.
- Output must be machine-parseable but also human-readable for the 90%
  case of a developer running `mytool __schema | jq`.
- Cost of reflection from Cobra's command tree should be low (Cobra
  already exposes most of what is needed).

## Considered Options

### A. Custom reflective JSON tree (ax-native format)

Lightweight format defined by ax-go: commands, subcommands, flags (with
type, default, required, description), example invocations, and the
success/error envelope shape per command.

Pros: full control; can require the things LLMs need (examples) by
construction; minimal dependencies.
Cons: yet-another schema; consumers must learn it.

### B. OpenAPI 3.x

Pros: massive ecosystem of tooling; standardized; well-known to LLMs.
Cons: shaped for HTTP APIs, not CLIs — awkward fit for subcommands, flag
groups, and stdin/stdout payloads.

### C. MCP-tool schema (Model Context Protocol)

Pros: directly compatible with MCP server framing — `ax-go mcp-server`
could wrap any CLI as an MCP server with zero per-tool effort.
Cons: MCP is younger and less stable; ties output format to one upstream
spec.

### D. Option A + adapter to C (hybrid)

Primary format is ax-native (A); a companion `__schema --as=mcp` flag
emits MCP-compatible output for cheap CLI→MCP wrapping.

Pros: best of both worlds; CLI-first stance preserved while staying
MCP-composable.
Cons: two formats to maintain; adapter rot risk if MCP evolves.

## Decision

Adopt **Option D** (hybrid).

Primary format is the ax-native reflective JSON tree, with examples
required for every command. A companion `__schema --as=mcp` adapter
emits MCP-tool-compatible output so an ax-go CLI can be wrapped as an
MCP server via `ax-go mcp-server` without per-tool work.

Rationale: preserves the CLI-first stance while staying MCP-composable
for the broader agent ecosystem.

## Consequences

- The base package owns the ax-native schema and (if D) the MCP adapter.
- Every command author must supply at least one example to satisfy the
  schema's example requirement.
- Schema output includes a `schema_version` field consistent with the
  error-envelope `schema_version` (ADR-0002).
