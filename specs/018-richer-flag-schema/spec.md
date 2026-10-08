# Feature Specification: Richer Per-Flag `__schema` Semantics

**Feature Branch**: `018-richer-flag-schema`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "Richer per-flag __schema semantics (defaults/enums/examples). `__schema` lists flags, types, and examples but does not commit to per-flag semantics an agent needs to construct a correct invocation without trial-and-error: defaults, enums/allowed-values, required-vs-optional, per-flag examples, and a side-effect / capability class."

## Overview

An LLM agent grounds itself in a CLI by reading its `__schema` output. Today that
output already tells the agent each flag's name, type, default, usage text, and
whether it is required, and the `--as=mcp` adapter already advertises those as an
MCP-compatible input schema. What it does **not** tell the agent is which values a
flag will actually accept, what a well-formed value looks like, or whether calling
a command changes the world. The agent learns those facts the expensive way: it
guesses, the CLI rejects the guess with a validation error, and the agent tries
again — the exact trial-and-error loop the AX contract exists to eliminate.

This feature enriches the per-flag and per-command schema so an agent can infer a
correct, side-effect-aware invocation from the contract alone:

- **Allowed values (enums)** for flags that accept only a fixed set — advertised in
  the schema and **enforced at parse time**, so an out-of-set value is rejected with
  a structured error before the command runs.
- **A per-flag example** for flags whose valid shape is not obvious from the type.
- **A command-level capability / side-effect class** — a class drawn from a fixed
  vocabulary plus an optional free-form note — so the agent knows whether a command
  reads, creates, mutates, or deletes before it calls.

Note: `default` and `required` — named in the original issue — are **already
emitted** today (derived automatically from the CLI framework). This feature does
not re-add them; it closes the remaining gaps and keeps deriving the automatic
fields without author re-declaration.

## Clarifications

### Session 2026-07-24

- Q: Capability-class fixed vocabulary — which members? → A: `read-only`,
  `create`, `mutate`, `delete`, `external-network`, `admin` (6 classes; `admin`
  is a distinct privileged/administrative class).
- Q: What does `__schema` report for a command with no capability declaration?
  → A: Omit the class field; absence means "unclassified" (no default asserted),
  preserving backward compatibility for existing CLIs.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agent picks a valid value on the first try (Priority: P1)

An agent must invoke a command with a flag that accepts only a fixed set of values
(for example, an output format that is one of `json`, `table`, or `yaml`). The CLI
author has declared that allowed set. The agent reads `__schema`, sees the
permitted values for the flag, and constructs a valid invocation immediately —
without submitting an invalid value, receiving a validation error, and retrying.
The same allowed set appears in the `--as=mcp` output as a standard JSON-Schema
`enum`, so an MCP client constrains the argument the same way. As a safety net, if
any caller still passes an out-of-set value, ax-go rejects it deterministically
before the command runs — a structured validation error, never a partial side
effect.

**Why this priority**: Invalid enum-style values are the single most common
first-try failure and the highest-leverage discoverability win called out in the
AX audit. Eliminating them removes an entire class of validation-error round trips
and is independently valuable even if nothing else in this feature ships.

**Independent Test**: Build a CLI whose command has a flag constrained to a fixed
value set, declare that set, and run both `__schema` and `__schema --as=mcp`.
Confirm the allowed set appears in both outputs, that a flag with no declared set
omits it entirely, that two runs produce byte-identical output, and that invoking
the command with an out-of-set value is rejected with the validation exit code and
no side effect.

**Acceptance Scenarios**:

1. **Given** a flag with a declared allowed-value set, **When** an agent reads
   `__schema`, **Then** the flag entry lists exactly those permitted values in a
   deterministic order.
2. **Given** the same flag, **When** an agent reads `__schema --as=mcp`, **Then**
   the flag's input-schema property carries a JSON-Schema `enum` with the same
   values.
3. **Given** a flag with no declared allowed-value set, **When** an agent reads
   either output, **Then** no allowed-value field is present for that flag (the
   field is absent, not an empty list).
4. **Given** a flag with a declared allowed-value set, **When** a caller passes a
   value outside that set, **Then** ax-go rejects it before the command's action
   runs, emitting the `ax.Error` envelope on `stderr` with the validation exit code
   (2), naming the offending flag and the permitted set, and causing no side effect.
5. **Given** the same out-of-set value **and** `--dry-run`, **When** the caller
   invokes the command, **Then** the value is still rejected — enum validation
   precedes side-effect suppression, so a bad value never yields a "successful"
   dry-run envelope.

---

### User Story 2 - Agent learns the exact input shape from an example (Priority: P2)

An agent must supply a value whose type alone is ambiguous — a duration like `30s`,
an RFC 3339 timestamp, a resource-ID format, or a comma-separated list. The type
says "string"; the example says *which* string. The CLI author has attached a
concrete example value to the flag. The agent reads `__schema`, sees the example,
and formats its value correctly without trial-and-error. The example is also
reflected into the `--as=mcp` input schema.

**Why this priority**: Examples resolve format ambiguity that types cannot express,
but the win is narrower than enums because many flags are self-evident from their
type and usage text. Valuable, and independently shippable, after P1.

**Independent Test**: Declare a per-flag example on a format-ambiguous flag, run
`__schema` and `__schema --as=mcp`, and confirm the example appears in both, that
flags without an example omit the field, and that output stays deterministic.

**Acceptance Scenarios**:

1. **Given** a flag with a declared example value, **When** an agent reads
   `__schema`, **Then** the flag entry carries that example.
2. **Given** the same flag, **When** an agent reads `__schema --as=mcp`, **Then**
   the flag's input-schema property carries the example in the standard
   JSON-Schema example form.
3. **Given** a flag with no declared example, **When** an agent reads either
   output, **Then** no example field is present for that flag.

---

### User Story 3 - Agent classifies a command's side effects before calling (Priority: P3)

An agent decides whether it is safe to call a command speculatively, or whether it
should first gate the call behind `--dry-run` or supply an `--idempotency-key`. The
CLI author has classified the command with a capability class drawn from a fixed
ax-go vocabulary (for example, read-only versus a command that creates, mutates, or
deletes state, or reaches an external network), and may attach an optional
free-form note with author-specific detail (for example, "idempotent by name").
The agent branches on the fixed class — which means the same thing across every
ax-go CLI — while a human reader gets the extra colour from the note. The agent
reads both from the command's `__schema` node and from the corresponding `--as=mcp`
tool, and applies its own safety policy accordingly.

**Why this priority**: This complements the agent-safety primitives (`--dry-run`,
`--idempotency-key`) and the non-deterministic-fields declaration, but it depends
on agreeing a classification vocabulary and is the least urgent of the three.

**Independent Test**: Declare a capability class on a command, run `__schema` and
`__schema --as=mcp`, and confirm the class surfaces on both the command node and
the MCP tool, that an undeclared command omits the class (absence = unclassified),
and that reserved/hidden commands stay excluded from the MCP tool set.

**Acceptance Scenarios**:

1. **Given** a command with a declared capability class (and optional note),
   **When** an agent reads `__schema`, **Then** the command node reports the class
   from the fixed vocabulary and, when present, the note.
2. **Given** the same command, **When** an agent reads `__schema --as=mcp`, **Then**
   the corresponding MCP tool conveys the class and, when present, the note.
3. **Given** a command whose declared class is not a member of the fixed vocabulary,
   **When** the schema is built, **Then** the condition is surfaced to the author as
   an authoring error rather than emitted as an unrecognised class.
4. **Given** a reserved or hidden command that carries a declaration, **When** an
   agent reads `__schema --as=mcp`, **Then** that command is still absent from the
   advertised tool set.

---

### User Story 4 - CLI author declares the semantics once (Priority: P1)

A CLI author building on ax-go wants to attach allowed values, an example, or a
capability class to a flag or command in one place and have every discoverability
surface reflect it. Where the CLI framework already expresses a fact — a flag's
default value, or that a flag is required — the author must **not** have to restate
it; ax-go continues to derive those automatically. The declaration mechanism is
consistent with the existing way authors declare per-command metadata, so there is
one idiom to learn.

**Why this priority**: Nothing in User Stories 1–3 can be tested or delivered
without an authoring path to supply the data, so the declaration mechanism is
foundational and ships alongside P1.

**Independent Test**: Using the public declaration mechanism, attach each kind of
metadata to a small CLI, then assert every kind appears in `__schema` output —
proving the authoring path end-to-end — while confirming defaults and required-ness
still appear without any re-declaration.

**Acceptance Scenarios**:

1. **Given** an author who declares an allowed-value set, an example, and a
   capability class, **When** the CLI emits `__schema`, **Then** all three appear
   without the author having touched the schema output directly.
2. **Given** an author who declares none of the new metadata, **When** the CLI
   emits `__schema`, **Then** the output is identical to today's except that the
   new fields are absent — no existing consumer breaks.
3. **Given** a flag with a framework-supplied default and a required marker,
   **When** the author declares only an allowed-value set, **Then** default and
   required still appear automatically alongside the newly declared set.

---

### Edge Cases

- **Out-of-set value at parse time**: a value outside a flag's declared allowed set
  must be rejected before the command's action runs, with the `ax.Error` envelope on
  `stderr`, the validation exit code (2), and no side effect — including under
  `--dry-run`, where enum validation must precede side-effect suppression.
- **Declared default outside the allowed set**: if a flag's automatically derived
  default value is not a member of its declared allowed-value set, the condition must
  be surfaced to the author (an authoring mistake) — with enforcement active, an
  unreachable-or-rejected default is a contradiction that must not ship silently.
  An **empty** default means "no default" and is exempt, so an optional enum flag
  need not list `""`. A **non-empty implicit** default is not exempt: an integer
  flag declared without an explicit default carries `0`, which must be a member.
  The flag's declared default is always **accepted** at parse time — passing it
  explicitly yields the same state as omitting the flag — so resetting a flag to
  its default (as the MCP server does between calls) can never be rejected.
- **Capability class outside the fixed vocabulary**: a declared class that is not a
  member of ax-go's fixed vocabulary must be surfaced to the author as an authoring
  error, not emitted as an unrecognised class.
- **Example inconsistent with the flag's own constraints**: an example that
  violates the flag's type or its declared allowed set is an authoring mistake and
  must be surfaced rather than advertised as valid. Where ax-go cannot type-check a
  value — an author-defined custom flag type — the example is accepted unchecked,
  and that limitation is documented on the declaration function.
- **Inherited / persistent flags**: metadata declared on a parent command's flag
  must appear on child commands exactly as `default` and `required` already
  propagate, with no duplication.
- **Empty or malformed declaration**: a declaration with no usable content must
  fail closed — the field is omitted rather than emitted partial or invalid —
  matching how the non-deterministic-fields declaration already fails closed.
- **Non-string allowed values**: an allowed-value set on an integer flag must be
  represented consistently between `__schema` (CLI-form strings, as `default` is
  today) and the `--as=mcp` JSON-Schema `enum` (typed numbers). Allowed-value sets
  are supported on string, integer, and author-defined custom flag types only;
  declaring one on a boolean, counter, floating-point, duration, or list flag is an
  authoring error (FR-013), because those types either make an enum meaningless or
  have no single canonical spelling to compare against.
- **Ordering / determinism**: the emitted order of allowed values must be stable
  across runs for identical input, so agents diffing two runs never see spurious
  drift.
- **Reserved / hidden commands**: `__schema`, `mcp-server`, `completion`, and any
  hidden command remain excluded from the MCP tool set even if they carry a
  capability declaration.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `__schema` MUST emit, per flag, an optional allowed-value set when the
  CLI author has declared one; flags without a declared set MUST omit the field
  entirely (absent, not empty).
- **FR-002**: `__schema` MUST emit, per flag, an optional example value when the
  author has declared one; flags without one MUST omit the field.
- **FR-003**: `__schema` MUST emit, per command, an optional capability /
  side-effect class when the author has declared one; a command without a
  declaration MUST omit the class field entirely, and its absence MUST be
  interpreted as "unclassified" — no side-effect behaviour is asserted on the
  command's behalf.
- **FR-004**: The `--as=mcp` adapter MUST reflect the same information for MCP
  clients: the allowed-value set as a JSON-Schema `enum` on the flag's input-schema
  property, the per-flag example in the standard JSON-Schema example form, and the
  command capability class on the corresponding MCP tool. The live `mcp-server`
  runtime advertises the same input schema (`enum`, examples) and conveys the
  capability class through the standard MCP tool hints only; carrying the exact
  class and note on the live path is out of scope for this feature.
- **FR-005**: ax-go MUST provide a public declaration mechanism for authors to
  attach an allowed-value set, an example, and a capability class, consistent with
  the existing per-command metadata declaration idiom.
- **FR-006**: Where the CLI framework already expresses a fact (a flag's default
  value, whether a flag is required), ax-go MUST continue to derive it
  automatically and MUST NOT require the author to re-declare it.
- **FR-007**: Output MUST remain deterministic: the same command tree plus the same
  declarations MUST produce byte-identical `__schema` and `--as=mcp` output, modulo
  the fields already documented as non-deterministic. Allowed-value ordering MUST be
  stable for identical input.
- **FR-008**: The new fields MUST be additive and backward-compatible: a CLI that
  declares none of them MUST produce output identical to today's except for the
  absent new fields, so existing schema consumers do not break.
- **FR-009**: Malformed or empty declarations MUST fail closed — the schema omits
  the field rather than emitting a partial or invalid value.
- **FR-010**: Declared flag metadata MUST propagate to inherited / persistent flags
  across the command tree consistently with how `default` and `required` already
  propagate, without duplication.
- **FR-011**: Reserved and hidden commands MUST remain excluded from the `--as=mcp`
  tool set even when they carry a capability declaration.
- **FR-012**: The enriched `__schema` and `--as=mcp` output MUST be covered by
  golden-file tests, because the schema is public API and must not silently drift.
- **FR-013**: An authoring inconsistency — a derived default outside a declared
  allowed set, an example that violates the flag's type or allowed set, or a
  capability class outside the fixed vocabulary — MUST be surfaced to the author
  rather than emitted as a contradictory or unrecognised contract. An example for
  an author-defined custom flag type, whose values ax-go cannot parse, is exempt
  from the type check.
- **FR-014**: Declared allowed-value sets MUST be **enforced at parse time**: for a
  flag with a declared set, ax-go MUST reject any value outside the set before the
  command's action runs, emitting the `ax.Error` envelope on `stderr` with the
  validation exit code (2), naming the offending flag and the permitted set, and
  causing no side effect. Enforcement MUST precede `--dry-run` side-effect
  suppression, so an out-of-set value never produces a "successful" dry-run
  envelope. The rejection MUST be deterministic for identical input. The rejection
  MUST NOT echo the offending value back: it may be a secret mistakenly passed to
  the wrong flag, and `stderr` is shipped to log aggregation; the flag name, the
  permitted set, and suggested corrections are sufficient to repair the call. A
  flag's declared default value is always accepted.
- **FR-015**: The command capability / side-effect class MUST be expressed as a
  **required class drawn from a fixed, ax-go-defined and documented vocabulary**,
  plus an **optional free-form note** for author-specific detail. The fixed class is
  the machine-comparable value an agent branches on across CLIs; the note is
  descriptive only and MUST NOT be required for the class to be usable. A declared
  class that is not a member of the fixed vocabulary MUST be surfaced to the author
  as an authoring error (see FR-013), not emitted as an unrecognised class. The fixed
  vocabulary is exactly `read-only`, `create`, `mutate`, `delete`, `external-network`,
  and `admin` (the last being a distinct privileged/administrative class).

### Key Entities *(include if feature involves data)*

- **Flag schema entry**: the per-flag record in `__schema`. Today carries name,
  shorthand, type, default, usage, and required. This feature adds an optional
  allowed-value set and an optional example.
- **Command schema entry**: the per-command node in `__schema`. This feature adds
  an optional capability / side-effect class.
- **Flag metadata declaration**: the author-supplied attachment of an allowed-value
  set and/or example to a specific flag.
- **Command capability declaration**: the author-supplied classification of a
  command's side effects.
- **MCP tool input-schema property**: the `--as=mcp` per-flag JSON-Schema property
  that must gain `enum` and example support and, on the tool, the capability class.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a command whose flag has a declared allowed-value set, an agent
  reading only `__schema` can construct a valid invocation of that flag on the
  first attempt in 100% of cases (no invalid-value validation error required to
  discover the permitted set).
- **SC-002**: 100% of flags with a declared allowed-value set expose that set in
  both `__schema` and `__schema --as=mcp`, and 100% of flags without one omit the
  field in both.
- **SC-003**: Two consecutive runs of `__schema` (and of `__schema --as=mcp`) on an
  unchanged command tree with unchanged declarations produce byte-identical output.
- **SC-004**: A CLI that adopts none of the new metadata produces `__schema` output
  that differs from the pre-feature output only by the absence of the new fields —
  zero breakage for existing consumers.
- **SC-005**: An MCP client consuming `__schema --as=mcp` sees a valid JSON-Schema
  `enum` for every flag with a declared allowed-value set, so it can constrain the
  argument without any ax-go-specific knowledge.
- **SC-006**: For a command with a declared capability class, an agent can
  determine from `__schema` alone whether the command is read-only or state-changing
  before invoking it, in 100% of declared cases, using a class value that is
  identical in meaning across every ax-go CLI.
- **SC-007**: For every flag with a declared allowed-value set, passing any
  out-of-set value results in a validation-code (2) `ax.Error` on `stderr` and zero
  side effects — including under `--dry-run` — in 100% of cases.

## Assumptions

- Source inputs: GitHub issue #28. The issue references amending "ADR-0003", but the
  ADR log is frozen and ADR-0003 no longer exists as a file; per the constitution
  and AGENTS.md, this decision is specified through the Spec Kit feature workflow and
  any surviving ADR context is absorbed into `research.md` during planning, not into
  a new or amended ADR. The acceptance criterion "ADR-0003 amended" is therefore
  reinterpreted as "the schema contract change is specified and recorded here."
- `default` and `required` are treated as **already delivered** (the flag schema
  emits them today, derived automatically) and are out of scope for re-implementation;
  this feature only adds allowed-values, per-flag examples, and the command
  capability class.
- The new fields are additive to a machine-payload shape that is additive-tolerant
  pre-v1.0 under the Stability & SemVer principle, so shipping them does not require
  a breaking change as long as absent declarations leave output unchanged.
- Allowed-value ordering preserves the author's declared order (a deterministic
  input) rather than re-sorting, because the order of an allowed set can carry
  meaning; the requirement is only that ordering be stable for identical input.
- Allowed-value sets are **enforced** (author decision, resolved from FR-014): ax-go
  rejects out-of-set values at parse time and maps them to the validation exit code,
  reusing the existing `ax.Error` envelope and stream/exit-code contract rather than
  inventing a new error shape.
- The capability class is a **fixed vocabulary plus optional free-form note** (author
  decision, FR-015). The fixed vocabulary is locked (Clarifications 2026-07-24) to
  exactly `read-only`, `create`, `mutate`, `delete`, `external-network`, and `admin`.
  A command without a declaration omits the class (absence = "unclassified"), which
  preserves the FR-008 backward-compatibility guarantee for existing CLIs.
- A single example per flag is sufficient for the initial scope; supporting multiple
  examples per flag is a possible future extension and is not required here.
- The declaration mechanism follows the existing per-command metadata idiom
  (the same approach already used to declare non-deterministic fields), so authors
  learn one pattern; the exact public surface is a planning/design detail.
- Determinism, stream separation, exit-code mapping, and golden-file guarding of the
  schema are governed by the constitution (Machine Discoverability, Deterministic
  Output) and are inherited constraints, not re-decided here.
