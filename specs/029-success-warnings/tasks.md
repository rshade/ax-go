# Tasks: Success warnings and strict escalation

**Input**: `specs/029-success-warnings/`

**Prerequisites**: plan.md, spec.md, research.md

## Phase 1: Contract shape

- [x] T001 Add `Warning`, omitempty `Envelope.Warnings`, and
  `WithWarnings` in `contract/`, with a test that drops blanks and
  preserves order.
- [x] T002 Re-export `Warning` and `WithWarnings` from the root package.

## Phase 2: User story 1 and 2 (execute)

- [x] T003 Add `FlagStrict` and mount it from `prepareCommand`.
- [x] T004 Record warnings from root `WithWarnings` and escalate in
  `Execute` when `--strict` is set. Tests cover exit 0 without the
  flag, exit 2 with empty stdout and `warnings_as_errors` on stderr,
  success under `--strict` when there are no warnings, and masked
  byte-identity of two success runs.

## Phase 3: User story 3 (schema) and example

- [x] T005 Add `known_codes` to the error envelope in `__schema`,
  including `warnings_as_errors`, and update schema goldens.
- [x] T006 Add an integration `warn` command, goldens for success and
  `--strict`, and a README note.

## Phase 4: Gates

- [x] T007 `gofmt`, targeted tests, and `make surface-check` after the
  baseline update for the new exported symbols.
