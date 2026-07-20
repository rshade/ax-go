# ax-go stability model (reference)

Background for the `upgrade-ax-go` skill. Read this when you need the precise
contract behind an upgrade decision.

## SemVer, pre-v1.0 (Constitution Principle XI)

ax-go is in the `0.x` series. The digit that moves tells you the risk:

| Bump | Meaning | Safe to take? |
|------|---------|---------------|
| `0.x.PATCH` | Bug fixes only | Always |
| `0.MINOR.0` | MAY break the Go API surface **or** a machine-payload shape | Read the CHANGELOG first |

A breaking change rides the minor digit and never auto-promotes to `1.0.0`.
"Machine-payload shape" means the JSON contracts agents depend on — principally
the `ax.Error` envelope and `__schema` output — which are additive-tolerant
(new fields are not a break; renamed/removed/retyped fields are).

The apidiff-gated public surface is the root package `ax` plus the public
packages `config`, `contract`, `id`, `mcp`, and `schema`. Anything under
`internal/` is not importable by consumers, so it can never be the source of a
break in your code.

## Deprecation lifecycle (Constitution Principle XII)

Removal is never a surprise:

1. The symbol gains a `// Deprecated:` doc-comment paragraph with a migration
   note.
2. That note ships in **at least one** published `0.MINOR.0` release.
3. Only in a later minor is the symbol removed.

`staticcheck`'s `SA1019` flags every call site of a deprecated symbol. Run it
through `golangci-lint` (`golangci-lint run --default=none --enable=staticcheck
./...`) — that is how ax-go's own lint runs it, and its bundled staticcheck is
version-matched to the Go toolchain. A standalone `staticcheck` release lags the
toolchain and errors on a newer module
(`... but Staticcheck was built with go1.MM`), so prefer golangci-lint. The
practical rule: **a project with zero SA1019 against version N is safe to take
N+1's removals**, because anything N+1 removes was already deprecated in N.

## Exit codes (stable contract)

If the consumer's tests assert on exit codes, these are the deterministic
meanings and should not change across an upgrade unless a CHANGELOG note says so:

| Code | Meaning |
|------|---------|
| `0` | success |
| `1` | unknown / internal error |
| `2` | validation / bad input |
| `3` | network / timeout |
| `4` | authentication / permission |

## Machine-payload shapes to grep for (Step 6)

These change without any compile error or SA1019, so only the CHANGELOG
breaking notes reveal them. When a note mentions one, search the consumer for
code or golden files that depend on the old shape:

- **`ax.Error` envelope** — JSON error shape on stderr. Look for snapshot/golden
  files and code reading envelope fields by name (e.g. `error_code`,
  `retryable`, `retry_after_seconds`).
- **`__schema` output** — the structured command/flags/examples JSON, and its
  `__schema --as=mcp` adapter. Golden files that pin this are common.
- **Streaming/output format** — bounded payloads are strict minified JSON;
  streaming/unbounded sets are NDJSON. A change here breaks parsers.
- **Non-deterministic fields are exempt** — `timestamp`, `trace_id`, `span_id`,
  and auto-generated `idempotency_key` are documented as varying; do not treat a
  diff in those as a break.

## Where the truth lives

- `CHANGELOG.md` in the module cache — canonical record of what each version
  changed; release-please-generated from Conventional Commits. Never hand-edit
  a consumer's own CHANGELOG to reflect an ax-go upgrade.
- `// Deprecated:` doc comments in the module source — the per-symbol migration
  instructions; also visible via `go doc github.com/rshade/ax-go`.
- The constitution (`.specify/memory/constitution.md` in the ax-go repo) is the
  supreme source for these rules if you ever need to go deeper than the
  CHANGELOG.
