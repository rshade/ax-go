# Quickstart: Marking a Non-Deterministic Output Field

**Feature**: `015-schema-nondeterministic-fields`

This walks through what a command author does to get a domain-specific
output field correctly enumerated in `__schema`'s `non_deterministic_fields`.
The three built-in envelope fields (`meta.trace_id`, `meta.span_id`,
`meta.idempotency_key`) require no per-field tags, but the command must be
registered with its `contract.Envelope[T]` payload type so `__schema` knows
those `meta.*` fields actually appear in that command's success output (see
`research.md` D1/D2). Commands that do not emit a standard success envelope can
skip registration and will get an explicit empty command-scoped list.

> **Note**: The `report`/`reportPayload` names below are illustrative only —
> this exact command is not added to `examples/integration`. To follow along
> against the real built binary, substitute the actual registered root
> command (`hello`, payload `helloPayload`, tagged field `data.entity_id`)
> documented in `examples/integration/main.go` (see tasks.md T037).

## 1. Tag the field on your payload struct

```go
type reportPayload struct {
    ReportID    string `json:"report_id"    ax:"nondeterministic"`
    GeneratedAt string `json:"generated_at" ax:"nondeterministic"`
    Status      string `json:"status"`
}
```

Only `ReportID` and `GeneratedAt` vary between otherwise-identical runs;
`Status` does not, so it is left untagged.

## 2. Register the type when you build the command

```go
cmd := &cobra.Command{
    Use:   "report",
    Short: "generate a report",
    RunE: func(cmd *cobra.Command, _ []string) error {
        payload := reportPayload{
            ReportID:    generateReportID(),
            GeneratedAt: time.Now().UTC().Format(time.RFC3339),
            Status:      "complete",
        }
        return ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), payload))
    },
}
ax.WithNonDeterministicFields[reportPayload](cmd)
```

The registration call and the struct definition are the only two places
that need to change — there is no third list to keep in sync, and renaming
`ReportID` to `ID` (with its tag) is picked up automatically the next time
`__schema` runs, with no change needed at the registration call site. The same
registration also adds the standard `meta.trace_id`, `meta.span_id`, and
`meta.idempotency_key` locators for this command.

## 3. Verify with `__schema`

```console
$ ax-integration __schema | jq '.command.commands[] | select(.use=="report") | .non_deterministic_fields'
[
  "data.generated_at",
  "data.report_id",
  "meta.idempotency_key",
  "meta.span_id",
  "meta.trace_id"
]
```

## 4. Confirm the diff-safety guarantee

```console
ax-integration report > /tmp/run1.json
ax-integration report > /tmp/run2.json
jq 'del(.meta.trace_id, .meta.span_id, .meta.idempotency_key, .data.generated_at, .data.report_id)' /tmp/run1.json > /tmp/run1.masked.json
jq 'del(.meta.trace_id, .meta.span_id, .meta.idempotency_key, .data.generated_at, .data.report_id)' /tmp/run2.json > /tmp/run2.masked.json
diff /tmp/run1.masked.json /tmp/run2.masked.json && echo "byte-identical modulo documented fields"
```

An agent automating this step reads the field list directly out of
`__schema` instead of hardcoding it (`jq '.command.commands[] | select(.use=="report") | .non_deterministic_fields'`
above), so it does not need to know in advance which fields your command
will vary.

## 5. Regenerate golden fixtures

After adding or changing a `non_deterministic_fields` entry anywhere in the
tree, regenerate the pinned fixtures:

```console
go test ./examples/integration -run TestGolden -update
```

The root-package fixtures (`testdata/schema_ax.golden.json`,
`testdata/schema_mcp.golden.json`) have no `-update` flag and must be
hand-edited to match.
