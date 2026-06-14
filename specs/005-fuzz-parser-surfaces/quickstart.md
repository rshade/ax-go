# Quickstart: Fuzz Tests for Every Parser Surface

**Feature**: `005-fuzz-parser-surfaces` | **Date**: 2026-06-13

This feature adds fuzz coverage for the uncovered parser surfaces (config read,
idempotency-key round-trip, and **two** error-envelope targets — build/round-trip
and arbitrary-bytes unmarshal), extends the traceparent fuzzer to cover
`TRACESTATE`, and commits seed corpora. It is test-only — nothing in the public
`ax` API changes.

## What lands

| File | Change | Function |
|------|--------|----------|
| `config_fuzz_test.go` | MODIFY (append) | `FuzzParseConfig` |
| `id_fuzz_test.go` | NEW | `FuzzIdempotencyKey` |
| `error_fuzz_test.go` | NEW | `FuzzErrorEnvelope` + `FuzzErrorEnvelopeUnmarshal` |
| `telemetry_fuzz_test.go` | MODIFY | `FuzzTraceparentExtraction` (+ `TRACESTATE`) |
| `testdata/fuzz/FuzzParseConfig/*` | NEW | seed corpus (incl. cap−1/cap/cap+1) |
| `testdata/fuzz/FuzzIdempotencyKey/*` | NEW | seed corpus |
| `testdata/fuzz/FuzzErrorEnvelope/*` | NEW | seed corpus |
| `testdata/fuzz/FuzzErrorEnvelopeUnmarshal/*` | NEW | seed corpus |
| `testdata/fuzz/FuzzTraceparentExtraction/*` | NEW | seed corpus |

## Run it

```bash
# The CI contract — replays every committed corpus entry, race-enabled:
go test -race ./...

# Explore one surface for 30s (finds new crashers; writes any to testdata/fuzz/):
go test -run=^$ -fuzz=FuzzParseConfig -fuzztime=30s .

# Replay ONLY the committed corpus for a function (fast, deterministic):
go test -run=FuzzParseConfig/ .
```

## Authoring a committed corpus entry

Create a file under `testdata/fuzz/<FuncName>/` (any descriptive name). The
format is one header line plus one typed literal per fuzz argument, in order.

`FuzzParseConfig` (args: `[]byte`, `int64`) — the cap+1 boundary case:

```text
go test fuzz v1
[]byte("{\"a\":1}")
int64(3)
```

`FuzzIdempotencyKey` (arg: `string`):

```text
go test fuzz v1
string("")
```

`FuzzErrorEnvelope` (args: `string`, `string`, `string`):

```text
go test fuzz v1
string("validation_error")
string("bad input")
string("underlying cause")
```

`FuzzErrorEnvelopeUnmarshal` (arg: `[]byte`):

```text
go test fuzz v1
[]byte("{\"error_code\":\"x\",\"context\":{\"k\":1}}")
```

`FuzzTraceparentExtraction` (args: `string`, `string`):

```text
go test fuzz v1
string("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
string("vendor=value")
```

> The arg count and types MUST match the fuzz function signature, or `go test`
> errors while loading the corpus.

## Definition of done (maps to spec Success Criteria)

- [ ] `FuzzParseConfig`, `FuzzIdempotencyKey`, `FuzzErrorEnvelope`, `FuzzErrorEnvelopeUnmarshal` exist; `FuzzTraceparentExtraction` covers `TRACESTATE` (SC-006).
- [ ] All five functions have committed corpora with ≥ 3 entries spanning valid/boundary/invalid (SC-003).
- [ ] `go test ./...` replays corpora with zero failures (SC-001).
- [ ] `go test -race ./...` green (SC-002).
- [ ] `golangci-lint run` and `make doc-coverage` clean (SC-004).
- [ ] 30s `-fuzz` run per function panic-free (SC-005, validation step).

## Next step

`/speckit-implement` to execute `tasks.md`. Per Constitution VII (test-first),
within each fuzz function write the `f.Add` seeds and assertions first and confirm
they pass against the existing implementation before broadening corpus coverage.
