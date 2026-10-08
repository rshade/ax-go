# Implementation Plan: Success warnings and strict escalation

**Branch**: `issue-123` | **Date**: 2026-10-08 | **Spec**: `specs/029-success-warnings/spec.md`

**Input**: Feature specification from `specs/029-success-warnings/spec.md`

## Summary

Add `Warning` and an omitempty `warnings` field on the success envelope.
`WithWarnings` copies the caller's list, drops blank entries, and records
the kept list on the execution context. `ax.Execute` mounts `--strict`.
When that flag is set and the recorded list is non-empty, Execute exits
2 with `warnings_as_errors` on stderr and empty stdout. `__schema`'s
error envelope gains `known_codes`, including `warnings_as_errors`.

## Technical Context

**Language/Version**: Go 1.27.1

**Primary Dependencies**: Cobra, existing contract and schema packages

**Storage**: N/A

**Testing**: `go test` table tests, golden files, `ExampleNewEnvelope`

**Target Platform**: library and CLIs that call `ax.Execute`

**Project Type**: library

**Performance Goals**: no new hot-path allocation requirement; warnings
are command-result metadata, not a per-log emit path

**Constraints**: additive envelope field; stdout empty on escalation;
deterministic warning order; contract package holds the shape

**Scale/Scope**: one flag, one struct, one execute branch, schema
metadata, integration example

**Governing ADR(s)**: N/A. ADRs are frozen. The decision is in
`research.md`.

## Constitution Check

- Stream separation: escalated runs write only stderr. Pass.
- Exit 2 for the escalated case, 0 otherwise. Pass.
- Structs, not maps, for the warning item. Pass.
- No new public subpackage. `Warning` lives in `contract` and is
  aliased at the root. Pass.
- `__schema` and goldens update in the same change. Pass.

## Project Structure

### Documentation

```text
specs/029-success-warnings/
├── spec.md
├── plan.md
├── research.md
├── tasks.md
└── checklists/requirements.md
```

### Source

```text
contract/json.go          Envelope.Warnings
contract/warnings.go      Warning, WithWarnings, known codes
json.go                   Warning alias and ax.WithWarnings
execute.go                --strict hold and escalation
internal/cli/cli.go       FlagStrict
schema/schema.go          ErrorSchemaInfo.KnownCodes
examples/integration/     warn command and goldens
```

## Post-design constitution check

No new violation. The public surface gains a field, a type, a function,
and a schema field. That is an intentional additive change, not a
removal or retype.
