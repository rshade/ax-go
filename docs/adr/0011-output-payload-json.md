# ADR-0011: Output Payload Format — Strict JSON (and NDJSON for streams)

## Status

ACCEPTED — 2026-05-28.

## Context

Per the Golden Rule (GEMINI.md, ADR-0002), `stdout` carries data
payloads. Agents and pipeline tools (jq, fx, gron) need a
deterministic, parseable format. Mixing the Hujson input format into
output would force every downstream parser to handle a non-standard
format.

A second wrinkle: for `list`-style commands with unbounded result
sets, emitting a single monolithic JSON array forces the agent to
buffer the entire output before parsing. NDJSON (one JSON object per
line) lets agents process results incrementally — which matters when
the result set is large or when latency-to-first-result matters more
than total throughput.

## Decision Drivers

- Agents, jq, and standard pipeline tools parse strict JSON natively.
- Streaming results from list/search commands shouldn't force buffering.
- One serialization library by default; benchmark before swapping.
- Predictable output shape per command — declared in the schema
  (ADR-0003).

## Considered Options

### A. Strict minified JSON envelope (single-shot)

For commands that return a bounded result (a single record, a small
set, a status).

Pros: universal; agents and jq handle natively.
Cons: forces buffering for large result sets.

### B. NDJSON / JSON Lines for streaming

One JSON object per line on `stdout`. Agents and `jq -c` consume
incrementally.

Pros: streaming-friendly; trivial to chunk; standard format.
Cons: not a single self-describing document; downstream needs to know
"this is NDJSON" up front.

### C. Pretty-printed JSON

For humans only — use `--format=human` (ADR-0001) for that.

### D. YAML / TOML output

Non-standard for agents; rejected.

### E. Protocol Buffers / FlatBuffers

Agent-unfriendly without schema distribution; rejected for the CLI
output surface.

## Decision

Both **Option A** and **Option B**, command-declared:

- Single-shot results: strict minified JSON envelope on `stdout`.
- Unbounded / large result sets: NDJSON (one JSON object per line).
- Each command declares its output mode in `__schema` (ADR-0003).

Default serializer is `encoding/json` from the standard library.
Benchmark against `github.com/bytedance/sonic` or
`github.com/goccy/go-json` only when a specific hot path warrants
swapping — do not pre-optimize.

## Consequences

- Use `encoding/json` for output by default; alternatives are
  benchmark-justified swaps, not blanket replacements.
- Commands documenting NDJSON output MUST include a streaming example
  in `__schema` so agents know to parse line-by-line.
- The error envelope (ADR-0002) emits to `stderr` and is always strict
  JSON, regardless of whether the success path uses NDJSON.
- A `--pretty` flag (or `--format=human`) is the only acceptable path
  to pretty-printed output; non-default by design.
