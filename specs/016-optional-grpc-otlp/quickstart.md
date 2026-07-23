# Quickstart: Independently Opt-Out OTLP Export and gRPC Dial Adapter

**Feature**: `016-optional-grpc-otlp` | **Date**: 2026-07-22

---

## For consumers: shrinking your binary

If you use ax-go's tracing but never export traces over OTLP and never call
`ax.GRPCDial`, decline both and rebuild:

```bash
go build -tags=ax_no_grpc,ax_no_otlp -ldflags="-s -w" ./cmd/yourcli
```

Expect roughly a **63% smaller** stripped artifact.

**You keep**: W3C `TRACEPARENT`/`TRACESTATE` extraction, a recording root span
around `Execute`, `trace_id`/`span_id` on every log line, `AX_OTEL_DEBUG` span
output, `ax.HTTPClient`, and byte-identical `__schema` / `ax.Error` payloads.

**You give up**: OTLP network export, and `ax.GRPCDial`.

### ⚠️ Decline both, or neither

Each tag alone removes one of *two independent roots* over the same gRPC
subtree, so one alone buys you almost nothing:

| Tags | Δ size | grpc pkgs left |
| --- | ---: | ---: |
| `ax_no_grpc` | −0.00% | 66 |
| `ax_no_otlp` | −15.1% | 64 |
| **both** | **−63.3%** | **0** |

(linux/amd64, stripped; windows/amd64 gives −0.04% / −14.9% / −62.5%.)

### If you still need OTLP export

Don't set `ax_no_otlp`. With it set, a configured
`OTEL_EXPORTER_OTLP_ENDPOINT` is *not* an error — you get one
`ax: otel exporter disabled: …` line on `stderr` and the command succeeds
normally. That is by design (fail-open), but it means a misconfigured build
degrades silently to no export. Check for that diagnostic.

### If you need `ax.GRPCDial`

Don't set `ax_no_grpc`. With it set you get `undefined: ax.GRPCDial` at build
time. Go gives library authors no way to customise that message; see the doc
comment in the root package's `grpc_disabled.go` for the explanation.

### Note: the thin packages are not a tracing escape hatch

`contract`, `config`, `schema`, and `id` link zero gRPC today and always have —
but they provide **no live tracing**. `contract.TraceIDFromContext` reads a value
previously stored in the context; it does not resolve an active span. If you
want real tracing with a small binary, the root facade plus both declines is the
path.

---

## For contributors: working on this feature

### Build all four configurations

```bash
go build ./...
go build -tags=ax_no_grpc ./...
go build -tags=ax_no_otlp ./...
go build -tags=ax_no_grpc,ax_no_otlp ./...
```

### Run the gates

```bash
make surface-check   # new: surface inventory across 4 configs × 6 profiles
make cover-check
make bench-check
make doc-coverage
make ci
```

`make test`, `make validate`, and `make lint` each iterate all four
configurations (`BUILD_TAG_MATRIX` in the Makefile), so `make ci` covers the
whole matrix. `make build-example-minimal` builds the integration example with
both declines.

### Regenerate the surface baseline (after an intentional API change)

```bash
make surface-update                                # or: go run ./internal/cmd/surfacecheck -update
git diff internal/cmd/surfacecheck/baseline.json   # review every line
```

The baseline is **not** a one-way ratchet — additions and removals both surface
as drift and are resolved by a reviewed regeneration. A regeneration on an
unchanged tree is byte-identical, so any diff at all is real signal.

### ⚠️ Tagged code is invisible to the default toolchain

`golangci-lint` accepts only one tag set per run, and a bare `go vet ./...` or
`go test -race ./...` passes no tags at all. Code behind `//go:build ax_no_grpc`
is invisible to all ~90 linters, to vet, and to the test suite unless you pass
tags explicitly:

```bash
go test -race -tags=ax_no_grpc,ax_no_otlp ./...
go vet -tags=ax_no_grpc,ax_no_otlp ./...
golangci-lint run --build-tags=ax_no_grpc,ax_no_otlp
```

`.golangci.yml` now has a `run.build-tags` key, but it is deliberately **empty**
— that is the default build. Do not add the ax tags to it; doing so would stop
linting the default configuration rather than adding coverage. Use the four
`make lint` invocations instead.

Never assume a green default run covers the declined configurations.

---

## Reproducing the size measurement (FR-028)

Portable, from a clean checkout. Requires only the Go toolchain.

⚠️ It measures the **committed** tree (`git archive HEAD`), so commit your work
first or the numbers will describe `HEAD`, not your working copy.

```bash
#!/usr/bin/env bash
set -euo pipefail
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT

git archive HEAD | (mkdir -p "$WORK/axgo" && tar -x -C "$WORK/axgo")

mkdir -p "$WORK/fixture"
cat > "$WORK/fixture/main.go" <<'GO'
package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	ax "github.com/rshade/ax-go"
)

func main() {
	root := &cobra.Command{Use: "fixture"}
	root.AddCommand(ax.NewSchemaCommand(root))
	root.RunE = func(cmd *cobra.Command, args []string) error {
		ax.NewLogger(cmd.Context()).Info(cmd.Context()).Str("k", "v").Msg("hello")
		return nil
	}
	os.Exit(ax.Execute(context.Background(), root))
}
GO

cat > "$WORK/fixture/go.mod" <<'MOD'
module fixture

go 1.26.5

require (
	github.com/rshade/ax-go v0.0.0
	github.com/spf13/cobra v1.10.2
)

replace github.com/rshade/ax-go => ../axgo
MOD

cd "$WORK/fixture" && go mod tidy >/dev/null 2>&1

for TAGS in "" "ax_no_grpc" "ax_no_otlp" "ax_no_grpc,ax_no_otlp"; do
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -tags="$TAGS" -ldflags="-s -w" -o "$WORK/bin" .
  printf '%-24s size=%-10s pkgs=%-5s grpc=%-3s pb=%-3s otlp=%-3s gw=%s\n' \
    "${TAGS:-<default>}" \
    "$(stat -c%s "$WORK/bin")" \
    "$(go list -tags="$TAGS" -deps . | wc -l)" \
    "$(go list -tags="$TAGS" -deps . | grep -c '^google\.golang\.org/grpc' || true)" \
    "$(go list -tags="$TAGS" -deps . | grep -c '^google\.golang\.org/protobuf' || true)" \
    "$(go list -tags="$TAGS" -deps . | grep -c '^go\.opentelemetry\.io/proto' || true)" \
    "$(go list -tags="$TAGS" -deps . | grep -c 'grpc-gateway' || true)"
done
```

### Reference output (2026-07-22, linux/amd64)

Measured against the implemented tree; the script above runs as written.

```
<default>                size=14893218  pkgs=410   grpc=66  pb=36  otlp=4  gw=3
ax_no_grpc               size=14893218  pkgs=406   grpc=66  pb=36  otlp=4  gw=3
ax_no_otlp               size=12640418  pkgs=386   grpc=64  pb=33  otlp=0  gw=0
ax_no_grpc,ax_no_otlp    size=5460130   pkgs=264   grpc=0   pb=0   otlp=0  gw=0
```

windows/amd64: 15,401,984 → 5,772,800 (−62.5%), likewise reaching
grpc=0 pb=0 otlp=0 gw=0 under both tags.

Note `ax_no_grpc` alone leaves the stripped size **bit-identical** on
linux/amd64 (−0.04% on windows/amd64): the linker already dead-strips an
unreferenced `ax.GRPCDial`, so the tag's contribution is removing the *second
root* over the gRPC subtree, which only pays off once `ax_no_otlp` has removed
the first. That is the whole reason the two tags must be adopted together.
