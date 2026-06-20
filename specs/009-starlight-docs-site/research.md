# Phase 0 Research: Astro Starlight Docs Site

**Feature**: `009-starlight-docs-site` | **Date**: 2026-06-19

This feature is documentation infrastructure, not Go library behavior. **No
governing ADR applies** — `docs/adr/*` records concern the Go runtime contract,
not the docs toolchain — so there is no "Decision Records Absorbed" section and
no ADR-retirement task.

Each decision below uses: **Decision / Rationale / Alternatives considered**.

---

## D1: Generator and framework versions

**Decision**: Astro 5.x + `@astrojs/starlight` (current stable for Astro 5) +
`@astrojs/sitemap` 3.x. Exact versions are pinned in `docs/package-lock.json`
at scaffold time via `npm create astro@latest -- --template starlight`. Package
manager is **npm** (matches the repo's existing `npx`/npm tooling); a committed
`package-lock.json` is the version source of truth.

**Rationale**: Starlight is the portfolio standard (matches finfocus). Astro 5 +
Starlight is the current line. Because ax-go is the **first adopter** (see D7),
the pinned lockfile is what later sites reproduce — pinning is the mechanism
that makes "consistent by construction" real rather than aspirational.

**Alternatives considered**: VitePress / Docusaurus / mdBook — rejected: not the
portfolio standard, would reintroduce the divergence this effort exists to
prevent. Leaving versions unpinned (`latest`) — rejected: non-reproducible
builds undermine ax-go's role as the canonical reference.

**Resolved at implementation (2026-06-19)**: `npm create astro@latest` (the
scaffolder T001 prescribes) now emits the **Astro 6** line, not Astro 5. The
maintainer chose to adopt the current stable line, so the committed lockfile
pins **`astro` ^6.4.x + `@astrojs/starlight` ^0.40 + `@astrojs/sitemap` ^3.7 +
`starlight-links-validator` ^0.24** (resolved to astro 6.4.8 / starlight 0.40.0
at install). The contracts are version-agnostic across these lines (the `--sl-*`
bridge variables, `customCss`, `site`/`base`, the sitemap `filter`, and the
links-validator plugin all behave identically), so adopting Astro 6 changes only
the pinned numbers, not the wiring later sites copy.

---

## D2: Node runtime and CI toolchain

**Decision**: Node 20 LTS, installed in CI via `actions/setup-node@v4`
(`node-version: 20`, `cache: npm`, `cache-dependency-path: docs/package-lock.json`).
Build with `npm ci` + `npm run build` run with `working-directory: docs`.

**Rationale**: Astro 5 requires Node ≥18.20.8 / ≥20.3 / ≥22. Node 20 is the
safe LTS. The repo's root `package.json` declares `engines.node >=18`; Node 20
satisfies it. `cache-dependency-path` scoped to `docs/package-lock.json` keeps
the docs cache independent of any root Node tooling.

**Alternatives considered**: Node 18 — works but closer to Astro's floor;
Node 22 — fine but 20 is the conservative LTS. `withastro/action` composite —
viable, but the explicit `setup-node` + `npm ci` + `upload-pages-artifact` path
is more transparent and easier for later sites to copy verbatim (D7).

---

## D3: Project layout — Astro project co-located in `docs/`

**Decision**: The Astro project root is `docs/` itself. Authored content lives
under `docs/src/content/docs/`. The existing `docs/sources.md` is **moved**
(`git mv`) into `docs/src/content/docs/sources.md` with Starlight frontmatter
prepended. The existing `docs/adr/` directory is **left in place, untouched, and
outside `src/`**, so the Astro build never sees it.

**Rationale**: Issue #68 prescribes `docs/`. Astro only processes `src/`,
`public/`, and config — anything else under the project root (`docs/adr/`,
`docs/theme/`'s non-asset files) is ignored by the build, which is exactly how
ADR exclusion (FR-007) is achieved "for free." `docs/src/` is **not** a Go
`src/` violation: the AGENTS.md "no `pkg/`/`src/`" rule guards the *module root*
import path; `docs/` contains no `.go` files and is not part of the Go package
tree, so `docs/src/` is invisible to the Go toolchain.

**Alternatives considered**: A separate top-level `website/` or `site/` dir —
rejected: contradicts the issue and the portfolio convention. Migrating ADRs
into the content collection — rejected in the spec (maintainer decision: ADRs
excluded; frozen log retired in place).

---

## D4: Subpath / base configuration

**Decision**: `astro.config.mjs` sets `site: 'https://rshade.github.io'` and
`base: '/ax-go'`. Internal links in authored content use Starlight-resolved
links (relative slugs / Starlight nav), which honor `base`. The `site` value is
also what makes the sitemap emit correct absolute URLs (D5).

**Rationale**: ax-go publishes to a **project** Pages site served from the
`/ax-go/` subpath, not a root domain. `base` makes Starlight prefix nav, asset,
and internal links; `site` + `base` together yield correct absolute URLs in the
sitemap and canonical tags. This directly satisfies FR-002, FR-003, SC-001.

**Alternatives considered**: Root domain / CNAME — out of scope; ax-go has no
custom domain. Omitting `base` — rejected: every internal link and asset would
become root-relative and 404 under `/ax-go/` (the classic project-Pages bug).

---

## D5: Sitemap wiring (`@astrojs/sitemap`)

**Decision**: Add `@astrojs/sitemap` **explicitly** to the `integrations` array
in `astro.config.mjs`, alongside `starlight()`. Configure a defensive `filter`
that drops any URL whose path contains `/adr/` (belt-and-suspenders for FR-007),
even though ADRs are already excluded by not being in the content collection.

**Rationale**: The user explicitly requested wiring `@astrojs/sitemap`. Starlight
auto-adds the sitemap integration when `site` is set, **but** if a sitemap
integration is already present in the user config, Starlight detects it and does
**not** double-register — so adding it manually is the supported way to take
explicit control (and to attach a `filter`). The sitemap is generated from the
**built page set**, so excluding ADRs from the build excludes them from the
sitemap automatically; the `filter` is a cheap regression guard, not the primary
mechanism. Satisfies FR-015, SC-008.

**Alternatives considered**: Rely solely on Starlight's implicit sitemap —
works, but does not satisfy the explicit "wire `@astrojs/sitemap`" instruction
and offers no `filter` hook. Adding `sitemap()` without checking Starlight's
auto-add — risk of a duplicate-integration warning; mitigated because Starlight
skips auto-add when it is already present.

---

## D6: Deployment — Pages-from-Actions

**Decision**: A new workflow `.github/workflows/docs.yml` builds and deploys via
the official Pages-from-Actions pattern: `actions/checkout@v7` with
`submodules: recursive`, `actions/setup-node@v4`, `npm ci && npm run build` in
`docs/`, then `actions/upload-pages-artifact@v3` (`path: docs/dist`) →
`actions/deploy-pages@v4`. Triggers: `push` to `main` filtered to
`paths: ['docs/**', '.github/workflows/docs.yml']`, plus `workflow_dispatch`.
Permissions are least-privilege: `contents: read`, `pages: write`,
`id-token: write`. `concurrency: { group: pages, cancel-in-progress: false }`.
The repository's Pages **source** must be set to **GitHub Actions**.

**Rationale**: Pages-from-Actions is the modern, branchless deploy model (no
`gh-pages` branch). Path-filtering to `docs/**` keeps Go-only changes from
rebuilding the site (FR-008 auto-deploy without waste). `submodules: recursive`
satisfies FR-009 (theme present in the build). `checkout@v7` matches the repo's
existing CI (the issue said `@v5`; the repo standard is `@v7`, which wins —
documented divergence). The Go `ci.yml` is untouched, so the Go quality gates
are unaffected (FR-012).

**Alternatives considered**: `actions/checkout@v5` (issue text) — superseded by
the repo's `@v7` standard. Deploy-from-branch (`gh-pages`) — rejected: legacy,
requires a build-commit dance and a publishing branch. `withastro/action` —
viable but less transparent for the reference (D7).

---

## D7: First-adopter posture — ax-go IS the reference

**Decision**: Treat the theme-bridge wiring, `astro.config.mjs` core, and the
deploy workflow as the **canonical reference** that later sites (gh-aw-fleet
issue #138 and others) copy. Keep the must-copy surface minimal and free of
ax-go-specific logic. `quickstart.md` doubles as the copyable adoption guide.

**Rationale**: gh-aw-fleet #138 is **not confirmed working**; ax-go is the first
site to wire this pattern. The spec inverted the original "copy the reference
verbatim" framing accordingly (FR-005). The single hard external dependency is
that `rshade-theme` exposes the four referenced tokens; ax-go validates the
wiring end-to-end, de-risking the portfolio rollout.

**Alternatives considered**: Wait for gh-aw-fleet to land first — rejected by the
maintainer (ax-go is intentionally the first test site). Invent richer
ax-go-specific styling — rejected: defeats the shared-token single-source-of-
truth goal and makes the reference non-copyable.

---

## D8: `rshade-theme` token contract and repository visibility

**Decision**: Consume `rshade-theme` as a git submodule at `docs/theme`,
importing `docs/theme/tokens.css` from `docs/src/styles/theme-bridge.css` via
`@import "../../theme/tokens.css"`. The bridge maps exactly four tokens:
`--font-sans`, `--font-mono`, `--color-accent`, `--color-accent-hover` →
`--sl-font`, `--sl-font-mono`, `--sl-color-accent`, `--sl-color-accent-high`.
**`rshade-theme` is PUBLIC** (maintainer-confirmed 2026-06-19; design tokens are
non-secret), so the deploy workflow's `submodules: recursive` checkout works with
the default `GITHUB_TOKEN` — no cross-repo credential needed. (Adopter note for
later sites: a *private* theme repo would require
`token: ${{ secrets.THEME_REPO_TOKEN }}` on the checkout, since `GITHUB_TOKEN`
cannot read other private repos.)

**Rationale**: `actions/checkout@vN` with `submodules: recursive` initializes
submodules using the job token; that token only has access to the current repo.
A public theme repo "just works"; a private one needs an explicit cross-repo
credential. Design tokens (fonts, colors) carry no secrets, so public is the
expected and recommended posture. This is the one decision that should be
**confirmed with the maintainer** — but it does not block planning, because the
only delta is one `token:` line in the workflow.

**Alternatives considered**: Vendoring a copy of `tokens.css` into the repo —
rejected: breaks the single-source-of-truth goal (the whole point). npm-package
distribution of the theme — out of scope; the issue prescribes a submodule.

---

## D9: Markdownlint interaction with the existing lint gate

**Decision**: Add a repo-root `.markdownlintignore` excluding the Astro
subtree's non-authored markdown:

```text
docs/node_modules/
docs/theme/
docs/dist/
docs/.astro/
```

Keep authored content (`docs/src/content/docs/**/*.md`, `docs/adr/**/*.md`)
lintable. Ensure the migrated `sources.md` passes markdownlint after frontmatter
is prepended.

**Rationale**: The root `package.json` `lint:md` script and `ci.yml` lint job
run `markdownlint ... 'docs/**/*.md'`. After scaffolding, that glob would scan
the theme submodule's own markdown, installed dependencies' markdown, and build
output — none of which ax-go controls — and likely fail, breaking the existing
gate (violating FR-012). `markdownlint`/`markdownlint-cli` honor
`.markdownlintignore`. This keeps the Go/lint gate exactly as green as before.

**Alternatives considered**: Narrowing the glob in `package.json` to
`docs/src/content/**/*.md` — also works but changes shared tooling more
invasively and would silently stop linting `docs/adr/*`; the ignore-file is the
least-surprise, most-targeted fix. Disabling markdownlint for docs — rejected:
authored content should stay linted.

---

## D10: Version-control hygiene

**Decision**: Astro build/cache artifacts are git-ignored. The root `.gitignore`
already ignores `node_modules/`, `dist/`, and `build/` (unanchored → they match
`docs/node_modules/` and `docs/dist/` too). Add `docs/.astro/` (Astro's content
cache) — or the unanchored `.astro/` — which is not yet covered. `git submodule
add` creates `.gitmodules` and the gitlink at `docs/theme`; both are committed.

**Rationale**: Keeps generated JS artifacts out of the Go repository (FR-014)
while committing the config, content, lockfile, and submodule pointer that make
the build reproducible.

**Alternatives considered**: Committing `dist/` — rejected: build output does not
belong in source control under Pages-from-Actions.

---

## D11: Link verification as the docs "test gate"

**Decision**: Add `starlight-links-validator` as a Starlight plugin so the
`npm run build` fails on broken internal links and missing anchors. This is the
docs analog of the Go test gate and operationalizes SC-001 / FR-003 (no
root-relative 404s under `/ax-go/`).

**Rationale**: Constitution Principle VII's spirit is "verify, don't assert."
Astro does not fail on broken internal links by default; the validator turns
SC-001 into a build-time check rather than a manual one. It runs only at build
time and adds no runtime surface. It is kept logically separate from the
must-copy theme wiring so the canonical reference (D7) stays minimal.

**Alternatives considered**: Manual link checking — rejected: not verifiable,
drifts. An external link crawler in CI post-deploy — heavier and slower than a
build-time plugin; can be added later if external-link coverage is wanted.

---

## Resolved unknowns summary

| Topic | Resolution |
|-------|-----------|
| Framework / versions | Astro 5 + Starlight (Astro-5 line) + sitemap 3, npm, pinned lockfile (D1) |
| Node / CI | Node 20 LTS, `setup-node@v4`, build in `docs/` (D2) |
| Layout | Astro project in `docs/`; `sources.md` → `src/content/docs/`; ADRs left out of `src/` (D3) |
| Subpath | `site` + `base: '/ax-go'` (D4) |
| Sitemap | explicit `@astrojs/sitemap` integration + `/adr/` filter (D5) |
| Deploy | Pages-from-Actions, `submodules: recursive`, path-filtered, `checkout@v7` (D6) |
| Reference posture | ax-go is canonical; minimal copyable surface (D7) |
| Theme dependency | submodule `docs/theme`, 4-token map; public repo assumed, PAT fallback (D8) |
| Markdownlint | `.markdownlintignore` for theme/node_modules/dist/.astro (D9) |
| VCS hygiene | ignore `.astro/`; commit config/content/lockfile/submodule (D10) |
| Verification | `starlight-links-validator` build-time gate (D11) |

**No NEEDS CLARIFICATION markers remain.** One item (D8 theme-repo visibility)
is flagged for maintainer confirmation but has a documented default and a
one-line fallback, so it does not block.
