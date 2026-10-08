# Quickstart: Richer Per-Flag `__schema` Semantics

## For a CLI author

Declare the semantics where you build the command. Check every error: each one
is an authoring mistake that ax-go refuses to advertise.

```go
func newDeployCommand() (*cobra.Command, error) {
    cmd := &cobra.Command{Use: "deploy", Short: "Deploy a release", RunE: runDeploy}
    cmd.Flags().String("output", "json", "output format")
    cmd.Flags().Int("replicas", 1, "replica count")
    cmd.Flags().Duration("timeout", 30*time.Second, "deadline")

    if err := ax.WithFlagEnum(cmd, "output", "json", "table", "yaml"); err != nil {
        return nil, err
    }
    if err := ax.WithFlagEnum(cmd, "replicas", "1", "3", "5"); err != nil {
        return nil, err
    }
    if err := ax.WithFlagExample(cmd, "timeout", "45s"); err != nil {
        return nil, err
    }
    if err := ax.WithCapability(cmd, ax.CapabilityMutate, "idempotent by release name"); err != nil {
        return nil, err
    }
    return cmd, nil
}
```

You do **not** declare `default` or `required` again. ax-go still derives them
from Cobra.

The following are rejected with an error wrapping `ax.ErrInvalidDeclaration`:

```go
ax.WithFlagEnum(cmd, "output", "table", "yaml")      // default "json" not in set
ax.WithFlagEnum(cmd, "replicas", "1", "01")          // duplicates after canonicalisation
ax.WithFlagEnum(cmd, "timeout", "1s")                // duration: unsupported enum type
ax.WithFlagExample(cmd, "replicas", "three")         // not an int
ax.WithCapability(cmd, ax.Capability("write"), "")   // not in the vocabulary
```

## For an agent

```bash
app __schema | jq '.command.commands[] | select(.use=="deploy")'
# flags[].enum, flags[].example, capability.class

app __schema --as=mcp | jq '.tools[] | select(.name=="app-deploy") | {inputSchema, capability, annotations}'
```

A value outside the set is rejected before anything runs:

```bash
app deploy --output=xml --dry-run; echo "exit=$?"
# stdout: (empty)
# stderr: {"error_code":"validation_error","message":"flag --output: value is not one of the allowed values",
#          "context":{"allowed":["json","table","yaml"],"flag":"output","value":"xml"},
#          "suggestions":["--output=json","--output=table","--output=yaml"],...}
# exit=2
```

## Verifying the feature (maintainer)

```bash
go test -race ./...                                  # all 4 configs via: make test
go test -race -run 'Golden|Schema|Enum|Capability' ./... ./schema/... ./internal/...
go test -run '^$' -fuzz FuzzEnumCanonicalise -fuzztime 30s ./internal/schema
make lint validate doc-coverage cover-check bench-check
make surface-update && git diff internal/cmd/surfacecheck/baseline.json   # review every added line
make surface-check
```

Expected results:

- `testdata/schema_ax.golden.json` and `testdata/schema_mcp.golden.json` are
  **unchanged**. `git diff --exit-code testdata/schema_ax.golden.json
  testdata/schema_mcp.golden.json` must pass, which proves SC-004.
- The new `*_enriched.golden.json` files hold every new field.
- The surface baseline gains only additions: the `schema` and `ax` identifiers
  listed in `contracts/declaration-api.md` and the new struct fields.
