# Contract: build-time discovery document

## Caller contract

```text
GenerateLLMsTxt(ctx, destination, commandTree, options...) error
```

Options:

- Opening text.
- Opening file path. The read counts at most 1 MiB plus one byte. The later opening option replaces the earlier one.
- Link sections, in author order.

`ctx` is checked before the file read and before the write. A cancelled context returns that error and writes nothing.

Validation failures return the shared validation error code and write nothing:

- Missing destination or missing command tree.
- No opening.
- Opening whose title line or summary quote has no text.
- Opening file missing, unreadable, or larger than 1 MiB. An unreadable file keeps the file error as the cause.
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

Lines that do not apply are omitted. Reflected prose has newlines folded to spaces. Flag defaults, enum members, and examples replace only line breaks. Command paths and flag names are lexicographic. Two calls with the same inputs return the same bytes.

## Non-contract

`__schema`, `__schema --as=mcp`, and live `mcp-server` lists do not gain a field, a flag, or a command from this feature. There is no expanded `llms-full.txt` document.
