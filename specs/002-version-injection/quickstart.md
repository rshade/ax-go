# Quickstart: Build-time Version Injection

**Feature**: `002-version-injection` | **Date**: 2026-06-08

How to build an ax-go CLI so `__schema.version` reports a real version — and how an
adopter replicates the pattern with one helper call and one build flag.

## 1. Wire the version variable and helper (adopter `main`)

```go
package main

import "github.com/rshade/ax-go"

// Linker-writable variable. MUST be a var, not a const.
var version string // set via -ldflags "-X main.version=..."

func run(/* ... */) int {
    resolved := ax.ResolveVersion(version) // injected value → build-info → sentinel

    logger := ax.NewLogger(ctx,
        ax.WithLoggerLabels(ax.Labels{Application: "mytool", Version: resolved}),
    )
    _ = logger

    return ax.Execute(ctx, root, ax.WithVersion(resolved) /* , ... */)
}
```

One `resolved` value feeds `__schema.version`, the `ax.Error` envelope `version`,
and the logger `version` label — they cannot disagree.

## 2. Build with injection

```sh
make build-example
# or, by hand:
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" \
  -o bin/ax-integration ./examples/integration
```

```sh
./bin/ax-integration __schema | jq -r '.version'
# -> 5bf9b77-dirty     (commit SHA today; v1.2.3 once a tag exists)
```

Override for a release or a reproducible build:

```sh
make build-example VERSION=v1.2.3
```

## 3. Even un-injected builds report a real version

```sh
go run ./examples/integration __schema | jq -r '.version'
# -> <commit revision>[-dirty]   (from runtime/debug build info — never empty)
```

| How you built it | `__schema.version` |
|------------------|--------------------|
| `make build-example` (or any `-ldflags -X`) | `git describe` value, e.g. `v1.2.3` / `5bf9b77-dirty` |
| `go install <module>@v1.2.3` | `v1.2.3` (embedded module version) |
| `go run` / bare `go build` in a Git tree | commit revision, `-dirty` if modified |
| no VCS context, no injection | `0.0.0-unknown` (sentinel — never empty, never bare `dev`/`unknown`) |

## 4. Verify locally

```sh
go test -race ./...          # resolver table tests + example non-empty assertion
make doc-coverage            # ExampleResolveVersion present on the gated primary API
make build-example && ./bin/ax-integration __schema | jq -e '.version != ""'
go vet ./... && golangci-lint run
```

## Cheat-sheet

- The version is a **`var`**, never a `const` — `-X` cannot rewrite a constant.
- `ax.ResolveVersion` never returns empty, never returns bare `dev`/`unknown`,
  and never panics; the floor is `0.0.0-unknown`.
- Resolve **once**, pass to `ax.WithVersion` *and* `ax.WithLoggerLabels`.
- `version` is a **label** (low cardinality) — never promote it to payload.
- `make build` (library) does not inject; `make build-example` (binary) does.
