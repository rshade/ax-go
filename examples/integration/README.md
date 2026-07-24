# Integration Example

`examples/integration` is a runnable Cobra command that exercises the public
`ax-go` scaffold from outside the root package.

It intentionally uses the root package for full CLI runtime behavior:

```go
import ax "github.com/rshade/ax-go"
```

Use the root facade when a CLI needs `ax.Execute`, schema command wiring,
telemetry lifecycle, logging, and trace-aware envelopes. Thin consumers that
only need shared machine contracts can instead use the isolated contract packages:
`github.com/rshade/ax-go/contract`,
`github.com/rshade/ax-go/config`, `github.com/rshade/ax-go/schema`, and
`github.com/rshade/ax-go/id`.

## Related examples

This example is deliberately the **maximal** one: it exercises every Core AX
Mandate through the root facade, including Loki direct push and `ax.Flush`, and
that coverage is what `AUDIT.md` maps. Two much smaller programs exist alongside
it for a different purpose:

| Example | Imports | Purpose |
| --- | --- | --- |
| [`examples/logging`](../logging/) | `github.com/rshade/ax-go/logging` only | the import-isolated counterpart, and the subject of the binary-size gate |
| [`examples/rootlogging`](../rootlogging/) | root `ax` only | the same program on the root facade; the size-ratio denominator |

Those two are byte-for-byte identical apart from one import and one call, so the
difference between their binary sizes isolates exactly one variable. They are
built and compared on every PR by `make size-check`. Keep them diff-clean
against each other; do not grow them into a second integration example.

Run the default bounded JSON payload:

```sh
go run ./examples/integration --format=json --idempotency-key=demo-key --name=Ada
```

Parse Hujson config from stdin:

```sh
printf '{name:"Ada",count:2,}' | go run ./examples/integration --format=json --config=-
```

Emit NDJSON envelopes:

```sh
go run ./examples/integration stream --format=json --count=3
```

Patch a Hujson config file in place, preserving comments:

```sh
printf '{\n// keep me\n"name":"Ada",\n"count":2,\n}' > /tmp/config.hujson
go run ./examples/integration patch-config --format=json \
  --config=/tmp/config.hujson \
  --patch='[{"op":"replace","path":"/count","value":5}]'
```

With `--dry-run`, `patch-config` rehearses the patch — it reads the config and
applies the patch in memory so the same errors surface as a real run — but
writes nothing. The success envelope is identical apart from `meta.dry_run`:

```sh
go run ./examples/integration patch-config --format=json --dry-run \
  --config=/tmp/config.hujson \
  --patch='[{"op":"replace","path":"/count","value":5}]'
```

Inspect the reflected command schema:

```sh
go run ./examples/integration __schema
go run ./examples/integration __schema --as=mcp
```

Build the example with version injection and inspect the same schema field:

```sh
make build-example
./bin/ax-integration __schema
```

`make build-example` derives `VERSION` from
`git describe --tags --always --dirty` and injects it with:

```sh
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" \
  -o bin/ax-integration ./examples/integration
```

The resulting `__schema.version` is non-empty and VCS-derived, such as a tag
or commit identifier with a dirty marker. Override `VERSION` for release or
reproducible builds:

```sh
make build-example VERSION=v1.2.3
```

Return a structured error envelope on `stderr`. Each command below maps to a
distinct deterministic exit code so an agent can branch on the category:

```sh
go run ./examples/integration fail --format=json    # exit 2 — validation
go run ./examples/integration fetch --format=json    # exit 3 — network (retryable + retry_after_seconds)
go run ./examples/integration authz --format=json    # exit 4 — auth/permission (not retryable)
go run ./examples/integration crash --format=json    # exit 1 — internal (bare error → internal_error envelope)
```

`fetch` carries the feature 013 recovery fields (`retryable: true`,
`retry_after_seconds: 5`) so an agent knows it is safe to back off and retry;
`authz` sets `retryable: false`. `crash` returns a plain Go error to show how
`ax.Execute` wraps any unexpected error into the framework's `internal_error`
envelope with exit code 1.

Run this CLI as a live MCP server (it mounts `mcp.NewCommand`, so every
non-hidden command becomes an MCP tool with no per-tool work):

```sh
go run ./examples/integration mcp-server                          # stdio
go run ./examples/integration mcp-server --transport=http --addr=127.0.0.1:8080
```

The server speaks MCP over the transport channel and keeps logs on `stderr`. A
non-loopback HTTP bind is fail-closed without `--allow-non-loopback`. See the
[Running as an MCP server](../../README.md#running-as-an-mcp-server) section for
the full contract.

## Building without OTLP export and the gRPC dial helper

The example builds and behaves identically under ax-go's two optional negative
build constraints, which is the point of keeping it here — a consumer adopting
them ships something shaped like this:

```sh
make build-example-minimal
./bin/ax-integration-minimal __schema
```

Measured on linux/amd64 for this example (stripped, `-ldflags="-s -w"`):

| Build | Size | Forbidden-tree packages |
| --- | ---: | ---: |
| default | 17,535,241 | 109 |
| `-tags=ax_no_grpc,ax_no_otlp` | 10,289,417 | **0** |

That is −41%, a smaller ratio than the −63% a minimal root-facade consumer sees,
because this example also links the MCP SDK and the Hujson parser — a larger
baseline the constraints do not touch.

`__schema` output is byte-identical between the two builds, and the `ax.Error`
envelope differs only in `trace_id`, which is documented as non-deterministic.
Exit codes, stream separation, `--dry-run`, and `--idempotency-key` are
unchanged. See the root [README](../../README.md#slimming-the-binary-optional-otlp-and-grpc).

## Mandate audit and golden fixtures

[`AUDIT.md`](AUDIT.md) maps every Core AX Mandate to the subcommand and test that
exercises it — this example covers each mandate exactly once. The `testdata/`
golden fixtures pin `__schema`, `__schema --as=mcp`, and every subcommand's
success and error envelope; any drift fails CI under `go test ./...`. Because
`ax.Execute` generates fresh `trace_id`/`span_id` per run, the fixtures mask
those (and `idempotency_key`) to `MASKED` while pinning everything else. Inject a
fixed version so `version` stays stable, then regenerate after an intentional
contract change:

```sh
go test ./examples/integration -run TestGolden -update
```
