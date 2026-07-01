---
title: Why Agentic Experience?
description: The reasoning behind ax-go's contracts — why determinism, stream separation, and self-description matter for LLM agents.
sidebar:
  order: 1
---

`ax-go` exists to make Go command-line tools that an LLM agent can drive as
reliably as a human can. This page explains *why* its contracts take the shape
they do. It is background reading, not a set of instructions — the
[tutorial](/ax-go/tutorials/build-your-first-cli/) and
[how-to guides](/ax-go/guides/expose-schema/) show you how to apply what follows.

## A new kind of user

The term **Agent Experience (AX)** was coined by Mathias Biilmann in early 2025
to name something UX and DX had never had to account for: software whose *user*
is an autonomous agent. `ax-go` adopts the widely-used "Agentic Experience"
variant of the term (see [Sources](/ax-go/sources/) for provenance) and asks a
narrow, practical question: what does a CLI owe an agent that a human never
needed?

The answer starts from how the two consumers differ. A human reads output,
tolerates a stray log line, notices when something looks off, and adapts. An
agent does none of that. It pipes your `stdout` into a parser, compares outputs
across runs to detect change, branches on your exit code, and retries on
failure. Every one of those behaviors is brittle in a way a human's is not — and
each of ax-go's contracts exists to remove a specific source of that
brittleness.

## Why streams are separated

`stdout` carries the machine payload and nothing else; logs, progress, and
diagnostics go to `stderr`. To a human, mixing them is a cosmetic annoyance. To
an agent piping `stdout` straight into a JSON parser, a single log line printed
to the wrong stream is a parse error — the run fails not because the work failed
but because the output was contaminated. Separating the streams is what lets an
agent trust that whatever arrives on `stdout` is the answer, unconditionally.

## Why output is deterministic

Given the same inputs, two runs of the same command must produce byte-identical
`stdout`, apart from fields documented as non-deterministic (timestamps, trace
IDs, an auto-generated idempotency key). This matters because agents *diff*
outputs to decide whether something changed. If a payload's field order wanders
because it was built from a Go map, or a value is formatted differently between
runs, the agent sees a change that isn't there and acts on a phantom. Determinism
is why ax-go insists on structs over maps for envelopes, RFC 3339 UTC
timestamps, and never a bare float for an ID or a money value. Non-determinism
does not announce itself; it silently corrupts the agent's model of the world.

## Why exit codes are a contract

`0` success, `1` internal error, `2` validation, `3` network/timeout, `4`
auth/permission — always, across every ax-go CLI. An agent branches on the exit
code to decide what to do next: a `2` means *fix the input*, a `3` means *back
off and retry*, a `4` means *re-authenticate*. Collapse those into an
undifferentiated "non-zero" and the agent is forced to parse error text to
recover — exactly the fragile, locale-sensitive guessing the structured
`ax.Error` envelope is meant to eliminate. Stable codes turn recovery from
guesswork into a lookup.

## Why the CLI describes itself

Every ax-go CLI answers `__schema` with a structured description of its command
tree, flags, types, and version — and `__schema --as=mcp` reshapes that into MCP
tool definitions, so `mcp-server` can expose the whole CLI as an MCP server with
no per-command work. The point is to remove the human from the integration loop.
Traditionally, teaching an agent to use a tool meant a person reading `--help`
and hand-writing a wrapper. Self-description lets the agent learn the tool
directly, and lets the tool's author change it without breaking every downstream
integration by hand. The schema is a machine-readable contract, which is why it
is treated as part of the public API and pinned in tests.

## Why agent-safety primitives are first-class

Agents retry, and agents act on the world. `--idempotency-key` exists because a
retry without one can execute a create twice; with a key echoed back in the
envelope, a retry can prove it is the same logical request. `--dry-run` exists
because an agent often needs to preview an effect before committing to it —
producing the same envelope shape with the side effect suppressed lets it look
before it leaps. These are not conveniences bolted on; they are the minimum an
agent needs to operate safely against something that changes state.

## The dual-audience tension

None of this comes at the human's expense, and that is deliberate. The same
binary resolves its output mode from `--format`, then `AGENT_MODE`, then TTY
detection — so a human at a terminal gets readable output while an agent gets
strict JSON from the identical command. `ax-go` refuses the usual trade-off
between a tool that is pleasant for people and one that is safe for machines.
Its wager is that the discipline required to serve agents well — determinism,
clean streams, honest exit codes, self-description — makes a tool more
trustworthy for humans too. The contracts are not overhead. They are the
difference between a CLI an agent can depend on and one it can only guess at.

## Related

- **Tutorial:** [Build your first agent-ready CLI](/ax-go/tutorials/build-your-first-cli/)
  — see these contracts in working code.
- **How-to:** [Expose your command tree with `__schema`](/ax-go/guides/expose-schema/)
- **Reference:** [Sources](/ax-go/sources/) — provenance of the AX discipline.
