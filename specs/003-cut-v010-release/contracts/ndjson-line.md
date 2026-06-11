# Contract: NDJSON Streaming Line Golden Fixture

**Fixture**: `testdata/ndjson_line.golden.json`

**Producer**: `ax.WriteJSONLine(w, v)`

**Status**: frozen at v0.1.0 — any byte-level change to this output is a
breaking change requiring a Spec Kit feature.

## Normative Pinned Inputs

Same seams as the success-envelope contract (research D4), with a **distinct
payload** so the two fixtures are not interchangeable files (research D5):

| Input | Pinned value | Seam |
|---|---|---|
| trace ID | `0102030405060708090a0b0c0d0e0f10` | `trace.ContextWithSpanContext` |
| span ID | `0102030405060708` | same `SpanContextConfig` |
| idempotency key | `00000000-0000-4000-8000-000000000002` | `ax.WithIdempotencyKey(ctx, key)` |
| `data` payload | `struct { Item string \`json:"item"\`; Seq int \`json:"seq"\` }{Item: "stream-record", Seq: 42}` | test-local struct |

## Expected Bytes

One self-contained JSON record terminated by exactly one `\n`:

```json
{"data":{"item":"stream-record","seq":42},"meta":{"trace_id":"0102030405060708090a0b0c0d0e0f10","span_id":"0102030405060708","idempotency_key":"00000000-0000-4000-8000-000000000002"}}
```

## Guarantees Locked by This Fixture

1. Each NDJSON line is itself strict, minified JSON — parseable standalone.
2. Exactly one `\n` terminator per line; no record separators, framing, or
   pretty-printing.
3. The line shape is wire-identical to a success envelope serialized on one
   line (today's delegation), **asserted independently** so any future
   divergence of the streaming path is caught even if `WriteJSON` stays green.

## Failure Mode

Any byte-level drift → `assertGolden` fails `go test -race ./...` → CI red
(FR-006). There is no update mode (research D7).
