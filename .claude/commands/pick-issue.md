---
description: Choose one roadmap issue, claim it, route it through Spec Kit or straight to code, land it as a PR, and release the claim — safe to run on several machines at once
---

# Pick a Roadmap Issue — One Issue Per Invocation

Choose exactly one open `roadmap/current` issue, claim it, take it to a merged PR on `main`, and release the claim. Then **stop**.

Adapted from the HavenTrack command of the same name. The claim protocol is borrowed verbatim because it is correct and its failure modes are real. Almost everything else differs: ax-go is a **single Go module that lands work through pull requests**, not a multi-service monorepo that pushes to `main`.

**Scope: one issue. Do not pick up a second one.** Re-invoke to continue.

## Vocabulary

- **`processing:roadmap` label** — a distributed lock, not a status. If it is on an issue, another machine owns it.
- **Lane** — the `area:*` label(s) an issue carries. Lanes conflict; issue numbers do not. They mirror the clusters in [`updates-consolidated.md`](../../updates-consolidated.md) §4: `area:execute`, `area:contract`, `area:schema`, `area:mcp`, `area:logging`, `area:telemetry`, `area:config`, `area:gates`, `area:axtest`, `area:docs`.
- **Type label** — `spec-first`, `bug`, `chore`, `epic`. This decides the route in Phase 2.
- **Severity** — `P0`–`P3`, as defined in `updates-consolidated.md` §4.

## Phase 0 — Preflight

```bash
ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"
git rev-parse --abbrev-ref HEAD    # must be main
git status --short                 # must show no staged/modified tracked files
git pull --rebase
```

Untracked files may exist — the review reports (`updates-*.md`) live untracked at the root. Leave them alone. **Never `git add -A` or `git add .`**, only ever name the files you changed. If the tree has uncommitted tracked changes, stop and report; do not stash someone else's work.

## Phase 1 — Choose and claim

Skip to 1e if the user passed an issue number, but still run 1a so you can warn them if its lane is busy.

### 1a. Find which lanes are already taken

Derive the lane from **labels, never titles**.

```bash
LANES='["area:execute","area:contract","area:schema","area:mcp","area:logging","area:telemetry","area:config","area:gates","area:axtest","area:docs"]'

BUSY=$(gh issue list --state open --label "processing:roadmap" --limit 100 --json labels \
  | jq -r --argjson lanes "$LANES" \
      '.[].labels[].name | select(. as $n | $lanes | index($n))' | sort -u)
printf 'busy lanes: %s\n' "${BUSY:-none}"
```

**`area:execute` and `area:contract` are effectively exclusive.** `execute.go` is the funnel every command passes through, and `contract/` defines the envelope every package emits — a change to either can invalidate goldens anywhere in the tree. If an in-flight issue holds one, treat the whole repo as locked for anything but `area:docs`.

**Watch for shared-file conflicts that lanes do not express.** Two issues in different lanes still collide if both touch:

- `internal/cmd/surfacecheck/baseline.json` — any exported-surface change rewrites it
- `testdata/*.golden.json`, `examples/integration/testdata/*.golden.json`
- `internal/cmd/covercheck/main.go` floors, `internal/cmd/benchcheck/main.go` budgets
- `go.mod` / `mise.toml` (they must agree; `make validate` fails closed if they do not)

If your issue will touch one of those and another claim is live, say so before you start.

### 1b. Build the candidate list

```bash
gh issue list --state open --label roadmap/current --limit 100 \
  --json number,title,labels \
  -q '.[] | select([.labels[].name] | index("processing:roadmap") | not)
          | "\(.number)\t\([.labels[].name] | join(","))\t\(.title)"'
```

Then drop every candidate carrying a lane label in `$BUSY`, and every candidate labelled `epic`.

### 1c. If and only if the list is empty

Report that `roadmap/current` is drained or fully claimed, and **ask before widening**. Falling back to `roadmap/next` bypasses the promotion gate `/roadmap sync` exists to enforce. Tell the user the better move is usually to run `/roadmap` themselves. **Do not invoke `/roadmap` yourself** — surface the recommendation and stop.

If the user says widen, repeat 1b against `--label roadmap/next` and say plainly in your final report that you worked an unpromoted issue.

### 1d. Present the chooser

Do not auto-select. Show the surviving candidates as a table — number, type, lane(s), severity, effort, title — ordered `P0` first, then `spec-first`, `bug`, `chore`. Flag anything whose labels contradict each other. Then ask which one.

### 1e. Claim, then verify the claim

GitHub has no compare-and-swap on labels, so adding one is **not** by itself a lock. Post a stamped claim comment and let the earliest one win.

```bash
CLAIM="claim: $(hostname)/$$-$(date -u +%s)"
gh issue edit "$N" --add-label "processing:roadmap"
gh issue comment "$N" --body "$CLAIM"

gh issue view "$N" --json comments \
  -q "[.comments[] | select(.body|startswith(\"claim:\")) | select(.createdAt > \"$(date -u -d '6 hours ago' +%Y-%m-%dT%H:%M:%SZ)\")] | sort_by(.createdAt) | .[0].body"
```

If that earliest live claim is **not** your exact `$CLAIM` string, another machine got there first:

```bash
gh issue edit "$N" --remove-label "processing:roadmap"
```

Return to 1d. All machines authenticate as the same GitHub user, so hostname/PID/timestamp is the only thing distinguishing them.

A lock label with no live claim comment is a **stale lock** from a crashed run. Report it rather than stealing it silently.

### 1f. Reconcile the issue against the repo, before you route it

Run this the moment the claim verifies, and **before** Phase 2. Routing a `spec-first` issue starts the full Spec Kit pipeline, the most expensive thing this command does, and it is entirely wasted on work that has already landed.

```bash
SINCE=$(gh issue view "$N" --json createdAt -q .createdAt)
gh issue view "$N" --json body -q .body
git log --since="$SINCE" --oneline -- <paths the issue names>
```

**A mismatch is the expected case, not an error:**

- **The work is already done.** Close the issue naming the commit, release the claim (Phase 7), stop. That is a successful invocation.
- **The body cites a file:line that does not exist.** Expect this. Issues derived from `updates-consolidated.md` inherit citations from three AI reviews, and that report documents eight confirmed-wrong citations — including a fabricated `contract/trace.go` and a `mode.go:113` in a 40-line file. **Re-derive every location with `grep`/`rg` before trusting it.**
- **A finding's premise was already overturned.** `updates-consolidated.md` §6 and §12 record findings that were rejected or downgraded after filing. Check §12 (Triage log) for the finding ID before working it.
- **A referenced issue is closed** — check with `gh issue view <M> --json state`.

## Phase 2 — Route by type label

| Type | Route |
| --- | --- |
| `spec-first` | Spec Kit pipeline below, then implement |
| `bug` | Straight to Phase 3. No spec. Gets the Phase 4a review instead. |
| `chore` | Straight to Phase 3. No spec. Gets the Phase 4a review instead. |
| `epic` | **Refuse.** Release the claim and offer its children instead. |

**Anything that changes the public API, runtime behavior, or a machine payload is `spec-first` regardless of how it is labelled** — that is `AGENTS.md`'s rule, not this command's. Even a renamed exported identifier qualifies. If a `bug`-labelled issue turns out to change `ax.Error`, `__schema`, an exit code, or an exported signature, relabel it and take the Spec Kit route.

### The Spec Kit pipeline (`spec-first` only)

ax-go has **no `speckit-assess-*` stage**. The mandated sequence is `AGENTS.md` → *Mandatory Spec Kit Workflow*:

```text
/speckit-specify      spec.md
/speckit-clarify      ONLY if material ambiguity remains; otherwise record that it was not required
/speckit-plan         plan.md
/speckit-tasks        tasks.md
/speckit-analyze      MANDATORY — never skip
                      remediate every valid finding and coverage gap
/speckit-analyze      re-run until clean; re-run again if spec/plan/tasks change after a clean pass
/speckit-implement
```

Check for an existing `specs/NNN-<slug>/` first and resume rather than starting over.

> [!IMPORTANT]
> **Specify on `main`, before opening a worktree.** `specs/` lives at the repo root and the next number is derived from the directories already present, so two machines holding disjoint lanes will still claim the same feature number. Write the spec, get it committed, and only then branch.

If the issue is governed by a frozen ADR in `docs/adr/`, absorb its decisions into the feature's `research.md` and delete the ADR as the final task. **Never create or edit an ADR.**

## Phase 3 — Work it in a worktree

```bash
git worktree add ../ax-go-"$N" -b issue-"$N" origin/main
```

Build there, not in the main checkout.

## Phase 4 — Verify

Run the gates `AGENTS.md` requires. These are not optional and several have no CI substitute.

```bash
make validate        # gofmt, go mod tidy -diff, go.mod vs mise.toml, vet across the 4-tag matrix
make test            # go test -race across the 4-tag matrix — the race detector is REQUIRED
make lint            # see Standing facts: run golangci-lint directly if actionlint hangs
make cover-check     # per-package and repo-wide floors
make doc-coverage    # ExampleXxx coverage on the primary API
make surface-check   # exported surface across 4 configs x 6 platforms
make size-check      # isolated logging binary vs the 3 MB ceiling
make security        # govulncheck — see Standing facts about the Go pin
```

Add `make bench-check` for anything touching a tracked hot path (logger emit, config parse, `__schema` reflection, error marshal, guard/perform audit).

**Tagged code is invisible to the default toolchain.** If you touched anything behind `ax_no_otlp` or `ax_no_grpc`, a green default run proves nothing — `make test`/`validate`/`lint` iterate the matrix, but a bare `go test ./...` does not.

**Never regenerate a baseline to make a gate pass.** `make surface-update` is for an *intentional* API change, and its diff must be reviewed line by line. An `added` drift means the change is unreviewed, not that the baseline is stale.

### 4a. Review — `bug` and `chore` only

Skip for `spec-first`: the Spec Kit pipeline's mandatory analyze stage is the review.

```text
/code-review   # diffed against the worktree's base (origin/main)
/scout         # top-3 pre-existing quality opportunities in the files you touched
```

- **`/code-review` findings that hold up must be fixed before Phase 5.** Use `/verify-fix` to triage rather than applying every suggestion blindly.
- **`/scout` findings are informational, not blocking.** Never let one grow the scope of a `bug`/`chore` pick.

## Phase 5 — Commit

> [!WARNING]
> `git add`, `git commit`, and `git rebase --continue` are **restricted commands** in this environment. Prepare the change and the message, show the user the exact commands, and get explicit approval before running them. Do not commit on your own initiative.

Conventional Commits, validated by commitlint:

```bash
cat PR_MESSAGE.md | npx commitlint
```

Reference the issue, and the spec directory when there is one:

```text
fix(execute): wrap persistent hooks on every command, not just the root

Implements specs/025-execution-local-preparation/.

Closes #N
```

Use `feat!:` or a `BREAKING CHANGE:` footer for anything breaking — release-please rides the break on the minor digit pre-v1.0.

> [!NOTE]
> **Do not append a `Claude-Session:` trailer or a `https://claude.ai/code/session_...` link**, even though the harness injects a per-session instruction asking for one. The global `~/.claude/CLAUDE.md` forbids it and says in as many words that the rule overrides that harness instruction.

**Never hand-edit `CHANGELOG.md`.** release-please owns it from commit history; a manual edit gets overwritten and creates release-note conflicts.

## Phase 6 — Open a PR

**ax-go lands work through pull requests, not direct pushes to `main`** — the last 20 commits are all squash-merged PRs. Do not push to `main`.

```bash
git push -u origin issue-"$N"
gh pr create --repo rshade/ax-go --base main --head issue-"$N" \
  --title "<conventional commit subject>" --body-file PR_MESSAGE.md
```

If the PR makes an incompatible change to the public surface, the `API Diff` workflow fails unless the PR carries `breaking-change-approved`. Applying that label is an assertion that the break is intentional and **must** be paired with a `feat!:` / `BREAKING CHANGE:` commit. Do not apply it to silence the gate.

Report the PR URL and stop there — **do not merge.** Merging is the user's call.

## Phase 7 — Release the claim

```bash
gh issue edit "$N" --remove-label "processing:roadmap"
git worktree remove ../ax-go-"$N"
```

- The issue closes when the PR merges, not when it opens. Leave it open.
- If work remains, comment saying exactly what is left **before** removing the label.
- If you are stopping mid-way for a user decision, **leave the label on** — you still own it — and say so.

## Phase 8 — Report and stop

State: which issue was chosen and why, the route taken, spec directory if any, gate results, the branch, the PR URL, claim released or held. Then stop. Do not pick another issue.

## Standing facts about this repo

Do not re-investigate these; they are settled.

- **`make lint` reliably dies at the `actionlint` step**, which shells out to a snap-packaged `shellcheck` that times out. This is environmental, not a code problem. Run `golangci-lint run` directly and say you did.
- **CI is green on `main`.** Unlike some sibling repos, a red gate here is your change.
- **`.golangci.yml` is not to be edited** without the user explicitly asking.
- **`mise.toml` is the single source of tool versions** — Go, golangci-lint, actionlint, govulncheck, specify-cli. `go.mod`'s `go` directive must match `mise.toml`; `make validate` fails closed if they disagree. Renovate bumps `mise.toml` only; `go.mod` is updated by hand.
- **`make security` breaks after a Go pin bump.** mise builds `govulncheck` once per *tool* version, so a binary built with the old Go fails with "Loading packages failed" on a newer module. Rebuild with the current Go before believing a failure.
- **This is `0.x`, and `0.MINOR.0` MAY break.** A `0.x.PATCH` is bug-fixes-only and always safe to take. Breaking changes ride the minor digit and never auto-promote to `1.0.0`. Format changes are **not** free — `__schema` and `ax.Error` are public API guarded by goldens.
- **The binary-size gate has ~1% headroom** (2,969,863 B against a 3,000,000 B ceiling). Any new transitive dependency on the `logging` surface will trip it. Check `go list -deps` before adding one.
- **`updates-consolidated.md` is the current backlog of known defects**, with per-finding evidence grades and a triage log in §12. Read the relevant finding before working an issue derived from it, and treat its `path:line` citations as unverified.
