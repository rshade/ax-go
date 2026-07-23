# ax-go Brand & Logo

![The ax-go mascot: a teal-cyan robot gopher with glowing amber eyes, a head
antenna, and a wooden-handled axe resting on its shoulder](ax-go-logo-512.png)

The ax-go mascot is a **robot gopher shouldering an axe**. Every element maps to
what the library is:

- **Gopher** — ax-go is a foundation for **Go** CLI tools; the gopher plants it
  firmly in the Go ecosystem.
- **Robot** — ax-go exists to make CLIs first-class citizens for **autonomous
  agents** ("Agentic Experience"). The robot is the agent the library serves.
- **Axe** — the package is `ax`, and the library's flagship mandate is clean
  **stream separation** (`stdout` for the machine payload, `stderr` for logs).
  An axe cleaves; the tool cleaves one process into two clean streams.

The character is a deliberate *derivative* of the Go gopher, not a copy — the
robot treatment (antenna, panel seams, glowing eyes) keeps it recognizably Go
while making it unmistakably ax-go. See [Attribution](#attribution--licensing).

## Files

The kit has two marks — a detailed **mascot** (hero) and a simplified geometric
**logomark** (small surfaces) — plus rasterized derivatives. See
[Two-part brand system](#two-part-brand-system).

| File | Purpose |
| --- | --- |
| `ax-go-logo.svg` | **Primary mascot** mark: scalable, transparent, ~74 paths. Use everywhere it renders (web, docs, print). Auto-traced from the PNG master (see [Provenance](#provenance)). |
| `ax-go-logo-512.png` | 512px transparent PNG of the mascot for raster-only surfaces (chat avatars, embeds). Rendered from the SVG. |
| `ax-go-logo-256.png` | 256px transparent PNG of the mascot, sized for the repository `README.md` hero. |
| `ax-go-logo-dark.svg` | **Dark-garment variant** — the charcoal outline recolored to soft white so the mascot reads on black / navy / heather-dark apparel and backgrounds. Source for DTG prints on dark shirts. |
| `ax-go-logo.png` | Original 2816×1536 raster master from Gemini (light-grey ground). Kept as the generation artifact of record; regenerate derivatives from the SVG, not this. |
| `ax-go-mark.svg` | **Logomark** — a hand-drawn geometric axe cleaving one stream into two. The compact companion for nav bars, badges, and the docs site-header logo. |

The favicon derived from the logomark lives at
[`docs/public/favicon.svg`](../public/favicon.svg) (a cyan badge + axe, legible
to 16px) and is served automatically by the Starlight docs site.

## Color palette

The mascot's intended brand system is three colors on a light ground. Reuse
these values for related artwork (docs banners, badges, slides) so the brand
stays coherent.

| Role | Brand target | Notes |
| --- | --- | --- |
| Go cyan (body) | `#00ADD8` | The hero color; also the project's Go badge color. |
| Slate charcoal (outline / panels / nose) | `#1E2A32` | Outlines, panel seams, nose. |
| Warm amber (eyes / antenna / axe head) | `#F5A623` | The single accent; used sparingly. |

**Canonical color: Go-cyan `#00ADD8`.** The mascot SVG and its PNG derivatives
(`-512`, `-256`) have had their entire cyan family reconciled to a Go-cyan
anchor, so the mascot, logomark, and favicon now agree. The other elements
render as sampled below:

| Element | Value | Notes |
| --- | --- | --- |
| Body | `#00ADD8` | Go-cyan (reconciled); light/dark cyan shades are anchored to it. |
| Muzzle patch | light cyan | A lighter tint of the body — **not** tan. |
| Eyes / antenna | `#F5A62D` | Bright amber glow. |
| Axe head | `#DF8814` | Deeper orange-amber. |
| Outline / nose | `#29363F` | Slate charcoal. |
| Buck teeth | white | Two small white teeth. |
| Axe handle | wood tan | The only tan in the entire mark. |
| Background | transparent | The SVG and both PNG derivatives are transparent. |

Only the original `ax-go-logo.png` master still carries the as-generated teal
(`#1CAEC7`) body on a light-grey (`#C8CCCF`) ground — it is kept unchanged as
the generation artifact of record.

## Usage guidelines

**Do:**

- Give the mark clear space on all sides equal to at least the height of the
  antenna ball.
- Prefer `ax-go-logo.svg` (transparent, scalable) wherever it renders; fall back
  to `ax-go-logo-512.png` for raster-only surfaces. Reserve the PNG master for
  re-tracing — it carries a light-grey ground.
- Scale it proportionally.

**Don't:**

- Recolor the body away from Go cyan, or introduce colors outside the palette.
- Stretch, skew, rotate, or add drop shadows / gradients / 3D effects.
- Use this detailed mascot as a favicon — the fine panel seams and antenna
  disappear below roughly 48px. Use a simplified mark instead (see
  [Roadmap](#roadmap)).

## Two-part brand system

The **mascot** (`ax-go-logo.svg`) is the hero mark: README headers, the docs
landing page, stickers, social avatars, and slides. It is intentionally detailed
and warm — and it dissolves below ~48px, so it is never the favicon.

The **logomark** (`ax-go-mark.svg`) is the simplified companion for small,
functional surfaces — browser tabs, nav bars, app icons. It is the "Cleave": a
single geometric axe splitting one stream into two, drawn from the same charcoal
handle + amber blade as the mascot's axe so the two read as family. It survives
to 16px, which is why the favicon (`docs/public/favicon.svg`) is built from it.

## Attribution & licensing

The Go gopher was designed by **Renée French** and is distributed under a
**Creative Commons Attribution (CC-BY)** license. The ax-go mascot is an
original robot character *inspired by* that gopher; it is not the canonical
gopher artwork. If you publish or adapt this mascot, credit Renée French for the
gopher lineage. Note also that "Go" and the Go logo are trademarks of Google;
this mascot is a community derivative and is not affiliated with or endorsed by
the Go project or Google.

## Provenance

The master art was generated with **Google Gemini** (image generation) after
several iterations; this is the selected result. The prompt below is a
*reference* that reproduces this style — if you kept the exact prompt used for
`ax-go-logo.png`, paste it in to replace this one so the mark is precisely
reproducible.

```text
A friendly robot-gopher mascot for a Go tool called "ax", in the style of the
classic Go gopher: a tall rounded body with a thick charcoal outline, a lighter
muzzle patch with a small dark nose and two white buck teeth, and small ears on
the sides of the head. Robot touches: a short antenna with a glowing amber ball,
subtle metal panel seams, and large glowing amber eyes. The gopher holds a
wooden-handled axe resting on one shoulder, amber axe head. Flat vector cartoon,
solid fills, teal-cyan body, charcoal outline. Plain light background, no
gradient, no 3D. Front-facing, centered, charming, sticker-ready.
```

### Vectorization

`ax-go-logo.svg` was produced from the PNG master by automated color tracing,
reproducible as follows:

1. Key the `#C8CCCF` grey ground to transparent, composite on white, and
   downscale to 1200px wide (smooths anti-aliasing, reduces node count).
2. Trace with [VTracer](https://github.com/visioncortex/vtracer):
   `colormode=color, mode=spline, filter_speckle=10, color_precision=6`
   (the speckle filter also removes the corner generation sparkle).
3. Delete the full-canvas background path for transparency and add a
   `viewBox="0 0 1200 655"` for responsive scaling.
4. Render the 512px PNG derivative from the finished SVG.

The trace's cyan family was then reconciled to the Go-cyan (`#00ADD8`) brand
anchor by remapping every cyan-hued fill (body, shadow shades, and muzzle tint)
to that hue while preserving each shade's lightness. A full hand-redraw is still
the path to a brand-exact, minimal-node master.

## Merch & print

Plan for stickers and shirts. No print files are generated yet — this section
is the banked spec to work from when ordering.

**Locked decisions:**

- **Canonical color is Go-cyan `#00ADD8`** across all merch.
- **Source of truth is the vector** (`ax-go-logo.svg` mascot, `ax-go-mark.svg`
  logomark) — always scale from the SVG, never from a PNG.
- For **screen printing**, bright RGB cyan dulls in CMYK; specify a spot color
  (a Pantone near **PMS 312 C / 311 C**) rather than the hex, and confirm a
  proof.

### Stickers (die-cut)

- Add a ~3 mm **white keyline** offset around the silhouette — it defines the
  cut path and makes the sticker pop on a laptop lid.
- Supply the vector (SVG/PDF) or a 300 DPI transparent PNG at final size
  (3 in ≈ 900 px, 4 in ≈ 1200 px). Vendors such as Sticker Mule or StickerApp
  auto-generate the cut line from the transparent edge.
- Keep art ≥ 1/8 in inside the cut for a safe margin. The mascot is the primary
  sticker; the logomark makes a good smaller second sticker.

### Shirts

- **Dark-garment variant is required.** The `#1E2A32` charcoal outline
  disappears on black or navy, so a light/white-outline version is needed for
  dark shirts (a plain cyan mascot on charcoal/navy is the strongest look).
- **DTG (direct-to-garment)** for small runs: full color, supply a 300 DPI
  transparent PNG at print width (≈ 10–11 in ⇒ ~3000–3300 px), sRGB.
- **Screen printing** for volume: cheaper per unit but priced per color, so
  reduce to spot colors and supply the vector plus a Pantone per screen.

### Print files

`docs/brand/print/` holds vendor-ready exports, generated on demand from the
vectors:

- `ax-go-mascot-heatherblack-3300.png` — the dark-garment mascot at 300 DPI,
  ~11 in wide (3300 px), transparent — a DTG upload file for a **heather-black
  shirt** (blank: Bella+Canvas 3001CVC Heather Black).
- `ax-go-sticker-diecut.png` — the mascot with a rounded white keyline for a
  **die-cut sticker**, transparent, 1688 px wide (~5.6 in at 300 DPI; scale to
  the ordered size). Vendors cut on the outer transparent edge of the keyline.

Still to generate when needed: light-garment print PNGs and PDF/EPS exports for
screen-print vendors.

**Still to decide:** sticker size(s) and whether the logomark ships as its own
sticker.

## Roadmap

Follow-ups to turn this master into a complete brand kit:

- [x] Vectorize the mascot to a scalable `.svg`.
- [x] Export a transparent-background asset (the SVG, plus the 512px PNG).
- [x] Clean the faint generation sparkle in the lower-right corner.
- [x] Generate an optimized web size (512px) so pages don't ship the 4.8 MB
      master.
- [x] Design the simplified "Cleave" logomark (`ax-go-mark.svg`), derive the
      favicon (`docs/public/favicon.svg`), and wire it as the Starlight
      site-header logo.
- [x] Add the hero mark to the top of the repository `README.md`.
- [x] Reconcile the mascot's cyan family to the Go-cyan target (`#00ADD8`) so
      the mascot, logomark, and favicon agree.
- [ ] Hand-redraw the mascot for a brand-exact, minimal-node SVG (the trace is
      faithful but node-heavy).
- [ ] Add PNG/ICO favicon fallbacks if analytics show non-SVG-capable clients.
- [ ] Produce the print pack when ordering merch (see
      [Merch & print](#merch--print)).
