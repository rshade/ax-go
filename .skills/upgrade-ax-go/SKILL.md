---
name: upgrade-ax-go
description: >-
  Use this skill for any task that changes — or reacts to a change in — a
  project's github.com/rshade/ax-go dependency, even when it looks routine;
  don't do it by hand instead. Trigger it when someone bumps, updates, or
  upgrades github.com/rshade/ax-go (aka ax-go, the agentic-experience lib) in
  go.mod — by hand, via go get -u, or a renovate/dependabot PR; moves ax-go
  from one version to another, including old versions and multi-minor jumps;
  sees staticcheck SA1019 deprecation warnings on ax.* symbols after updating
  ax-go; hits a compile error like 'undefined: ax.NewID' after updating; or
  finds the ax.Error envelope or __schema output changed shape after a bump.
  It reads the module's own CHANGELOG breaking notes and //Deprecated: notes,
  applies each documented migration, and verifies the build and tests stay
  green. Do NOT use it for developing ax-go itself, scaffolding a new CLI on
  ax-go, upgrading a different dependency in an ax-go project, or general
  questions about ax-go.
---

# Upgrade ax-go

You are upgrading a **consuming project's** dependency on
`github.com/rshade/ax-go` from one version to another. You are working in the
consumer's repository, not in ax-go itself.

## Why this works the way it does

ax-go is pre-v1.0, and its stability rules make the upgrade path *knowable from
the module you just downloaded* rather than something you have to guess:

- A `0.x.PATCH` release is bug-fixes only and always safe to take.
- A `0.MINOR.0` bump MAY break — either the Go API surface **or** a
  machine-payload shape (`ax.Error`, `__schema` output). Breaks ride the minor
  digit; they never auto-promote to `1.0.0`.
- Nothing is removed without warning. A symbol is deprecated with a
  `// Deprecated:` doc comment carrying a migration note, ships in **at least
  one** published `0.MINOR.0`, and only then is removed. `staticcheck SA1019`
  flags every call site.

Two consequences drive this whole procedure:

1. **The knowledge travels with the dependency.** When you
   `go get github.com/rshade/ax-go@vX.Y.Z`, the full module source lands in
   `$(go env GOMODCACHE)/github.com/rshade/ax-go@vX.Y.Z/` — including
   `CHANGELOG.md` (release-please-generated, with any breaking-change section)
   and every `// Deprecated:` doc comment. You never need ax-go's repo checked
   out. Read the machine sources; do not restate them from memory.
2. **A clean tree is a safe tree.** Because a symbol always ships deprecated
   for a full minor before removal, a project whose `staticcheck` run is free
   of `SA1019` at version *N* is safe to take *N+1*'s removals. Clearing SA1019
   at each step is the safety property, not busywork.

See `references/stability-model.md` for the exit-code map, the machine-payload
shapes to watch, and the governing constitution principles.

## Procedure

### Step 0 — Establish the upgrade path

- **Current version**: read it from the consumer's `go.mod`, or run
  `go list -m github.com/rshade/ax-go`.
- **Target version**: use what the user asked for. If they said "latest,"
  resolve it with `go list -m -versions github.com/rshade/ax-go` (or
  `go list -m -u github.com/rshade/ax-go` to see the available update) and
  confirm the concrete version you land on.

**Already-updated, just clearing warnings?** If the user has *already* bumped
ax-go and only wants the SA1019 deprecation warnings cleared (no new target
version), there is no version to bump: skip Steps 1–3 and go straight to Step 4
(find call sites) → Step 5 (migrate) → Step 7 (verify). The `// Deprecated:`
notes on the version already in their `go.mod` are all you need.

### Step 1 — Step through minors one at a time

If the jump crosses more than one minor (e.g. `v0.1.0 → v0.3.0`), do **not**
`go get` straight to the target. Upgrade one minor at a time
(`v0.1.0 → v0.2.0 → v0.3.0`), running Steps 2–7 for each hop.

The reason is concrete: a symbol deprecated in `v0.2.0` may be **removed** in
`v0.3.0`. If you jump directly, the `// Deprecated:` migration note is already
gone from the module source by the time you need it, and all you have left is a
compile error with no guidance. Stepping through means the note is always
present at the version that still carries the deprecated symbol. Patch releases
(`0.x.PATCH`) can be collapsed into their minor — they never break.

### Step 2 — Bump and populate the module cache

From the consumer repo, for the next hop's target version:

```bash
go get github.com/rshade/ax-go@vX.Y.Z
go mod tidy
```

This rewrites `go.mod`/`go.sum` and downloads the module source into the cache,
which the next step reads.

### Step 3 — Read what actually changed (machine sources)

Run the bundled scanner, which reads the cached `CHANGELOG.md` section for the
hop and lists the module's `// Deprecated:` symbols:

```bash
scripts/scan.sh vX.Y.Z [vCURRENT]
```

Then read, in priority order:

- **CHANGELOG breaking notes** — a `feat!:` / `BREAKING CHANGE:` commit produces
  a breaking-changes section in the release-please CHANGELOG. This is the only
  place that describes breaks the compiler cannot see (see Step 6).
- **Deprecation notes** — for each deprecated symbol the scanner lists, read its
  `// Deprecated:` paragraph. It names the replacement and how to migrate. You
  can also read them with `go doc github.com/rshade/ax-go` on the target
  version.

If the scanner cannot find the cached module (offline, or `go mod download`
hasn't run), fall back to reading
`$(go env GOMODCACHE)/github.com/rshade/ax-go@vX.Y.Z/CHANGELOG.md` directly.

### Step 4 — Find the real call sites

Do not migrate from the CHANGELOG's prose alone — let the tools point at
concrete lines in the consumer's code.

**Removals (hard breaks)** are always caught by the compiler, no extra tooling:

```bash
go build ./...
go vet ./...
```

**Deprecations (soft breaks)** are flagged by staticcheck's `SA1019`. Prefer
running it through `golangci-lint`: its bundled staticcheck is built to match
the Go toolchain, whereas a standalone `staticcheck` release lags the toolchain
and refuses a newer module with
`module requires at least go1.NN, but Staticcheck was built with go1.MM`.

```bash
# preferred: version-matched to the toolchain, and what ax-go itself uses
golangci-lint run --default=none --enable=staticcheck ./...
# or, if the consumer already configures golangci-lint:
golangci-lint run ./...
# fallback only if golangci-lint is absent (may error on a newer Go module):
staticcheck ./...
```

The SA1019 message names both the deprecated symbol and its replacement — e.g.
`SA1019: ax.NewID is deprecated: ... call NewEntityID instead` — so it is at
once your call-site list *and* your migration instruction.

If no linter is available, fall back to `scripts/scan.sh`'s deprecated-symbol
list and grep the consumer for those identifiers. `go build` still guarantees
removals cannot slip through undetected.

### Step 5 — Apply the documented migrations

For each flagged site, apply the migration from that symbol's `// Deprecated:`
note (or the CHANGELOG breaking note). Preserve behavior exactly — the
replacement is documented, so you are transcribing a known mapping, not
inventing one.

**If a break has no documented migration note, stop and surface it to the user
rather than guessing.** ax-go guarantees byte-identical output across runs;
agents downstream compare outputs and a silently wrong "fix" corrupts their
flows in a way that is expensive to detect later. A missing note is a signal to
ask, not to improvise.

### Step 6 — Handle payload/behavioral breaks the compiler can't see

Some breaks never produce a compile error or an SA1019 — a renamed field in the
`ax.Error` envelope, a changed `__schema` shape, an exit-code remapping, an
output-format change. Only the CHANGELOG breaking notes describe these. For each
such note, search the consumer for code and tests that depend on the old shape:

- golden/snapshot files asserting on `ax.Error` or `__schema` JSON
- code that reads specific envelope fields by name
- tests asserting specific exit codes or stdout byte-for-byte

Update them to the new shape. `references/stability-model.md` lists the shapes
worth grepping for.

### Step 7 — Verify

Re-run until all three are clean:

```bash
go build ./...
go test ./...            # add -race if the consumer's suite uses it
staticcheck ./...        # zero SA1019 remaining
```

Green build + green tests + zero SA1019 means this hop is done. If the jump was
multi-minor, return to Step 2 for the next hop.

### Step 8 — Offer new capabilities (optional, opt-in)

The upgrade is already complete and safe once Step 7 is green — everything in
this step is additive and never required. A new version almost always *adds*
capabilities (the CHANGELOG `### Added` / `### Changed` entries for the hops you
applied). Surface them so the user can adopt what they want, but never adopt
silently: a version bump and a feature adoption are separate decisions, and an
upgrade must not quietly become a refactor.

Run this once, after every hop is green:

1. **Collect** the `### Added` and notable `### Changed` entries across all hops
   (the `scan.sh` CHANGELOG delta already prints them). Skip pure internal, CI,
   dependency, and docs entries — keep the ones that change what a consumer can
   call or configure.
2. **Offer them as a multi-select.** Use the `AskUserQuestion` tool with
   `multiSelect: true`, one option per capability, each labeled with the feature
   and a one-line note on what adopting it would touch. The user can pick any
   subset — or none.
3. **Adopt only what was selected.** For each chosen capability, make a focused,
   minimal change to wire it in, keeping those edits in a separate logical group
   from the migration so the diff shows the two intents apart. Then re-run
   Step 7 (build / test / SA1019) so the adoption is verified too.
4. **If nothing is selected, or you cannot prompt** (a non-interactive or
   headless agent run), adopt nothing and just record the available
   capabilities in the report. The upgrade stands on its own regardless.

### Step 9 — Report

Emit a concise, deterministic summary (template below). If any item needed a
judgment call or had no documented migration, list it under **Manual
follow-ups** so the user can review it.

## Report structure

Use this template:

```markdown
# ax-go upgrade: vA.B.C → vX.Y.Z

## Hops applied
- vA.B.C → v… (repeat per minor if multi-step)

## Breaking changes handled
- <symbol or shape>: <what changed> → <how resolved> (file:line)
- (or: "None — additive upgrade.")

## Deprecations cleared (SA1019)
- <old symbol> → <new symbol> (<N> call sites)
- (or: "None.")

## Verification
- go build ./...:   PASS
- go test ./...:    PASS
- staticcheck:      0 SA1019 remaining

## New capabilities in this upgrade
- Adopted (opt-in): <feature> (file:line) — or "None selected."
- Available, not adopted: <feature> — or "None."

## Manual follow-ups
- <anything without a documented migration, or that needs a human decision>
- (or: "None.")
```

## Guardrails

- **No documented migration → stop and ask.** Never guess a replacement for a
  break the module doesn't document (Step 5).
- **Never hand-edit the consumer's `CHANGELOG.md`.** That is owned by their own
  release tooling, exactly as ax-go's is owned by release-please.
- **Do not `git commit`/`push` unless the user asks.** Leave the working tree
  ready for their review.
- **Do not skip minors in a single `go get` leap** when any intervening minor
  is breaking (Step 1).
- **Never adopt new features silently.** New capabilities are offered as an
  opt-in multi-select after the upgrade is green (Step 8); adopt only what the
  user selects, and keep those edits separate from the required migration.

## Bundled resources

- `scripts/scan.sh` — reads the cached CHANGELOG section for a hop, lists the
  module's `// Deprecated:` symbols, and reports SA1019 via `golangci-lint`
  (falling back to `staticcheck`).
- `references/stability-model.md` — exit-code map, machine-payload shapes to
  watch, deprecation lifecycle, and the governing constitution principles.
