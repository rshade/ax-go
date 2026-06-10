# Quickstart: Cut v0.1.0 — Make ax-go a Consumable, Pinnable Release

**Feature**: `003-cut-v010-release` | **Date**: 2026-06-09

How to validate each workstream end-to-end once implemented. Run from the repo
root.

## Precondition (research D10)

```bash
git status --short        # must be clean — no UU conflicts, no stray modifications
make ci                   # test -race + vet + golangci-lint + doc-coverage: all green
```

## Workstream B — Frozen success contracts

```bash
# Both new golden tests run as part of the standard suite (FR-006):
go test -race -run 'Golden|Envelope|JSONLine' ./...

# Drift detection self-check: flip one byte in a fixture, confirm the test
# fails, then restore it:
sed -i 's/"count":1/"count":2/' testdata/success_envelope.golden.json
go test -race -run Envelope .   # MUST fail
git checkout -- testdata/success_envelope.golden.json
```

Audit (FR-007 / SC-003): all 11 fixtures present and exercised —

```bash
ls testdata/*.golden.json | wc -l    # expect 11
git ls-files testdata | wc -l        # expect 11 (patch fixtures committed, D6)
```

## Workstream A — Version injection verification

```bash
grep -rn 'const version' examples/          # expect: no output (SC-002)
make build-example
./bin/ax-integration __schema | head -c 400 # version field non-empty, VCS-derived
```

## Workstream C — Release pipeline

```bash
# 1. Workflow keeps the RELEASE_PLEASE_TOKEN PAT (maintainer decision,
#    2026-06-09 — supersedes research D2 / the original GITHUB_TOKEN plan):
grep -n 'RELEASE_PLEASE_TOKEN' .github/workflows/release-please.yml  # expect: 1 match

# 2. A commit on main carries the one-shot version override (D1):
git log --grep='Release-As: 0.1.0' --oneline                          # expect: 1 commit

# 3. README no longer claims scaffold status (FR-012):
grep -n 'Implementation scaffold' README.md                           # expect: no output

# 4. After push to main: release-please run is green and the release PR
#    proposes 0.1.0 (NOT 1.0.0 — see research D1 / release-please #2087):
gh run list --workflow=release-please.yml --limit 3
gh pr list --search 'release 0.1.0 in:title'
```

## Post-tag verification (FR-013, SC-001, SC-005, SC-007)

```bash
# Capture the repo root first, then resolve from a fresh scratch module:
REPO="$(pwd)"
cd "$(mktemp -d)" && go mod init scratch && time go get github.com/rshade/ax-go@v0.1.0  # < 30s

# Changelog generated, not hand-authored:
git -C "$REPO" log -1 --format='%an %s' -- CHANGELOG.md  # author = release-please bot
grep -n '## ' "$REPO/CHANGELOG.md" | head -3             # contains 0.1.0 section

# Tag-built example reports v0.1.0 everywhere (SC-005):
git checkout v0.1.0
make build-example
./bin/ax-integration __schema | grep -o '"version":"[^"]*"'   # v0.1.0
```

## Definition of Done

- [ ] `make ci` green on the commit that becomes `v0.1.0` (SC-006)
- [ ] 11 golden fixtures committed and exercised (SC-003)
- [ ] Release PR proposed `0.1.0`, merged, workflow green (SC-004)
- [ ] `go get github.com/rshade/ax-go@v0.1.0` resolves (SC-001)
- [ ] Tag-built example reports `v0.1.0` in all three surfaces (SC-005)
- [ ] `CHANGELOG.md` 0.1.0 section fully generated (SC-007)
