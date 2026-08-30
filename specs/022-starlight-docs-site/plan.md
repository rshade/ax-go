# Implementation Plan: Astro Starlight Docs Site Consuming rshade-theme

**Branch**: `009-starlight-docs-site` | **Date**: 2026-06-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/009-starlight-docs-site/spec.md`

## Summary

Scaffold an Astro 5 + Starlight documentation site **inside `docs/`** that
consumes the shared `rshade-theme` design tokens via a git submodule
(`docs/theme`) and publishes to GitHub Pages at `https://rshade.github.io/ax-go/`
through a Pages-from-Actions workflow with `submodules: recursive`. The shared
theme is wired through a minimal, copyable `theme-bridge.css` (four token
mappings, no bespoke styling); `@astrojs/sitemap` is wired explicitly for
machine-discoverability. The existing `docs/sources.md` is migrated into the
content collection; `docs/adr/*` is **deliberately excluded** (frozen log,
retired in place). ax-go is the **first adopter**, so the wiring is the canonical
reference later sites copy. The docs toolchain is fully isolated under `docs/`
and changes neither `go.mod` nor any Go quality gate; a `.markdownlintignore`
keeps the existing markdownlint gate green.

## Technical Context

**Language/Version**: JavaScript/TypeScript on Node 20 LTS (docs only). The Go
module (`github.com/rshade/ax-go`, Go 1.26.4) is untouched.

**Primary Dependencies** (scoped to `docs/package.json`, NOT `go.mod`):
`astro` ^5, `@astrojs/starlight` (Astro-5 line), `@astrojs/sitemap` ^3,
`starlight-links-validator`. Exact versions pinned in `docs/package-lock.json`.

**Storage**: N/A — static site; no runtime data, no database.

**Testing**: Build-time verification — `npm run build` fails on broken internal
links/anchors (`starlight-links-validator`, D11) and on a missing theme submodule
(unresolvable `@import`, FR-011). External: `curl` HTTP 200 + sitemap validity
(see `quickstart.md` walkthrough). Go gates (`go test -race ./...`, `go vet`,
`golangci-lint run`, `make doc-coverage`) remain the Go module's own,
unaffected.

**Target Platform**: GitHub Pages (static hosting) at the `/ax-go/` project
subpath; built on `ubuntu-latest`.

**Project Type**: Static documentation site, co-located in `docs/` of a Go
library repo (independent build).

**Performance Goals**: N/A (static site). Deploy completes in the workflow's
normal time (SC-003).

**Constraints**: Must not alter `go.mod` or any Go quality gate (FR-012);
must serve correctly from the `/ax-go/` subpath (FR-003); must not publish ADRs
(FR-007); must not silently deploy unstyled (FR-011); theme is a single source
of truth via submodule, never vendored (D8).

**Scale/Scope**: One Astro project under `docs/` (~10 config/content files),
one migrated page (`sources.md`), one landing page, one deploy workflow, one
`.markdownlintignore`, `.gitmodules` + gitlink, `.gitignore` addition.

**Governing ADR(s)**: **N/A.** Docs tooling is not governed by any
`docs/adr/*` record (those govern Go runtime behavior). No ADR is absorbed or
retired by this feature.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

The constitution governs the **Go library** (CLI contracts, determinism, IDs,
Go idioms). Most principles are N/A to a static docs site; the relevant ones
pass. This feature touches **no** Go public API or machine-payload surface.

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | ✅ N/A | No CLI; no stdout/stderr behavior |
| II. Deterministic Output & Exit Codes | ✅ N/A | No `ax` CLI output; build is reproducible via pinned lockfile |
| III. `__schema` | ✅ N/A | No commands added |
| IV. Agent-Safety Primitives | ✅ N/A | No CLI flags |
| V. Asymmetric JSON I/O | ✅ N/A | No config read/write path |
| VI. ADR-Governed Scope (Library, Not Application) | ✅ PASS | Adds no domain commands, no persistent state, no Go subpackages. `docs/src/` is not a module-root `src/` (no `.go` files; invisible to Go toolchain). **Frozen-ADR governance respected**: ADRs excluded from the site, retired in place untouched (FR-007). |
| VII. Test-First Discipline | ✅ PASS (adapted) | Docs analog: build-time link validation + missing-theme build failure are the verification gate; golden outputs are the contract checks in `contracts/`. Go test gates unaffected (SC-005). |
| VIII. Observability & ID Discipline | ✅ N/A | No logging/IDs |
| IX. Security & Resource Safety | ✅ PASS | Least-privilege Pages permissions (`contents: read`, `pages: write`, `id-token: write`); no secrets in the site; private-theme PAT (if needed) is a secret, never inlined; no TLS bypass. |
| X. Idiomatic Go & Dependency Minimalism | ✅ PASS | `go.mod` gains nothing; JS deps isolated under `docs/` (C-ISO-3). |
| XI. Stability & SemVer | ✅ PASS | No public Go API or `ax.Error`/`__schema` change → no library version bump. Maps to a `docs:`/`ci:` Conventional Commit (C-ISO-4). |
| XII. Deprecation Lifecycle | ✅ N/A | No exported symbols changed |

**ADR absorption gate (Constitution §Governance)**: N/A — no governing ADR.
`research.md` records no "Decision Records Absorbed" section and `tasks.md` will
include **no** ADR-retirement task. (The ADRs continue to be retired by the
Go features that absorb them, independently of this docs site.)

**Post-Phase 1 re-check**: All gates still pass. The Phase 1 design introduces no
Go changes, no `go.mod` entries, and no coupling between the docs build and the
Go quality gates; `.markdownlintignore` keeps the one shared gate (markdownlint)
green. **No violations → Complexity Tracking is empty.**

## Project Structure

### Documentation (this feature)

```text
specs/009-starlight-docs-site/
├── plan.md                    # This file
├── research.md                # Phase 0: D1–D11 decisions (no ADR absorption — N/A)
├── data-model.md              # Phase 1: config/content/workflow entities
├── contracts/
│   ├── theme-bridge.md        # Phase 1: canonical token-mapping contract (copyable)
│   └── site-and-deploy.md     # Phase 1: URL/sitemap/deploy/Go-isolation contracts
├── quickstart.md              # Phase 1: local dev + adoption guide + acceptance walkthrough
├── checklists/
│   └── requirements.md        # Spec quality checklist (all items pass)
└── tasks.md                   # Phase 2 (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
docs/                                  # Astro project root (co-located)
├── astro.config.mjs                   # NEW: site, base:'/ax-go', starlight()+sitemap()+linksValidator
├── package.json                       # NEW: docs-scoped deps (astro, starlight, sitemap, validator)
├── package-lock.json                  # NEW: pinned versions (reference source of truth)
├── tsconfig.json                      # NEW: from Starlight template
├── theme/                             # NEW: git submodule → rshade/rshade-theme (provides tokens.css)
├── public/                            # NEW: static assets (favicon, etc.)
├── src/
│   ├── styles/
│   │   └── theme-bridge.css           # NEW: @import tokens.css + 4 sl-var mappings (canonical)
│   ├── content.config.ts              # NEW: docsLoader() + docsSchema()
│   └── content/docs/
│       ├── index.mdx                  # NEW: minimal landing page
│       └── sources.md                 # MOVED from docs/sources.md (+ Starlight frontmatter)
├── adr/                               # UNCHANGED: stays outside src/ → excluded from the build
│   └── 00NN-*.md                      # (frozen log, retired in place by Go features)
└── sources.md                         # REMOVED (git mv into src/content/docs/sources.md)

.github/workflows/docs.yml             # NEW: Pages-from-Actions deploy, submodules: recursive
.gitmodules                            # NEW: created by `git submodule add`
.markdownlintignore                    # NEW: exclude docs/theme, node_modules, dist, .astro
.gitignore                             # MODIFIED: add docs/.astro/ (node_modules/dist already ignored)
CLAUDE.md                              # MODIFIED: SPECKIT plan pointer → this plan
```

**Structure Decision**: Astro project **co-located in `docs/`** (per issue #68).
Authored content under `docs/src/content/docs/`; `docs/adr/` deliberately left
outside `src/` so the build ignores it (the cleanest way to satisfy FR-007). The
Go module is untouched — `docs/` contains no `.go` files and is invisible to the
Go toolchain, so `docs/src/` does not violate the module-root "no `src/`" rule.

## Complexity Tracking

> No Constitution Check violations to justify. All applicable principles pass;
> the rest are N/A to a static docs site.

---

## Implementation Guide

> Detailed enough to drive `/speckit-tasks`. Ordered so each step is verifiable.

### 1. Scaffold the Astro project (`docs/`)

- `npm create astro@latest -- --template starlight` targeting `docs/` (or add
  `@astrojs/starlight` to a fresh Astro project rooted at `docs/`).
- Add deps: `npm install @astrojs/sitemap starlight-links-validator` (inside
  `docs/`). Commit `docs/package.json` + `docs/package-lock.json` (D1).
- Keep the toolchain scoped to `docs/`; do NOT touch the repo-root
  `package.json` or `go.mod` (C-ISO-3).

### 2. Add the theme submodule

```bash
git submodule add https://github.com/rshade/rshade-theme docs/theme
```

Creates `.gitmodules` + the `docs/theme` gitlink (commit both). If `rshade-theme`
is **private**, the deploy workflow needs a PAT (step 6, D8) — otherwise no token.

### 3. Wire the theme bridge (canonical, copyable)

Create `docs/src/styles/theme-bridge.css` exactly per `contracts/theme-bridge.md`
(four `var(--token)` mappings, `@import "../../theme/tokens.css"` first). No hex
literals, no font literals (C-TB-2, FR-005).

### 4. Configure `docs/astro.config.mjs`

Per `contracts/site-and-deploy.md`: `site: 'https://rshade.github.io'`,
`base: '/ax-go'`, `starlight({ title, customCss:['./src/styles/theme-bridge.css'],
plugins:[starlightLinksValidator()], sidebar:[…] })`, and
`sitemap({ filter: p => !/\/adr\//.test(p) })`. (Starlight won't double-add the
sitemap when it's present in the user config — D5.)

### 5. Migrate content; keep ADRs out of `src/`

- `git mv docs/sources.md docs/src/content/docs/sources.md`; prepend Starlight
  frontmatter (`title: Sources`, `description: …`). Ensure it passes markdownlint
  after the move (D9, FR-012).
- Add a minimal `docs/src/content/docs/index.mdx` landing page.
- Add `docs/src/content.config.ts` (`docsLoader()` + `docsSchema()`).
- **Leave `docs/adr/` untouched and outside `src/`** — the build ignores it
  (FR-007). Do NOT add ADR entries to the sidebar.

### 6. Add the deploy workflow

Create `.github/workflows/docs.yml` from the skeleton in
`contracts/site-and-deploy.md`: path-filtered `push` to `main` + `workflow_dispatch`;
`checkout@v7` with `submodules: recursive`; `setup-node@v4` Node 20 with npm cache
on `docs/package-lock.json`; `npm ci && npm run build` in `docs/`;
`upload-pages-artifact@v3` (`docs/dist`) → `deploy-pages@v4`; least-privilege
permissions; `concurrency: pages`. Add the `token:` line **only** if the theme
repo is private (D8). Must pass `actionlint` (existing CI gate).

### 7. Guard the existing gates + VCS hygiene

- Add repo-root `.markdownlintignore`: `docs/node_modules/`, `docs/theme/`,
  `docs/dist/`, `docs/.astro/` (D9, C-ISO-2). Confirm `npm run lint:md` (root)
  and the CI lint job stay green.
- Add `docs/.astro/` to `.gitignore` (node_modules/dist already covered by
  unanchored patterns — D10).

### 8. Enable Pages + first deploy

- Repo Settings → Pages → **Source: GitHub Actions** (FR-010). (Manual repo
  setting; note it in `tasks.md` as an operator step.)
- Push to `main` (path under `docs/**`) → workflow builds + deploys.

### 9. Verify against the contracts

Run the `quickstart.md` acceptance walkthrough: HTTP 200 at the subpath, no
root-relative 404s, theming matches finfocus, `sources.md` reachable, zero ADR
URLs, valid sitemap under `/ax-go/`, auto-deploy on docs push, Go gates
unchanged, and a missing-theme build fails (FR-011).

### 10. Docs (no ADR retirement)

Update `README.md` to link the published docs site (`https://rshade.github.io/ax-go/`).
**No ADR-retirement task** — none govern this feature.

---

## Artifact Summary

| Artifact | Path | Status |
|----------|------|--------|
| Spec | `specs/009-starlight-docs-site/spec.md` | ✅ Complete |
| Research (D1–D11; no ADR absorption) | `specs/009-starlight-docs-site/research.md` | ✅ Complete |
| Data model | `specs/009-starlight-docs-site/data-model.md` | ✅ Complete |
| Theme-bridge contract | `specs/009-starlight-docs-site/contracts/theme-bridge.md` | ✅ Complete |
| Site & deploy contract | `specs/009-starlight-docs-site/contracts/site-and-deploy.md` | ✅ Complete |
| Quickstart / adoption guide | `specs/009-starlight-docs-site/quickstart.md` | ✅ Complete |
| Requirements checklist | `specs/009-starlight-docs-site/checklists/requirements.md` | ✅ Complete |
| Tasks | `specs/009-starlight-docs-site/tasks.md` | ⏳ `/speckit-tasks` |

**Resolved (D8)**: `rshade-theme` is **public** (maintainer-confirmed
2026-06-19) → the deploy workflow works as written with the default
`GITHUB_TOKEN`; no PAT/secret required. The private-repo `token:` fallback is
retained in `contracts/site-and-deploy.md` only as guidance for future adopters.
