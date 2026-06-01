# ADR-0001: Agent-Mode Trigger

## Status

ACCEPTED — 2026-05-28.

## Context

ax-go CLIs serve two audiences with different output expectations:

- **Humans** running interactively: prefer pretty-formatted output,
  colors, progress indicators.
- **LLM agents / orchestrators** running programmatically: require
  deterministic, parseable JSON envelopes; ANSI color codes break parsing.

The base package must decide which mode is active at startup. The
mechanism affects both reliability (agents need it to be unambiguous) and
ergonomics (humans should not have to opt in every invocation).

## Decision Drivers

- Agents need deterministic mode selection — no surprises.
- Humans should not have to pass flags on every invocation.
- Override paths must exist for edge cases (CI logs, interactive debug of
  an agent flow, etc.).
- Detection must be cheap (sub-millisecond).
- Behavior must be unambiguous when multiple signals conflict.

## Considered Options

### A. Global `--format=json|human` flag only (explicit)

Pros: 100% explicit, no hidden behavior.
Cons: agents must remember to pass it on every call; tedious for humans
who want the default.

### B. `AGENT_MODE=1` environment variable only

Pros: orchestrator sets once, all child CLI invocations inherit; no flag
clutter.
Cons: silently inherited; can surprise humans running an agent's shell.

### C. TTY auto-detection only

Pros: zero configuration; behaves correctly by default in 95% of cases.
Cons: CI environments often have no TTY but want human-readable output;
agent contexts can be misdetected; not overridable per-invocation.

### D. Hybrid with precedence — `--format` flag > `AGENT_MODE` env > TTY-detect

Pros: explicit override always wins; orchestrator-friendly env var;
sensible default; covers all observed cases.
Cons: slightly more code; documentation burden to explain precedence.

## Decision

Adopt **Option D** with precedence: explicit `--format` flag overrides
`AGENT_MODE` env var, which overrides TTY-based auto-detection.
TTY-attached process defaults to human-formatted output; non-TTY
defaults to JSON.

Rationale: covers all observed cases, explicit override always wins,
orchestrator-friendly env var, sensible default.

## Consequences

- Every entrypoint in the base package must consult the resolved mode
  before emitting any output.
- `__schema` output (ADR-0003) must document the default-mode-detection
  rule so agents know what to expect.
- The mode must be carried in `ctx` so deep-business-logic code can
  branch on it without re-detecting.
