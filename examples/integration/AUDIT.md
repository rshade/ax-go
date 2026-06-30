# Core AX Mandate Audit — `examples/integration`

This document maps every **Core AX Mandate** (`AGENTS.md` → *Core AX Mandates*)
to the function, subcommand, and test in `examples/integration` that exercises
it. It is the audit deliverable for issue #15 and the canonical answer to the
question "where is mandate X demonstrated?".

The integration example is the migration template for any downstream Go CLI
adopting ax-go: every mandate is exercised **exactly once**, with a clear
pattern a consumer can copy. A drift between this table and the code (or its
golden fixtures) is a real breaking change — it means the framework contract a
consumer relies on has shifted.

## Command surface

| Command | Exit code(s) | Mandate focus |
|---------|--------------|---------------|
| *(root)* | `0` | bounded JSON payload, Hujson config, idempotency, mode, trace correlation |
| `stream` | `0`, `2` | NDJSON streaming output |
| `patch-config` | `0`, `2` | Hujson AST patch, `--dry-run` side-effect suppression |
| `fail` | `2` | validation `ax.Error` envelope |
| `fetch` | `3` | network/timeout `ax.Error` with retry recovery fields |
| `authz` | `4` | authentication/permission `ax.Error` envelope |
| `crash` | `1` | unexpected internal error → framework `internal_error` envelope |
| `__schema` *(auto)* | `0` | reflective schema + `--as=mcp` adapter |
| `mcp-server` *(mounted)* | `0` | live MCP server over stdio/HTTP |

`__schema`, `--dry-run`, and `--idempotency-key` are injected by `ax.Execute`
(`execute.go`); the example never declares them. Testing them here therefore
validates the **framework contract**, not just this CLI.

## Mandate → evidence map

| # | Mandate | Exercised by | Test |
|---|---------|--------------|------|
| 1 | Stream separation (stdout = payload, stderr = logs + errors) | root (log→stderr, envelope→stdout), `stream` (stderr empty), `fail`/`fetch`/`authz`/`crash` (stdout empty) | `TestRunDefaultCommand`, `TestRunStreamCommandEmitsNDJSON`, `TestExitCodeMatrix` |
| 2 | Deterministic exit codes `0/1/2/3/4` | `0` root, `1` `crash`, `2` `fail`, `3` `fetch`, `4` `authz` | `TestExitCodeMatrix` |
| 3 | `__schema` with examples per subcommand | auto-mounted `__schema`; every command sets `Example` | `TestRunSchemaCommand`, `TestGoldenSchema` |
| 4 | `__schema --as=mcp` adapter | auto-mounted `__schema` `--as=mcp` flag | `TestRunSchemaMCPAdapter`, `TestGoldenSchemaMCP` |
| 5 | Hujson input; strict JSON / NDJSON output | root `--config` (Hujson read), root (bounded JSON), `stream` (NDJSON) | `TestRunAcceptsHujsonConfigFromStdin`, `TestGoldenStreamSuccess` |
| 6 | `--idempotency-key` (auto-gen UUIDv4 when absent, surfaced) | `ax.Execute` injects + surfaces in `meta.idempotency_key` | `TestRunGeneratesIdempotencyKeyWhenAbsent`, `TestRunDefaultCommand` |
| 7 | `--dry-run` (no side effects, `meta.dry_run: true`) | `patch-config` via `ax.Perform` | `TestRunPatchConfigCommandDryRunHasNoSideEffects` |
| 8 | Mode precedence `--format` > `AGENT_MODE` > TTY | `ax.Execute` resolves; root payload echoes resolved `mode` | `TestModePrecedence` |
| 9 | `ax.Error` envelope per exit-code category | `fail` (2), `fetch` (3), `authz` (4), `crash` (1) | `TestExitCodeMatrix`, `TestGoldenErrorEnvelopes` |
| 10 | OTel trace correlation on every log line (`trace_id`/`span_id`) | every stderr log line shares the envelope's trace context | `TestLogLinesCarryTraceCorrelation` |
| 11 | Output determinism (byte-identical modulo documented fields) | root + `stream` run twice, compared after masking | `TestDeterminismSuccessPath`, `TestDeterminismStreamPath` |

## Non-deterministic fields (documented exceptions)

`ax.Execute` starts an OTel span, so each run gets fresh random `trace_id` /
`span_id`. Golden fixtures keep the envelopes byte-stable by:

- `data.entity_id` — pinned through the `runWithEntityID` test seam.
- `version` — injected as `v9.9.9-golden` (otherwise `ResolveVersion` falls
  through to the git revision, which changes every commit).
- `meta.trace_id` / `meta.span_id` / `meta.idempotency_key` — masked to `MASKED`
  via `testutil.MaskNonDeterministic` (these are documented non-deterministic
  fields).
- `data.path` (patch-config only) — the temp config path is masked to
  `<config-path>`.

## Coverage gates

- Golden fixtures under `testdata/` run inside `go test ./...`; any drift in
  `__schema` output or an envelope shape fails CI (AC#4).
- `AGENTS.md` → *Development Workflow* step 4 names this example as the
  canonical pattern (AC#5).
