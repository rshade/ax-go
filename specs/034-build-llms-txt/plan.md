# Implementation Plan: Build-time static discovery document

**Branch**: `034-build-llms-txt` | **Date**: 2026-10-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/034-build-llms-txt/spec.md`

## Summary

Add a build-time generator in the root package that writes an llms.txt-shaped
document to a caller-supplied writer. The curated opening and link list come
from the caller. The command and flag reference comes from the same Cobra
reflection `__schema` already uses (`internal/schema.BuildCommand`). The
integration example is the first consumer: a test regenerates its committed
document from `newRootCommand`. Runtime `__schema`, `__schema --as=mcp`, and
`mcp-server` stay untouched. No new dependency and no new `cmd/` binary.

## Technical Context

**Language/Version**: Go 1.27.3 (pinned in `mise.toml` and `go.mod`)

**Primary Dependencies**: Existing Cobra reflection in `internal/schema`.
Standard library `os`, `io`, `sort`, `strings`, `bytes`. No new module
requirement.

**Storage**: The caller owns the destination. This repository commits
`examples/integration/llms.txt` and `examples/integration/llms.preamble.md`.

**Testing**: Table-driven tests and one golden file for the generator.
`ExampleGenerateLLMsTxt` with `// Output:`. The integration example compares
a fresh generation to the committed file. No fuzz target (no parser). No new
benchmark (no performance claim).

**Target Platform**: Library, all platforms the module already supports.

**Project Type**: Go library plus one existing example program.

**Performance Goals**: None. Generation is a build step, not a hot path.
`BenchmarkBuildCommand` must stay within the existing budget because the
generator calls `BuildCommand` and does not change it.

**Constraints**: Byte-identical output. Validation failures write zero bytes.
Opening files larger than 1 MiB (`config.DefaultMaxBytes`) fail closed.
Context is the first parameter because the file option and the writer are I/O.
No `__schema --as=llms-txt`. No `llms-full.txt`.

**Scale/Scope**: One public function, three options, two small structs, one
example, one integration document.

**Governing ADR(s)**: N/A. Constitution Principle III is the decision record.
The issue's ADR-0013 is not created. ADR-0003 is already retired. The 1 MiB
cap is Principle V, not an ADR to absorb.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Requirement | Result |
|-----------|-------------|--------|
| I. Stream separation | Generated text goes to the caller-supplied writer, never to `__schema` stdout | PASS |
| II. Determinism | Identical inputs, lexicographic order, no host or time fields | PASS |
| III. Discoverability | No new `__schema` projection. Live `mcp-server` lists unchanged. Generator matches the build-time rule | PASS |
| IV. Agent safety | Not a command run. No dry-run or idempotency key on the document | PASS |
| V. JSON I/O | Document is markdown, not a machine payload. Opening read uses the 1 MiB cap | PASS |
| VI. Library scope | Cross-cutting generator only. Adopter publishes the file. No `cmd/` application | PASS |
| VII. Tests first | Failing tests and the example land before the renderer | PASS |
| VIII. IDs and logs | No trace or entity IDs in the document | PASS |
| IX. Resource safety | Size-capped read. Context cancellation before read and write | PASS |
| XI. Stability | Additive exported API. `__schema` goldens unchanged | PASS |
| XII. Deprecation | Nothing removed | PASS |

**ADR absorption gate**: Governing ADR(s) is N/A. No ADR file is created or deleted.

Post-design re-check: the same rows hold. The signature adds `context.Context`
ahead of the issue's proposed parameters so Principle IX's I/O rule is met.
That is recorded in research.md R1.

## Project Structure

### Documentation (this feature)

```text
specs/034-build-llms-txt/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── llms-txt.md
├── checklists/
│   └── requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
llmstxt.go                  # GenerateLLMsTxt and options
llmstxt_test.go             # table tests and golden
example_test.go             # ExampleGenerateLLMsTxt
testdata/llms.golden.txt
examples/integration/llms.preamble.md
examples/integration/llms.txt
examples/integration/llms_test.go
internal/cmd/doccover/      # requiredSymbols + baseline
internal/cmd/surfacecheck/baseline.json
specs/023-internalize-helpers/public-surface-audit.json
README.md
examples/integration/README.md
```

**Structure Decision**: The renderer lives in root package `ax` because that
package already depends on Cobra and `internal/schema`. The import-isolated
`schema` package is not given a markdown renderer. `cmd/` stays empty: a
module-level binary cannot see an adopter's command tree, and the integration
example already can.

## Complexity Tracking

No constitution violations require justification.
