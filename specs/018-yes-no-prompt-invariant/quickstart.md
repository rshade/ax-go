# Quickstart: confirmation-gated commands

## Add a gate before a side effect

```go
RunE: func(cmd *cobra.Command, _ []string) error {
	outcome, err := ax.Confirm(cmd.Context(), "delete the selected record")
	if err != nil {
		return err // Execute emits confirmation_required on stderr and exits 2.
	}
	if outcome == ax.ConfirmationPromptRequired {
		// The adopting CLI owns the human prompt. The gate itself never prompts.
		if !askHuman(cmd.InOrStdin(), cmd.ErrOrStderr()) {
			return nil
		}
	}
	return deleteRecord(cmd.Context())
}
```

Run the command in agent/machine mode without approval:

```text
$ mycli delete --format=json
{"error_code":"confirmation_required",...,"actionable_fix":"pass --yes to confirm this operation"}
$ echo $?
2
```

Approve explicitly:

```text
$ mycli delete --format=json --yes
{"...":"the normal success envelope"}
$ echo $?
0
```

`--yes` is auto-mounted by `ax.Execute`; the CLI author does not declare it.
An author-declared `yes` flag is preserved by the same collision rule as
`--dry-run` and `--idempotency-key`.

The gate itself never reads stdin or writes to stdout/stderr. In human mode,
`ConfirmationPromptRequired` deliberately leaves the actual prompt to the
caller; in machine mode, missing approval fails closed with exit code 2. A
missing mode is treated as machine mode for the same fail-closed reason.

## Context-only gate usage

The gate is safe to call directly in tests or helper code:

```go
ctx := ax.WithApproval(ax.WithMode(context.Background(), ax.ModeJSON), true)
outcome, err := ax.Confirm(ctx, "apply change")
// outcome == ax.ConfirmationApproved; err == nil
```

With no mode and no approval, the gate fails closed as blocked. With human mode
and no approval, it returns `ConfirmationPromptRequired` so the adopting CLI can
decide how to ask its user.

## MCP calls

The same command exposed by `mcp.NewCommand` accepts approval per call:

```json
{"name":"mycli-delete","arguments":{"yes":true}}
```

Omitting `yes` observes the structured refusal; passing a JSON string such as
`{"yes":"true"}` is a validation error. The decision applies only to that
call and does not affect the next `tools/call`.

## Verification

The feature's implementation must preserve the repository gates:

```bash
go test -race ./...
go vet ./...
golangci-lint run
make doc-coverage
make surface-check
```

The integration example demonstrates the approved gated path with
`examples/integration --yes` (and the blocked path with machine mode and no
approval); its ordinary stdin config input remains a data channel, not a prompt.
