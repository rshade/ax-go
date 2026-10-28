# Data model: Build-time static discovery document

## Link

A curated destination. Fields: title, URL, optional notes. The generator does not fetch the URL and does not reorder links inside a section.

## Link section

An author-ordered group. Fields: heading, one or more links. An empty heading or an empty link list is a validation error. Sections render in the order supplied.

## Curated opening

Author text. It must contain at least one non-empty line, the first of those must be a title line beginning with `# `, and a later line must be a summary quote beginning with `> `. The bytes are copied into the document unchanged. Source is either an in-memory string or a file. The file is read only after a size check. Larger than 1 MiB fails. Exactly 1 MiB is allowed. When both sources are set, the later option replaces the earlier one.

## Generated command

A node from command reflection. Fields used: name, path, short description, long description, examples, flags. Hidden commands never appear. `__schema` and `mcp-server` never appear. Sibling order is lexicographic by name.

## Generated flag

A flag that applies to that command, including inherited flags, once each. Fields used: name, shorthand, type, default, usage, required, enumeration, example. Order is lexicographic by name. A local flag already recorded under the same name is kept and the inherited duplicate is dropped, which is what reflection already does.

## Discovery document

Opening, then each link section, then the generated command reference. No other sections. No timestamp, trace identifier, host, or user. Validation failure produces no bytes.

## State

There is no stored state. Each call reads its inputs and writes once, or writes nothing.
