# Quickstart: Import-Isolated Logging Package

**Feature**: `017-import-isolated-logging` | **Date**: 2026-07-22

How to build, verify, and reproduce the claims of this feature. Every command
is repo-relative; run from the module root unless stated.

## For consumers

### Small binary, logging only

```go
package main

import (
    "context"

    "github.com/rshade/ax-go/logging"
)

func main() {
    ctx := context.Background()
    log := logging.NewLogger(ctx,
        logging.WithLoggerLabels(logging.Labels{
            Application: "my-cli",
            Environment: "prod",
        }),
    )
    log.Info(ctx).Str("stage", "startup").Msg("ready")
}
```

No build tags required. The binary contains no OTLP exporter, no gRPC, no
Cobra, and no `net/http`.

### Existing root-facade code is unchanged

```go
import ax "github.com/rshade/ax-go"

log := ax.NewLogger(ctx, ax.WithLokiFromEnv())
defer func() { _ = ax.Flush(ctx, log) }()
```

Still compiles, still ships to Loki, still behaves identically. Log shipping
remains available **only** through root `ax`.

### Choosing a surface

| You need | Import |
|---|---|
| Logging only, smallest binary | `logging` |
| Logging **plus** Loki direct push | root `ax` |
| Logging plus OTel export or `Execute` | root `ax` |

## Verification

### The full gate set

```bash
gofmt -l .
go vet ./...
go test -race ./...
golangci-lint run
make doc-coverage
make cover-check
make surface-check
make size-check          # new in this feature
```

### All four build configurations

Per AGENTS.md, a green default run covers none of the others:

```bash
for tags in "" ax_no_grpc ax_no_otlp ax_no_grpc,ax_no_otlp; do
  go build ${tags:+-tags=$tags} ./...
  go test  -race ${tags:+-tags=$tags} ./...
done
```

`make test`, `make validate`, and `make lint` iterate `BUILD_TAG_MATRIX`
already.

### Import isolation, by hand

```bash
go list -deps github.com/rshade/ax-go/logging | grep -E \
  '^github.com/rshade/ax-go$|^github.com/rshade/ax-go/internal/telemetry|^go.opentelemetry.io/otel/sdk|^go.opentelemetry.io/otel/exporters|^google.golang.org/grpc|^google.golang.org/protobuf|^github.com/spf13/cobra|^net/http$|^crypto/tls$'
```

Expected output: nothing. Any line is a defect.

**Anchor the patterns.** An earlier draft of this command used a bare
`internal/telemetry`, which matches
`go.opentelemetry.io/otel/trace/internal/telemetry` — an internal helper of the
OTel **trace API**, which this surface legitimately depends on. That false
positive reports a defect where none exists. `internal/testutil`'s
`ForbiddenLoggingImports` has always anchored on the full `github.com/rshade/ax-go/`
path, so the automated assertion was never affected; only this documented
by-hand command was.

For scale, the boundary is visible in the package count alone:

```bash
go list -deps github.com/rshade/ax-go/logging | wc -l   # 103 packages
go list -deps github.com/rshade/ax-go         | wc -l   # 410 packages
```

Confirm the permitted dependencies are present:

```bash
go list -deps github.com/rshade/ax-go/logging | grep -E 'zerolog|otel/trace'
```

## Reproducing the size claims

The two probes are committed programs, so reproduction needs no synthesised
module, no `replace` stanza, and no network. `examples/logging` imports only the
isolated surface; `examples/rootlogging` is the same program with
`logging.NewLogger` swapped for `ax.NewLogger`. `make size-check` measures
exactly these two.

```bash
OUT="$(mktemp -d)"
for p in logging rootlogging; do
  go build -trimpath -ldflags="-s -w" -o "$OUT/$p.bin" "./examples/$p"
  printf '%-12s %s bytes\n' "$p" "$(stat -c%s "$OUT/$p.bin")"
done

awk -v iso="$(stat -c%s "$OUT/logging.bin")" \
    -v root="$(stat -c%s "$OUT/rootlogging.bin")" \
    'BEGIN { printf "reduction: %.1f%%\n", (1 - iso / root) * 100 }'
```

Reference values on linux/amd64, Go 1.26.5, measured 2026-07-22:

| Program | Bytes |
|---|---|
| `examples/rootlogging` | 12,017,929 |
| `examples/logging` | ~2,250,000 |
| Reduction | ~9,770,000 (−81.3%) |

Absolute values shift with toolchain version; the **ratio** is the durable
claim. That is why `sizecheck` gates both, and why they are adjusted under
different rules — see below.

## Adjusting the size gate

`sizecheck` enforces two constants, and they are **not** equally adjustable.

**The absolute ceiling (SC-001)** may be raised for a reviewed reason, on the
coverage/benchmark/surface protocol:

1. Confirm the increase is intentional and understood — a jump usually means a
   new transitive dependency, so check `go list -deps` before touching the
   constant.
2. Edit the ceiling constant in `internal/cmd/sizecheck/main.go`.
3. Verify with `make size-check`.
4. Record the reason in the commit message. Changes are auditable via
   `git blame`.

**The minimum reduction ratio (SC-002)** is toolchain-independent: both probes
move together when the compiler changes, so a ratio breach is never explained by
"a newer Go". It means the isolated surface gained weight the root facade did
not, which is the exact regression this feature exists to prevent. Lowering it
is a re-negotiation of the feature's headline claim, not a calibration — treat it
as a spec change and update SC-002 in the same commit.

Never move either constant to silence a failure whose cause you have not
identified.

## Updating the public surface baseline

`logging` is the seventh gated public package.

```bash
make surface-check                                   # expect drift on first run
make surface-update                                  # regenerate
git diff internal/cmd/surfacecheck/baseline.json     # review EVERY line
```

Both allowlists must change together or a guard test fails CI:

- `PublicPackages` in `internal/cmd/surfacecheck/inventory.go`
- `allowedPackages()` in `internal/cmd/apidiff-verdict/main.go`

## Troubleshooting

**`*lokiWriter does not implement logcore.Sink (unexported method drain)`** —
`lokiWriter`'s methods must be renamed to exported `Drain` and
`SanctionLabels`. Unexported method names are qualified by their defining
package, so a type in `ax` can never satisfy an interface in `logcore` that
requires a lowercase method. See `research.md` R3.

**`import cycle not allowed`** — one of two mistakes. Either `logcore` imported
root `ax` (it must not; only the reverse direction is legal), or root `ax` was
wired to delegate through the public `logging` package instead of straight to
`internal/logcore`, which makes `logging`'s parity test — an importer of `ax` —
circular. The two public surfaces are siblings over `logcore`, never a chain
(`research.md` R7).

**Isolation test fails on `net/http`** — something reachable from `logging`
pulled it in. It is the single largest size lever; find the edge with
`go list -deps` before changing anything else.

**`go-apidiff` reports a breaking change** — check whether the finding is a
**type relocation** before treating it as one. `go-apidiff` keys type identity on
the declaring package, so moving a type into another package and leaving an
identity-preserving alias reads as incompatible even though every consumer
compiles unchanged. ax-go shipped exactly that in v0.1.0 → v0.2.0 as a
non-breaking `feat:`.

`internal/cmd/apidiff-verdict` classifies these and prints them under
**"Type relocations (not gated)"**, leaving `public_breaking=false`. Anything
still listed under **"Incompatible changes"** is a real break. The usual causes
here: `ax.Logger` was redeclared (`type Logger interface{...}`) instead of aliased
(`type Logger = logcore.Logger`), or `ax.Flush` became a `var`. Compile-time
guards for both live in `logger_test.go` and `logging/identity_test.go`, so either
mistake should fail the build before the API gate sees it.

Note that `go-apidiff` needs its working directory **inside** the repository it is
diffing; passing only `--repo-path` from elsewhere silently produces an empty
report, which reads as "no changes" and is the most dangerous failure mode this
gate has:

```bash
cd "$REPO" && go-apidiff <base-ref> --repo-path=.
```

**Lint fails on `crosscompile.yml` SC2086** — inherited from PR #150, not
caused by this feature. See `research.md` R9.
