# Feature Specification: Build-time static discovery document

**Feature Branch**: `034-build-llms-txt`

**Created**: 2026-10-28

**Status**: Draft

**Input**: GitHub issue rshade/ax-go#32. An adopting CLI author wants a static discovery document, in the llms.txt shape, generated from the live command tree plus a short curated introduction and link list, so a coding assistant that only fetches the repository sees commands, flags, and examples that cannot drift from the binary. Constitution Principle III (v1.3.0, issue #29) already decided the split: the library owns generation, the adopting CLI owns publication, and runtime discovery stays `__schema` JSON, `__schema --as=mcp`, and live `mcp-server` lists. The operator invoked `/pick-issue 32` on 2026-10-28, which fires the issue's deferred trigger. The integration example is the in-repo consumer the issue names. No new ADR: that decision log is frozen, and Principle III is the record.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An author generates a discovery document from the live command tree (Priority: P1)

A CLI author has a command tree with nested commands, flags, and examples. They supply a curated opening (a title, a one-paragraph summary, and optional introduction) and an optional list of curated links. They generate a single document. A coding assistant pointed at the committed file can name the commands and flags without executing the binary, and a second generation from the same tree and the same curated text is byte-identical.

**Why this priority**: This is the feature. Without a generated document, assistants keep reading a hand-written file that drifts from the real CLI.

**Independent Test**: Build one small command tree, supply a fixed opening and a fixed link list, and generate twice. Both results match one stored document: title, summary, curated links, then a generated command reference in name order.

**Acceptance Scenarios**:

1. **Given** a command tree and a curated opening that starts with a title and a summary quote, **When** the author generates the document, **Then** the document begins with that title and summary, then the curated link sections, then one clearly delimited generated section that lists every included command and its flags.
2. **Given** the same tree, the same opening, and the same links, **When** the author generates the document again, **Then** the two documents are byte-identical and contain no timestamp, trace identifier, host name, or user name.
3. **Given** commands and flags registered in a non-alphabetical order, **When** the document is generated, **Then** commands are ordered lexicographically by path and flags are ordered lexicographically by name.
4. **Given** a nested command, **When** the document is generated, **Then** the nested command appears under its full path with its own description, examples, and flags.

---

### User Story 2 - A missing or oversized opening cannot publish a degenerate file (Priority: P1)

The llms.txt shape requires a title and a blockquote summary. An author who forgets the opening, points at a missing file, or points at a file larger than the read cap must not receive a document that looks publishable.

**Why this priority**: A link-less or title-less file is the failure mode the issue forbids, and an uncapped read is a resource-safety failure.

**Independent Test**: Generate with no opening, with a path that does not exist, and with an opening larger than 1 MiB. Each attempt yields no document and a validation failure the author can see.

**Acceptance Scenarios**:

1. **Given** no curated opening, **When** the author generates, **Then** nothing is written and the failure is a validation error.
2. **Given** an opening file that does not exist or is not readable, **When** the author generates, **Then** nothing is written and the failure is a validation error.
3. **Given** an opening file larger than 1 MiB, **When** the author generates, **Then** nothing is written and the failure is a validation error, not an out-of-memory condition.
4. **Given** an opening file of exactly 1 MiB that is otherwise valid, **When** the author generates, **Then** generation succeeds.
5. **Given** a valid opening and no curated links, **When** the author generates, **Then** generation succeeds and the generated command reference still follows the opening.

---

### User Story 3 - Runtime discovery is unchanged (Priority: P1)

An agent that executes the binary still uses `__schema` and, when declared, the live MCP server. The new document is not a fourth runtime projection.

**Why this priority**: Principle III forbids a runtime adapter. A generator that leaks into `__schema` would change a public machine payload.

**Independent Test**: Run `__schema` and `__schema --as=mcp` before and after the feature. The payloads are byte-identical to the current goldens. No flag or command selects an llms.txt projection.

**Acceptance Scenarios**:

1. **Given** any command tree, **When** `__schema` or `__schema --as=mcp` runs, **Then** its payload is unchanged and contains no generated discovery document.
2. **Given** the public command surface, **When** an author looks for a runtime llms.txt mode, **Then** there is no `__schema --as=llms-txt` adapter and no `__llms` command.
3. **Given** a live `mcp-server`, **When** a client lists tools, prompts, or resources, **Then** those lists are unchanged by this feature.

---

### User Story 4 - The integration example is the first published consumer (Priority: P2)

This repository generates the integration example's discovery document from that example's real command tree and keeps the file next to the example, so the generator is proven against a CLI a reader can run.

**Why this priority**: The issue's trigger is a real consumer. The example is that consumer. The library is still useful without the committed file, which is why this story is second.

**Independent Test**: Run the documented generation step. The committed file matches a fresh generation. The example's README tells an adopter to generate, commit, and publish the file themselves.

**Acceptance Scenarios**:

1. **Given** the integration example's command tree and its curated opening, **When** the documented generation step runs, **Then** it writes one discovery document and does not print that document on the example's machine-payload stream.
2. **Given** the committed document, **When** generation runs again with the same inputs, **Then** the result matches the committed file byte for byte.
3. **Given** a reader of the example, **When** they follow the documented workflow, **Then** they see that publication stays with the adopting CLI and that the library only generates.

---

### Edge Cases

- A command tree with only a root command and no subcommands still generates a reference for that root.
- Hidden commands are omitted. The reserved `__schema` and `mcp-server` commands are omitted.
- A flag inherited from a parent is listed on the command where it applies, once. A local flag that replaces a parent flag is listed as the local definition.
- An opening that does not begin with a title line, or that has no summary quote, is a validation error and writes nothing.
- Curated link sections keep the author's order. The generator does not sort or rewrite them.
- Commands or flags whose names differ only by case stay distinct and sort by byte order.
- A writer that fails mid-write returns that failure. A validation failure occurs before any write, so the caller receives zero bytes.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: An adopting author MUST be able to generate one static discovery document from a command tree plus a curated opening.
- **FR-002**: The document MUST follow the llms.txt shape: a title, a blockquote summary, optional introduction prose, zero or more curated link sections, then one generated command-and-flag reference.
- **FR-003**: The generated reference MUST include each included command's path, description, examples, and flags. Each flag MUST include its name, description, and any declared enumeration or example.
- **FR-004**: Commands MUST be ordered lexicographically by path, and flags lexicographically by name, independent of registration order.
- **FR-005**: Identical inputs MUST produce byte-identical documents. The document MUST NOT contain a timestamp, trace identifier, host name, user name, or other environment data.
- **FR-006**: Generation MUST write only to a destination the caller supplies. It MUST NOT write to the `__schema` machine-payload stream.
- **FR-007**: The module MUST NOT add a runtime `__schema --as=llms-txt` adapter or a `__llms` command. Existing `__schema`, `__schema --as=mcp`, and live `mcp-server` list results MUST stay byte-identical to their current contracts.
- **FR-008**: A missing opening, an opening that lacks a title or a summary quote, a missing opening file, and an opening file larger than 1 MiB MUST each fail as a validation error and write nothing.
- **FR-009**: An opening file of exactly 1 MiB that is otherwise valid MUST be accepted. The cap is the same 1 MiB read boundary used for configuration reads.
- **FR-010**: Curated links MUST be optional. When present, they MUST appear after the opening and before the generated reference, in the author's order, unchanged.
- **FR-011**: Hidden commands and the reserved `__schema` and `mcp-server` commands MUST be omitted from the generated reference.
- **FR-012**: The integration example MUST be able to regenerate its own discovery document from its live command tree, and the repository MUST document that the adopting CLI commits and publishes that file.
- **FR-013**: The primary generation entry point MUST have a verified usage example that a reader can run.
- **FR-014**: The expanded form of the discovery document (`llms-full.txt`) MUST NOT be generated.

### Key Entities

- **Curated opening**: Author-written title, summary quote, and optional introduction. It is the only author text the document shape requires.
- **Link section**: An author-ordered group of titled links with optional notes. The generator does not invent links.
- **Generated reference**: The command and flag listing derived from the live command tree. Readers can tell it apart from the curated sections.
- **Discovery document**: The combined opening, link sections, and generated reference. It is a documentation file, not a machine payload.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a fixed command tree and fixed curated text, two generations match exactly, including ordering.
- **SC-002**: Every validation failure in FR-008 writes zero bytes and is distinguishable from an internal failure.
- **SC-003**: Existing schema and MCP discovery fixtures remain unchanged after the feature ships.
- **SC-004**: A reader of the integration example can regenerate its discovery document with one documented step and see a file that matches the committed copy.
- **SC-005**: The primary generation entry point has one verified usage example, and a reader can find the publication workflow without reading the implementation.

## Assumptions

- Source inputs: GitHub issue #32 and Constitution Principle III. No ADR governs this feature. ADR-0003, ADR-0010, and ADR-0012 are cited by the issue and are not absorbed here: ADR-0003 is already retired, the 1 MiB read cap already lives in Principle V, and `cmd/` remains reserved for support binaries. Clarification was not required. The issue, the earlier emit-versus-delegate decision, and Principle III already fix the document-versus-runtime split, the rejection of a runtime adapter, the deferral of the expanded document, and the requirement that a missing opening must not publish a file.
- The operator's explicit `/pick-issue 32` fires the issue's `first-downstream-consumer-needs-published-llms-txt` trigger. The integration example is that consumer. This issue was labelled `roadmap/future`.
- "Warn rather than emit a degenerate file" is implemented as a validation failure that writes nothing. A warning that still wrote the file would be the silent-publish failure the issue forbids.
- A module-level docs binary cannot see an adopting CLI's command tree. Adopters call the library with the tree they already built. This repository's runnable consumer is the integration example, not a second copy of that tree under `cmd/`.
- The public entry point's name and option shape are a planning decision. The issue proposes one generator that writes to a caller-supplied destination and takes the opening and the links as options. Planning records the final names.
- Lane for this claim: `area:docs`. No other `area:*` lane was busy.
