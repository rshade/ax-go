# Contract: Prompt and Resource Declarations

## Go API (package `schema`; root `ax` re-exports each identically)

```go
func DeclarePrompt(cmd *cobra.Command, prompt Prompt) error
func DeclareResource(cmd *cobra.Command, resource Resource) error
```

- Success returns `nil` and appends the declaration to `cmd`'s annotations.
- Failure returns a `*contract.Error` with `error_code`
  `invalid_schema_declaration` and exit code `2`. `context.field` names the
  offending field (`cmd`, `name`, `template`, `arguments[i].name`, `uri`, ...),
  and `context.reason` is a stable, lowercase snake_case reason (`nil_command`,
  `required`, `invalid_charset`, `invalid_utf8`, `duplicate`,
  `undeclared_placeholder`, `not_absolute`, `malformed`, `too_long`,
  `invalid_character`, `corrupt_annotation`). `actionable_fix` gives a
  reason-specific remedy, and the message echoes at most 128 bytes of the
  key. The command is left unchanged.

## `__schema` (ax-native), additive

```json
{"command":{"use":"app","prompts":[{"name":"triage","title":"Triage a spike","description":"...","arguments":[{"name":"window","description":"lookback","required":true}],"template":"Run app report --since={{window}}"}],"resources":[{"uri":"app://docs/pricing","name":"pricing","mime_type":"text/markdown"}],"non_deterministic_fields":[]}}
```

## `__schema --as=mcp`, additive

```json
{"tools":[...],"prompts":[{"name":"triage","title":"Triage a spike","description":"...","arguments":[{"name":"window","description":"lookback","required":true}],"template":"Run app report --since={{window}}"}],"resources":[{"uri":"app://docs/pricing","name":"pricing","mimeType":"text/markdown"}]}
```

`prompts` and `resources` are omitted when empty. Trees with no declarations
are byte-identical to the pre-feature output.

## Duplicate failure (`__schema`, both formats)

stdout is empty, exit is `2`, and stderr carries one envelope:

```json
{"error_code":"validation_error","message":"duplicate prompt name \"triage\" declared on more than one command","actionable_fix":"Rename or remove one of the duplicate declarations.","context":{"commands":["app","app run"],"key":"triage","kind":"prompt"},"trace_id":"...","tool":"...","version":"...","schema_version":"1.0.0"}
```

Envelope key order follows the `contract.Error` struct. The example is
illustrative, and `testdata/schema_duplicate_declaration.golden.json` is
authoritative. The resource variant reads `duplicate resource URI ...` with `kind:"resource"`.

## Corrupt annotation failure (`__schema`, both formats)

A declaration annotation written by hand that does not decode, or holds an
invalid entry, makes `__schema` exit `2` with empty stdout and one
`validation_error` envelope whose context carries `kind`, `key` (the annotation
key), `reason: "corrupt_annotation"`, and `commands` (the one offending path).
`testdata/schema_corrupt_declaration.golden.json` is authoritative.

## Live `mcp-server` (unchanged in Phase 1)

The `initialize` result advertises no `prompts` or `resources` capability,
`prompts/list` and `resources/list` return empty lists, and `tools/list` is
unchanged, whatever the tree declares.
