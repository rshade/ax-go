# Contract: Success Envelope (`Envelope[T]`) Golden Fixture

**Fixture**: `testdata/success_envelope.golden.json`

**Producer**: `ax.NewEnvelope[T](ctx, data)` → `ax.WriteJSON(w, envelope)`

**Status**: frozen at v0.1.0 — any byte-level change to this output is a
breaking change requiring a Spec Kit feature.

## Normative Pinned Inputs

The golden test MUST construct its input exactly as follows (research D4 —
existing seams only, no harness normalization, FR-008):

| Input | Pinned value | Seam |
|---|---|---|
| trace ID | `0102030405060708090a0b0c0d0e0f10` | `trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{TraceID: ..., SpanID: ...}))` |
| span ID | `0102030405060708` | same `SpanContextConfig` |
| idempotency key | `00000000-0000-4000-8000-000000000001` (valid UUID v4 shape) | `ax.WithIdempotencyKey(ctx, key)` |
| dry-run | unset (false → `dry_run` omitted) | default context |
| `data` payload | `struct { Name string \`json:"name"\`; Count int \`json:"count"\` }{Name: "example", Count: 1}` | test-local struct (never a map — Constitution II) |

## Expected Bytes

Strict minified JSON, single trailing newline (`WriteJSON` contract):

```json
{"data":{"name":"example","count":1},"meta":{"trace_id":"0102030405060708090a0b0c0d0e0f10","span_id":"0102030405060708","idempotency_key":"00000000-0000-4000-8000-000000000001"}}
```

(one line + `\n`; shown wrapped here only if your renderer wraps)

## Guarantees Locked by This Fixture

1. Top-level key order: `data` before `meta`.
2. `meta` key order: `trace_id`, `span_id`, `idempotency_key` (struct
   declaration order).
3. `span_id` and `idempotency_key` are present when populated (omitempty
   exercised in the populated state).
4. `dry_run` is absent when false.
5. No `version` field exists in the success envelope.
6. Minified output (no insignificant whitespace) terminated by exactly one
   `\n`.

## Failure Mode

Any byte-level drift → `assertGolden` fails `go test -race ./...` → CI red
(FR-006). There is no update mode (research D7).
