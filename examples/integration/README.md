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

Return a structured error envelope on `stderr`:

```sh
go run ./examples/integration fail --format=json
```

