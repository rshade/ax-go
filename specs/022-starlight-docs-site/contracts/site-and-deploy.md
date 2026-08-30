# Contract: Site URLs, Sitemap & Deploy

**Feature**: `009-starlight-docs-site`

The observable, externally-checkable contract for the published site. These are
the assertions a reviewer (or an agent) can verify against the live deployment.

## Published-URL contract

| ID | Contract | Trace |
|----|----------|-------|
| C-URL-1 | `GET https://rshade.github.io/ax-go/` returns HTTP 200 with a rendered Starlight page | FR-002, SC-001 |
| C-URL-2 | Every internal link and asset resolves under `/ax-go/`; zero root-relative 404s | FR-003, SC-001 |
| C-URL-3 | `sources.md` content is reachable from the site navigation | FR-006, SC-004 |
| C-URL-4 | No ADR page is reachable via nav, sitemap, or direct URL | FR-007, SC-004 |

## Sitemap contract

| ID | Contract | Trace |
|----|----------|-------|
| C-SM-1 | A sitemap is served (e.g. `…/ax-go/sitemap-index.xml`) and is valid XML | FR-015, SC-008 |
| C-SM-2 | Every entry is an absolute URL under `https://rshade.github.io/ax-go/` | FR-015, SC-008 |
| C-SM-3 | The sitemap lists every published page | FR-015 |
| C-SM-4 | The sitemap contains zero `/adr/` URLs | FR-007, SC-008 |

`astro.config.mjs` integration shape (sitemap with defensive filter):

```js
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import sitemap from '@astrojs/sitemap';
import starlightLinksValidator from 'starlight-links-validator';

export default defineConfig({
  site: 'https://rshade.github.io',
  base: '/ax-go',
  integrations: [
    starlight({
      title: 'ax-go',
      customCss: ['./src/styles/theme-bridge.css'],
      plugins: [starlightLinksValidator()],
      sidebar: [{ label: 'Reference', items: [{ label: 'Sources', slug: 'sources' }] }],
    }),
    sitemap({ filter: (page) => !/\/adr\//.test(page) }),
  ],
});
```

> Starlight auto-adds `@astrojs/sitemap` when `site` is set, but detects a
> user-provided sitemap integration and does not double-register — so adding it
> explicitly (as above) is the supported way to attach `filter` (D5).

## Deploy contract — `.github/workflows/docs.yml`

| ID | Contract | Trace |
|----|----------|-------|
| C-DEP-1 | Triggers on `push` to `main` filtered to `docs/**` (+ the workflow file) and `workflow_dispatch` | FR-008 |
| C-DEP-2 | Checkout initializes submodules (`submodules: recursive`) so the theme is present | FR-009 |
| C-DEP-3 | Uses Pages-from-Actions (`upload-pages-artifact` → `deploy-pages`); repo Pages source = GitHub Actions | FR-010 |
| C-DEP-4 | Least-privilege permissions: `contents: read`, `pages: write`, `id-token: write` | Principle IX |
| C-DEP-5 | A docs change pushed to `main` results in the live site updating with no manual step | FR-008, SC-003 |
| C-DEP-6 | The build fails (does not deploy) if the theme submodule is absent/uninitialized — via the unresolvable `@import`, or an explicit token-presence check if `@import` only warns (see C-TB-5) | FR-011, SC-007 |
| C-DEP-7 | The workflow does not run, modify, or depend on the Go toolchain or `ci.yml` | FR-012, SC-005 |

Canonical workflow skeleton:

```yaml
name: Deploy Docs
on:
  push:
    branches: [main]
    paths: ['docs/**', '.github/workflows/docs.yml']
  workflow_dispatch:
permissions:
  contents: read
  pages: write
  id-token: write
concurrency:
  group: pages
  cancel-in-progress: false
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          submodules: recursive          # FR-009 — if rshade-theme is PRIVATE, add:
          # token: ${{ secrets.THEME_REPO_TOKEN }}   # (D8 fallback)
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
          cache-dependency-path: docs/package-lock.json
      - run: npm ci
        working-directory: docs
      - run: npm run build
        working-directory: docs
      - uses: actions/upload-pages-artifact@v3
        with:
          path: docs/dist
  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - id: deployment
        uses: actions/deploy-pages@v4
```

## Go-isolation contract

| ID | Contract | Trace |
|----|----------|-------|
| C-ISO-1 | `go build ./...`, `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `make doc-coverage` produce identical results before and after this feature | FR-012, SC-005 |
| C-ISO-2 | The existing markdownlint gate stays green: `.markdownlintignore` excludes `docs/theme/`, `docs/node_modules/`, `docs/dist/`, `docs/.astro/` | FR-012, D9 |
| C-ISO-3 | `go.mod` gains no entries; the docs JS toolchain lives only under `docs/` | FR-001, FR-012, Principle X |
| C-ISO-4 | This change maps to a `docs:`/`ci:` Conventional Commit and triggers **no** library version bump (no public Go API or machine-payload change) | Principle XI |
