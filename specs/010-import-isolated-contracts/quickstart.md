# Quickstart: Import-Isolated Contracts

**Feature branch**: `010-import-isolated-contracts`

This guide describes the expected developer workflow after implementation.

## Use the Root Package for Full CLI Runtime

Use the root package when building a complete ax-go CLI:

```go
import ax "github.com/rshade/ax-go"
```

Root `ax` remains the surface for execution, telemetry lifecycle, logging, schema command wiring, and transport helpers.

## Use Contract Packages for Thin Consumers

Use isolated packages when a consumer only needs shared machine contracts.

```go
import (
    "github.com/rshade/ax-go/config"
    "github.com/rshade/ax-go/contract"
    "github.com/rshade/ax-go/id"
    "github.com/rshade/ax-go/schema"
)
```

Typical use cases:

- `config`: parse or patch bounded Hujson config without the root runtime package.
  Use `config.Parse`, `config.ParseFile`, `config.Patch`, `config.PatchFile`,
  and `config.WithMaxBytes`.
- `contract`: build or validate standard envelopes, error shapes, mode values, exit codes, and strict JSON output.
  Use `contract.NewEnvelope`, `contract.NewError`, `contract.ResolveMode`,
  `contract.WriteJSON`, and `contract.WriteJSONLine`.
- `schema`: reflect command trees into ax-native or MCP-compatible schema data.
  Use `schema.BuildSchema`, `schema.BuildMCPSchema`, and
  `schema.NewSchemaCommand`.
- `id`: generate UUID v4 idempotency keys or UUID v7 entity IDs.
  Use `id.NewIdempotencyKey` and `id.NewEntityID`.

## Verify Import Isolation

Run focused package tests:

```sh
go test -race ./contract ./config ./schema ./id
```

Run the full required gate before handing work back:

```sh
go test -race ./...
go vet ./...
golangci-lint run
make doc-coverage
```

## Expected Boundary Behavior

A thin consumer importing only `config` and `schema` should not pull in root execution, telemetry exporters, logger/Loki, HTTP instrumentation, or gRPC instrumentation.

The import-isolation tests should fail if any of those runtime adapters appear in a contract surface's dependency graph.

## Compatibility Check

Existing root imports continue to work:

```go
import ax "github.com/rshade/ax-go"
```

Existing root examples and golden fixtures remain the compatibility proof. A migration to isolated packages is optional for consumers that want thinner imports.
