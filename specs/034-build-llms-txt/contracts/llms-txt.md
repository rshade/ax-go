# Contract: build-time discovery document

## Caller contract

```text
GenerateLLMsTxt(ctx, destination, commandTree, options...) error
```

Options:

- Opening text.
- Opening file path. Read cap is 1 MiB. The later opening option replaces the earlier one.
- Link sections, in author order.

`ctx` is checked before the file read and before the write. A cancelled context returns that error and writes nothing.

Validation failures return the shared validation error code and write nothing:

- Missing destination or missing command tree.
- No opening.
- Opening whose first non-empty line is not a title, or that has no summary quote.
- Opening file missing or larger than 1 MiB.
- A link section with an empty heading or no links.

Any other failure, including a destination write error, is returned as-is. The function does not panic.

## Document contract

1. The curated opening, unchanged.
2. A blank line, then each link section:

```text
## Heading

- [title](url): notes
```

The `: notes` suffix is omitted when notes are empty.

3. A blank line, then the generated reference:

```text
## Commands

Generated from the live command tree.

### command path

short description

long description

Examples:

    example text

- `--name`, `-s` (type, required): usage. Default: `value`. Values: `a`, `b`. Example: `sample`.
```

Lines that do not apply are omitted. Reflected text has newlines collapsed to spaces. Command paths and flag names are lexicographic. Two calls with the same inputs return the same bytes.

## Non-contract

`__schema`, `__schema --as=mcp`, and live `mcp-server` lists do not gain a field, a flag, or a command from this feature. There is no expanded `llms-full.txt` document.
