# Research: Build-time static discovery document

## R1: Signature includes context

**Decision**: `GenerateLLMsTxt(ctx context.Context, w io.Writer, root *cobra.Command, opts ...LLMsTxtOption) error`.

**Rationale**: Principle IX requires `context.Context` as the first parameter of any function that does I/O. The opening file is a read and the destination is a write. A cancelled context fails before either, and a validation failure still writes nothing.

**Alternatives considered**: The issue's signature omitted context. That matches a pure function, and it conflicts with the constitution once `WithPreambleFile` reads the filesystem. Context wins.

## R2: Root package, existing reflection

**Decision**: Implement the renderer in root `ax`. Reflect with `internal/schema.BuildCommand` and `CollectFlags`. Do not add a dependency. Do not put the renderer in package `schema`.

**Rationale**: `BuildCommand` already drops hidden commands and collects local plus inherited flags without duplicates. Root `ax` already imports Cobra. Package `schema` is an import-isolated contract surface; a markdown writer there would mix a documentation render into the machine-payload package.

**Alternatives considered**: A new `internal/llmstxt` package. Rejected. The renderer is one file and has no second caller inside the module. A `cmd/ax-gen-docs` binary. Rejected. It cannot import an adopter's `main` command tree, and copying the integration tree would drift.

## R3: Missing opening is a validation error

**Decision**: No opening, an opening whose title line or summary quote has no text, a missing or unreadable file, and a file larger than `config.DefaultMaxBytes` (1 MiB) return `*contract.Error` with `contract.ErrorCodeValidationError` and write zero bytes. The size check counts bytes read, up to the cap plus one, so a file that grows during the read is still rejected. An unreadable file keeps that error as the cause. An opening of exactly 1 MiB is accepted. The last preamble option wins.

**Rationale**: The issue says not to emit a degenerate file. A warning that still wrote bytes would be that failure. Reusing `validation_error` avoids a new `known_codes` entry and the golden churn that comes with one. `contract.NewError` keeps the code registry's source walk happy. The cause stays off the JSON envelope and lets the caller use `errors.Is`.

**Alternatives considered**: A new `llms_txt_invalid` code. Rejected. It would change every `known_codes` golden for a build-time authoring mistake. A stderr warning plus a written file. Rejected by FR-008.

## R4: Document shape

**Decision**: The opening is inserted unchanged after validation. Each `LinkSection` becomes an H2 and a list of `- [title](url): notes`, notes omitted when empty, author order preserved. The generated section is `## Commands`, introduced by one fixed sentence, then an H3 per command path. Flags are one list item each. Newlines and carriage returns in reflected prose are folded into a single space. A flag default, an enum member, and a flag example replace only line breaks, so spaces and tabs inside those literals stay exact.

**Rationale**: The llms.txt shape needs the title and the blockquote from the author, then H2 link groups. Folding newlines keeps one flag on one line so a usage string cannot break the list. A string enum matches the declared text exactly, so folding spaces inside a member would document a value the flag rejects. The fixed sentence tells a reader the following section is generated.

**Alternatives considered**: Sorting curated links. Rejected. FR-010 keeps author order. Emitting `llms-full.txt`. Rejected. FR-014.

## R5: What the reference includes

**Decision**: Walk the reflected tree. Skip a command whose name is `__schema` or `mcp-server`. Sort sibling commands and flags with `sort.Strings` on the name. The displayed path is the names joined by a space. Each command lists path, short description, long description when it differs, examples, and flags. A flag lists name, shorthand, type, required, usage, default, enum, and example when present.

**Rationale**: `BuildCommand` already omits hidden commands. The two reserved names are discovery surfaces, not author commands. Byte-wise string sort is deterministic and locale-independent.

**Alternatives considered**: Keep Cobra registration order. Rejected. FR-004. Include `__schema`. Rejected. FR-011.

## R6: First consumer

**Decision**: `examples/integration` holds `llms.preamble.md` and a committed `llms.txt`. A test in that package builds `newRootCommand`, calls `GenerateLLMsTxt`, and requires the bytes to match the committed file. README files describe generate-and-commit. `ax.GenerateLLMsTxt` is added to `doccover` `requiredSymbols`. Options are shown inside `ExampleGenerateLLMsTxt`, not gated alone. New root exports are added to the surface baseline and the root audit as `supported`.

**Rationale**: The example is the consumer the issue named. A test is the generation step that fails when the tree and the file drift. A second binary cannot call `newRootCommand` from another package without exporting it, and exporting it is a wider API than this feature needs.

**Alternatives considered**: `cmd/ax-gen-docs`. Rejected in R2.
