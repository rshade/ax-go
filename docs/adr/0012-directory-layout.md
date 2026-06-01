# ADR-0012: Directory Layout

## Status

ACCEPTED - 2026-05-31.

This ADR supersedes the initial layout draft that was written before grounding
the decision in official Go module layout guidance.

## Context

ax-go is primarily an importable Go library with package name `ax` and module
path `github.com/rshade/ax-go`. It also has example CLIs and will eventually
grow runnable support commands such as `ax-go mcp-server`.

Go directory layout is part of the public API because directories become import
paths. Moving the public package under a directory such as `pkg/ax` would make
consumers import `github.com/rshade/ax-go/pkg/ax` instead of the intended
`github.com/rshade/ax-go`.

Official Go guidance supports:

- Keeping importable package code at the module root when the module exposes a
  primary package.
- Using `internal/` for packages that must not be imported outside the parent
  tree.
- Using `cmd/` for installable command packages when a repository contains
  commands as well as importable packages.
- Naming packages for what they provide, with short, clear, lower-case names.

References:

- <https://go.dev/doc/modules/layout>
- <https://go.dev/blog/package-names>

## Decision Drivers

- Preserve the public import path: `github.com/rshade/ax-go` as package `ax`.
- Follow official Go module layout guidance rather than generic repository
  layout conventions.
- Prevent private implementation mechanics from becoming accidental public API.
- Keep optional backends such as Loki and OTel exporters out of root facade
  files.
- Avoid meaningless import path segments such as `pkg`.

## Considered Options

### A. Public root package plus private `internal/` packages

Root `.go` files define public package `ax`. Private implementation packages
live under `internal/`. Runnable binaries live under `cmd/` only when real
command behavior exists. Examples stay under `examples/`.

Pros: matches the intended import path, follows Go module guidance, keeps one
public package for consumers, and lets private internals evolve before v1.0.
Cons: root facade files need small conversion layers to map internal
implementation shapes to exported public types.

### B. Move public code under `pkg/ax`

Pros: visually separates public code from repository metadata and examples.
Cons: worsens the public import path to `github.com/rshade/ax-go/pkg/ax`; `pkg`
does not describe a domain concept; this is not the default official Go module
layout.

### C. Create public subpackages by subsystem

Examples: `schema`, `logger`, `telemetry`, `config`, `mcp`.

Pros: clear subsystem folders.
Cons: prematurely expands the public compatibility contract and forces
consumers and agents to learn multiple packages for a foundation intended to
present one cohesive `ax` API.

### D. Keep every implementation detail in the root package

Pros: simplest while the codebase is tiny.
Cons: backend, schema, CLI, and config mechanics become hard to distinguish
from the supported public API.

## Decision

Adopt **Option A**.

The module root remains public package `ax`. The root owns stable exported
types and functions such as `Execute`, `Error`, `Envelope`, `ParseConfig`,
`Mode`, ID helpers, logger construction, telemetry lifecycle, outbound
HTTP/gRPC helpers, and schema builders.

Private implementation mechanics belong under `internal/`:

- `internal/cli` for Cobra execution, flag, and pre-run plumbing.
- `internal/config` for bounded Hujson read/decode mechanics and future AST
  patch support.
- `internal/schema` for Cobra command-tree reflection helpers.
- `internal/mcp` for MCP adapter/server internals.
- `internal/telemetry` for OTel propagator/exporter construction details.
- Future Loki direct push code goes under `internal/loki`, not in `logger.go`.

Runnable support commands belong under `cmd/`, starting with `cmd/ax-go` only
when real `ax-go` command behavior exists. Do not create placeholder commands.

Stable public JSON fixtures belong under root `testdata/`, with
implementation-only fixtures colocated under the relevant `internal/` package.

Do not add `pkg/`, `src/`, `lib/`, or broad public subpackages without a new or
amended ADR.

## Consequences

- Adding any public subpackage requires a new or amended ADR because it expands
  the compatibility contract.
- Root files should remain facade-oriented: expose public API and delegate
  implementation detail to `internal/*`.
- Internal package names should be domain names, not dependency-wrapper names.
- `README.md`, `AGENTS.md`, examples, and golden tests must stay current when
  public behavior changes.
