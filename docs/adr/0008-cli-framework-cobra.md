# ADR-0008: CLI Framework — Cobra

## Status

ACCEPTED — 2026-05-28.

## Context

ax-go needs a Go CLI framework that supports subcommands, persistent
flags, shell completion, and integrates cleanly with the agent-mode
primitives (output mode resolution, OTel context propagation, error
envelopes).

The framework choice is foundational — every ax-go CLI inherits the
shape of whatever is chosen here, and switching later means rewriting
every adopting tool.

## Decision Drivers

- Subcommand-first ergonomics (the `kubectl verb noun` shape that LLMs
  recognize and humans expect).
- Plugin pattern for the ax base to wrap user commands without
  requiring per-command boilerplate.
- Mature, idiomatic, widely used — meaning LLM agents already know the
  patterns and humans don't have to learn a new framework.
- Composes with OTel context (`RunE` returning errors,
  `PersistentPreRun` for setup hooks).
- Reflection over the command tree must be cheap (powers ADR-0003's
  `__schema`).

## Considered Options

### A. `spf13/cobra`

The industry default for Go CLI frameworks. Used by kubectl, hugo,
etcd, gh, helm, and most major Go CLI tools.

Pros: mature; massive prior art; well-known to LLM agents; rich
ecosystem (cobra-cli generator, cobra-completion); subcommand tree is
trivial to walk for `__schema`.
Cons: opinionated structure; reflection cost on init; some
boilerplate.

### B. `urfave/cli`

Older, simpler alternative.

Pros: simpler API; smaller binary.
Cons: less subcommand-first; smaller ecosystem; agents see Cobra
patterns far more often in training data.

### C. `alecthomas/kong`

Modern, struct-tag-driven CLI parsing.

Pros: declarative struct tags; less imperative boilerplate.
Cons: smaller user base; LLM agents are less familiar with it; less
ecosystem support.

### D. `stdlib flag`

Too minimal for a subcommand-heavy library. Rejected without further
analysis.

## Decision

Adopt **Option A** — `github.com/spf13/cobra`.

The ax base wraps `cobra.Command.Execute()` via `ax.Execute()`
(see ADR-0005 for the OTel flush-on-exit wrapper and ADR-0001 for the
mode-resolution `PersistentPreRun` hook).

## Consequences

- Direct dependency on `github.com/spf13/cobra`.
- All ax-go documentation and ADRs assume Cobra constructs (Commands,
  Flags, PersistentPreRun, RunE).
- `__schema` (ADR-0003) reflects Cobra's command tree.
- Migration off Cobra would require coordinated changes across every
  adopting tool — treat the choice as effectively permanent.
