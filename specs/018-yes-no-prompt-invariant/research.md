# Research: --yes no-prompt invariant

## Decision Summary

This feature adds a per-invocation approval decision to the existing agent-safety
pipeline. It does not intercept arbitrary third-party prompts: callers opt into
the invariant at a confirmation point by calling the public gate.

## D1 — Public gate shape and three outcomes

- **Decision**: Add `ax.Confirm(ctx, subject) (ConfirmationOutcome, error)`
  with the typed outcomes `ConfirmationApproved`, `ConfirmationBlocked`, and
  `ConfirmationPromptRequired`. Approved and prompt-required return a nil error.
  Blocked returns `ConfirmationBlocked` plus an `*ax.Error` whose code is
  `confirmation_required` and whose exit code is 2.
- **Rationale**: A result enum makes all three outcomes distinguishable without
  parsing message text. Returning the structured error lets the established
  `ax.Execute` path perform the one canonical stderr serialization and exit-code
  mapping. The gate remains a decision oracle and does no prompt I/O.
- **Alternatives considered**:
  - `bool` plus `error`: rejected because it cannot distinguish blocked from
    prompt-required without an additional convention.
  - An error for the human prompt-required state: rejected because the spec
    requires that state not terminate the command and leaves prompting to the
    adopting CLI.
  - A gate that accepts a prompt callback: rejected because it would make ax-go
    own I/O and violate the no-prompt/no-editor/no-pager boundary.

## D2 — Approval context carrier

- **Decision**: Add `contract.WithApproval(ctx, bool)` and
  `contract.ApprovalFromContext(ctx) bool`, with root `ax` forwarding aliases.
  The absent value reads as false; nil contexts are normalized to
  `context.Background()` by the gate before lookup.
- **Rationale**: This mirrors `WithDryRun`/`DryRunFromContext` and keeps the MCP
  dispatcher import-isolated from the root runtime facade. A boolean accessor is
  sufficient because approval has only granted/not-granted state; mode determines
  whether the not-granted state is blocked or prompt-required.
- **Alternatives considered**:
  - A public `WithYes` name: rejected as flag-oriented rather than semantic and
    less clear to library callers.
  - A richer approval object: rejected because there is no identity, timestamp,
    or mutable approval lifecycle in this feature.
  - A package global: rejected by the no mutable global/state rule and would leak
    approval between calls.

## D3 — Mode resolution and fail-closed behavior

- **Decision**: Resolve `--yes` in the same `PersistentPreRunE` wrapper that
  resolves mode, dry-run, and idempotency key. If the context lacks a resolved
  mode, `Confirm` treats it as non-interactive and returns blocked unless
  approval is true.
- **Rationale**: Resolution in one pre-run step preserves the existing execution
  contract. Fail-closed handling prevents library callers that bypass
  `ax.Execute`, and nil/state-free contexts, from accidentally assuming a human
  is available.
- **Alternatives considered**:
  - Infer human mode from TTY inside `Confirm`: rejected because it would bypass
    the explicit mode precedence and make a context helper environment-sensitive.
  - Treat missing mode as human: rejected as unsafe for agents and contrary to
    FR-018.

## D4 — Flag installation and collision behavior

- **Decision**: Add `internal/cli.FlagYes = "yes"` and install it with the existing
  `EnsurePersistentBoolFlag` helper in both `prepareCommand` and
  `internal/mcpserver.ensurePersistentFlags`. An existing local or persistent
  author flag named `yes` is preserved, matching existing primitives.
- **Rationale**: One shared flag name and installer preserve schema reflection,
  direct MCP embedding, and collision behavior without special-casing the
  schema builder.
- **Alternatives considered**:
  - Install only in `ax.Execute`: rejected because directly embedded MCP servers
    must have the same agent-safety surface.
  - Override an author's flag: rejected because it silently changes an adopting
    CLI's public contract.
  - Add an environment fallback: rejected by the spec; approval is an explicit
    per-invocation decision.

## D5 — MCP per-call threading

- **Decision**: Extend the existing dispatcher `callConfig` with `approval`.
  Accept a `yes` tool argument only as a JSON boolean, include `--yes=true` in
  the isolated argv when true, count the injected flag as satisfied for required
  flag checks, and apply `contract.WithApproval` to the per-call context.
  False/absent is the default. A non-boolean value returns the existing
  validation envelope with exit 2.
- **Rationale**: This exactly mirrors dry-run while preserving the per-call
  isolation boundary. The dispatcher already resets flags and serializes command
  execution, so no approval state can survive into the next call.
- **Alternatives considered**:
  - Force approval for all MCP tools: rejected because clients must observe the
    refusal and explicitly choose the bypass.
  - Ignore `yes` and rely on a command-line-only path: rejected because it makes
    gated commands unreachable over MCP.
  - Store approval on the dispatcher: rejected because it leaks state across
    requests and violates per-call semantics.

## D6 — Error envelope, output, and determinism

- **Decision**: Construct the existing error envelope with code
  `confirmation_required`, a deterministic message containing the confirmation
  subject, `WithActionableFix("pass --yes to confirm this operation")`, and
  `WithErrorExitCode(ExitValidation)`. Do not add fields or alter success
  envelopes. `Execute` writes it to stderr; MCP returns it in `CallToolResult`
  with `IsError: true`.
- **Rationale**: `actionable_fix` is already the envelope's remediation field,
  and the spec explicitly forbids a new hint field. The fixed remediation is
  deterministic and lets an agent recover from the envelope alone.
- **Alternatives considered**:
  - Add `hint` or `confirmation_flag`: rejected as an unnecessary envelope
    shape change.
  - Write directly from `Confirm`: rejected because it would duplicate output,
    break caller-controlled streams, and make MCP serialization inconsistent.
  - Put the refusal on stdout: rejected by stream separation.

## D7 — Integration and documentation example

- **Decision**: Add a small confirmation-gated command to
  `examples/integration` that calls `ax.Confirm` before its recorded effect and
  demonstrates the approved path with `--yes`. Add `ExampleConfirm` on the root
  API using explicit contexts to exercise approved, blocked, and prompt-required
  decisions without interactive input.
- **Rationale**: The integration command is the repository's real-Cobra adoption
  example, while the package example is the stable godoc contract. Neither needs
  to make the gate itself prompt.
- **Alternatives considered**:
  - Document only prose: rejected by the required ExampleXxx contract.
  - Add a new example package: rejected by the no-new-public-package rule and
    binary/surface scope.

## D8 — Governing decision records

No ADR is absorbed. The repository's frozen ADRs cover Cobra and trace-ID
formatting, not confirmation approval or prompt behavior. The constitution's
Principles I–IV, VI–VII, IX–XI are the governing decisions, so no ADR deletion
or retirement task is needed.

## Resolved Unknowns

All plan-template unknowns are resolved: language/toolchain, dependencies,
storage, test strategy, target matrix, project type, performance posture,
constraints, scope, public API shape, MCP behavior, error serialization, and
governance are specified above.
