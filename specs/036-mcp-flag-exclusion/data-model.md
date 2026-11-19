# Data Model: Per-Flag MCP Exclusion (`mcp.ExcludeFlag`)

## 1. Flag Exclusion Annotation (`pflag.Flag.Annotations`)

No new structs are introduced. Per-flag exclusion is stored on the target `*pflag.Flag` in its `Annotations` (`map[string][]string`) field:

| Key | Canonical Excluded Value | Non-Canonical / Absent Values |
| --- | --- | --- |
| `"github.com/rshade/ax-go/mcp/exclude"` (`internalmcp.ExcludeAnnotationKey`) | `[]string{"true"}` | `nil`, `[]string{}`, `[]string{"TRUE"}`, `[]string{"1"}`, `[]string{""}`, `[]string{"true", "false"}` → treated as **not excluded** (fail-closed) |

## 2. Effect Across Surfaces

| Surface | Non-Excluded Flag | Flag Marked with `mcp.ExcludeFlag` |
| --- | --- | --- |
| `<cli> <cmd> --help` | Present | Present (unchanged) |
| Direct CLI execution (`ax.Execute`) | Accepted | Accepted (unchanged) |
| `<cli> __schema --as=ax` (`schema.BuildSchema`) | Present in `command.flags` | Present in `command.flags` (unchanged) |
| `<cli> __schema --as=mcp` (`schema.BuildMCPSchema`) | Present in `inputSchema.properties` (and `inputSchema.required` if required) | Omitted from `inputSchema.properties` and `inputSchema.required` |
| `mcp-server` `tools/list` | Present in `inputSchema.properties` (and `inputSchema.required` if required) | Omitted from `inputSchema.properties` and `inputSchema.required` |
| `mcp-server` `tools/call` (flag omitted by caller) | Uses default (or fails `validation_error` if required) | Uses default (or fails `validation_error` if required, since it can never be supplied) |
| `mcp-server` `tools/call` (flag supplied by caller) | Validated and passed to command | Rejected in `buildArgs` with `IsError: true`, single `TextContent` item carrying `ax.Error` (`error_code: "validation_error"`, exit `2`, message `unknown argument "<flag>" for tool "<tool>"`); `Run`/`RunE` never runs |
