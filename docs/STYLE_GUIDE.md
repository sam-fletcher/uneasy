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

**Chip trios** — the one formula for tinted chips/badges/banners:
`bg` = the family's darkest step, `border` = hue-700, `text` = hue-200
(families without a 200 use their pale semantic). Quiet badges/banners use
the trio as-is; *interactive selected or CTA states* (vote buttons,
aid/interfere) may swap in the bright semantic (`--color-success`,
`--color-danger`, `--color-accent`) for border+text — brightness marks
tappability, not a new colour.

| trio | bg | border | text |
|---|---|---|---|
| `--color-chip-red-*` | red-950 | red-700 | red-200 |
| `--color-chip-green-*` | green-950 | green-700 | `--color-success` (green-300) |
| `--color-chip-gold-*` | gold-850 | gold-700 | gold-200 |
| `--color-chip-violet-*` | violet-950 | violet-600 | violet-200 |
| warning (= the orange chip) | orange-900 | orange-700 | orange-200 |

Family meanings (the role map — `adr/COLOR_ROLES_PLAN.md` rulings):

- **gold** — the brand: accent, active/selected states, warm borders.
- **parchment** — paper and body text; the only card-face ground (never
  bright white).
- **neutral** — cool chrome: the elevation ladder, borders, plain text.
- **orange** — the one warning family: leveraged, pending-war, and every
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

**Player colours** (categorical, owned by `lib/playerColor.ts`; reference
block in `app.css`, sync enforced by `designTokens.test.ts`) are jewel
tones balanced to a 5.2–7.0 contrast band on the page bg — legible as
small byline text, no player louder than another. Spend them sparingly:
in chat a message colours its **byline only** (never byline + rule), a
**system-log body colours the player names inside it** (vivid — the log
reports what a player did, not what a character said; see the `playerMark`
convention under Typography), and
**in-character speech wears the muted mask-cast** —
`color-mix(in srgb, <player> 55%, var(--color-text-secondary))` — because
a character's words aren't the player's own voice. Vivid = the player as
themselves; muted = the mask. **A peer's colour is always its owning
retinue's**, never the player currently roleplaying it (scene present
lists, IC bylines, the persona picker); the byline's faint `(name)` tag is
what identifies a borrower. Grey (`--player-unknown`) is a defensive
fallback for when no player can be resolved at all — it is never used for
OOC/table-talk speech, which keeps the speaking player's own colour, and
never means "a quieter player."

## Typography

- **Fully serif.** `--font-serif` (Spectral) drives body, headings, prose.
  `--font-display` (IM Fell English) is texture: the big cover-style hero
  **only** — never at small sizes.
- **Italic** marks asset names in running text and log bodies (rendered
  from the `assetMark` `**…**` convention). Other quoted user text stays
  quoted, not styled.
- **Typography identifies what a word is, not how loud it is.** Log bodies
  carry two server-written marks and no more: `**name**` for an asset
  (italic) and `@@id|name@@` for a player (that player's colour). Verbs and
  everything else stay plain — importance is already carried by the row's
  tier. Both marks are emitted in `handler/system_posts.go` and parsed in
  `lib/logMarkup.ts`; posts written before a mark existed simply render
  plain.
- **Bold** is reserved for standalone numeric counters (rank/status
  numbers, badge counts). Names, labels, and values stay regular weight.
- Uppercase labels (badges, section headings) carry letter-spacing
  (`0.05–0.14em`) and small sizes; they are labels, not emphasis.

## Layout widths

Every content region is a phone-width column; desktop layouts differ only
in how many columns sit side by side. The numbers are **derived** — when a
new device class ships, recompute from the formulas rather than inventing
values (`adr/LAYOUT_WIDTHS_PLAN.md` has the derivations, decision history,
and the layout-by-range spec).

| token | px | derivation |
|---|---|---|
| touch minimum | 44 | Apple HIG / WCAG 2.5.5; the record rail is exactly this wide at every viewport |
| gutter | 8 | the shell's gap and edge padding |
| design band | 360–440 | narrowest (Galaxy S) → widest (iPhone Pro Max) mainstream phone |
| record-phase content min | 300 | 360 − 44 rail − 2×8. **A content box, not a viewport** — the narrowest box Main-Event/Shake-Up content (every Plan and Scene UI) must render in |
| record-phase content cap | 380 | 440 − 44 − 2×8; one design target whether or not the rail is present |
| column cap | 440 | top of the band. No other content column — chat, prologue, modals, profile — is ever wider; extra space becomes centering margins |
| record width | 316 | 360 − 44: at the viewport floor the overlay peek is exactly the touch minimum (retuned up from the eye-frozen 280, 2026-07-17; no-chip-wrap floor ≈ 246); overlay = docked panel = this token, home is `RECORD_WIDTH_PX` in `lib/breakpoints.ts` |
| chat dock | 790 | 44+8+360+8+360+8: chat docks right as soon as both columns fit at the band floor |
| record dock | 1070 | 8+316+8+360+8+360+8 = 1068, pinned round; the rail/overlay becomes a permanent panel |

Rules:

- Viewport `@media` / `matchMedia` may appear **only** in the shell (the
  table page, ChatPanel, PublicRecord) and only with the two dock
  literals. JS reads them from `lib/breakpoints.ts`.
  `layoutTokens.test.ts` fails anything else.
- Components adapt with `@container` against their column, or with fluid
  CSS — never against the viewport. Container-width allowlist: 420 (the
  prologue tile grid's 2→3 column flip, i.e. "the column is at its cap").
- Design and verify at 360 first; 344 (foldable covers) must not break
  (fluid layout + `min-width: 0`, no dedicated styles).
- Cap-and-center: above its cap a column stops growing and margins take
  the slack.

## Layout & interaction

- **Mobile-first.** Design and verify at a narrow viewport (360px) before
  desktop. Tap targets ≥ 44px (`min-height: 44px` on buttons/rows).
- Chat is a bottom strip below 790px, a docked right column at ≥ 790px.
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
| `LogMark.svelte`† | the 14 chat-log family marks (SVG); also the ranking mark on the Public Record — see **Log marks** below |

† `LogMark.svelte` lives in `components/`, not `shared/` — the name
undersells it, since it is reused outside the chat feed.

Plus `plans/shared/` (Buffet, DifficultyMeter) for plan flows and
`HelpContent` for the ?-panel/lobby help.

### Log marks

`LogMark.svelte` (`frontend/src/lib/components/`) holds 14 house SVG marks for
the chat log — one per system-post family — retiring the old Unicode
`FAMILY_GLYPHS`, of which only `§` actually existed in Spectral (the rest each
resolved to a different fallback font per platform, the real cause of the
metric fudges the old CSS fought). House icon idiom, same as
`AssetTypeIcon`/`CrownGlyph`: 24×24 viewBox, `stroke=currentColor`,
stroke-width 2, round caps/joins; die pips are the one filled/unstroked
exception. An unknown family renders nothing — every family is meant to have a
mark, so a blank is the louder bug than a fallback bullet.

`markForCode(code)` in `$lib/chatFeed.ts` routes a `system_code` to a family
(covered by `chatFeed.test.ts`). `handler/system_posts_marks_test.go` guards
the other direction: no mark character may be baked back into a Go body string.

**Sizing.** The mark box is owned by the caller's `.log-mark` span, not the
component: `width/height: 16px`, `align-self: center`, `flex-shrink: 0`,
`color: var(--color-text-muted)` — overridden to `--color-accent` on
`.log.important` and `--color-text-faint` on `.log.minor`. Centre, **not
baseline**: a geometric mark centres, a letterform sits on the baseline. This
holds the `.log` row at its measured **21.00px** — `line-height` is never set
anywhere in the chain, so it resolves to `normal` and Spectral's own `hhea`
metrics (ascender 1059 + descender 463 over 1000 upm) decide the row. A 16px
centred mark grows it by 0px; a 16px baseline-aligned one would add 1.50px.
Don't add a `line-height` to "fix" this, and don't switch centring to
baseline or start (owner ruling: centring is right, including on rows that
wrap to two lines).

**The rules the set follows** — owner-approved 2026-07-22, don't re-litigate:

1. One mark per family, every family. No bullet fallback.
2. The mark never encodes outcome, severity or magnitude. Severity is the
   gold rule, identity is the player's colour, objects are italic; the mark is
   only the *noun* — which part of the game the line belongs to.
3. Never bake a mark into a body string. Bodies are prose owned by the
   log-markup renderer; the mark slot is the mark slot. (The Go guard test
   enforces this — it caught the scales headline and the crown emoji.)
4. The mark must render from something the app ships, or its shape, weight and
   colour are the reader's OS's choice.

**Hostile-verb amendment.** Verb class has no channel of its own, and tearing
isn't a *severity* of writing — it's a different act on the same noun. So a
family carries a *second* mark when it contains a categorically hostile verb.
Exactly two qualify: `marginalia` (`torn` → torn sheet) and `asset`
(`taken`/`destroyed`/`leveraged` → crossed swords). The hostile mark is a
different object, not a damaged one — at 16px a crack or a nick is invisible.

**One ranking mark, everywhere.** The `ranking` podium is *not* chat-only.
`PublicRecord.svelte` reuses the same `<LogMark family="ranking">` on the rail
divider (`.rail-rank`, 14px) and the expanded engrailed dividers
(`.engrailed-rank`, 16px), so the Public Record and the chat card share one
component and can't drift. Consequently `★` now means **only Main Character**
(the `AssetCardSelectable` badge) and `✶` **only the Shake-Up**, app-wide —
neither doubles as the ranking marker any longer. The Help "record" schematic
is the deliberate exception: its engrailed rows stay a heavier accent line,
no podium, because the 4-bar shape doesn't read at that scale.
