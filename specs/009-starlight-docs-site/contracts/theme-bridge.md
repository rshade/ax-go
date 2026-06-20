# Contract: Theme Bridge (canonical, copyable)

**Feature**: `009-starlight-docs-site`

ax-go is the **first adopter** of this pattern (D7), so this file IS the
reference that gh-aw-fleet #138 and every later rshade Starlight site copies.
Keep it minimal and free of ax-go-specific styling.

## Upstream token contract (provided by `rshade-theme`)

`docs/theme/tokens.css` MUST export these four custom properties on `:root`:

| Token | Meaning |
|-------|---------|
| `--font-sans` | Portfolio body typeface (Inter) |
| `--font-mono` | Portfolio monospace typeface (JetBrains Mono) |
| `--color-accent` | Portfolio accent color |
| `--color-accent-hover` | Portfolio accent (hover/high) color |

If any token is renamed or removed upstream, the corresponding site styling
falls back to Starlight defaults (visual regression) — that is an upstream
coordination event, never patched with bespoke local literals (FR-005, edge
case "Shared token renamed upstream").

## The bridge file — `docs/src/styles/theme-bridge.css`

Verbatim canonical content (the only intended styling customization point):

```css
@import "../../theme/tokens.css";

:root {
  --sl-font: var(--font-sans);
  --sl-font-mono: var(--font-mono);
  --sl-color-accent: var(--color-accent);
  --sl-color-accent-high: var(--color-accent-hover);
}
```

### Contract rules

- **C-TB-1**: The bridge MUST `@import` the submodule's `tokens.css` first.
  Path `../../theme/tokens.css` resolves `docs/src/styles/` →
  `docs/theme/tokens.css`.
- **C-TB-2**: The bridge MUST contain ONLY `var(--token)` mappings — no color
  hex codes, no font-family literals, no per-site overrides (FR-005). Bespoke
  values here are the exact divergence this effort prevents.
- **C-TB-3**: The bridge MUST be wired through Starlight's `customCss`
  (`customCss: ['./src/styles/theme-bridge.css']`), not imported ad hoc.
- **C-TB-4**: The four mappings above are the complete required set. Additional
  mappings MAY be added later **only** if introduced into the shared reference
  (so all sites stay identical), never ax-go-only.
- **C-TB-5**: The missing-theme condition MUST be **fail-closed**. The primary
  mechanism is the unresolvable `@import` (C-TB-1): a missing/uninitialized
  `docs/theme` submodule makes `../../theme/tokens.css` unresolvable and the
  build fails. If the toolchain treats a missing `@import` as a non-fatal
  warning, an explicit build-time presence check on `docs/theme/tokens.css`
  (e.g. a `prebuild` npm script that exits non-zero) MUST be added so the build
  still fails (FR-011, SC-007). Scope: this covers a **missing submodule**; an
  upstream token **rename/removal** still resolves the import and is detected as
  a visual regression (the upstream-token-contract above), never patched with
  local literals.

### Verification

- **Visual**: rendered body font, code font, and accent color match finfocus and
  the rshade blog (SC-002).
- **Structural**: grep the bridge for hex colors / `font-family:` literals →
  expect none (C-TB-2).
- **Build**: a missing/uninitialized `docs/theme` submodule makes the `@import`
  unresolvable and the build FAILS rather than silently shipping unstyled
  (FR-011, SC-007). If `@import` only warns, add the C-TB-5 presence check so the
  build still fails.
