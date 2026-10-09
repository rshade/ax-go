# Quickstart: adding or checking an error code

## Run the drift guard

```bash
go test -count=1 -run 'TestErrorCodesAreRegistered|TestErrorCodeRegistryConsistency' ./contract/
go test -count=1 -run TestFindErrorCodeViolations ./internal/testutil/
```

A failure lists one line per site, for example:

```text
internal/mcpserver/server.go:85: call_code: "validation_error"
```

## Add a new runtime code (for example #130's `interrupted`)

1. Add the constant to `contract/codes.go` with a doc comment that states the
   condition and the exit code:

   ```go
   // ErrorCodeInterrupted reports a run stopped by SIGINT. The process exits 1.
   ErrorCodeInterrupted = "interrupted"
   ```

2. Add `ErrorCodeInterrupted` to `KnownErrorCodes()` in byte-wise order.
3. Emit it by name: `contract.NewError(ctx, contract.ErrorCodeInterrupted, ...)`.
4. Update `ExampleKnownErrorCodes` and every `__schema` golden. Search all file
   types, because `ExampleBuildSchema` embeds `__schema` JSON in Go:

   ```bash
   grep -rln '"known_codes"' --include='*.go' --include='*.json' .
   ```

5. `make surface-update`, then review that `baseline.json` gained exactly one
   `added` `contract` constant.

Skipping step 2 fails `TestErrorCodeRegistryConsistency`. Skipping step 1 or
emitting a literal fails `TestErrorCodesAreRegistered`.

## Add an authoring-time code

Follow the same steps, but put the constant in `authoringTimeErrorCodes()`
instead of `KnownErrorCodes()`. Only codes returned while the command tree is
built, before command dispatch, belong there (spec 032 FR-001). No golden
changes.

## Verify nothing moved

```bash
go test -count=1 -race ./...
make surface-check
```

No golden file changes, and the surface diff is exactly the four new `contract`
constants.
