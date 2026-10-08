# Contract: Machine-Payload Output

All additions are **additive**. Under Principle XI, consumers must tolerate
unknown fields. Every new field is absent when the author has not declared it
(FR-008, SC-004). Golden files pin each shape:

- `testdata/schema_ax.golden.json` and `testdata/schema_mcp.golden.json`
  (fixture with no declarations) stay **unchanged**.
- `testdata/schema_ax_enriched.golden.json` and
  `testdata/schema_mcp_enriched.golden.json` are **new**.

## 1. `__schema` (ax-native)

The flag entry gains `enum` and `example`. Values are in CLI string form, which
is how `default` is already emitted. `enum` keeps the author's order.

```json
{"name":"output","type":"string","default":"json","usage":"output format",
 "enum":["json","table","yaml"]}
{"name":"replicas","type":"int","default":"1","usage":"replica count",
 "enum":["1","3","5"]}
{"name":"timeout","type":"duration","default":"30s","usage":"deadline",
 "example":"45s"}
```

The command node gains `capability`. The whole object is absent when the
command is unclassified, and `note` is absent when it is empty.

```json
{"use":"deploy","short":"…","flags":[…],
 "capability":{"class":"mutate","note":"idempotent by release name"},
 "non_deterministic_fields":[…]}
```

Field order follows the struct declaration. `capability` comes after
`commands` and before `non_deterministic_fields`, and `encoding/json` emits it
in that order every time.

## 2. `__schema --as=mcp` (static adapter)

The input-schema property gains a typed `enum` and an `examples` array.
`encoding/json` sorts the keys of the `inputSchema` map.

```json
"output":{"default":"json","description":"output format","enum":["json","table","yaml"],"type":"string"}
"replicas":{"default":1,"description":"replica count","enum":[1,3,5],"type":"integer"}
"timeout":{"default":"30s","description":"deadline","examples":["45s"],"type":"string"}
"tags":{"description":"tags","examples":[["a","b"]],"items":{"type":"string"},"type":"array"}
```

The tool gains `capability`, the ax-specific object, and `annotations`, the
standard MCP hints. Both are absent when the command is unclassified.

```json
{"name":"app-deploy","description":"…","inputSchema":{…},
 "nonDeterministicFields":[…],
 "capability":{"class":"mutate","note":"idempotent by release name"},
 "annotations":{"destructiveHint":true}}
```

The class-to-hint mapping is in research.md R6. For a `read-only` command,
`annotations` is `{"readOnlyHint":true}`.

Hidden commands and the reserved `__schema`, `mcp-server` and `completion`
commands are **never** emitted as tools, even when they carry a declaration
(FR-011).

## 3. `mcp-server` (live) `tools/list`

`inputSchema` is identical to §2, because both paths call
`internal/mcp.BuildTool`. `Tool.annotations` uses the SDK's
`ToolAnnotations` with the same hint mapping. The ax-specific `capability`
object is not emitted on the live path in this feature (research.md R6).

## 4. Enum rejection envelope (stderr, exit 2)

Invocation: `app deploy --output=xml` (with or without `--dry-run`).

- stdout: **empty**
- stderr: exactly one minified `ax.Error`, shown here pretty-printed:

```json
{
  "error_code": "validation_error",
  "message": "flag --output: value is not one of the allowed values",
  "trace_id": "<non-deterministic>",
  "tool": "app",
  "version": "<injected>",
  "schema_version": "<ErrorSchemaVersion>",
  "context": {"allowed": ["json","table","yaml"], "flag": "output"},
  "suggestions": ["--output=json", "--output=table", "--output=yaml"]
}
```

- exit code: `2`

The offending value (`xml`) appears nowhere in the envelope (research.md R9).

The same input always produces the same envelope, apart from `trace_id`.
Neither `PersistentPreRunE` nor `RunE` runs.

**Live `mcp-server`**: a `tools/call` with `{"output":"xml"}` returns
`CallToolResult{IsError: true}` whose content is the same structured envelope.
The dispatcher's `FlagErrorFunc` keeps the `*contract.Error` instead of
re-wrapping its message.
