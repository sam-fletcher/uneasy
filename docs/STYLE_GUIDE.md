# Style guide

The living reference for Uneasy's visual language. Decision history lives
in `adr/009-design-tokens.md` (colour architecture) and the git log; this
file is what you consult while building.

## Colour

Two tiers, both defined in [frontend/src/app.css](../frontend/src/app.css):

1. **Primitives** — `--<family>-<step>` ramps (`--gold-400`, `--red-950`).
   Eight families: `neutral`, `parchment`, `gold`, `orange`, `red`,
   `green`, `blue`, `violet` (`amber` is retired — orange is the one
   warning family; see `adr/COLOR_ROLES_PLAN.md`). Steps run 50
   (lightest) → 950 (darkest). Primitives are the **only** place hex
   literals may appear, app-wide — `src/lib/designTokens.test.ts` fails
   the build otherwise.
2. **Semantics** — `--color-*` aliases of primitives. Components reference
   these wherever a role fits (`--color-danger`, `--color-surface-warm`);
   go straight to a primitive only for a true one-off with no role.

Rules of thumb:

- **Adding a colour = picking an existing step.** A genuinely new primitive
  requires updating ADR-009 (the bar: no step within ΔE ≈ 6 fits the role).
- **Never average, never invent midpoints** — when two colours should be
  one, keep the incumbent (token first; it's the value most likely to be
  deliberately tuned, e.g. `--neutral-300` is the AA floor on the page bg).
- **Semantic names describe roles, not usage sites.** `--color-card-spent`
  (grandfathered) is the anti-pattern; `--color-danger-muted` is the goal.
- **State variants are recipes, not hand-picked hexes**:
  - fill hover: `color-mix(in srgb, <fill> 92%, white)`
  - border hover: `color-mix(in srgb, <border> 75%, white)`
  - tinted wash: `color-mix(in srgb, <hue> 12%, var(--color-surface))`
  - focus ring: `outline: 2px solid var(--color-accent)` — one ring
    colour app-wide, no per-component hues.
- `rgba(0,0,0,…)` / `rgba(255,255,255,…)` washes are fine — they're
  opacity effects (shadows, scrims), not palette.

Family meanings (the role map — `adr/COLOR_ROLES_PLAN.md` rulings):

- **gold** — the brand: accent, active/selected states, warm borders.
- **parchment** — paper and body text; the only card-face ground (never
  bright white).
- **neutral** — cool chrome: the elevation ladder, borders, plain text.
- **orange** — the one warning family: leveraged, war-mixed, and every
  "careful now" signal.
- **red** — danger, which *includes* the at-risk game state (one concept);
  war; and `--color-suit-red` (red-600), which is bespoke and never merges.
- **green** — success and tone-include.
- **blue** — attention: `--color-highlight` (activity/prepare cue) and
  `--color-info` (calm informational fill).
- **violet** — procedural, "the machinery of resolution is in motion":
  roll voting, stage chips, the prologue track.

Ledger warmth lives in the **frame, not the fill**: asset/marginalia tiles
use the plain surface ladder for backgrounds and `--color-border-warm`
(gold-850) for borders. There is no warm fill scale.

## Typography

- **Fully serif.** `--font-serif` (Spectral) drives body, headings, prose.
  `--font-display` (IM Fell English) is texture: the big cover-style hero
  **only** — never at small sizes.
- **Italic** marks asset names in running text and log bodies (rendered
  from the `assetMark` `**…**` convention). Other quoted user text stays
  quoted, not styled.
- **Bold** is reserved for standalone numeric counters (rank/status
  numbers, badge counts). Names, labels, and values stay regular weight.
- Uppercase labels (badges, section headings) carry letter-spacing
  (`0.05–0.14em`) and small sizes; they are labels, not emphasis.

## Layout & interaction

- **Mobile-first.** Design and verify at a narrow viewport (375px) before
  desktop. Tap targets ≥ 44px (`min-height: 44px` on buttons/rows).
- Chat is a bottom strip on mobile, right column at ≥ 1024px.
- Wide content scrolls inside its own container — the page never scrolls
  horizontally.
- Disabled-but-tappable: prefer `aria-disabled` + an explanation on tap
  over `disabled`, so mobile users can discover *why* (see Make Demands
  eligibility).

## Shared components

Reuse before writing new CSS — these live in
`frontend/src/lib/components/shared/`:

| file | what it is |
|---|---|
| `actionButton.css` | the standard button (primary gold / secondary muted) |
| `cardGlyph.css` | inline playing-card face chip (parchment ground) |
| `cornerBadge.css` | corner count badge on tiles |
| `marginaliaTile.css` | the warm ledger tile for marginalia |
| `modalShell.css` | sheet/modal frame |
| `rankChip.css`, `rankStrip.css` | rank track pieces |
| `statusText.css` | status/annotation text conventions (incl. `.muted`) |

Plus `plans/shared/` (Buffet, DifficultyMeter) for plan flows and
`HelpContent` for the ?-panel/lobby help.
