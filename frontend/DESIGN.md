# Design System — Verdant Oasis (绿洲)

> Read this before any visual or UI change. Tokens live in `tailwind.config.ts`
> and `app/globals.css`; primitives in `components/ui.tsx`. Don't deviate from the
> system without a deliberate reason — and when you do, say why in the PR.

## Product context
- **What:** high-trust infrastructure for AI training-data exchange, where the
  seller's raw data never leaves its sandbox and buyers take only the verifiable
  result of an audited algorithm ("available-but-invisible" / Compute-to-Data).
- **Who:** data sellers, AI/ML buyers, and platform operators (ops console).
- **Type:** hybrid — marketing/trust destinations (editorial) + app tooling (utility).

## The memorable thing
**"Verifiable infrastructure, presented like a peer-reviewed paper."** Every move
should reinforce: documented, audited, cryptographically real. The product's edge
is honesty + proof — the design must look like it, not like a generic SaaS template.

## Aesthetic direction
- **Editorial-technical.** Warm paper, big serif claims, monospace as literal proof.
- **Decoration:** minimal. Type, rules (hairlines), and whitespace do the work. No
  gradients, no drop shadows, no decorative blobs, no icon-in-colored-circle grids.
- **Restraint is the brand.** Display serif is reserved for destinations (home,
  /trust, /compute, marketplace) and hero moments. Utility tool pages (orders,
  admin, sell workbench) stay in clean sans — not everything shouts.

## Typography
- **Display:** `Instrument Serif` (`font-display`) — editorial gravitas + authority.
  Used for hero claims and major destination H1s. Sizes: `display-sm/md/lg/xl`.
- **Body:** `Geist` (`font-sans`) — precise, technical, warm. Default everywhere.
- **Mono:** `Geist Mono` (`font-mono`) — PROOF POINTS: certificate IDs, hashes,
  sample counts, kickers, stage numbers, the L-tier honesty footnotes. Mono is the
  signal "this is real data, not marketing." Use it deliberately, not decoratively.
- **Kicker:** `text-kicker` (mono, uppercase, wide tracking) sits above big headings.
- Loaded via `next/font/google` in `app/layout.tsx` with `display: "swap"`.

## Color
Warm, near-monochrome with two accents used sparingly.
- `paper #fafaf7` — page background (printed-page warmth, not screen white).
- `ink #18181b` — primary text + primary buttons.
- `muted #71717a` — secondary text, mono labels.
- `rule #e7e5e0` — hairline borders, dividers (the editorial "rule").
- `forest #047857` (+ 50/100/200/700/900) — the trust/privacy accent. Sandbox
  boundary, "what it guarantees", stage-2. Used sparingly.
- `gold #b45309` (+ 50/100/700) — the WAX-SEAL accent. Appears ONLY on verified-
  certificate cues (the cert seal, sample cert IDs, "honest limits" markers).
  Gold is precious — if it's everywhere it means nothing.
- No dark mode (the app declares `color-scheme: light`).

## Spacing & layout
- Generous vertical rhythm on destinations: section gaps `space-y-20`–`space-y-32`.
  Utility pages stay tighter (`space-y-6`–`space-y-8`).
- Max content width `max-w-6xl`, `px-4`.
- Radii: inputs `rounded-lg`, cards `rounded-2xl`, buttons + tabs `rounded-full`.
- Cards: `border border-rule bg-white` — flat, no shadow.

## Components (`components/ui.tsx`)
- **Button:** pill (`rounded-full`). primary = ink/paper; secondary = bordered;
  danger = red-700; ghost. All carry a visible `focus-visible:ring-ink`.
- **Input/Textarea/Select:** `rounded-lg`, `border-rule`, `focus:border-ink` + faint ring.
- **Card:** `rounded-2xl border-rule bg-white p-6`.
- **Badge:** status pill, color-mapped by status string.
- **ComputeFlowDiagram:** the signature schematic. Mono stage numbers, serif
  titles, dashed forest sandbox boundary, a single gold ✓ seal. Scrolls
  horizontally on mobile (intrinsic 720px width inside `overflow-x-auto`).

## Motion
Minimal-functional. `transition` on interactive elements only. No scroll
choreography, no entrance animations.

## Anti-slop guardrails (do NOT introduce)
Purple gradients, 3-column icon-circle feature grids, centered-everything, gradient
CTA buttons, bubble-radius on everything, stock-photo heroes, `system-ui` as a
display font, Inter/Roboto/Space Grotesk as primary type.

## Decisions log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-06-16 | Editorial-technical system created (Instrument Serif + Geist + Geist Mono, paper/ink, forest+gold) | Lift from generic SaaS template to a memorable, proof-forward identity that matches the "honest + verifiable" moat. |
