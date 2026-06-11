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

Return a structured error envelope on `stderr`:

```sh
go run ./examples/integration fail --format=json
```
