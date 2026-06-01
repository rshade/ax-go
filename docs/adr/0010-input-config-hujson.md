# ADR-0010: Input Config Format — Hujson

## Status

ACCEPTED — 2026-05-28.

## Context

ax-go CLIs accept human-written configuration files. Strict JSON is
hostile to humans (no comments, no trailing commas, error messages
that don't surface intent). YAML and TOML introduce different mental
models. Hujson ("Human JSON", from Tailscale) is JSON with comments
and trailing commas added — minimal mental overhead from JSON.

LLM agents also benefit: they routinely emit trailing commas and
comments when generating config files, and Hujson tolerates that
without errors.

## Decision Drivers

- Humans write config files; tolerance for comments and trailing
  commas reduces friction.
- LLM agents generating config files often slip in trailing commas;
  Hujson absorbs that without parse errors.
- Stay close to JSON so the broader tooling ecosystem (jq, JSON
  Schema, IDE support) works.
- A single, well-maintained Go parser (`tailscale/hujson`).

## Considered Options

### A. `tailscale/hujson`

JSON with comments and trailing commas. Parses to a standard AST,
then `Standardize()` strips extensions and produces strict JSON that
`encoding/json.Unmarshal` can consume.

Pros: tiny diff from JSON mental model; well-maintained; AST patching
for in-place edits preserving comments.
Cons: cannot Marshal Go structs back to Hujson with comments — reads
are Hujson; writes are strict JSON.

### B. Strict `encoding/json`

Pros: stdlib, zero dependencies.
Cons: human-hostile (no comments, no trailing commas).

### C. YAML (`gopkg.in/yaml.v3`)

Pros: widely used; comment-friendly.
Cons: separate mental model from the JSON output (ADR-0011);
whitespace sensitivity bites both humans and LLMs; richer feature
surface than needed.

### D. TOML (`BurntSushi/toml`)

Pros: human-readable; comment-friendly.
Cons: flat-friendly schema; deeply nested config awkward; another
mental model.

### E. Starlark / Pkl / Jsonnet

Pros: expressive.
Cons: huge overkill for config; agents handle them poorly compared
to JSON-shaped formats.

## Decision

Adopt **Option A** — `github.com/tailscale/hujson` for human-facing
input configuration.

Read path: bounded read (default 1 MiB, configurable with
`WithMaxConfigBytes`) → `Parse` Hujson → `Standardize` to strict JSON →
`encoding/json.Unmarshal` to Go structs. Oversized configs fail as
validation errors.

Write path (when a tool needs to update a config file): if the file
exists, use Hujson's AST `Patch` to preserve user comments and
formatting; if creating from scratch, emit strict JSON (Hujson can't
Marshal with comments).

## Consequences

- Direct dependency on `github.com/tailscale/hujson`.
- `ax.ParseConfig(path, dst)` is the canonical helper in the base.
- Config reads are bounded at the read boundary so user-provided config cannot
  trigger unbounded memory growth.
- The read-Hujson / write-JSON asymmetry must be documented in the
  user-facing config guide so consumers aren't surprised when a CLI
  rewrites their config and strips comments.
- For tools that frequently mutate user config, prefer AST `Patch`
  over Marshal+Write to preserve formatting.
