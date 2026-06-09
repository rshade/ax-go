# Integration Example

`examples/integration` is a runnable Cobra command that exercises the public
`ax-go` scaffold from outside the root package.

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

Return a structured error envelope on `stderr`:

```sh
go run ./examples/integration fail --format=json
```
