---
title: ax-go and the Go Proverbs
description: An audit of the codebase against Rob Pike's nineteen Go Proverbs — where the design aligns, where it strains, and why the tensions are held by CI gates rather than resolved.
sidebar:
  order: 2
---

The [Go Proverbs](https://go-proverbs.github.io/) are nineteen aphorisms from
Rob Pike's talk at Gopherfest SV 2015 that compress the language's design
values into single sentences. This page reads the `ax-go` codebase against all
nineteen: where the design embodies them, where it strains against them, and —
most interestingly — what the project does when a proverb and a requirement
genuinely pull in opposite directions.

This is background reading, not a checklist to follow. It is also a snapshot:
the audit was performed in August 2026, shortly after the `v0.5.0` release.
The specific counts below will drift; the design commitments they illustrate
are the durable part.

## The scorecard

| # | Proverb | Verdict |
|---|---------|---------|
| 1 | Don't communicate by sharing memory, share memory by communicating | Pass |
| 2 | Concurrency is not parallelism | Pass |
| 3 | Channels orchestrate; mutexes serialize | Pass |
| 4 | The bigger the interface, the weaker the abstraction | Pass, one watched exception |
| 5 | Make the zero value useful | Pass |
| 6 | `interface{}` says nothing | Pass |
| 7 | Gofmt's style is no one's favorite, yet gofmt is everyone's favorite | Pass |
| 8 | A little copying is better than a little dependency | Pass, under real tension |
| 9 | Syscall must always be guarded with build tags | Not applicable |
| 10 | Cgo must always be guarded with build tags | Not applicable |
| 11 | Cgo is not Go | Not applicable |
| 12 | With the unsafe package there are no guarantees | Pass |
| 13 | Clear is better than clever | Pass |
| 14 | Reflection is never clear | Pass, contained |
| 15 | Errors are values | Strong pass |
| 16 | Don't just check errors, handle them gracefully | Strong pass |
| 17 | Design the architecture, name the components, document the details | Strong pass |
| 18 | Documentation is for users | Strong pass |
| 19 | Don't panic | Strong pass |

No proverb is violated. Sixteen pass outright, and the three "not applicable"
entries (`syscall`, cgo) are upheld in spirit: the module imports none of the
three, and applies the same guard-it-with-build-tags discipline to its own
optional dependency trees instead.

## Channels orchestrate; mutexes serialize

The concurrency primitives in this codebase divide exactly along the
proverb's line, which makes it the cleanest illustration of the nineteen.

Channels do the orchestration. The Loki direct-push pipeline in `loki.go`
moves log entries to a single worker goroutine over a buffered channel, so the
worker alone owns the buffer state — which is also proverb 1 in action.
Flushing works as a channel of channels: the requester sends an
acknowledgement channel into `flushRequests` and waits on it, so completion is
communicated rather than polled. Shutdown is a closed `done` channel.

Mutexes do the serialization, and only that. A read-write mutex guards the
label map; a locked writer wraps `stderr` so OpenTelemetry exporters, zerolog
hooks, and shutdown diagnostics cannot interleave torn lines; `sync.Once`
guards the one-time telemetry install. No mutex coordinates workflow, and no
channel protects a variable.

Proverb 2 shows up somewhere unexpected: the benchmark gate pins every run to
`-cpu=1`, precisely so that measurements of the *concurrent* design are never
confused by incidental *parallelism* on whatever machine happens to run them.

## Small interfaces, and one honest exception

The module defines four interfaces in non-test code: `flusher` (one method,
unexported), `LabelSanctioner` (one method), `Sink` (two methods), and
`Logger` (six methods). The first three are the proverb applied consciously —
`LabelSanctioner` exists as a separate capability rather than a third `Sink`
method because a file rotator or ring-buffer sink has no label concept and
must not be forced to pretend otherwise. The doc comment in
`internal/logcore/sink.go` says exactly that.

`Logger` is the watched exception. Six methods, and two of them expose
concrete zerolog types (`*zerolog.Event`, `*zerolog.Logger`) rather than
abstractions. The proverb warns against big interfaces because they make weak
abstractions — but `Logger` is documented as *not being an abstraction at
all*. It is a migration seam frozen by Constitution Principle VI, which
forbids pluggable logger backends outright. An interface that leaks its
backend on purpose, and says so, is not pretending to abstract anything. The
watch-item is only that it never grows.

## A little copying is better than a little dependency

This is the one proverb under genuine pressure, and the project's response is
more interesting than either compliance or violation would be.

The module carries eighteen direct dependencies — Cobra, zerolog, uuid,
Hujson, the OpenTelemetry suite, gRPC, protobuf, the MCP SDK. That is heavy
for a foundation library, and no amount of framing makes it light. Three
things keep the verdict a pass rather than a fail:

1. **Every dependency is a governed decision.** Each one traces to the
   constitution or a Spec Kit feature, not to convenience. The AGENTS.md
   dependency rule requires justifying each addition in the PR that makes it.
2. **The cost has escape hatches.** The `ax_no_grpc` and `ax_no_otlp` build
   tags let a consumer decline whole dependency trees, and the
   import-isolated `logging` package gives a logging-only consumer 103 linked
   packages and roughly 2.3 MB stripped, against 410 packages and 12 MB
   through the root facade.
3. **The cost is measured, not asserted.** A CI binary-size gate builds both
   probe programs on every change and fails if the isolation claim decays.

And in one place the proverb is applied verbatim: the surface-check tool
*deliberately copies* the public package list from the apidiff tool instead
of importing it, with a guard test that parses the original and fails on
drift. A little copying, chosen consciously, with a tripwire.

## Reflection, treated as hazardous material

Reflection cannot be avoided here — the `__schema` self-description walks
command envelope types at runtime — so the question is containment. The
`reflect` import appears in exactly two non-test files: the internal schema
walker and the single `reflect.TypeFor[T]()` call in the public `schema`
package that feeds it. The walker guards against cyclic types, and the whole
path is tracked in CI as a named benchmark under the performance regression
budget. The proverb says reflection is never clear; the codebase compensates
with confinement and measurement rather than pretending otherwise.

## Errors are values — literally the product

Proverbs 15 and 16 are where the codebase is strongest, because structured
errors are not an internal discipline here but the shipped feature. The
`ax.Error` envelope is a value with a stable JSON shape, a deterministic exit
code mapping, and a tri-state `Retryable` field that distinguishes "do not
retry" from "unknown" — all golden-file tested as public contract.

The mechanical hygiene backs it up. At audit time the module contained
ninety-four `fmt.Errorf` calls in non-test code: every one that wraps an
underlying error uses `%w`, and the remainder construct leaf errors with no
cause to wrap. There is not a single `%v` or `%s` flattening an error chain,
so `errors.Is` and `errors.As` work against everything the library returns.
Sink draining collects failures with `errors.Join` rather than
short-circuiting, so one failing sink never hides another. The handful of
discarded returns are each deliberate and documented — the clearest being a
telemetry constructor whose doc comment states the error return is reserved
and always nil today, because setup is fail-open by contract.

And proverb 19 is absolute: there are zero `panic` calls in non-test code
across the entire module. Even the internal CI gate tools return exit codes
from `main` instead of panicking.

## The pattern: proverbs as gates

Step back from the individual verdicts and one pattern explains most of them.
Where a proverb states a value, this repository converts it into a
machine-enforced gate:

| Value | Gate |
|-------|------|
| Gofmt's style is everyone's favorite | `gofmt` and `golangci-lint` in CI |
| Documentation is for users | `doccover` ratchet; executed `ExampleXxx` functions that cannot drift |
| A little copying beats a dependency | `sizecheck` binary-size budget |
| Reflection is never clear | `benchcheck` regression budget on the reflection path |
| The bigger the interface, the weaker the abstraction | `surfacecheck` inventories every method across 24 build loads |

The proverbs assume a human reviewer holding the line. This project's wager —
explained in [Why Agentic Experience?](/ax-go/explanation/why-agentic-experience/)
— is that agents write much of the code, so the line has to be held by
tooling. It is no coincidence that the two verdicts with the most tension
(dependencies and the `Logger` interface) are also the two with the most CI
machinery around them. Tension is exactly what gates are for.

## Open nits

An honest audit records its residue. Three small items, none a violation:

- The repository's own rule that every `any` use site carries a comment
  explaining why is stricter than proverb 6, and not every site carries one.
  Most are self-evident JSON boundaries, so this is drift against the house
  rule, not against Go idiom.
- `Execute` discards the telemetry constructor's error. Safe today by
  documented contract, but if that reserved error ever becomes real, this
  call site will swallow it silently. A tripwire for a future maintainer.
- The internal gate tools discard `Close` errors on read-only files in
  deferred closures — acceptable in exit-code-driven tools, noted for
  completeness.

## Related

- **Explanation:** [Why Agentic Experience?](/ax-go/explanation/why-agentic-experience/)
  — the reasoning behind the contracts the proverbs are audited against.
- **Tutorial:** [Build your first agent-ready CLI](/ax-go/tutorials/build-your-first-cli/)
- **How-to:** [Choose a logging surface](/ax-go/guides/choose-a-logging-surface/)
  — the import-isolation story behind the proverb 8 verdict.
- **External:** [Go Proverbs](https://go-proverbs.github.io/), with links to
  Rob Pike's talk for each proverb.
