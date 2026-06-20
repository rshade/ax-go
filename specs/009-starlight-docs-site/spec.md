# Feature Specification: Astro Starlight Docs Site Consuming rshade-theme

**Feature Branch**: `009-starlight-docs-site`

**Created**: 2026-06-19

**Status**: Draft

**Input**: User description: "Scaffold an Astro Starlight docs site under `docs/` that consumes the shared `rshade-theme` tokens (via git submodule) and publishes to GitHub Pages at `rshade.github.io/ax-go/`. ax-go is the **first** adopter of this pattern — the gh-aw-fleet #138 reference is not confirmed working — so ax-go establishes the canonical wiring (using the prescribed theme-bridge token mapping) instead of copying a landed reference, with finfocus as the visual-consistency target. Wire `@astrojs/sitemap` so published pages are machine-discoverable. Existing `docs/sources.md` is reachable in the nav; `docs/adr/*` is deliberately excluded from the published site (and from the sitemap)." (GitHub issue #68)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Visitor reaches a live, auto-deploying docs site (Priority: P1)

A human engineer or an LLM agent evaluating ax-go navigates to
`https://rshade.github.io/ax-go/`. They get a rendered documentation site
with working navigation, not a 404 or a raw markdown file listing. When a
maintainer later pushes a documentation change to the default branch, the
published site updates on its own with no manual deploy step.

**Why this priority**: This is the entire point of the feature — ax-go has no
published docs site today, only raw markdown under `docs/`. A live,
self-updating site is the minimum deliverable that turns "files in a repo"
into "documentation people and agents can read." Without it nothing else
matters.

**Independent Test**: Can be fully tested by requesting
`https://rshade.github.io/ax-go/` and confirming an HTTP 200 with a rendered
Starlight page, then pushing a trivial docs change to the default branch and
confirming the live site reflects it without manual intervention. Delivers
value on its own: ax-go gains a published, auto-deploying docs home.

**Acceptance Scenarios**:

1. **Given** the site is deployed, **When** a visitor requests `https://rshade.github.io/ax-go/`, **Then** the server returns HTTP 200 and a rendered Starlight site with a working navigation sidebar.
2. **Given** the site is deployed under the project-pages subpath, **When** a visitor follows any internal navigation link or loads any page asset, **Then** the link/asset resolves correctly under `/ax-go/` with no root-relative 404.
3. **Given** a maintainer pushes a change to documentation content on the default branch, **When** the deploy pipeline runs, **Then** the published site reflects the change with no manual deploy step and without breaking unrelated pages.
4. **Given** the site is deployed under the project-pages subpath, **When** an agent or crawler requests the site's sitemap, **Then** the sitemap lists the published pages as correct absolute URLs under `https://rshade.github.io/ax-go/`.

---

### User Story 2 - Site is visually consistent with the rshade portfolio (Priority: P2)

A reader who knows finfocus or the rshade blog visits the ax-go docs and
immediately recognizes the same brand: the same typefaces (Inter for body,
JetBrains Mono for code) and the same accent color. Nothing about the ax-go
docs looks bespoke or hand-styled — it shares the single source of truth for
design tokens with every other property in the portfolio.

**Why this priority**: Brand divergence across the portfolio is the specific
problem this whole effort exists to prevent. Sharing tokens (rather than
re-inventing styling) is what makes "consistent by construction" true instead
of "consistent until someone forgets." A deployed site that looks wrong still
delivers content (hence P2, not P1), but a deployed site that diverges defeats
the strategic reason for adopting the standard.

**Independent Test**: Can be tested by loading the deployed site and comparing
its computed body font, monospace font, and accent color against finfocus and
the blog, confirming they match; and by confirming the styling is wired from
the shared theme tokens (not duplicated literals), following the prescribed
canonical mapping.

**Acceptance Scenarios**:

1. **Given** the shared theme is wired in, **When** the site renders, **Then** body text uses the shared sans typeface and code uses the shared monospace typeface, both sourced from the shared tokens rather than locally redeclared values.
2. **Given** the shared theme defines an accent color, **When** the site renders interactive/accent elements, **Then** those elements use the shared accent color, matching finfocus and the blog.
3. **Given** no landed reference adoption exists yet (ax-go is the first site to wire this pattern), **When** the theme wiring is implemented, **Then** it uses the prescribed token-to-style mapping with no bespoke styling divergence, establishing the canonical pattern later sites copy.

---

### User Story 3 - Existing reference content is reachable; ADRs are deliberately excluded (Priority: P2)

A reader looking for ax-go's curated source/reference material finds
`sources.md` reachable from the site navigation. The architecture decision
records under `docs/adr/` are intentionally **not** published — they remain an
internal, frozen legacy log that is being retired in place, so surfacing them
on a public site (where they would appear and then vanish as features delete
them) is avoided.

**Why this priority**: Publishing the existing curated content is what makes
this an ax-go docs site rather than an empty Starlight demo, so it ranks just
below "the site exists." Excluding the ADRs keeps the published site aligned
with the project's governance: `docs/adr/` is a frozen log that shrinks over
time as Spec Kit features absorb each ADR, so it is not stable, public-facing
documentation.

**Independent Test**: Can be tested by navigating the deployed site and
confirming `sources.md` content is reachable from the nav, and by confirming
no ADR page is reachable anywhere in the published site (no `docs/adr/*`
content surfaces in the navigation, sitemap, or by direct URL).

**Acceptance Scenarios**:

1. **Given** the site is deployed, **When** a visitor browses the navigation, **Then** the `sources.md` content is reachable and renders correctly.
2. **Given** the ADRs are excluded by design, **When** a visitor browses the navigation or sitemap, **Then** no `docs/adr/*` page appears and no ADR page is reachable by direct URL within the published site.
3. **Given** a future Spec Kit feature deletes an ADR file from `docs/adr/` (the retirement-in-place mechanic), **When** the docs site rebuilds, **Then** nothing in the published site is affected (because ADRs were never published).

---

### User Story 4 - Contributor previews and extends the docs locally (Priority: P3)

A contributor clones the repository, runs the documented setup, and gets a
local preview of the docs site that looks identical to production (theme
included). They can add a new page or edit existing content and see it
reflected locally before pushing.

**Why this priority**: Local reproducibility lowers the cost of every future
docs contribution and prevents "works in CI, broken locally" drift, but the
site can ship and auto-deploy without it, so it is the lowest-priority slice
here.

**Independent Test**: Can be tested by following the documented local steps on
a fresh clone (including theme submodule initialization) and confirming the
local preview renders with the shared theme applied, matching the deployed
look.

**Acceptance Scenarios**:

1. **Given** a fresh clone, **When** the contributor follows the documented setup (including initializing the theme submodule) and starts the local preview, **Then** the site renders locally with the shared theme applied.
2. **Given** a running local preview, **When** the contributor adds or edits a docs page, **Then** the change is reflected in the local preview.

---

### Edge Cases

- **Theme submodule not checked out in CI**: If the deploy pipeline checks out the repository without initializing submodules, the shared theme tokens are absent. The build MUST NOT silently deploy an unstyled site; this condition MUST surface as a build failure or an explicit error rather than a successful-looking deploy with fallback styling.
- **Subpath misconfiguration**: If the project-pages subpath (`/ax-go/`) is not configured, internal links and assets become root-relative and 404 on the deployed site. This MUST be prevented by configuration verified before deploy.
- **Shared token renamed upstream**: If `rshade-theme` renames or removes a token the bridge references, the affected typography/color silently falls back to defaults. The contract requires the wiring to follow the prescribed token mapping against `rshade-theme`'s published tokens; an upstream token rename is an upstream-coordination concern surfaced by visual regression, not absorbed by inventing local fallbacks.
- **Pages not enabled / wrong Pages source**: If the repository does not have Pages enabled with the Actions-based source, the deploy pipeline runs but nothing serves at the URL. Enabling Pages (Actions source) is part of delivering the feature.
- **First adopter, no landed reference**: ax-go is the first site to wire this pattern — the gh-aw-fleet #138 reference is not confirmed working — so the feature is NOT blocked on an upstream reference. ax-go validates the prescribed wiring end-to-end against `rshade-theme`'s tokens and becomes the canonical reference later adopters copy. The only hard external dependency is `rshade-theme` exposing the referenced tokens; if those tokens are absent or named differently, that surfaces as a build/visual failure, not something worked around with invented styling.
- **Sitemap drift from published pages**: If the sitemap were generated independently of the built pages, it could list ADR URLs or stale paths. The sitemap MUST be derived from the actual published page set so that excluding ADRs from the build excludes them from the sitemap automatically, and the subpath (`/ax-go/`) is reflected in every sitemap URL.
- **Docs project leaking into the Go build**: If the docs project's tooling or files are picked up by the Go build or quality gates, they could break unrelated CI. The docs project MUST stay self-contained under `docs/` and have no effect on `go build`, `go test`, `go vet`, `golangci-lint`, or `make doc-coverage`.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The repository MUST contain a documentation site project under `docs/` that builds into a static site, self-contained such that its package manifest and toolchain are scoped to `docs/`.
- **FR-002**: The deployed site MUST be served at `https://rshade.github.io/ax-go/` and return HTTP 200 with a rendered site at that URL.
- **FR-003**: The site MUST be configured for the project-pages subpath so every internal link and asset resolves under `/ax-go/`, with no root-relative 404s on the deployed site.
- **FR-004**: The site MUST consume the shared `rshade-theme` design tokens via a git submodule, so the site's typography (shared sans + shared monospace) and accent color are driven by the shared tokens rather than locally duplicated values.
- **FR-005**: The shared-theme wiring MUST use the prescribed token-to-style mapping (shared sans/mono fonts and accent/accent-hover colors mapped onto the docs framework's styling variables) with no bespoke styling divergence introduced for ax-go. Because ax-go is the first adopter, this wiring establishes the canonical pattern later sites copy; it MUST stay minimal and copyable rather than ax-go-specific.
- **FR-006**: The existing `docs/sources.md` content MUST be reachable and navigable from the published site.
- **FR-007**: The architecture decision records under `docs/adr/` MUST NOT be published to the site; no ADR page may be reachable via the navigation, sitemap, or direct URL. The ADRs remain an internal, frozen legacy log retired in place.
- **FR-008**: A continuous-deployment pipeline MUST build and publish the site automatically on changes to the default branch, with no manual deploy step.
- **FR-009**: The deploy pipeline's checkout MUST initialize the theme submodule so the shared styling is present in the deployed output.
- **FR-010**: The deploy pipeline MUST use the Pages-from-Actions delivery model, and the repository MUST have Pages enabled with the Actions-based source.
- **FR-011**: A missing or uninitialized theme submodule during the build MUST NOT result in a silently unstyled successful deploy; the condition MUST fail the build or surface an explicit error (fail-closed). The primary enforcement mechanism is the unresolvable `@import` of the theme's `tokens.css`; if the build toolchain treats a missing `@import` as a non-fatal warning rather than an error, an explicit build-time presence check on the theme tokens MUST be added so the fail-closed guarantee still holds. This guarantee covers a **missing or uninitialized submodule** only; an upstream token **rename or removal** still resolves the import and instead surfaces as a visual regression (see the "Shared token renamed upstream" edge case), not a build failure.
- **FR-012**: The docs project MUST NOT affect the Go module's build or quality gates: `go build`, `go test -race ./...`, `go vet ./...`, `golangci-lint run`, and `make doc-coverage` MUST behave exactly as they did before the docs project existed, and the docs build MUST NOT require the Go toolchain.
- **FR-013**: A contributor MUST be able to build and preview the site locally using documented commands, including initializing the theme submodule, and the local preview MUST render with the shared theme applied.
- **FR-014**: Files generated by the docs toolchain (dependencies, build output) MUST be excluded from version control so they do not pollute the Go repository.
- **FR-015**: The site MUST generate a sitemap (via the Astro sitemap integration) covering all published pages, with entries as correct absolute URLs under `https://rshade.github.io/ax-go/`. The sitemap MUST be derived from the built page set so it lists no unpublished content (no `docs/adr/*` URLs).
- **FR-016**: The repository's `README.md` MUST link to the published docs site (`https://rshade.github.io/ax-go/`) so the site is discoverable from the repository root.

### Key Entities

- **Docs site project**: The self-contained documentation site living under `docs/`, responsible for building the static site and declaring its own toolchain dependencies, independent of the Go module.
- **Shared theme (submodule)**: The `rshade-theme` repository included as a git submodule under `docs/`, providing the single source of truth for design tokens (fonts, accent color) shared across the portfolio.
- **Theme bridge**: The thin styling layer that maps the shared theme's tokens onto the docs framework's styling variables, mirroring the reference implementation; the only intended customization point and deliberately minimal.
- **Published content set**: The set of pages exposed in the site (the curated `sources.md` plus a minimal landing page); explicitly excludes `docs/adr/*`.
- **Sitemap**: The machine-discoverable index of published page URLs (absolute, under the `/ax-go/` subpath), generated by the sitemap integration from the built page set; lists only published content and never ADR URLs.
- **Deploy pipeline**: The CI workflow that checks out the repository with submodules, builds the site, and publishes it to Pages on default-branch changes.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A visitor loading `https://rshade.github.io/ax-go/` receives a rendered site (HTTP 200) with a working navigation sidebar, with zero broken internal links or assets attributable to subpath misconfiguration.
- **SC-002**: The deployed site's body typeface, monospace typeface, and accent color match those of finfocus and the rshade blog, confirmed by direct visual/computed-style comparison, because all three are driven by the same shared tokens.
- **SC-003**: A documentation change pushed to the default branch is reflected on the live site automatically (no manual deploy action), completed within a single deploy-workflow run (typically a few minutes; no fixed numeric SLA is asserted for v1).
- **SC-004**: The published site contains zero ADR pages (none reachable by nav, sitemap, or direct URL), while `sources.md` content is reachable from the navigation.
- **SC-005**: Before and after this feature lands, the full Go quality gate (`go build`, `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `make doc-coverage`) produces identical clean results, and the docs site builds without the Go toolchain present.
- **SC-006**: On a fresh clone, a contributor following only the documented local-setup steps (including theme submodule initialization) obtains a local preview whose typography and accent color match the deployed site.
- **SC-007**: A deploy run in which the theme submodule is not initialized does not produce a live, unstyled site — it fails the build or surfaces an explicit error instead (fail-closed). This covers a missing/uninitialized submodule; an upstream token rename, which still resolves the import, is detected as a visual regression rather than a build failure.
- **SC-008**: The deployed site exposes a valid sitemap that lists every published page as an absolute URL under `https://rshade.github.io/ax-go/` and contains zero ADR URLs.

## Assumptions

- **Source input**: GitHub issue #68. There is no governing ADR: the frozen `docs/adr/*` records concern the Go library's runtime behavior, not docs tooling, so none are absorbed or retired by this feature.
- **ADR handling decision (modifies the issue)**: Per maintainer direction, the published site **excludes** the ADRs (`docs/adr/*`) rather than migrating or sourcing them. This intentionally supersedes the issue's "Existing ADRs and `sources.md` are reachable in the site nav" acceptance criterion: only `sources.md` (and authored pages) are published. Rationale: `docs/adr/` is a frozen log being retired in place; publishing transient content that disappears as features delete it is undesirable.
- **First-adopter status (supersedes the issue's "copy the reference verbatim" note)**: The gh-aw-fleet #138 reference adoption is NOT confirmed working — ax-go is the first site to wire this pattern. The feature is therefore NOT blocked on gh-aw-fleet; ax-go validates the prescribed wiring itself and becomes the canonical reference later adopters (including gh-aw-fleet) copy, de-risking the broader portfolio rollout. The one remaining hard dependency is that `rshade-theme` exposes the referenced tokens (shared sans font, shared monospace font, accent color, accent-hover color); the wiring follows the prescribed token mapping and does not invent styling. finfocus remains the **primary** visual-consistency target; the rshade blog is a **secondary** comparison baseline. Both are driven by the same shared `rshade-theme` tokens, so matching either confirms correct token wiring (this is the baseline referenced by SC-002 and User Story 2).
- **Sitemap**: A sitemap is generated via the Astro sitemap integration (`@astrojs/sitemap`), which relies on the `site` and `base` configuration from FR-002/FR-003 to emit correct absolute subpath URLs. Because the sitemap is derived from the built page set, excluding ADRs from the build automatically excludes them from the sitemap — no separate ADR-filtering step is required.
- **Framework**: The docs site uses the portfolio-standard generator (Astro Starlight), matching finfocus, per the issue. The exact framework/version follows the reference implementation.
- **Pages model**: GitHub Pages is configured with the GitHub-Actions source (Pages-from-Actions), not "deploy from a branch." Enabling Pages with this source is part of delivering the feature.
- **Deploy scope**: The deploy pipeline is scoped to documentation changes (changes under `docs/`) on the default branch, so Go-only changes do not rebuild the site. Pull-request preview builds may be added but are not required for v1.
- **Project-pages subpath**: ax-go publishes to a project-pages URL, so the site is served from the `/ax-go/` subpath (not a root domain); base-path and site configuration reflect that.
- **v1 content scope**: This feature is scaffolding. The site ships with a minimal landing page plus the migrated `sources.md`. Net-new authored guides (getting-started, API reference, tutorials) are out of scope and tracked separately.
- **Theme consumption mechanism**: The shared theme is consumed as a git submodule (per the issue), which requires submodule initialization in both CI checkout and local development; a vendored copy is explicitly not used, to keep a single source of truth.
- **Repository independence**: The docs project's dependencies and build output are gitignored and do not enter the Go module's import graph or affect its tooling.
