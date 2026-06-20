---

description: "Task list for Astro Starlight docs site consuming rshade-theme"
---

# Tasks: Astro Starlight Docs Site Consuming rshade-theme

**Input**: Design documents from `specs/009-starlight-docs-site/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: This feature has no Go/unit test framework. Per the constitution's
Principle VII (adapted for a static docs site), verification is **build-time +
acceptance**: `starlight-links-validator` fails the build on broken subpath
links, a missing theme submodule fails the build, and each story has an explicit
acceptance task tracing to `contracts/` and the `quickstart.md` walkthrough. No
TDD "write failing tests first" tasks are generated (there is nothing to unit
test).

**Organization**: Tasks are grouped by user story. The Astro project (a building,
serveable site) is the shared spine built in Setup + Foundational; each user
story then adds one independently-verifiable capability on top.

**No governing ADR** → there is intentionally **no ADR-retirement task** (see
Polish phase note).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US4)
- **[MANUAL]**: Operator action (repo settings / live verification), not a code edit
- Exact file paths are included in each description

## Path Conventions

The Astro project is co-located in `docs/` (per plan.md Structure Decision). All
docs paths are repo-relative under `docs/`. The Go module at the repo root is not
touched by any task.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Initialize the Astro/Starlight project under `docs/` and protect the
existing Go/lint quality gates from the new JS toolchain.

- [X] T001 Scaffold Astro 5 + Starlight into `docs/` via `npm create astro@latest -- --template starlight` (creates `docs/package.json`, `docs/package-lock.json`, `docs/astro.config.mjs`, `docs/tsconfig.json`, `docs/src/`, `docs/public/`). Keep the toolchain scoped to `docs/`; do NOT modify the repo-root `package.json` or `go.mod` (FR-001, C-ISO-3).
- [X] T002 Add docs dependencies `@astrojs/sitemap` and `starlight-links-validator` with `npm install` run in `docs/`; commit the updated `docs/package.json` + `docs/package-lock.json` with pinned versions (research.md D1, D5, D11).
- [X] T003 [P] Create repo-root `.markdownlintignore` excluding `docs/node_modules/`, `docs/theme/`, `docs/dist/`, `docs/.astro/` so the Astro subtree cannot break the existing markdownlint gate (FR-012, research.md D9, contract C-ISO-2).
- [X] T004 [P] Add `docs/.astro/` to the repo-root `.gitignore` (`node_modules/`, `dist/` are already covered by unanchored patterns) (FR-014, research.md D10).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Make the site actually **build and serve** a page. Every user story
needs a buildable site before it can be verified independently.

**⚠️ CRITICAL**: No user story work can be verified until this phase is complete.

- [X] T005 Edit `docs/astro.config.mjs` to set `site: 'https://rshade.github.io'` and `base: '/ax-go'` on the scaffolded config (FR-002, FR-003, research.md D4).
- [X] T006 [P] Verify/adjust `docs/src/content.config.ts` to declare the `docs` collection with Starlight's `docsLoader()` + `docsSchema()` (data-model.md "Content Collection").
- [X] T007 [P] Replace the scaffolded sample content with a minimal ax-go landing page at `docs/src/content/docs/index.mdx` (`title` frontmatter required) so `npm run build` produces a serveable site (data-model.md "Content Collection").
- [X] T008 Run `npm run build` in `docs/` and confirm it succeeds and emits `docs/dist/` (foundation gate before any story verification).

**Checkpoint**: `docs/` builds to a serveable static site at the `/ax-go/` base. User stories can now begin.

---

## Phase 3: User Story 1 - Visitor reaches a live, auto-deploying docs site (Priority: P1) 🎯 MVP

**Goal**: A live site at `https://rshade.github.io/ax-go/` (HTTP 200) that
auto-deploys on docs changes, with internal links and a sitemap that resolve
correctly under the `/ax-go/` subpath.

**Independent Test**: `curl -sI https://rshade.github.io/ax-go/` returns 200;
nav links and assets resolve under `/ax-go/` with no root-relative 404; the
sitemap lists pages as correct `/ax-go/` absolute URLs; a pushed docs change goes
live with no manual step.

### Implementation for User Story 1

- [X] T009 [US1] Wire the `@astrojs/sitemap` integration into `docs/astro.config.mjs` `integrations: [...]` (alongside `starlight()`), producing absolute sitemap URLs under `https://rshade.github.io/ax-go/` (FR-015, US1 scenario 4, contract C-SM-1/2/3).
- [X] T010 [US1] Wire the `starlight-links-validator` plugin into the `starlight({ plugins: [...] })` config in `docs/astro.config.mjs` so `npm run build` fails on broken internal/subpath links (FR-003, SC-001, research.md D11).
- [X] T011 [P] [US1] Create `.github/workflows/docs.yml` from the skeleton in `contracts/site-and-deploy.md`: path-filtered `push` to `main` (`paths: ['docs/**', '.github/workflows/docs.yml']`) + `workflow_dispatch`; `actions/checkout@v7` with `submodules: recursive` (no-op until the theme lands in US2, forward-compatible); `actions/setup-node@v4` Node 20 with npm cache on `docs/package-lock.json`; `npm ci && npm run build` in `docs/`; `actions/upload-pages-artifact@v3` (`path: docs/dist`) → `actions/deploy-pages@v4`; permissions `contents: read`, `pages: write`, `id-token: write`; `concurrency: { group: pages, cancel-in-progress: false }` (FR-008, FR-009, FR-010, contract C-DEP-1..7). `rshade-theme` is public, so no `token:` line is needed (research.md D8).
- [ ] T012 [MANUAL] [US1] In repo Settings → Pages, set **Source: GitHub Actions** to enable Pages-from-Actions delivery (FR-010, contract C-DEP-3).
- [ ] T013 [MANUAL] [US1] Verify US1: push a docs change to `main`; confirm `GET https://rshade.github.io/ax-go/` is HTTP 200, internal links/assets resolve under `/ax-go/` (no root-relative 404), `…/ax-go/sitemap-index.xml` is valid XML with `/ax-go/` absolute URLs, and the change went live with no manual deploy step (SC-001, SC-003, SC-008, contracts C-URL-1/2, C-SM-1/2, C-DEP-5).

**Checkpoint**: The docs site is live and auto-deploys. This is the MVP — deployable and demoable on its own (un-themed Starlight defaults, default content).

---

## Phase 4: User Story 2 - Site is visually consistent with the rshade portfolio (Priority: P2)

**Goal**: The site renders with the shared portfolio fonts (Inter / JetBrains
Mono) and accent color, sourced from `rshade-theme` tokens via the canonical
theme bridge.

**Independent Test**: The deployed/preview site's body font, monospace font, and
accent color match finfocus and the rshade blog; the bridge contains only token
mappings; a missing theme submodule fails the build.

### Implementation for User Story 2

- [X] T014 [US2] Add the `rshade-theme` submodule with `git submodule add https://github.com/rshade/rshade-theme docs/theme`; commit the resulting `.gitmodules` and the `docs/theme` gitlink (FR-004, research.md D8, data-model.md "Theme Submodule").
- [X] T015 [US2] Create `docs/src/styles/theme-bridge.css` verbatim per `contracts/theme-bridge.md`: `@import "../../theme/tokens.css"` first, then the four `var(--token)` → `--sl-*` mappings; NO hex colors or font-family literals (FR-005, contract C-TB-1/2).
- [X] T016 [US2] Add `customCss: ['./src/styles/theme-bridge.css']` to the `starlight(...)` config in `docs/astro.config.mjs` (FR-004, contract C-TB-3).
- [ ] T017 [MANUAL] [US2] Verify US2 — two distinct checks:
  - **(a) Theming (SC-002, C-TB-2):** `npm run build` succeeds with the theme applied; rendered body/mono fonts and accent color match finfocus (primary) and the rshade blog (secondary); `grep -E '#[0-9a-fA-F]{3,6}|font-family' docs/src/styles/theme-bridge.css` finds nothing.
  - **(b) Fail-closed gate (FR-011, SC-007, contract C-DEP-6/C-TB-5):** temporarily de-initialize `docs/theme` (e.g. `git submodule deinit -f docs/theme`) and confirm `npm run build` in `docs/` exits **non-zero**. If the build instead only warns and succeeds (Vite/Astro may treat a missing CSS `@import` as non-fatal), this requirement is NOT met — add an explicit build-time presence check for `docs/theme/tokens.css` (e.g. a `prebuild` npm script that exits non-zero when the token file is absent) per C-TB-5, then re-verify. Restore the submodule (`git submodule update --init --recursive`) when done.

**Checkpoint**: User Stories 1 AND 2 both hold — the live site is now brand-consistent.

---

## Phase 5: User Story 3 - Existing content reachable; ADRs deliberately excluded (Priority: P2)

**Goal**: `sources.md` is reachable in the nav; `docs/adr/*` appears nowhere in
the published site or sitemap.

**Independent Test**: `sources.md` reachable from nav; no ADR page in nav,
sitemap, or by direct URL; sitemap contains zero `/adr/` URLs; `docs/adr/` is
untouched and outside `src/`.

### Implementation for User Story 3

- [X] T018 [US3] `git mv docs/sources.md docs/src/content/docs/sources.md` and prepend Starlight frontmatter (`title: Sources`, `description: …`); confirm it passes markdownlint after the move (FR-006, research.md D3/D9, data-model.md "Content Collection").
- [X] T019 [US3] Add a `Sources` entry to the `starlight({ sidebar: [...] })` config in `docs/astro.config.mjs` (`slug: 'sources'`); add NO ADR entries (FR-006, FR-007, contract C-URL-3).
- [X] T020 [US3] Add the defensive `filter: (page) => !/\/adr\//.test(page)` option to the `sitemap(...)` integration in `docs/astro.config.mjs` (FR-007, SC-008, research.md D5, contract C-SM-4).
- [ ] T021 [MANUAL] [US3] Verify US3: `sources.md` is reachable from the nav; no ADR page is reachable via nav, `…/ax-go/sitemap-index.xml`, or any direct `…/adr/…` URL; `docs/adr/` is unchanged and outside `docs/src/` (SC-004, SC-008, contracts C-URL-3/4, C-SM-4).

**Checkpoint**: User Stories 1, 2, AND 3 all hold — live, themed, populated, ADR-clean.

---

## Phase 6: User Story 4 - Contributor previews and extends the docs locally (Priority: P3)

**Goal**: A contributor can reproduce the deployed site locally (theme included)
and see edits live.

**Independent Test**: On a fresh clone, documented setup (including submodule
init) yields a themed local preview; editing a page is reflected live.

### Implementation for User Story 4

- [X] T022 [US4] Confirm `docs/package.json` exposes working `dev`, `build`, and `preview` scripts (Astro defaults); ensure `quickstart.md` "Local development" steps match the actual scripts and submodule-init command (FR-013).
- [ ] T023 [MANUAL] [US4] Verify US4: from a fresh clone, run `git submodule update --init --recursive`, `npm ci` and `npm run dev` in `docs/`; confirm the local preview renders with the shared theme at `http://localhost:4321/ax-go/` and that editing a `docs/src/content/docs/*` page is reflected live (SC-006, spec US4 scenarios 1–2).

**Checkpoint**: All four user stories are independently functional.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Wire the site into the repo's docs and confirm zero collateral
impact on the Go module.

- [X] T024 [P] Add a link to the published docs site (`https://rshade.github.io/ax-go/`) in `README.md` (FR-016; no `CHANGELOG.md` edit — release-please owns it; capture the change as a `docs:` Conventional Commit, contract C-ISO-4).
- [X] T025 [P] [MANUAL] Confirm the Go quality gates are unchanged: `go build ./...`, `go test -race ./...`, `go vet ./...`, `golangci-lint run`, and `make doc-coverage` are all clean, and the root markdownlint gate (`npm run lint:md`) is green with the new `.markdownlintignore` (FR-012, SC-005, contracts C-ISO-1/2).
- [ ] T026 [MANUAL] Run the full `quickstart.md` acceptance walkthrough end-to-end, confirming every contract row (C-URL-*, C-SM-*, C-TB-*, C-DEP-*, C-ISO-*) passes against the live deployment.

> **No ADR-retirement task.** This feature is governed by no `docs/adr/*` record
> (docs tooling is outside the Go runtime contract), so the template's final
> ADR-retirement task is intentionally omitted (plan.md Constitution Check; the
> ADRs continue to be retired by the Go features that absorb them).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T001 first (scaffolds the project); T002 depends on T001; T003, T004 are independent of T001/T002.
- **Foundational (Phase 2)**: Depends on Setup. T005/T006/T007 then T008 (build gate). BLOCKS all user stories.
- **User Stories (Phase 3–6)**: All depend on the Foundational build gate (T008).
  - US1 (P1) is the MVP and should land first.
  - US2 and US3 are independent of each other and can proceed in parallel after Foundational; both edit `docs/astro.config.mjs`, so the *config edits* serialize even when the stories are worked in parallel.
  - US4 (P3) is best verified after US2 (so the local preview shows the theme), but its only hard dependency is the Foundational build.
- **Polish (Phase 7)**: After the user stories you intend to ship. No ADR-retirement step (N/A).

### Critical same-file constraint

`docs/astro.config.mjs` is edited by T005 (base), T009 (sitemap), T010 (links
validator), T016 (customCss), T019 (sidebar), and T020 (sitemap filter). These
edits MUST be sequential (same file) — they are NOT `[P]` with one another even
across stories.

### Within Each User Story

- Config edits (same file) before the story's verification task.
- US2: submodule (T014) → bridge file (T015) → config wiring (T016) → verify (T017).
- US3: content move (T018) → sidebar (T019) → sitemap filter (T020) → verify (T021).

---

## Parallel Opportunities

- **Setup**: T003 and T004 run in parallel (different files), independent of the T001→T002 chain.
- **Foundational**: T006 and T007 run in parallel (different files); T005 is independent too but is the config spine.
- **US1**: T011 (`.github/workflows/docs.yml`, different file) runs in parallel with the T009→T010 `astro.config.mjs` edits.
- **Across stories**: US2 and US3 can be staffed in parallel after Foundational, except their shared `docs/astro.config.mjs` edits serialize.
- **Polish**: T024 and T025 run in parallel (different files/scopes).

### Parallel Example: Setup phase

```bash
# After T001 (scaffold) + T002 (deps), run the gate-protection files together:
Task: "Create repo-root .markdownlintignore excluding docs/{node_modules,theme,dist,.astro}"   # T003
Task: "Add docs/.astro/ to repo-root .gitignore"                                                # T004
```

### Parallel Example: User Story 1

```bash
# The deploy workflow is a different file from astro.config.mjs:
Task: "Create .github/workflows/docs.yml (Pages-from-Actions, submodules: recursive)"           # T011  [P]
# …authored while the astro.config.mjs sitemap + links-validator edits proceed (T009 → T010)
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup (T001–T004).
2. Phase 2 Foundational (T005–T008) — **build gate**.
3. Phase 3 US1 (T009–T013) — wire sitemap + links validator, add the deploy
   workflow, enable Pages, verify live.
4. **STOP and VALIDATE**: the site is live and auto-deploys at
   `https://rshade.github.io/ax-go/`. Demoable MVP (default styling/content).

### Incremental Delivery

1. Setup + Foundational → buildable site.
2. Add US1 → **live, auto-deploying site** (MVP).
3. Add US2 → brand-consistent theming.
4. Add US3 → migrated content, ADRs excluded.
5. Add US4 → documented, reproducible local dev.
6. Polish → README link, Go-gate confirmation, full acceptance walkthrough.

Each increment is independently testable and adds value without regressing the
previous one.

---

## Notes

- `[P]` = different files, no dependencies; the shared `docs/astro.config.mjs`
  edits are never `[P]` with each other.
- `[MANUAL]` tasks are operator/verification actions (repo settings, live curl,
  visual comparison), not code edits — they still gate story completion.
- The deploy workflow ships with `submodules: recursive` in US1 even though the
  theme submodule lands in US2; this is intentional forward-compatibility (a
  no-op until the submodule exists).
- `rshade-theme` is **public** (research.md D8), so no PAT/secret is needed.
- Never hand-edit `CHANGELOG.md`; capture changes as `docs:`/`ci:` Conventional
  Commits (release-please owns the changelog).
- This feature introduces no Go public-API or machine-payload change → no
  library version bump (plan.md Constitution Check, Principle XI).
