# Contract: Import Isolation

**Feature branch**: `010-import-isolated-contracts`
**Applies to**: `contract`, `config`, `schema`, and `id` public package surfaces

## Required Boundary

Each contract surface MUST be importable without the root runtime facade and without runtime adapter dependencies.

## Forbidden Imports

The dependency graph for every contract surface MUST NOT include:

- `github.com/rshade/ax-go`
- `github.com/rshade/ax-go/internal/telemetry`
- `go.opentelemetry.io/otel/exporters/`
- `go.opentelemetry.io/otel/sdk`
- `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`
- `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc`
- `google.golang.org/grpc`
- `github.com/rs/zerolog`
- root or internal Loki implementation packages

## Allowed Dependencies by Surface

| Surface | Allowed dependency categories |
|---------|-------------------------------|
| `contract` | Standard library only |
| `config` | Standard library, `contract`, Hujson parsing dependency |
| `schema` | Standard library, `contract`, Cobra/pflag command reflection dependencies |
| `id` | Standard library, `github.com/google/uuid` |

## Verification Contract

Each surface MUST have an import-isolation test that:

1. Resolves transitive package dependencies for the surface.
2. Fails if any forbidden import appears.
3. Reports the surface and forbidden dependency in the failure message.
4. Runs as part of `go test -race ./...`.

## Acceptance Examples

These should pass:

```text
github.com/rshade/ax-go/contract -> stdlib only
github.com/rshade/ax-go/config   -> contract + hujson + stdlib
github.com/rshade/ax-go/schema   -> contract + cobra/pflag + stdlib
github.com/rshade/ax-go/id       -> google/uuid + stdlib
```

These should fail:

```text
github.com/rshade/ax-go/config -> github.com/rshade/ax-go/internal/telemetry
github.com/rshade/ax-go/schema -> github.com/rshade/ax-go
github.com/rshade/ax-go/contract -> go.opentelemetry.io/otel/sdk
github.com/rshade/ax-go/id -> github.com/rs/zerolog
```
