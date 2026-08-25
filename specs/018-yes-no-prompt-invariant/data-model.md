# Data Model: --yes no-prompt invariant

The feature is stateless. These are per-run values and decision types, not
persisted records.

## Approval state

| Field | Type | Required | Meaning |
|---|---|---:|---|
| `granted` | `bool` | yes | Whether the invocation supplied explicit approval. Defaults to `false`. |

Sources:

- Direct CLI: persistent `--yes`, resolved in `Execute`'s persistent pre-run.
- MCP: per-call `yes` argument, validated as JSON boolean and injected into the
  isolated invocation.
- Context API: `WithApproval(ctx, granted)` and `ApprovalFromContext(ctx)`.

Approval has no memory, expiration, principal, or cross-call identity. A missing
carrier is equivalent to `granted: false`.

## Resolved mode

The existing `Mode` carrier remains unchanged:

- `ModeJSON` / machine mode + `granted=false` → `ConfirmationBlocked` and a
  `confirmation_required` error.
- `ModeHuman` + `granted=false` → `ConfirmationPromptRequired` and nil error.
- Either mode + `granted=true` → `ConfirmationApproved` and nil error.
- Missing mode is treated as non-interactive by `Confirm`, so it follows the
  blocked row unless approval is granted.

## Confirmation outcome

`ConfirmationOutcome` is a closed enum with three mutually exclusive values:

| Outcome | Error | Caller meaning |
|---|---|---|
| `ConfirmationApproved` | nil | Proceed without presenting a prompt. |
| `ConfirmationBlocked` | `*Error` with code `confirmation_required`, exit 2 | Stop; the run is non-interactive and needs explicit approval. |
| `ConfirmationPromptRequired` | nil | A human mode is available; the adopting CLI may perform its own prompt. |

The caller must branch on the outcome before performing the gated effect. The
gate does not run the effect and holds no mutable state.

## Confirmation-required envelope

The serialized shape is the existing `contract.Error` / `ax.Error` struct:

```json
{"error_code":"confirmation_required","message":"confirmation required: apply the change","trace_id":"…","tool":"…","version":"…","schema_version":"1.0.0","actionable_fix":"pass --yes to confirm this operation"}
```

The example is illustrative: trace, tool, and version normalization follow the
existing `Execute`/MCP paths. No new field is introduced, and success envelopes
are unchanged.

## State transitions

There is no persistent transition. Each gate call evaluates the immutable
per-invocation context independently:

```text
context approval=false + machine/unknown mode ──Confirm──> blocked + Error
context approval=false + human mode            ──Confirm──> prompt-required
context approval=true  + any mode              ──Confirm──> approved
```

Calling `Confirm` twice with the same context produces the same result twice.
