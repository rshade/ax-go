# Contributing to ax-go

## Change workflow

All public-API or runtime-behavior changes go through the **Spec Kit feature
workflow**. Read the constitution
(`.specify/memory/constitution.md`) before starting new work. Use the
`/speckit-specify`, `/speckit-plan`, `/speckit-tasks`, and
`/speckit-implement` commands in order.

Do not create or edit ADRs; record decisions in the feature's `research.md`
and, when cross-cutting, amend the constitution.

## Conventional Commits

`CHANGELOG.md` is generated automatically by release-please from Conventional
Commit messages. **Never hand-edit `CHANGELOG.md`.**

| Commit prefix | Effect |
| ------------- | ------ |
| `feat:` | Minor bump (new feature; may break under pre-v1.0 rules) |
| `fix:` | Patch bump (bug fix; always backward-compatible) |
| `feat!:` or `BREAKING CHANGE:` footer | Minor bump pre-v1.0 |
| `docs:`, `refactor:`, `perf:` | Minor or patch depending on scope |

See [`release-please-config.json`](release-please-config.json) for the full
section mapping.

## Compatibility matrix

The `## Compatibility` section of [`README.md`](README.md) documents which
ax-go version pairs with which Go version and which downstream consumer
versions.

### When to update the matrix

Update `README.md` → `## Compatibility` when:

1. **The minimum Go version changes** — bump the `go` value in
   [`mise.toml`](mise.toml) (the single source of truth for the toolchain
   version), then update `go.mod`'s `go` directive and the ax-go row's
   "Minimum Go version" column to match. `make validate` (and CI's
   `validate` job) fails closed if `go.mod` and `mise.toml` disagree.
2. **A new ax-go minor or major is released** — add or update the ax-go row.
3. **A downstream consumer tags its first ax-go-pinned release** — add a
   consumer row with the consumer name, the pinned ax-go version, and a brief
   note (e.g. `first consumer release`).

### Release checklist

When release-please opens a release PR it is labelled `autorelease: pending`,
which triggers `.github/workflows/release-checklist.yml`. That workflow posts
(or updates) a checklist comment on the PR automatically — no manual step
needed to surface the items.

The checklist items are:

- `README.md` → `## Compatibility` table reflects the new ax-go version
- If `mise.toml`'s pinned Go version changed, `go.mod`'s `go` directive and
  the "Minimum Go version" column are updated to match
- Any new downstream consumer that pinned this release has a row in the "Downstream consumers" table
- `CHANGELOG.md` preview looks correct (no missing or miscategorised entries)

To change the checklist content, edit the `body:` field in
`.github/workflows/release-checklist.yml`.

## Quality gates

Go, golangci-lint, actionlint, and govulncheck are pinned once in
[`mise.toml`](mise.toml). Install [mise](https://mise.jdx.dev/), then run
`make ensure` to fetch every pinned tool at the version CI uses.

Before opening a PR, run:

```bash
gofmt -l .
go test -race ./...
go vet ./...
golangci-lint run
make doc-coverage
make cover-check
```

All commands must exit cleanly. See [`AGENTS.md`](AGENTS.md) for the full
development workflow and coverage floor policy.

## Documentation standards

- Every exported Go symbol must carry a doc comment (`golangci-lint` gates this).
- Write contracts, not narration: state inputs, outputs, errors, exit codes,
  and invariants. Never restate what the code already says.
- Keep `README.md` and `examples/integration/` in sync with public behavior.
