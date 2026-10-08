---
description: "Task list for 018-richer-flag-schema"
---

# Tasks: Richer Per-Flag `__schema` Semantics

**Input**: Design documents from `specs/018-richer-flag-schema/`

**Prerequisites**: plan.md, spec.md, research.md (R1–R10), data-model.md,
contracts/declaration-api.md, contracts/schema-output.md, quickstart.md

**Tests**: REQUIRED. Constitution Principle VII (Test-First, NON-NEGOTIABLE)
and spec FR-012 (golden files) apply. In every story phase, the test tasks come
first and MUST fail for the right reason before the implementation task that
satisfies them starts.

**Organization**: Tasks are grouped by user story. US4 (P1, "declare once")
supplies the authoring path that US1–US3 each add a piece of, so each story
adds its own declaration function. The US4 phase is the end-to-end proof that
needs all three pieces.

**ADR retirement**: none. plan.md records *Governing ADR(s): N/A*, because
ADR-0003 no longer exists. No ADR deletion task is included.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1–US4, matching spec.md

## Conventions used by every task

These conventions apply to every task:

- **Annotation keys**: `github.com/rshade/ax-go/schema/example` is a flag
  annotation. `github.com/rshade/ax-go/schema/capability` and
  `github.com/rshade/ax-go/schema/capability-note` are command annotations.
- **Authoring errors** (revised 2026-10-07, Phase 8): declaration failures
  share spec 028's contract. The internal `Add…` functions return a
  `*Violation{Field, Reason}`; the public `Declare…` functions turn it into an
  `*ax.Error` with `invalid_schema_declaration`, exit 2, and
  `context {field, reason}` through 028's `declarationError`. A failed
  declaration mutates nothing. Phases 2–6 originally used an
  `ErrInvalidDeclaration` sentinel (exit 1) and `With…` names; Phase 8 replaces
  both.
- **Enum rejection**: `contract.NewError(context.Background(), "validation_error", "flag --<name>: value is not one of the allowed values", contract.WithErrorExitCode(contract.ExitValidation), contract.WithErrorContext(map[string]any{"flag": name, "allowed": allowed}), contract.WithSuggestions("--<name>=<v>"...))`.
  The raw input never appears anywhere in the envelope, neither `message` nor `context` (research.md R9).
- **Default exemption**: `enumValue.Set` accepts any value equal to the live `flag.DefValue` before the membership check (research.md R5).
- **Comments**: doc comments state contracts (inputs, outputs, errors, exit
  code, fail-closed behaviour), never narration. Every new exported identifier
  needs one, because `godoclint` enforces it.

---

## Phase 1: Setup

**Purpose**: Capture the pre-feature baseline that SC-004 is measured against.

- [X] T001 Record the SHA-256 checksums of `testdata/schema_ax.golden.json`, `testdata/schema_mcp.golden.json`, `testdata/mcp_tools_list.golden.json`, `examples/integration/testdata/schema_ax.golden.json` and `examples/integration/testdata/schema_mcp.golden.json` in a scratch file outside the repo. Confirm `make test` is green on the current branch before changing anything. The three files under `testdata/` MUST be byte-identical at the end of the feature (T047).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared conversion helpers, the sentinel, the capability vocabulary
and flag lookup. Every story depends on these.

**⚠️ CRITICAL**: No user-story work can begin until this phase is complete.

- [X] T002 [P] Write failing table-driven tests in `internal/schema/convert_test.go` for the shared helpers that T003 will create:
  - `JSONSchemaType(flagType) string`
  - `JSONSchemaArrayItemType(flagType) (string, bool)`
  - `ScalarJSON(flagType, value string) (any, bool)`: bool, int/uint at each bit size, float, string; returns false for an unparsable value or an empty string
  - `ArrayItemJSON(value, itemType, flagType string) (any, bool)`
  - `ValidateValue(flagType, value string) error`: type-checks one CLI-form value for every built-in scalar type, including `duration` via `time.ParseDuration` and integer range at the bit size (`int8` rejects `300`). Slice types split as CSV with each element checked. A custom type returns nil, meaning unchecked.
  - `CanonicalEnumValue(flagType, value string) (string, error)`: string and custom types are returned unchanged; integer types are parsed at the bit size and re-formatted base 10, so `"03"` becomes `"3"`.
- [X] T003 Create `internal/schema/convert.go`. Move the conversion logic of `jsonSchemaType`, `jsonSchemaArrayItemType`, `jsonSchemaScalarDefault` and `convertArrayDefault` out of `internal/mcp/mcp.go` and into the T002 helpers, then rewrite `internal/mcp/mcp.go` to call them. Done when T002 passes and every existing test in `internal/mcp/` (`input_schema_test.go`, `mcp_test.go`) passes **unmodified**. Keep behaviour identical: `testdata/schema_mcp.golden.json` must not change.
- [X] T004 Add `FuzzEnumCanonicalise` in `internal/schema/declare_fuzz_test.go`. For random `(flagType ∈ {string,int,int8,uint16,int64,uint}, value)` pairs it asserts:
  - no panic;
  - when `CanonicalEnumValue` succeeds, canonicalising its output again returns the same string (idempotent);
  - for integer types, the canonical form parses back to the same number.

  Seed the corpus with `""`, `"03"`, `"-0"`, `"+5"`, `"300"` and `"9223372036854775808"`.
- [X] T005 Create `internal/schema/declare.go` with:
  - the three annotation-key constants;
  - `var ErrInvalidDeclaration = errors.New("schema: invalid declaration")`;
  - `IsCapability(class string) bool`, an exhaustive `switch` over the six vocabulary strings `read-only`, `create`, `mutate`, `delete`, `external-network` and `admin`;
  - `lookupFlag(cmd *cobra.Command, name string) (*pflag.Flag, error)`, which checks `cmd.Flags()` and then `cmd.PersistentFlags()` and returns an error wrapping `ErrInvalidDeclaration` that names the command path when `cmd` is nil or the flag is missing.

  Add table tests for `IsCapability` and `lookupFlag` in `internal/schema/declare_test.go` first.
- [X] T006 [P] Create `schema/declare.go` with:
  - `type Capability string`;
  - the six constants `CapabilityReadOnly`, `CapabilityCreate`, `CapabilityMutate`, `CapabilityDelete`, `CapabilityExternalNetwork` and `CapabilityAdmin`;
  - `var ErrInvalidDeclaration = internalschema.ErrInvalidDeclaration`.

  Write each doc comment as a contract, taking the "Meaning an agent may rely
  on" column of data-model.md for the constants. Add an import-isolation check:
  `schema/import_isolation_test.go` must still pass.

**Checkpoint**: The shared helpers and vocabulary exist. `go test -race ./internal/... ./schema/...` is green and `testdata/schema_mcp.golden.json` is unchanged.

---

## Phase 3: User Story 1 - Agent picks a valid value on the first try (Priority: P1) 🎯 MVP

**Goal**:

- An author declares an allowed-value set with `DeclareFlagEnum`.
- The set appears in `__schema` as `enum` (CLI strings, author order) and in
  `--as=mcp` as a typed JSON-Schema `enum`.
- Any value outside the set is rejected at parse time with `validation_error`
  and exit code 2, before `PersistentPreRunE` or `RunE`, including under
  `--dry-run`, through both `ax.Execute` and `mcp-server`.

**Independent Test**: Build a CLI with an enum-constrained flag. Run `__schema`
and `__schema --as=mcp` twice each and diff them. Invoke the command with an
out-of-set value, once plain and once with `--dry-run`, and assert exit 2, an
empty stdout, one envelope on stderr, and that `RunE` was never entered.

### Tests for User Story 1 ⚠️ write first, confirm they fail

- [X] T007 [P] [US1] Add `TestDeclareFlagEnum` in `internal/schema/declare_test.go`, table-driven.

  **Success cases**:
  - a string flag;
  - an `int` flag;
  - a `uint8` flag;
  - a custom `pflag.Value` with `Type()` `"format"`;
  - a persistent flag declared on its owner;
  - an empty string default, which is exempt from the membership check.

  **Error cases**, each asserting `errors.Is(err, ErrInvalidDeclaration)`:
  - nil cmd;
  - a missing flag;
  - a `bool` flag;
  - a `count` flag;
  - a `float64` flag;
  - a `duration` flag;
  - a `stringSlice` flag;
  - empty `values`;
  - `"x"` on an int flag;
  - `"300"` on an `int8` flag;
  - duplicates `"3"` and `"03"` on an int flag;
  - a non-empty default outside the set;
  - an example declared earlier that is outside the set.

  **Atomicity**: after a failed call, `flag.Value` is the same pointer as
  before. **Re-declaration**: a second call replaces the set and does not wrap
  the value twice, so the inner value is not an `enumValue`.
- [X] T008 [P] [US1] Add `TestEnumValueSet` in `internal/schema/enum_value_test.go`:
  - A member value calls the inner `Set`, so the bound variable updates.
  - `"03"` is accepted for a `"3"` member on an int flag.
  - A non-member returns `*contract.Error` with `ErrorCode` `validation_error` and `ExitCode()` 2. Its `Context` holds exactly `flag` and `allowed` (in declared order), with **no** `value` key. `Suggestions` holds `--<flag>=<v>` for each member in order. `Message` does not contain the raw input. The bound variable is **unchanged**.
  - `String()` and `Type()` delegate to the inner value.
  - The live default is always accepted: with `DefValue` `""` and a set that excludes `""`, `Set("")` succeeds and updates the inner value. This guards the MCP dispatcher's reset (C1).
  - Through `pflag.FlagSet.Parse`, the error is an `*pflag.InvalidValueError` for which `errors.As(err, **contract.Error)` succeeds.
- [X] T009 [P] [US1] Add `TestCollectFlagsEnum` in `internal/schema/schema_test.go`:
  - `Flag.Enum` follows the author's order;
  - it is nil when no enum is declared;
  - a persistent enum flag appears exactly once, with the enum, on a child's `CollectFlags` (FR-010);
  - **fail-closed**: when the `DefValue` is mutated to a non-member after declaration, `Enum` is nil;
  - **fail-closed**: when `flag.Value` is re-wrapped by a foreign `pflag.Value`, `Enum` is nil.
- [X] T010 [P] [US1] Add `TestInputSchemaEnum` in `internal/mcp/input_schema_test.go`:
  - a string flag produces `[]any{"json","table"}`;
  - an `int` flag produces `[]any{int64(1),int64(3)}`;
  - a `uint` flag produces `uint64` values;
  - there is no `enum` key when none is declared;
  - an inherited persistent flag on a child carries the enum.
- [X] T011 [P] [US1] Add `TestExecuteRejectsOutOfSetEnum` in `execute_test.go`, table-driven over these cases:
  - plain out-of-set;
  - out-of-set with `--dry-run`;
  - an out-of-set value on a parent's persistent flag, passed to a child command;
  - a non-canonical int member (`--n=03`), which **succeeds**.

  For each rejection, assert: exit code 2; stdout is empty; stderr holds
  exactly one minified envelope with `error_code` `validation_error` and the
  expected `context` (no `value` key) and `suggestions`; that the raw input appears nowhere in stderr; and that counters show neither the
  author's `PersistentPreRunE` nor `RunE` ran. Run the same rejection twice
  and assert the envelopes are byte-equal after masking `trace_id` (FR-007,
  SC-007).
- [X] T012 [P] [US1] Add `TestDispatchRejectsOutOfSetEnum` in `internal/mcpserver/dispatch_test.go`:
  - `tools/call` with an out-of-set value returns `IsError: true`;
  - the decoded envelope (`decodeErrorEnvelope`) has `error_code` `validation_error` and keeps `context` and `suggestions`, not a re-wrapped pflag message;
  - the command body never runs;
  - a later valid call on the same dispatcher succeeds, so flag reset still works with the wrapper;
  - **reset regression (C1)**: on a flag whose default is `""` and whose set excludes `""`, a call with `--output=table` followed by a call that omits `--output` must see `""` in the second call, not `table`;
  - the advertised tool `inputSchema` holds the `enum`.
- [X] T013 [P] [US1] Add `TestWithFlagEnum` in `schema/declare_test.go`:
  - the public function succeeds and its errors satisfy `errors.Is(err, schema.ErrInvalidDeclaration)`;
  - `BuildSchema` emits `FlagSchema.Enum`;
  - `BuildMCPSchema` emits a typed enum;
  - two `BuildSchema` and `WriteJSON` runs are byte-identical.

  In root `schema_test.go`, add a test that `errors.Is(err, ax.ErrInvalidDeclaration)` holds for an error from `ax.DeclareFlagEnum`.

### Implementation for User Story 1

- [X] T014 [US1] In `internal/schema/declare.go`, implement:
  - the unexported `enumValue{inner pflag.Value; flag *pflag.Flag; flagType string; allowed, canonical []string}`, whose `Set` first accepts any value equal to `flag.DefValue`, otherwise canonicalises with `CanonicalEnumValue`, checks membership, and only then calls `inner.Set`, and whose `String` and `Type` delegate to `inner`;
  - `DeclareFlagEnum(cmd *cobra.Command, name string, values []string) error`, which runs the validation order in data-model.md (checks 1–7) before mutating, re-uses the wrapper on re-declaration, and builds the rejection error exactly as the Conventions section specifies.

  Make T007 and T008 pass.
- [X] T015 [US1] In `internal/schema/schema.go`, add `Enum []string` to `Flag` and an exported `FlagEnum(*pflag.Flag) []string` reader (exported so `internal/mcp` can share it). It type-asserts `*enumValue`, returns a copy of `allowed`, and returns nil if a non-empty `DefValue` no longer canonicalises to a member. Populate it in `CollectFlags`. Done when T009 passes. A flag without an enum must add zero allocations: one type assertion only.
- [X] T016 [US1] In `internal/mcp/mcp.go`, have `flagProperty` call `internalschema.FlagEnum` and emit `"enum"` as a `[]any` converted per element with `ScalarJSON`. Omit the key when the enum is nil or when any element fails to convert. Make T010 pass.
- [X] T017 [US1] In `internal/mcpserver/dispatch.go`, change the `root.SetFlagErrorFunc` closure in `newDispatcher` so that when `errors.As(ferr, &contractErr)` finds a `*contract.Error`, it returns that error unchanged. Otherwise it keeps the existing `d.validationError(...)` path. Make T012 pass.
- [X] T018 [US1] In `schema/schema.go`, add `` Enum []string `json:"enum,omitempty"` `` to `FlagSchema`, after `Required`, and map it in `convertFlagSchemas`. In `schema/declare.go`, add `DeclareFlagEnum(cmd *cobra.Command, flag string, values ...string) error`, which forwards to `internalschema.DeclareFlagEnum`. Its doc comment states:
  - that enforcement happens at parse time, before `PersistentPreRunE` and `RunE`, including under dry-run;
  - that the rejection is exit 2;
  - the supported types;
  - that an empty default is exempt;
  - that re-declaration replaces the set;
  - that `flag.Value` is no longer the concrete pflag type.

  Make T013 pass.
- [X] T019 [US1] In root `schema.go`, add `var ErrInvalidDeclaration = isolatedschema.ErrInvalidDeclaration` and `func DeclareFlagEnum(cmd *cobra.Command, flag string, values ...string) error`, which forwards. Confirm T011 now passes. **No change to `execute.go`**, per research.md R1. If T011 fails, diagnose the error chain; do not add a special case in `Execute`.
- [X] T020 [US1] Add `ExampleWithFlagEnum` in `schema/example_test.go`. It declares an enum, writes `BuildSchema(...).Command.Flags` for that flag with `contract.WriteJSON`, and has a verified `// Output:` line.

**Checkpoint**: US1 is fully functional. `go test -race ./...` is green. `testdata/schema_*.golden.json` are unchanged.

---

## Phase 4: User Story 2 - Agent learns the exact input shape from an example (Priority: P2)

**Goal**: `DeclareFlagExample` attaches one CLI-form example. It appears as
`example` in `__schema` and as a typed one-element `examples` array in
`--as=mcp`. A value that doesn't fit the type, or is outside the flag's enum, is
an authoring error.

**Independent Test**: Declare an example on a `duration` flag and on a
`stringSlice` flag. Confirm both outputs carry it, that other flags omit it, and
that two runs are byte-identical.

### Tests for User Story 2 ⚠️ write first, confirm they fail

- [X] T021 [P] [US2] Add `TestDeclareFlagExample` in `internal/schema/declare_test.go`.

  **Success cases**:
  - `"svc-a"` on a string flag;
  - `"5"` on an int flag;
  - `"45s"` on a duration flag;
  - `"a,b"` on a stringSlice flag;
  - `"1,2"` on an intSlice flag;
  - anything on a custom type, which is unchecked;
  - a member of a declared enum.

  **Error cases**, each wrapping `ErrInvalidDeclaration`:
  - nil cmd;
  - a missing flag;
  - `""`;
  - `"three"` on an int flag;
  - `"abc"` on a duration flag;
  - `"1,x"` on an intSlice flag;
  - a value outside a declared enum.

  **Re-declaration** replaces the example. **Atomicity**: a failed call leaves
  the annotation unchanged.
- [X] T022 [P] [US2] Add `TestCollectFlagsExample` in `internal/schema/schema_test.go`:
  - `Flag.Example` is set;
  - it is empty when no example is declared;
  - an inherited persistent flag carries it on a child.

  **Fail-closed**: the field is empty when the annotation has 0 or 2
  elements, holds `""`, or was hand-edited to a value that fails
  `ValidateValue` or the enum.
- [X] T023 [P] [US2] Add `TestInputSchemaExamples` in `internal/mcp/input_schema_test.go`:
  - an int flag produces `"examples": []any{int64(5)}`;
  - a duration flag produces `[]any{"45s"}`;
  - a stringSlice flag produces `[]any{[]any{"a","b"}}`;
  - there is no `examples` key when none is declared.
- [X] T024 [P] [US2] Add `TestWithFlagExample` in `schema/declare_test.go`. Through the public function, it checks `errors.Is` against `schema.ErrInvalidDeclaration`, that `FlagSchema.Example` is set, that the MCP `examples` array is present, and that output is deterministic.

### Implementation for User Story 2

- [X] T025 [US2] In `internal/schema/declare.go`, add `DeclareFlagExample(cmd, name, example string) error`, which runs the data-model.md checks 1–4 and writes the annotation `[]string{example}`. In `internal/schema/schema.go`, add `Example string` to `Flag` and a fail-closed exported `FlagExample(*pflag.Flag) string` reader, and populate it in `CollectFlags`. Make T021 and T022 pass.
- [X] T026 [US2] In `internal/mcp/mcp.go`, make `flagProperty` emit `"examples"`. A scalar uses `ScalarJSON`. A slice is split as CSV, converted per element with `ArrayItemJSON`, and wrapped in a one-element array. Omit the key when conversion fails. Make T023 pass.
- [X] T027 [US2] In `schema/schema.go`, add `` Example string `json:"example,omitempty"` `` to `FlagSchema`, after `Enum`, and map it. In `schema/declare.go`, add `DeclareFlagExample(cmd *cobra.Command, flag string, example string) error` with a contract doc comment: CLI form, CSV for slices, custom types unchecked, must be an enum member, last call wins. In root `schema.go`, add the forwarding `DeclareFlagExample`. Make T024 pass.
- [X] T028 [US2] Add `ExampleWithFlagExample` in `schema/example_test.go`, with a verified `// Output:`.

**Checkpoint**: US1 and US2 both work independently. The existing goldens are unchanged.

---

## Phase 5: User Story 3 - Agent classifies a command's side effects before calling (Priority: P3)

**Goal**: `DeclareCapability` records one class from the vocabulary and an
optional note. They appear as `capability` on the command node in `__schema`,
and as `capability` plus the standard `annotations` hints on the `--as=mcp`
tool. The live `mcp-server` tool carries the hints. Hidden and reserved
commands stay excluded.

**Independent Test**: Classify a command, then check both outputs, an
unclassified command (field absent), and a hidden command and `__schema` that
both carry a declaration but are still not tools.

### Tests for User Story 3 ⚠️ write first, confirm they fail

- [X] T029 [P] [US3] Add `TestDeclareCapability` in `internal/schema/declare_test.go`:
  - each of the six classes is accepted;
  - `""`, `"Read-Only"`, `" mutate"`, `"write"` and a nil cmd each wrap `ErrInvalidDeclaration`, and a failed call leaves the annotations unchanged;
  - the note is trimmed, and a note that is empty after trimming deletes the note key;
  - re-declaration replaces both the class and the note.

  Add `TestCommandCapability` for the reader: it returns `(class, note, ok)`,
  and returns `ok=false` and no note when the annotation holds a class outside
  the vocabulary.
- [X] T030 [P] [US3] Add `TestBuildToolCapability` in `internal/mcp/mcp_test.go`:
  - a table over all six classes asserts `Tool.Capability` and the `Tool.Annotations` hints exactly as in the research.md R6 table;
  - an unclassified command has a nil `Capability` and nil `Annotations`;
  - a hidden command, and a command named `__schema`, `mcp-server` or `completion`, that carries a declaration is absent from `Build` (FR-011).
- [X] T031 [P] [US3] Add a `tools/list` test in `internal/mcpserver/server_test.go`. Each classified tool's `Annotations` equal the R6 mapping (`ReadOnlyHint`, `DestructiveHint`, `OpenWorldHint`), and an unclassified tool has nil `Annotations`.
- [X] T032 [P] [US3] Add `TestWithCapability` in `schema/declare_test.go`:
  - `CommandSchema.Capability` holds `{class, note}`;
  - `MCPTool.Capability` and `MCPTool.Annotations` are set;
  - both are nil when undeclared;
  - JSON field order matches contracts/schema-output.md §1;
  - `ax.CapabilityMutate == schema.CapabilityMutate` for each constant, asserted in root `schema_test.go`.

### Implementation for User Story 3

- [X] T033 [US3] In `internal/schema/declare.go`, add `DeclareCapability(cmd *cobra.Command, class, note string) error` and the fail-closed reader `CommandCapability(annotations map[string]string) (class, note string, ok bool)`. In `internal/schema/schema.go`, `Command` keeps reading from `Annotations`, which `BuildCommand` already clones, so no new field is needed. Make T029 pass.
- [X] T034 [US3] In `internal/mcp/mcp.go`, add these fields to `Tool`: `CapabilityClass string`, `CapabilityNote string`, and `Hints *Hints`, where `Hints{ReadOnly bool; Destructive, OpenWorld *bool}`. Add the `capabilityHints(class string) *Hints` mapping exactly per R6, and populate the fields in `BuildTool` through `internalschema.CommandCapability`. Make T030 pass.
- [X] T035 [US3] In `schema/schema.go`, add:
  - `` CapabilitySchema{Class Capability `json:"class"`; Note string `json:"note,omitempty"`} ``;
  - `` MCPToolAnnotations{ReadOnlyHint bool `json:"readOnlyHint,omitempty"`; DestructiveHint *bool `json:"destructiveHint,omitempty"`; OpenWorldHint *bool `json:"openWorldHint,omitempty"`} ``;
  - `` CommandSchema.Capability *CapabilitySchema `json:"capability,omitempty"` ``, placed after `Commands` and before `NonDeterministicFields`;
  - `` MCPTool.Capability *CapabilitySchema `json:"capability,omitempty"` `` and `` MCPTool.Annotations *MCPToolAnnotations `json:"annotations,omitempty"` ``, after `NonDeterministicFields`.

  Map them in `convertCommandSchema` and `BuildMCPSchema`. In
  `schema/declare.go`, add `DeclareCapability(cmd *cobra.Command, class
  Capability, note string) error` with a contract doc comment. The schema
  package must **not** import the MCP SDK. Make T032 pass.
- [X] T036 [US3] In `internal/mcpserver/server.go`:
  - `discoverTools` copies the new `schema.MCPTool` fields from `internalmcp.BuildTool`;
  - `newMCPServer` sets `sdk.Tool.Annotations` to `&sdk.ToolAnnotations{ReadOnlyHint: ..., DestructiveHint: ..., OpenWorldHint: ...}` when hints are present, and leaves it nil otherwise.

  The ax `capability` object is **not** put into `_meta` (deferred, R6). Make
  T031 pass.
- [X] T037 [US3] In root `schema.go`, add:
  - `type Capability = isolatedschema.Capability`;
  - the six typed constants (`const CapabilityReadOnly = isolatedschema.CapabilityReadOnly`, and so on);
  - `type CapabilitySchema = isolatedschema.CapabilitySchema`;
  - `type MCPToolAnnotations = isolatedschema.MCPToolAnnotations`;
  - the forwarding `DeclareCapability`.
- [X] T038 [US3] Add `ExampleWithCapability` in `schema/example_test.go`. It prints the command node's `capability` JSON with a verified `// Output:`.

**Checkpoint**: US1–US3 all work independently.

---

## Phase 6: User Story 4 - CLI author declares the semantics once (Priority: P1)

**Goal**: An end-to-end proof that one authoring idiom feeds every surface. It
shows that `default` and `required` are still derived without being declared
again, that a CLI declaring nothing is byte-identical to the pre-feature output,
and that the integration example demonstrates the API.

**Independent Test**: The enriched golden files cover every field. The original
golden files match their T001 checksums.

### Tests for User Story 4 ⚠️ write first, confirm they fail

- [X] T039 [P] [US4] In `schema/schema_test.go`, add `newEnrichedSchemaTestCommand()`. Its root has:
  - a persistent `--region` string flag, default `"us"`, with enum `us,eu` and example `eu`;
  - a local `--config` flag.

  Its child `deploy` has:
  - a required `--output` string flag, default `"json"`, with enum `json,table,yaml`;
  - an `--replicas` int flag, default `1`, with enum `1,3,5`;
  - a `--tags` stringSlice flag with example `a,b`;
  - a `--timeout` duration flag with example `45s`;
  - capability `mutate` with note `idempotent by release name`.

  It also has a child `status` with capability `read-only` and no note, and a
  hidden child `secret` with capability `admin`.

  Add `TestBuildSchemaEnrichedGolden` and `TestBuildMCPSchemaEnrichedGolden`,
  asserting `../testdata/schema_ax_enriched.golden.json` and
  `../testdata/schema_mcp_enriched.golden.json`, plus a two-run byte-equality
  check. Mirror both tests in root `schema_test.go` through the `ax` facade
  against `testdata/…_enriched.golden.json`.
- [X] T040 [P] [US4] Add `TestDerivedFieldsSurviveEnumDeclaration` in `schema/declare_test.go` (US4-AS3, FR-006). A required flag with a default, given only `DeclareFlagEnum`, still shows `default`, `required: true` and `enum` in `__schema`. In `--as=mcp`, the flag is still listed in `inputSchema.required` and carries a typed `default`.
- [X] T041 [P] [US4] In `buildtags_parity_test.go`, add assertions that the enriched goldens hold under every build configuration. The file is untagged, per the AGENTS.md parity rule.

### Implementation for User Story 4

- [X] T042 [US4] Generate `testdata/schema_ax_enriched.golden.json` and `testdata/schema_mcp_enriched.golden.json` from the T039 fixture. **Review them by hand** against contracts/schema-output.md:
  - enum order is the author's order;
  - MCP enums are typed;
  - `examples` is an array, with an array inside it for `--tags`;
  - `capability` has no `note` for `status`;
  - `annotations` follow R6;
  - `secret` is absent from the MCP tools;
  - the inherited `--region` appears on `deploy` exactly once.

  Make T039, T040 and T041 pass.
- [X] T043 [US4] Update `examples/integration/main.go` so it demonstrates the API:
  - `--count` on `stream` gets enum `1,2,3,5,10`, which keeps the default `3` and the values `2` and `3` that existing tests use;
  - `--patch` on `patch-config` gets example `[{"op":"replace","path":"/name","value":"Ada"}]`;
  - capabilities: root `read-only`; `stream` `read-only`; `patch-config` `mutate` with note `rewrites the file in place, preserving comments`; `fetch` `external-network`; `fail`, `authz` and `crash` `read-only`.

  Every declaration error must propagate. Change `newRootCommand` to return `(*cobra.Command, error)` and have `runWithEntityID` map a declaration error to exit code 1 on stderr through `ax.WriteError`.
- [X] T044 [US4] In `examples/integration/main_test.go`, add `TestStreamRejectsOutOfSetCount`. It runs `stream --count=4`, with and without `--dry-run`, and asserts exit 2, an empty stdout, and an envelope holding `context.allowed` `["1","2","3","5","10"]`, no `context.value`, and the five suggestions.
- [X] T045 [US4] Regenerate `examples/integration/testdata/schema_ax.golden.json` and `schema_mcp.golden.json`, then review the diff. It must contain **only** additions: `enum`, `example`/`examples`, `capability` and `annotations`. Update `examples/integration/README.md` with a short section showing the enum rejection (`stream --count=4`) and the new `__schema` fields, then run markdownlint on it.

**Checkpoint**: All four stories are verified end to end.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T046 [P] Update `README.md`. Extend the `__schema` section with the three declaration functions, the capability vocabulary table, the enum-rejection envelope (exit 2, before dry-run), and the new output fields, linking `specs/018-richer-flag-schema/contracts/`. Update `AGENTS.md` → Core AX Mandates → the `__schema` bullet with one sentence on the declaration idiom and parse-time enum enforcement. Run `npm run lint:md`.
- [ ] T047 Verify SC-004 and FR-008. `testdata/schema_ax.golden.json`, `testdata/schema_mcp.golden.json` and `testdata/mcp_tools_list.golden.json` must match their T001 checksums byte for byte. Also run `git diff --exit-code` on all three. A mismatch is a defect in the omitempty or nil handling: fix it, do not regenerate.
- [ ] T048 Run `make surface-update`, then review `git diff internal/cmd/surfacecheck/baseline.json`. Every line must be an **addition** naming an identifier or field from contracts/declaration-api.md or data-model.md, in `schema` or `ax`, with presence `"all"`. Any removed or changed line is a defect.
- [ ] T049 Append retained audit rows for every new **root-package** feature from T048 to `specs/015-internalize-helpers/public-surface-audit.json`:
  - `DeclareFlagEnum`, `DeclareFlagExample`, `DeclareCapability` and `ErrInvalidDeclaration`;
  - `Capability` and its six constants;
  - `CapabilitySchema` and `MCPToolAnnotations` with their fields;
  - the new `FlagSchema`, `CommandSchema` and `MCPTool` fields.

  Each row is classified `supported`, in lifecycle state `live`, with a
  one-line rationale citing spec 018. Seed the rows with `go run
  ./internal/cmd/surfacecheck -audit-seed`, then classify them by hand. Run
  `make surface-check` until it reports `"status":"pass"`.
- [ ] T050 Run `make bench-check`. `BenchmarkBuildCommand` must stay within budget (ns/op ≤ +5%, allocs/op ≤ +1). If it does not, remove the extra allocation on the no-declaration path; do not adjust the budget.
- [ ] T051 Run `make cover-check`. Every floor must hold, notably `internal/schema` 93%, `internal/mcp` 96.9%, root 85% and `examples/integration` 85%. Add tests if a floor slips; never lower one.
- [ ] T052 Run `make doc-coverage`. The required list is unchanged, and the three new `ExampleWithX` functions run with verified output.
- [ ] T053 Run the full gate: `gofmt -l .` must print nothing, then `make test` (all 4 build-tag configurations, `-race`), `make lint` (all 4 configurations), `go vet ./...`, `make validate` and `make size-check`. The logging probe must be unaffected, and `go list -deps ./examples/logging` must not newly include `schema`.
- [ ] T054 Run the verification commands in `specs/018-richer-flag-schema/quickstart.md`, including `go test -run '^$' -fuzz FuzzEnumCanonicalise -fuzztime 30s ./internal/schema`, and confirm each expected outcome listed there.
- [ ] T055 Remove debug code and make sure no `TODO` remains in the changed files. Confirm `CHANGELOG.md` is **not** modified, because release-please owns it. Draft a Conventional Commit subject, `feat(schema): add per-flag enum/example and command capability class to __schema (#28)`, and validate it with `npx commitlint`.

---

## Phase 8: Align with spec 028 (authoring-error contract and naming)

**Purpose**: Spec 028 (`DeclarePrompt`, `DeclareResource`) landed on main
first with `invalid_schema_declaration` / exit 2 and `Declare…` names. Ship
one contract and one naming style for every fallible declaration (research.md
R4 and R10, revised).

- [X] T056 Rewrite the authoring-error tests first: internal tests assert the returned `*Violation` (`Field`, `Reason`) for every case in contracts/declaration-api.md's reason table; public `schema` and root tests assert `errors.As` to `*contract.Error`, `ErrorCode` `invalid_schema_declaration`, `ExitCode()` 2, and `Context` `{field, reason}`. Confirm they fail.
- [X] T057 In `internal/schema`, rename `DeclareFlagEnum` / `DeclareFlagExample` / `DeclareCapability` to `AddFlagEnum` / `AddFlagExample` / `AddCapability` returning `*Violation`; add the reasons `flag_not_found`, `unsupported_type`, `invalid_value`, `not_in_enum` and `not_in_vocabulary` beside 028's; delete `ErrInvalidDeclaration`.
- [X] T058 In `schema` and root `ax`, rename the public functions to `DeclareFlagEnum` / `DeclareFlagExample` / `DeclareCapability`, route failures through `declarationError` (kinds `flag enum`, `flag example`, `capability`), and remove `ErrInvalidDeclaration` from both packages. Rename the three `ExampleWith…` functions to `ExampleDeclare…`.
- [X] T059 Update `examples/integration`, README.md, AGENTS.md and `examples/integration/README.md` to the new names and contract.
- [ ] T060 Re-run `make surface-update` and replace the spec 018 audit rows (drop `ErrInvalidDeclaration`, rename the three functions), then the full gate (T047–T054) on the mise-pinned toolchain.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: no dependencies.
- **Foundational (T002–T006)**: depends on T001. It BLOCKS every story. Within
  the phase, T003 depends on T002, and T005's tests come before its
  implementation.
- **US1 (T007–T020)**: depends on Phase 2. It is the MVP.
- **US2 (T021–T028)**: depends on Phase 2. It shares the cross-check with US1:
  "example must be an enum member" is tested in both T007 and T021, so if US2
  ships before US1, the enum half of T021 is deferred.
- **US3 (T029–T038)**: depends on Phase 2 only, and is independent of US1 and
  US2.
- **US4 (T039–T045)**: depends on US1, US2 and US3. It is the end-to-end proof
  that needs all three declarations. Its priority is P1, but it is verified
  last.
- **Polish (T046–T055)**: depends on every story. T048 → T049 → T053 run in
  sequence.

### Within Each Story

Tests ([P]) come first and must fail. Then the implementation runs in this
order: `internal/schema`, then `internal/mcp`, then `internal/mcpserver`, then
public `schema`, then root `ax` and the example. Each story ends at a green
`go test -race ./...` checkpoint.

### Parallel Opportunities

- Phase 2: T002 and T006 can run in parallel. T004 calls `CanonicalEnumValue`,
  so it runs after T003.
- US1 tests: T007–T013 all touch different files and can run in parallel.
- US2 tests: T021–T024 can run in parallel.
- US3 tests: T029–T032 can run in parallel.
- US2 and US3 can be implemented in parallel after Phase 2, but both edit
  `internal/schema/declare.go`, `internal/mcp/mcp.go`, `schema/schema.go`,
  `schema/declare.go` and root `schema.go`. Serialize their implementation
  tasks, or merge carefully.
- Examples: T020, T028 and T038 all edit `schema/example_test.go`, so none is
  marked [P]; run them one at a time.

---

## Parallel Example: User Story 1

```bash
# All US1 contract tests at once (distinct files):
Task: "T007 TestDeclareFlagEnum in internal/schema/declare_test.go"
Task: "T008 TestEnumValueSet in internal/schema/enum_value_test.go"
Task: "T009 TestCollectFlagsEnum in internal/schema/schema_test.go"
Task: "T010 TestInputSchemaEnum in internal/mcp/input_schema_test.go"
Task: "T011 TestExecuteRejectsOutOfSetEnum in execute_test.go"
Task: "T012 TestDispatchRejectsOutOfSetEnum in internal/mcpserver/dispatch_test.go"
Task: "T013 TestWithFlagEnum in schema/declare_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 → Phase 2.
2. Phase 3 (US1): enum declaration, parse-time enforcement, and enum output on
   both surfaces.
3. **Stop and validate**: run the independent test and T047, which confirms the
   existing goldens are unchanged. This alone removes the most common first-try
   failure.

### Incremental Delivery

1. US1 → validate. This is shippable as `feat(schema): enum`.
2. US2 (examples) → validate.
3. US3 (capability class) → validate.
4. US4: enriched goldens and the integration example → Polish → PR.

All four stories ship as one PR on this branch unless they are deliberately
split. Every increment is additive and needs no `breaking-change-approved`
label.

---

## Notes

- The pre-existing golden files are a **regression proof**. Never regenerate
  them in this feature.
- Do not add a special case to `ax.Execute` for enum errors (R1). The
  `errors.As` chain through `pflag.InvalidValueError` is the design.
- Out of scope, follow-up candidates only: an enum on ax-go's own `--format`
  flag, exit-code mapping for non-ax flag-parse errors, ax `capability` in the
  live server's `_meta`, multi-class capabilities, and multiple examples per
  flag.
