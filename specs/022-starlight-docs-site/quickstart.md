# Quickstart: ax-go Docs Site (and the canonical adoption guide)

**Feature**: `009-starlight-docs-site` | **Date**: 2026-06-19

ax-go is the **first** site to wire this pattern, so this guide is the reference
later rshade Starlight sites copy. Two audiences:

- **Contributors** to ax-go docs → "Local development".
- **Adopters** standing up a new portfolio docs site → "Adopting the pattern".

---

## Local development (ax-go contributors)

```bash
# 1. Clone WITH submodules (the shared theme lives at docs/theme)
git clone --recurse-submodules https://github.com/rshade/ax-go
cd ax-go

# …or, if already cloned without submodules:
git submodule update --init --recursive

# 2. Install docs deps (scoped to docs/, never touches go.mod)
cd docs
npm ci

# 3. Run the local preview (shared theme applied)
npm run dev          # http://localhost:4321/ax-go/

# 4. Build exactly as CI does (fails on broken links or missing theme)
npm run build && npm run preview
```

Adding a page: drop a `.md`/`.mdx` file under `docs/src/content/docs/` with a
`title` in its frontmatter; add it to the `sidebar` in `docs/astro.config.mjs`
if it should appear in the nav.

> If the preview renders with default fonts/colors, your `docs/theme` submodule
> is not initialized — run `git submodule update --init --recursive` (FR-013).

---

## Adopting the pattern (new portfolio site)

The copyable surface is small and deliberately minimal (D7). Mirror it; do not
re-style.

1. **Scaffold Starlight** in the site's `docs/`:

   ```bash
   npm create astro@latest -- --template starlight
   ```

2. **Add the theme submodule + sitemap dep**:

   ```bash
   git submodule add https://github.com/rshade/rshade-theme docs/theme
   cd docs && npm install @astrojs/sitemap starlight-links-validator
   ```

3. **Copy the theme bridge** verbatim → `docs/src/styles/theme-bridge.css`
   (see `contracts/theme-bridge.md`). It contains ONLY token mappings.

4. **Set `astro.config.mjs`** — `site`, `base: '/<repo>'`, `customCss`, the
   `sitemap()` integration, and the links validator (see
   `contracts/site-and-deploy.md`).

5. **Copy the deploy workflow** → `.github/workflows/docs.yml`, changing only
   the path filter if needed. Set the repo's Pages **source** to *GitHub
   Actions*.

6. **Guard the existing lint gate** — add `.markdownlintignore` for
   `docs/theme/`, `docs/node_modules/`, `docs/dist/`, `docs/.astro/`.

Only `base` (the repo subpath) and `sidebar`/content differ per site. Fonts,
colors, sitemap behavior, and deploy mechanics are identical everywhere.

---

## Acceptance walkthrough (verify the contracts)

After the first deploy, confirm each user story's outcome:

| Check | Command / action | Expected | Trace |
|-------|------------------|----------|-------|
| Site is live | `curl -sI https://rshade.github.io/ax-go/` | `HTTP/2 200` | C-URL-1, SC-001 |
| Subpath links | click through nav; load assets | no 404s; all under `/ax-go/` | C-URL-2 |
| Theming | compare fonts/accent to finfocus | match (Inter / JetBrains Mono / accent) | C-TB / SC-002 |
| Sources reachable | open nav → Sources | renders | C-URL-3, SC-004 |
| ADRs excluded | search nav + fetch any old ADR slug | not present / 404 | C-URL-4, SC-004 |
| Sitemap valid | fetch `…/ax-go/sitemap-index.xml` | valid XML, `/ax-go/` URLs, no `/adr/` | C-SM-*, SC-008 |
| Auto-deploy | push a docs change to `main` | site updates, no manual step | C-DEP-5, SC-003 |
| Go gates intact | `go test -race ./... && golangci-lint run && make doc-coverage` | clean, unchanged | C-ISO-1, SC-005 |
| No silent unstyled deploy | build without `docs/theme` initialized | build fails | C-DEP-6, SC-007 |

---

## Guardrails

- **Never** put hex colors or font literals in `theme-bridge.css` — only
  `var(--token)` mappings (C-TB-2). Bespoke styling is the divergence this
  effort prevents.
- **Never** add the docs JS deps to the repo-root `package.json` or `go.mod` —
  the docs toolchain is scoped to `docs/` (C-ISO-3).
- **Never** publish `docs/adr/*` — they are a frozen log retired in place; the
  build excludes them by keeping them out of `src/`, and the sitemap `filter` is
  the backstop (C-URL-4, C-SM-4).
- If `rshade-theme` is private, the deploy checkout needs
  `token: ${{ secrets.THEME_REPO_TOKEN }}` (D8); a public theme repo needs no
  token.
