# Phase 1 Data Model: Astro Starlight Docs Site

**Feature**: `009-starlight-docs-site` | **Date**: 2026-06-19

This feature has no runtime data structures (it is a static site). The "model"
below is the **configuration and content schema** — the structured artifacts the
build consumes and produces. Each entity lists its fields, validation rules
(traced to FRs), and relationships.

---

## Entity: Site Configuration (`docs/astro.config.mjs`)

The single source of build-time truth for URL shape, theming, and integrations.

| Field | Value / Type | Validation (FR) | Notes |
|-------|--------------|-----------------|-------|
| `site` | `'https://rshade.github.io'` | MUST be set (FR-002, FR-015) | Required for absolute sitemap/canonical URLs |
| `base` | `'/ax-go'` | MUST be set (FR-003) | Project-Pages subpath; prefixes nav/assets/links |
| `integrations[]` | `[starlight(...), sitemap(...)]` | starlight + sitemap present (FR-004, FR-015) | Order: starlight first |
| `starlight.title` | `'ax-go'` | non-empty | Site/nav title |
| `starlight.customCss[]` | `['./src/styles/theme-bridge.css']` | MUST include the bridge (FR-004, FR-005) | The only styling entry point |
| `starlight.plugins[]` | `[starlightLinksValidator()]` | recommended (D11) | Build-time link gate (SC-001) |
| `starlight.sidebar[]` | nav groups | sources reachable (FR-006) | No ADR entries (FR-007) |
| `sitemap.filter` | drop `*/adr/*` | no ADR URLs (FR-007, SC-008) | Defensive regression guard |

**Relationships**: references the Theme Bridge (via `customCss`), the Content
Collection (Starlight sources `src/content/docs/`), and is consumed by the
Deploy Workflow's build step.

**State**: static config; no transitions.

---

## Entity: Theme Submodule (`docs/theme` → `rshade/rshade-theme`)

The shared design-token source, included as a git submodule.

| Field | Value | Validation | Notes |
|-------|-------|-----------|-------|
| submodule path | `docs/theme` | recorded in `.gitmodules` (FR-004) | Gitlink committed |
| submodule URL | `https://github.com/rshade/rshade-theme` | reachable in CI (FR-009) | Public assumed (D8); PAT if private |
| consumed file | `docs/theme/tokens.css` | MUST exist & export the 4 tokens (D8) | Imported by the bridge |
| provided tokens | `--font-sans`, `--font-mono`, `--color-accent`, `--color-accent-hover` | all 4 present (FR-005) | Upstream contract |

**Relationships**: imported by the Theme Bridge. Initialized by the Deploy
Workflow checkout (`submodules: recursive`, FR-009) and by local setup (FR-013).

**State**: pinned at a specific commit (the gitlink); updated only by an
explicit `git submodule update --remote` + commit.

---

## Entity: Theme Bridge (`docs/src/styles/theme-bridge.css`)

The canonical, minimal mapping from shared tokens onto Starlight CSS variables.
This is the **must-copy reference surface** (D7).

| Source token (rshade-theme) | → Starlight variable | Purpose |
|-----------------------------|----------------------|---------|
| `--font-sans` | `--sl-font` | Body typeface |
| `--font-mono` | `--sl-font-mono` | Code typeface |
| `--color-accent` | `--sl-color-accent` | Accent color |
| `--color-accent-hover` | `--sl-color-accent-high` | Accent (high/hover) |

**Validation**: MUST contain no bespoke literals — only `var(--token)` mappings
(FR-005). MUST `@import "../../theme/tokens.css"` first.

**Relationships**: imports the Theme Submodule's `tokens.css`; referenced by Site
Configuration `customCss`.

---

## Entity: Content Collection (`docs/src/content/docs/`)

The published page set. Each entry is a Starlight doc with frontmatter.

| Page | File | Frontmatter (required) | Source |
|------|------|------------------------|--------|
| Landing | `index.mdx` | `title`, `template: splash` (optional) | New (minimal) |
| Sources | `sources.md` | `title`, `description` | **Moved** from `docs/sources.md` (FR-006) |

**Content config** (`docs/src/content.config.ts`): declares the `docs`
collection using Starlight's `docsLoader()` + `docsSchema()`.

**Validation rules**:

- Every entry MUST carry a `title` (Starlight schema requirement).
- `sources.md` MUST pass markdownlint after frontmatter is prepended (D9, FR-012).
- The collection MUST NOT contain any ADR content (FR-007); `docs/adr/*` stays
  outside `src/`.

**Relationships**: rendered into the published Site and enumerated into the
Sitemap. Sidebar nav (in Site Configuration) references these entries.

**State**: authored markdown → built HTML pages (build-time transform).

---

## Entity: Sitemap (`/ax-go/sitemap-index.xml` + `sitemap-*.xml`)

Generated artifact; machine-discoverable index of published URLs.

| Field | Rule | Validation (FR) |
|-------|------|-----------------|
| URL entries | absolute, under `https://rshade.github.io/ax-go/` | FR-015, SC-008 |
| Coverage | every published page | FR-015 |
| Exclusions | zero ADR URLs | FR-007, SC-004, SC-008 |
| Derivation | from the built page set + `filter` | D5 |

**Relationships**: produced by the `@astrojs/sitemap` integration from the
Content Collection + Site Configuration (`site`/`base`).

---

## Entity: Deploy Workflow (`.github/workflows/docs.yml`)

CI pipeline that builds and publishes the site.

| Field | Value | Validation (FR) |
|-------|-------|-----------------|
| triggers | `push` to `main` filtered to `docs/**` + workflow file; `workflow_dispatch` | FR-008 |
| checkout | `actions/checkout@v7`, `submodules: recursive` | FR-009 |
| node | `actions/setup-node@v4`, Node 20, npm cache on `docs/package-lock.json` | D2 |
| build | `npm ci && npm run build` in `docs/` | FR-001, FR-011 (fails if theme/links missing) |
| publish | `upload-pages-artifact@v3` (`docs/dist`) → `deploy-pages@v4` | FR-010 |
| permissions | `contents: read`, `pages: write`, `id-token: write` | least-privilege (IX) |
| concurrency | `group: pages, cancel-in-progress: false` | safe serialized deploys |

**Relationships**: consumes Site Configuration + Theme Submodule; produces the
published Site + Sitemap. Independent of the Go `ci.yml` (FR-012).

**State**: per-push run → `build` job → `deploy` job → live site.

---

## Cross-cutting validation matrix

| Requirement | Enforced by entity/field |
|-------------|--------------------------|
| FR-002 canonical URL | Site Config `site` + Deploy Workflow publish |
| FR-003 subpath links | Site Config `base` + Link Validator (D11) |
| FR-004/005 theming | Theme Submodule + Theme Bridge + `customCss` |
| FR-006 sources reachable | Content Collection `sources.md` + sidebar |
| FR-007 ADRs excluded | ADRs outside `src/` + Sitemap `filter` |
| FR-009 submodule in CI | Deploy Workflow `submodules: recursive` |
| FR-011 no silent unstyled deploy | Build fails if `theme/tokens.css` import is missing (fallback: explicit token-presence check — C-TB-5) |
| FR-012 Go gates unaffected | `.markdownlintignore` + docs-scoped toolchain |
| FR-014 VCS hygiene | `.gitignore` (`.astro/`, node_modules, dist) |
| FR-015 sitemap | Sitemap entity (`@astrojs/sitemap`) |
