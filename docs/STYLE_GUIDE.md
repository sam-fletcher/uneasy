# Style guide

The living reference for Uneasy's visual language. Decision history lives
in `adr/009-design-tokens.md` (colour architecture) and the git log; this
file is what you consult while building.

**The look is mid-migration.** A redesign replaced the palette and added a
type scale, a gap scale and a set of corner radii. Every token below exists,
but screens move onto them one file at a time, so the live app looks mixed
until the last piece lands. Anything marked **retiring** still resolves
(nothing has been deleted yet) but doesn't belong in new work. A guard test
holds migrated files to the tokens; see "The migration marker" below.

**To see what exists, open `/dev/styleguide`.** It renders every token and every
shared piece below in each state, from the real components, with each value read
from the browser, so it can't drift from the app. See "The styleguide" under
Shared components.

## Tokens at a glance

| what | tokens | where it's guarded |
|---|---|---|
| colour | `--color-*` aliases of `--<family>-<step>` primitives | `designTokens.test.ts`: a colour literal anywhere but `app.css` fails |
| font size | `--text-label`, `-small`, `-body`, `-heading`, `-title` | `scaleTokens.test.ts` |
| padding, margin, gap | `--space-1` … `--space-6` | `scaleTokens.test.ts` |
| corner radius | `--radius-piece`, `-control`, `-chip`, `-sheet`, `-dot` | `scaleTokens.test.ts` |
| widths, heights, breakpoints | `--layout-*` and see **Layout widths** below | `layoutTokens.test.ts` |
| touch target | `--size-tap` (44px): the smallest a pressable thing may be | `tapTarget.test.ts`, for the shared recipes |

A token's name describes its job rather than its pixels, so tuning a value
means editing `app.css` and nothing else.

## Colour

Two tiers, both defined in [frontend/src/app.css](../frontend/src/app.css):

1. **Primitives** — `--<family>-<step>` ramps (`--gold-400`, `--red-950`).
   Families: `neutral`, `parchment`, `gold`, `orange`, `red`, `green`, `blue`
   and `violet` (retiring; `amber` is long gone). Steps run 50 (lightest) →
   950 (darkest), with in-between steps such as `neutral-875` where the new
   palette needed one. Primitives are the **only** place hex literals may
   appear, app-wide.
2. **Semantics** — `--color-*` aliases of primitives. Components reference
   these wherever a role fits; go straight to a primitive only for a true
   one-off with no role.

### Surfaces

Three levels, one for each depth a box can reach (boxes nest at most two
deep). Neighbouring levels are deliberately far enough apart to tell apart on
a phone in daylight.

| job | token | for |
|---|---|---|
| **page** | `--color-bg` | the panes themselves |
| **raised** | `--color-surface` | game pieces, chips and sheets |
| **raised twice** | `--color-surface-2` | a control standing on a raised thing (an input inside a sheet, say) |

The names predate the ladder; they were kept so no component had to change
when the values did. The page is a dark neutral grey rather than black, since
light type on pure black looks smeared while a phone scrolls. Fills stay
neutral: the warmth in the palette comes from the text, the golds and the
borders of game pieces. And a fill says something about a *control* (its
state), never about how important a region of the screen is.

### Text

Three parchment shades. **Faint is the dimmest any text gets**, and no text
uses a cool grey.

| job | token | on page | on raised | on raised twice |
|---|---|---|---|---|
| main | `--color-text` | 13.7:1 | 12.2:1 | 10.7:1 |
| secondary: secondary lines, log lines, tab labels | `--color-text-secondary` | 9.1:1 | 8.1:1 | 7.1:1 |
| faint: timestamps, captions | `--color-text-faint` | 5.9:1 | 5.2:1 | 4.6:1 |

WCAG asks for 4.5:1 on small text, and faint clears it on all three grounds,
so faint text may sit anywhere. It was picked to stay at least ΔE 6 from every
other shade in the palette, so it never reads as a near-duplicate of one.

Lines that used to be told apart by shade alone are now told apart by size
and wording. `--color-text-muted`, `--color-text-tertiary-warm` and
`--color-text-faint-warm` are **retiring** aliases of secondary and faint.

### Borders

| job | token | for |
|---|---|---|
| **piece** (warm) | `--color-border-piece` | game pieces: asset and peer cards, marginalia tiles |
| **control** (neutral) | `--color-border-control` | controls: buttons, chips, inputs |
| **divider** | `--color-border` | the thin line between neighbouring rows |
| **quiet** | `--color-border-quiet` | quieter still: the rules between the record's rows |

A die is a game piece, but it is the exception to the warm border: its edge
keeps its owner's colour.

Warm borders go on game pieces and neutral ones on controls, and a piece's
fill stays neutral. Both box borders are drawn to reach about 3:1 against the
page, the contrast WCAG expects of a control's edge, so that anything
touchable has a visible outline. (Against a raised fill the same two lines
measure about 2.7:1, and about 2.4:1 against raised twice.) The antique gold,
`--color-border-warm-antique`, survives only as ornament: the engrailed
divider and the duel's die tray.

`--color-border` is still the box border on every screen that hasn't been
migrated. The new box borders are separate names on purpose: a screen
switches to them in the same piece that removes its nested boxes, because
strong lines on a screen full of boxes inside boxes look heavy. Draw a box
only around something a player can touch: a control, a game piece or a sheet.
Group everything else (panes, steps, sections, lists) with a heading and
spacing; the most a list needs is a divider line between its rows.

### Gold: two tiers, two meanings

| job | token | for |
|---|---|---|
| **deep gold**: the game | `--color-accent` | headings, the brand, the main button, crowns, top rank, important log lines |
| **bright gold**: you | `--color-you` | the border of what you've picked, your focus ring, the waiting glyph on your chip |

Keep bright gold off headings and off fills. **A selected thing gets a 2px
`--color-you` border and no fill**, plus bright text on a chip or a tick on a
card. The main button stays deep gold, which leaves bright gold for exactly
three things: your picks, your focus and the waiting mark.

Focus ring: `outline: 2px solid var(--color-you)`. The shared recipes (buttons,
chips, rows, sheets) already use it; other sites still say `--color-accent`
until their file is migrated.

### Meaning colours

Never rely on colour alone. Pair each meaning colour with a word, and each
player colour with a shape, so the screen still works for colour-blind
players.

- **red** (`--color-danger`, `--color-danger-muted`): things lost or at risk
  (errors, at-risk assets, Mar, torn marks, the "never" tone), and war.
- **orange** (`--color-warning*`, `--color-leveraged`): caution. Three jobs,
  three names: `--color-warning-border` on dark grounds, `--color-warning-ink`
  (same paint) on a **parchment** ground, and `--color-warning` for text.
- **green** (`--color-success`, `--color-online`): success, Make, someone
  online.
- **blue** (`--color-highlight`, `--color-highlight-ink`, `--color-info`):
  something to gain, or information. Use `--color-highlight-ink`, not
  `--color-info`, for blue against a parchment card face.

Options a player is choosing between stay uncoloured (Aid and Interfere are
told apart by their words and position), because colouring one would steer
the choice. Results and states may take a meaning colour. Status words such
as "Resolving" or "Set aside" are plain secondary text, not chips.

### Player colours

Categorical, owned by `lib/playerColor.ts` (`DEFAULT_PALETTE`, with the names in
its comments), with a reference block in `app.css` whose agreement
`designTokens.test.ts` enforces. In seat order: **Lavender, Peridot, Rose,
Sapphire, Turquoise**.

- **Seats are filled in join order, and the table's creator is seat 1.** Most
  tables are small, so they use the earliest, most distinct colours. Re-check
  that order whenever a colour is tuned.
- Their lightness differs on purpose, between about 5 and 9:1 on the page,
  because colour-blind players separate them mostly by lightness. Each is also
  at least ΔE 15 from every meaning colour.
- **A player colour is paired with a shape** wherever it stands for a player
  and no name is nearby (owner marks, for instance, though not bylines), and
  beside every name in the player row so the key is always on screen. The
  shape replaces the old colour dot; it doesn't join it. In seat order:
  **Lavender diamond, Peridot triangle, Rose square, Sapphire drop, Turquoise
  gem.** `shared/PlayerShape.svelte` draws them from `lib/playerShape.ts`; a
  seat's colour and shape both come from `seatIndex()` in `playerColor.ts`, so
  pass the Player (or a seat) and never pick either by hand. 12px is the
  smallest size. A screen that hasn't migrated still draws the colour dot.
  Dice don't carry a shape yet: a die's edge stays in its owner's colour, and
  the log names whoever committed it. A player's shape is never a circle (the
  status dot's form) or a star (the main character's mark).
- Selection is never shown in a player colour.

Spend them sparingly: in chat a message colours its **byline only** (never
byline + rule), a **system-log body colours the player names inside it** (the
log reports what a player did, not what a character said; see the `playerMark`
convention under Typography), and **in-character speech wears the muted
mask-cast**, `color-mix(in srgb, <player> 55%, var(--color-text-secondary))`,
because a character's words aren't the player's own voice. **A peer's colour is
always its owning retinue's**, never the player currently roleplaying it; the
byline's faint `(name)` tag identifies a borrower. Grey (`--player-unknown`) is
a defensive fallback for when no player can be resolved at all: never used for
table-talk, and never means "a quieter player".

### Rules of thumb

- **Adding a colour = picking an existing step.** A genuinely new primitive
  requires updating ADR-009 (the bar: no step within ΔE ≈ 6 fits the role).
- **Never average, never invent midpoints**: when two colours should be one,
  keep the incumbent.
- **Semantic names describe roles, not usage sites.**
- **State variants are recipes, not hand-picked hexes**:
  - fill hover: `color-mix(in srgb, <fill> 92%, white)`
  - border hover: `color-mix(in srgb, <border> 75%, white)`; a control's is the
    token `--color-border-control-hover`
- `rgba(0,0,0,…)` / `rgba(255,255,255,…)` washes are fine: they're opacity
  effects (shadows, scrims), not palette. Only sheets cast shadows.

### Retiring colour tokens

Don't use these in new work; the cleanup piece deletes them once no screen
references them.

| token | why it goes | use instead |
|---|---|---|
| `--color-surface-sunken` | the recessed panels that needed it are going, and it was barely distinguishable from the page | the page, or raised |
| `--color-surface-active` | selected is a border now; gold tints over dark grey turn muddy brown | the selected recipe above |
| `--color-border-warm-strong` | the sunken panels it framed are gone | `--color-border-piece` / `-control` |
| `--color-text-muted`, `-tertiary-warm`, `-faint-warm` | six text shades in two families, folded into three | secondary, faint |
| violet: `--violet-*`, `--color-chip-violet-*` | "resolving" and "set aside" are secondary text now, and the first seat colour sat near it | secondary text |
| the other chip trios (`--color-chip-red/green/gold/blue-*`) | status words are plain coloured text and notices lose their tinted box, so the trios have no job left | plain coloured text; a control's border |
| `--radius-sm`, `-md`, `-lg` | named by size, not by the kind of thing | `--radius-*` below |
| `--neutral-850` | replaced by `neutral-875` | |

Also **superseded** for new work, though the redesign's rulings don't name
them: `--color-border-warm` and `--color-border-strong` (the old box borders;
`--color-border-piece` and `--color-border-control` are the one resting shade
each) and `--color-accent-bright` (the old pale "picked" gold, which
`--color-you` replaces).

## Type, gaps and corners

Three scales, each with a fixed number of steps and one job per step. Adding
a step is the owner's decision, not a convenience: a case that seems to need
13px is a case for changing the copy or the layout. Before the redesign,
sizes and gaps had drifted into dozens of near-duplicates differing by
fractions of a pixel, which is what the guard now prevents.

### Text sizes

Sizes are in rem, so the browser's text-size setting still applies. **12px is
the smallest anything gets**: thin serif strokes start to fragment on a phone
below it.

| token | px | for |
|---|---|---|
| `--text-label` | 12 | timestamps, uppercase labels |
| `--text-small` | 14 | secondary lines, log lines, chip names, tab labels, finished steps |
| `--text-body` | 16 | chat, step text, **every input** |
| `--text-heading` | 18 | the open step's heading, sheet titles, the title bar |
| `--text-title` | 22 | page titles |
| `--text-display` | fluid | the landing page's cover title only, in `--font-display`: the one documented exception |

Inputs use `--text-body` without exception: iOS Safari zooms the whole page
when a focused input is set below 16px.

### Gaps

One set for `padding`, `margin` and `gap`, in rem. The steps are spread far
enough apart that you can see which is which, so spacing alone can show what
belongs together, and that is what makes most boxes unnecessary.

| token | px |
|---|---|
| `--space-1` | 4 |
| `--space-2` | 8 |
| `--space-3` | 12 |
| `--space-4` | 16 |
| `--space-5` | 24 |
| `--space-6` | 32 |

### Corner radius

A corner radius tells you what sort of object you're looking at.

| token | px | for |
|---|---|---|
| `--radius-piece` | 4 | game pieces: asset and peer cards, marginalia tiles, dice. Close to square, like paper or a card |
| `--radius-control` | 8 | buttons and inputs |
| `--radius-chip` | fully round | chips |
| `--radius-sheet` | 12 | sheets, on the **top corners only**: `var(--radius-sheet) var(--radius-sheet) 0 0` |
| `--radius-dot` | circle | status dots only: a player's mark is a shape, so circles are reserved for status |

### Choosing a step

Pick by what the thing **is**, not by the nearest value. A `0.82rem` caption
becomes `--text-small` or `--text-label` according to its job, even if that
moves it by a pixel or two. Never nudge a size or a gap to cure a single word
stranded on a last line: `app.css` sets `text-wrap: balance` on headings,
labels and buttons and `text-wrap: pretty` on paragraphs, once, for the whole
app. A single stubborn case is fixed by editing the copy.

### What the guard accepts

`scaleTokens.test.ts` fails any `font-size`, `font`, `padding*`, `margin*`,
`gap`, `row-gap`, `column-gap` or `border-*radius*` whose value isn't a token.
It reads `<style>` blocks and style attributes: `style="…"`, `style={…}` and
`style:prop`, including string and template literals inside their expressions.

- **Allowed:** `0`; `auto`; `normal`; `inherit`, `initial`, `unset`, `revert`;
  a `var()` of the right family; a local custom property
  (`var(--chat-strip-height)`), though its fallback is judged too; and
  `calc()`, `min()`, `max()` and `clamp()` whose every operand is a token, `0`
  or a bare multiplier. A **negative margin** is `calc(-1 * var(--space-2))`;
  padding around a notch is `calc(var(--space-3) + env(safe-area-inset-bottom))`.
- **Rejected:** any other number with a unit, including `em`, `%` and `vw` (even
  inside `calc`), keyword sizes such as `small`, a token of the wrong family
  (`padding: var(--text-body)`), a token that doesn't exist (`--space-7` would
  silently render as nothing), the retiring `--radius-sm/md/lg`, and a gap
  smuggled through a custom property: one named `*-gap`, `*-gutter`,
  `*-padding`, `*-margin`, `*-radius` or `*-font-size` is held to the same rule.
  Writing `!important` after a literal doesn't help.
- **Not its job:** widths and heights (`layoutTokens.test.ts`), `line-height`,
  `letter-spacing`, border widths, and styles set from a `<script>` block.

### The migration marker

The redesign moves the app onto the scales one file at a time. A file that
still has literals starts with a marker on its **first line**, and the guard
skips it:

```svelte
<!-- tokens: not migrated -->
```

```css
/* tokens: not migrated */
```

- **The piece that migrates a file deletes its marker.** The guard also fails a
  marker on a file with nothing left to migrate, so a marker can't outlive its
  job and a migrating PR can't forget it.
- A marker anywhere but line 1, or in the wrong comment style for the file
  type, is an error: it would do nothing, and its author would think it did.
- A new file is never marked. Write it on the tokens.
- There is deliberately **no list of unmigrated files** anywhere: every
  migration PR would edit it and they would conflict on every merge.
- To see what's left: `grep -rl "tokens: not migrated" frontend/src`.

## State channels on a player row

Wherever the app draws a player — the table page's player row,
`TableRoster` (lobby seats, ready roster, retinue tallies), the profile's rows —
each fact gets **one** channel, and it is the same channel on every surface:

| fact | channel |
|---|---|
| **identity** | the player's shape and their name in their colour; the current player always reads **You** |
| **waiting on** | the clock mark: bright gold on you (`tone="you"`), secondary on anyone else |
| **at risk** | a warning mark in the at-risk red, with no number (the count is in the retinue) |
| **ready / not ready** | words in the row's trailing slot, never colour |

There is no gold border, no glow and no ring: the clock is the one way to say the
game is waiting on someone, and bright gold on it is reserved for you. A row where
everyone glowed would be shouting at you about other people's business, which is
why only your own clock is gold. A player's colour only ever identifies that
player; a selection and "you" are shown some other way.

**Online presence is not drawn on a player row.** It was a green ring on the
lobby seat and the profile pill; it was removed in the same pass. It put a
second colour channel on an element that already carries an identity colour
and a status colour, and turns here are days apart, so "online right now" is
weak information. Presence still reaches a screen reader through the row's
state words, and `RetinueView` still shows it **with a word beside it** —
which is the form to reuse if it ever comes back to the pills ("last seen
2 days ago" beats a dot, needs no legend, and cannot be mistaken for a
player colour). Do not reach for a green ring again: `--color-success` sits
ΔE 6.0 from player-3's jade, right on the near-duplicate bar.

**At-risk is red-500 rim + red-300 ink** where it is a box, as on `RetinueView`'s
marginalia slot and the closing stage's checklist row. On the player row it is the
warning mark in `--color-danger`. The retinue is where the count lives, because it
is where a player can act on it.

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

- **Sizes** are the five `--text-*` steps above. Fonts follow.
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
  numbers, badge counts), plus the label of the main button, where dark ink on
  a gold fill needs the weight to stay legible. Names, labels, and values stay
  regular weight.
- **Headings are not an exception.** `+layout.svelte` resets `h1`–`h6` to
  weight 400, because the UA default would otherwise put every heading in
  the app on Spectral's 600 cut. Headings carry hierarchy through size,
  gold, uppercase and letter-spacing; weight is not a fourth channel. A
  heading that needs weight overrides locally, with a reason.
- Uppercase labels (badges, section headings) carry letter-spacing
  (`0.05–0.14em`) and small sizes; they are labels, not emphasis.

## Layout widths

The table page has three panes: the **Public Record**, **Play** and **Chat**. A
wide window lays them out side by side as columns; a narrow one shows fewer at a
time and switches between them with tabs. Each pane is drawn at phone width, so
the layouts differ only in how many fit across. The numbers are **derived**: when a new device class ships, recompute them from
the formulas rather than inventing values.

| token | px | derivation |
|---|---|---|
| touch minimum | 44 | Apple HIG / WCAG 2.5.5 (`--size-tap`) |
| gutter | 8 | the gap between columns and the padding beside the outer two (`--space-2`) |
| column band | 360–440 | narrowest (Galaxy S) → widest (iPhone Pro Max) mainstream phone: `--layout-column-min` and `--layout-column-max` |
| two columns | 744 | 8 + 360 + 8 + 360 + 8, an upright iPad mini. Chat docks on the right |
| three columns | 1112 | 8 + 360 + 8 + 360 + 8 + 360 + 8, an iPad on its side. Every pane is a column |
| play content box | 344–424 | a column minus 8px of padding on each side: the box a plan or scene panel must render in |

The two breakpoints live in `lib/breakpoints.ts`, and `layoutTokens.test.ts`
checks that they are exactly where two and three minimum-width columns fit and
that `app.css` agrees about the band. Spare room is shared equally between
columns up to 440 each; past that the group is centred. The bar heights are tokens too: `--layout-title-bar`,
`--layout-column-head` (a column's name bar, the same height as Chat's header so
the three line up) and `--layout-tab-bar`.

| width | what the table shows |
|---|---|
| under 744 | one pane at a time, with a three-tab bar at the foot. The table always opens on Play |
| 744 to 1111 | two columns: Chat on the right, and on the left either Public Record or Play, with two tabs at the foot of that column |
| 1112 and up | three columns, no tabs, each starting with its name |

The title bar and the player row span the full width at every size, above the
columns. The Public Record's Events / Laws / Rumors chips sit at the foot of its
column at every size, above the tab bar, and show only while that pane does. A
sheet takes its column's width and opens over the column that launched it, leaving
the other columns visible and usable (see `RetinueSheet.svelte` and
`lib/sheetHost.ts`); on a phone it covers the screen.

Rules:

- Viewport `@media` / `matchMedia` may appear **only** where the layout itself
  changes, and only with the two breakpoint literals (or their `max-width`
  complements, 743 and 1111). JS reads them from `lib/breakpoints.ts`; the table
  page reads the layout once, through `lib/tableLayout.svelte.ts`.
  `layoutTokens.test.ts` fails anything else.
- Components adapt with `@container` against their column, or with fluid
  CSS — never against the viewport. The play column is the named container
  (`column`). Container-width allowlist: 300 (a squeezed column), 400 (the asset
  card's compact meta line) and 420 (the prologue tile grid's 2→3 column flip,
  i.e. "the column is at its cap").
- Design and verify at 360 first; 344 (foldable covers) must not break
  (fluid layout + `min-width: 0`, no dedicated styles).
- Cap-and-center: above its cap a column stops growing and margins take
  the slack. No width is ever a percentage of the window.
- The derivations above measure from the **raw viewport**, so the table
  route's `main.full-bleed` (`routes/+layout.svelte`) must add no
  horizontal padding and no auto inline margins — any gutter there comes
  straight out of the columns, and a shrink-to-fit `main` collapses
  it entirely. Pad inside the columns instead.
  `layoutTokens.test.ts` fails a gutter.
- A pane that isn't showing stays laid out but hidden (`visibility`, not
  `display`), so its scroll position and state survive a trip to another tab.

## Layout & interaction

- **Mobile-first.** Design and verify at a narrow viewport (360px) before
  desktop. Tap targets ≥ 44px (`min-height: var(--size-tap)` on buttons and rows).
- The panes are tabs below 744px and columns above it (see **Layout widths**).
  Which pane you picked is kept only for the visit and is never a history entry,
  so Back leaves the table; only sheets are dismissed by Back.
- Wide content scrolls inside its own container — the page never scrolls
  horizontally.
- Disabled-but-tappable: prefer `aria-disabled` + an explanation on tap
  over `disabled`, so mobile users can discover *why* (see Make Demands
  eligibility).
- **A caret expands here; an arrow takes you somewhere.** The mark on the
  right of a row states what tapping it does
  (`adr/LOBBY_AND_CHECKLIST_PLAN.md` D1):

  | mark | meaning | examples |
  |---|---|---|
  | `▾` caret | expands in place | primer, notifications, an inline form |
  | `›` arrow | opens a panel elsewhere | tones, "shore up at-risk assets" |
  | none | nothing to do | a completed item |

  Tones are the case that produced the rule: they live in a sheet, so there
  is nothing to inline and a caret would be a lie. The caret is the house
  filled triangle (see **Motion & the deck** — the app has no chevrons
  *among expanders*); the arrow is deliberately the one chevron, sized
  larger, because the two must never be mistaken for each other at phone
  size. Both are drawn by `shared/RowMark.svelte` (from paths, not typed ▾
  and › characters, which came from a different font on every platform).
  `shared/ChecklistRow.svelte` picks the mark from its `action` prop, so use
  it rather than hand-rolling a row. A tappable row has **no box**: it is a
  44px line marked by its caret or arrow, with a thin rule beneath (see
  Recipes).

  **Roles are named, not worn.** A player's role never rides on their seat as
  a title — the roster carries identity and state (colour, **You**, online,
  waiting-on) and nothing else. Say the role in the sentence where it has a
  consequence instead: the lobby's verdict ("alice will start the game once
  everyone has arrived") and the ending vote's tie-break ("If the vote ties,
  alice's side wins"). The facilitator tag was removed from `TableRoster` on
  2026-08-16 under this rule — `is_facilitator` gates exactly one visible
  control (Start Prologue) and breaks ending-vote ties, both of which already
  name the person, and the word is defined nowhere in `HelpContent`. A badge
  that repeats a fact the prose already carries, in a vocabulary the app never
  teaches, is noise on every screen it appears on.

  An item offered on two screens keeps **one leading glyph on both** — the
  lobby's "Adjust the tone of the game" and the closing stage's "Tones — last
  chance" are the same item at different urgencies, so the flag is a
  component (`FlagGlyph.svelte`, beside `HelpGlyph`/`CrownGlyph`) rather than
  a path drawn twice. Urgency lives in the row's `tone` and its copy, never
  in the mark.

## Shared components

Reuse before writing new CSS — these live in
`frontend/src/lib/components/shared/`:

| file | what it is |
|---|---|
| `actionButton.css` | **the** button: main (deep gold), secondary, greyed out; see Recipes |
| `chip.css` + `Chip.svelte` | **the** chip: a fully rounded control for choosing among a few things. Selected = 2px bright gold border and bright gold text, no fill |
| `tapRow.css` | **the** tappable row: a 44px line with a rule beneath, no box |
| `RowMark.svelte` | the caret (expands in place) and the arrow (opens elsewhere) at the end of a tappable row |
| `Notice.svelte` | **the** notice: a mark and coloured text, no box or tint |
| `PlayerShape.svelte` | a player's shape in their colour: the five seat shapes, 12px and up |
| `ClockMark.svelte` | the waiting glyph: bright gold for you, secondary for anyone else the game waits on |
| `BarbuteMark.svelte` | war's mark, in war red |
| `ToneMark.svelte` | the four marks a notice wears (a warning triangle, a danger octagon, a success tick, an info "i"), drawn once. `Notice` takes its default mark from it; so does anything else that needs a tone's mark, including the tick in a selected card's checkbox |
| `StarMark.svelte` | the main character's star: solid for the main character, an outline for the control that makes one. `★` means only this, and no player's shape is ever a star |
| `DieMark.svelte` | the die, in place of the 🎲 emoji |
| `NewDot.svelte` | the pale "new since you looked" dot |
| `choicePip.css` | the prologue choice pip — one gold disc with two homes (turn card ↔ category header), plus the spend animation |
| `cornerBadge.css` | corner count badge on a glyph that is itself a count of secrets |
| `jumpPulse.css` | the "you landed here" accent flash after the app scrolls the reader somewhere (chat jump, prologue claim). Carries its own reduced-motion guard |
| `marginaliaTile.css` | **the** marginalia tile (`.m-tile`): a game piece, with the warm border, the piece corner and the raised surface. Torn is a line through faint words with a red line; an empty slot is a dashed outline |
| `modalShell.css` | the sheet's scrim, close button and title, and the full-screen sheet variant; see Recipes |
| `rankStrip.css` | the Power / Knowledge / Esteem rank strip at the top of a retinue |
| `statusText.css` | caption and error text (`.muted-text`, `.error-text`) and **the** status word (`.status-word`: small capitals in a colour, never a chip) |
| `trackCode.css` | the prologue ranking track as three letters (`POW`/`KNO`/`EST`, and `ANY` dashed) — **the** way a track reaches the screen in a tight slot. Read the letters from `choosing.ts`'s `trackCode()`, never a literal |
| `ChecklistRow.svelte` | **the** checklist/disclosure row: glyph · title+subtitle · state word · caret-or-arrow, with an optional body. A `tapRow.css` row, so no box. Used by the lobby's *While you wait* and the prologue's closing stage; `action` picks the affordance (see the caret rule above) |
| `TableRoster.svelte` | who's at the table, as plain lines: the seat's shape, **You** for the current player, a clock on whoever the game waits on (bright gold on you), an arrow when the row is tappable — and no role tags, see **Roles are named, not worn** below. Caller owns the heading, the count, per-row `trailing` content and the lobby's invite chair |
| `ErrorText.svelte` | **the** way an error reaches the screen — see **Errors** below |
| `HelpDisclosure.svelte` | the collapsible "? How X works" panel: a `tapRow.css` row with the help mark and a caret, and its body below. **The** way step-local rules reach a play screen — see **Local help** below |
| `WeightMeter.svelte` | a prologue card's value as the house segmented meter (1–4). Takes the raw card value, never a pre-computed weight |
| `components/table/*` | **the table page's frame** (a folder, not one piece): `TableTitleBar` (Home, the title, Help), `PlayerRow` (a shape and a name per player, with the clock and the at-risk mark), `ShellColumn` (a pane's column, which sheets cover), `PaneTabs` and `RecordTabs` (the tab bar and the Events / Laws / Rumors chips), `PaneName` and `PaneMarks` (a column's name and the marks it shares with its tab), `TonesSheet`, and the Waiting On stopgap `WarEntry`. Which panes show, which tabs and which marks are pure functions in `panes.ts` |
| `LogMark.svelte`† | the house SVG marks (14 chat-log families + `chat`, `clock`, `war`); also the ranking mark on the Public Record, the mobile chat bar's icon, and the closing stage's at-risk row — see **Log marks** below. The newer marks have a component of their own that adds the size, colour rule and label |

† `LogMark.svelte` lives in `components/`, not `shared/` — the name
undersells it, since it is reused outside the chat feed.

Plus `plans/shared/` (Buffet, DifficultyMeter, EndsNote: the one sentence under a submit
that ends a plan or sub-flow; show it only in states where it is true) for plan flows and
`HelpContent` for the ?-panel/lobby help.

### Recipes

The shared pieces above, as one list of rules. The files' header comments say
more; `/dev/styleguide` shows every state.

- **Buttons** (`actionButton.css`). One **main** button per step, in deep gold
  with dark ink; everything else is **secondary** (raised surface, neutral
  control border, main text). A button that can't be pressed yet is **greyed
  out**: raised-twice surface, faint text, no border, never opacity. That is how
  Send reads before there is text, and Join before there is a code. Use
  `aria-disabled="true"` where a tap should say why. 44px tall at least; the
  corner is `--radius-control`. `.action-btn.small` (36px) survives only for
  the few dense spots that already use it, and goes in the cleanup piece so the
  touch floor has no exceptions.
- **Chips** (`chip.css`, `Chip.svelte`). A chip is a control, so it has a neutral
  border and a raised fill; something that can't be pressed (a status, a label)
  must not be drawn in that shape. **Selected is a 2px bright gold border and
  bright gold text, with no fill**, and stays that way on hover. The second
  pixel is an inset shadow, so choosing a chip never moves or resizes it. A
  chosen chip that is locked keeps its gold border and takes faint text. A
  choice wears no meaning colour and no player colour. Use `kind="tab"` for a
  chip standing in for a tab (it sets `aria-current` instead of
  `aria-pressed`).
- **Tappable rows** (`tapRow.css`, `ChecklistRow.svelte`, `RowMark.svelte`). A 44px
  line with a hairline under it and a caret or an arrow at the end. There is no
  border or fill: the end mark is the only sign the row can be pressed. Hover
  lifts the row to the raised surface; focus draws the ring inside it.
- **Status words** (`.status-word` in `statusText.css`). Small capitals in a
  colour: "Resolving", "Set aside", "Make", "Mar", a checklist state. Plain
  secondary text for a state; a meaning colour only for a result, always beside
  its word. Never a border or a fill.
- **Game pieces** (`marginaliaTile.css`, `AssetCardSelectable.svelte`, the tiles in
  `RetinueView.svelte`). An asset card and a marginalia tile are pieces: the warm piece border, the
  piece corner, the raised surface. A tile sits inside a card, and that is the only nesting.
  **Selected** is the 2px bright gold border with no fill, plus a tick in the card's checkbox (the
  success tone's tick from `ToneMark`); the second pixel is an inset shadow, so picking never moves the card. Whose a
  card is shows as the owner's **shape** beside its name, never as a coloured edge; the owner's
  colour is kept for that and gold alone says "picked". Status is **words under the name** (the type, "Leveraged", "Destroyed"), never a
  badge: leveraged is orange beside the die and its word, destroyed is red, and the type is plain
  secondary text. A card that can't be picked drops to the neutral hairline and faint text; nothing
  fades with opacity, which would take words under the contrast floor. The **at-risk count** lives in
  the retinue sheet as a danger notice in words, and the empty slot to fill keeps its red rim. See the
  "Game pieces" section of `/dev/styleguide`.
- **Notices** (`Notice.svelte`). A mark and coloured text, like a log line. A
  notice sits inside a step, so it is a group and gets no box or tint. Pass a
  house mark of the noun as `mark` when one fits better than the tone's default.
  The update banner is the one place that keeps a tinted background, since it is
  app chrome rather than game content.
- **Sheets** (`RetinueSheet.svelte`, `modalShell.css`). A raised surface, rounded
  on its top corners, with a scrim and a soft shadow: the one layer that floats
  above the panes, so the only thing that casts a shadow. A 44px close button,
  a deep gold `.sheet-title` first in the body, and no border. A control that
  has to read as raised again inside it takes the raised-twice fill. Sheets
  never nest.
- **Marks** (`PlayerShape`, `ClockMark`, `BarbuteMark`, `DieMark`, `NewDot`). One
  component each, so a mark is drawn once. The clock, barbute and die are
  `LogMark` families that the component wraps. The new dot means "new since you
  looked" everywhere. It is deliberately not gold: its form says "new", and
  colour is kept for saying whose something is (bright gold means you).
- **Focus** is always `outline: 2px solid var(--color-you)`, offset from the
  edge (inside it for a row).

### The styleguide

`/dev/styleguide` (`frontend/src/routes/dev/styleguide/`) shows every token and
every piece above, in each state, at any width. It is built to stay true:

- It imports the **real components and stylesheets**, never copies, so a
  restyled piece restyles the page.
- Every token value and contrast ratio is **measured from the browser** at
  render time. `tokens.ts` holds only names and one-line jobs, and
  `styleguide.test.ts` fails if a text, gap or radius token exists in `app.css`
  but not on the page, or the reverse.
- It ships to the live site but **nothing links to it** (a test enforces that),
  it carries a `noindex` meta tag, and it holds no game data or rule text.

When you add a shared piece, add it to the page in the same PR. Hover and focus
can't be frozen in a still, so they are described in a caption; tab or hover to
see them.

### Local help

Two help surfaces, two jobs. `HelpContent` (the global **?** panel and the
lobby) teaches **the game** — read once, browsed later.
`shared/HelpDisclosure.svelte` teaches **one step**, sitting on the play
screen right above the control it is about, collapsed by default.

Reach for a disclosure when a step needs rules a new player can't infer from
the controls, and those rules would otherwise push the controls themselves
off the first screen. Live in three places today: the prologue's choosing and
ANY-spending steps (`prologue/PrologueHelp.svelte` holds both bodies) and
scene setup.

`HelpDisclosure` draws its own head from the shared row recipes: `tapRow.css`
for the 44px line and `RowMark` for the caret, with the help mark at its start.
It no longer wraps `ChecklistRow`. Its API is unchanged, plus `tone` and
`defaultOpen` pass-throughs. Change the recipe in `tapRow.css` or `RowMark`,
which both rows use, rather than restyling one row alone.

The lobby is the one place `HelpContent` is mounted **inside a row** — a
`ChecklistRow`, not a `HelpDisclosure`, because the two help surfaces keep
their separate jobs: what the lobby offers is the whole-game primer, opened
by default, not the rules of one step. Don't reach for `HelpDisclosure` just
because you want that shape.

- **Give it a unique `id`.** It's the `aria-controls` target and two
  disclosures can be mounted at once.
- **Title it as the question the player would ask** — "How the rankings
  work", not "Rankings".
- **The body is your markup, so style it yourself.** The component only owns
  the frame and the body's flow (a flex column with a gap, and `margin: 0` on
  your paragraphs). It cannot reach your prose otherwise: the body carries
  the *caller's* scope class.
- **Don't restate the panel below it.** A disclosure that repeats the
  control's own copy just makes the reader check twice.
- **Show it to watchers too** — don't gate it on whose turn it is (owner
  ruling, 2026-08-09). The best moment to learn how a step works is while
  someone else takes it, with nothing to fill in and no turn to lose. That
  means writing copy the whole table can read: a body that only ever says
  "you decide…" about controls a watcher can't touch is a sign the copy
  needs widening, not that the panel needs hiding. Where the screen greys
  out for watchers (scene setup's read-only mirror), the disclosure stays at
  full brightness — it is the one thing there they can still operate.

### Pages outside a game

The landing and sign-in pages, the app frame, Help and the feedback form. Each
rule here applies the recipes above to a screen that has no game in it.

- **A form is a section, not a box.** The sign-in, reset and locked-out forms sit
  on the page with space between their parts. Only the controls in them are
  boxed (inputs, the chips and the buttons).
- **Log in and Sign up are two chips.** The one you're on wears the selected
  border, like any other choice.
- **Errors are a danger notice:** the danger mark beside the words, which is what
  `ErrorText` draws. A success that the user has just caused, such as a request
  sent or a password changed, is a success notice with `live` set.
- **One main button per screen.** The sign-in and reset pages each have one. A
  sheet with no step in it, such as Help, has none: its footer action is
  secondary.
- **Help's topics are chips** (`Chip` with `compact`), and its figures take no box.
  Each figure's space and caption set it apart.
- **Diagrams are drawn at their real size.** An SVG's font size counts in the
  drawing's own units, so a diagram scaled down from a larger drawing can't use
  the text tokens. The Public Record diagram in Help is drawn 1:1 for that reason.
- **The landing title is the one `--text-display` user.** Its small word is half
  that size, written as `calc(var(--text-display) * 0.5)`, which the guard accepts.
- **The update banner keeps its tint,** the one tinted strip in the app. Its
  token is the sunken surface, pending the question in `docs/work/questions/`.

## Motion & the deck

**Reduced motion.** Every animation and transition needs a
`@media (prefers-reduced-motion: reduce)` guard, written in the same file as
the keyframes so the two can't drift. The rule the guards follow
(`adr/PROLOGUE_UX_ROUND2_PLAN.md` §3c, owner's ruling 2026-08-04): `reduce`
removes **motion**, not **feedback**. A guarded interaction still scrolls to
its target, still changes state, still shows its confirmation — it just
arrives instead of travelling. For the part CSS can't reach, ask
`lib/reducedMotion.ts` (`scrollBehavior()` for `scrollIntoView`); never
hard-code `behavior: 'smooth'`.

**No playing cards.** Suits and card faces were retired from the game UI on
2026-08-04 (`adr/PROLOGUE_UX_ROUND2_PLAN.md`, decision 1 + the Session 3
ruling). A suit was a strict alias for two facts the UI can state directly —
the asset type it makes (`AssetTypeIcon`) and the ranking track it feeds
(`trackCode.css`) — and the two readings collide, so a player who learned one
was misled about the other. Card *value* survives as weight
(`WeightMeter.svelte`). The API still speaks suits and values; that is the
server's storage format, not vocabulary for the screen. `SuitGlyph.svelte`
and `cardGlyph.css` are gone — don't reintroduce them.

**Three layers, three words for the wild.** The API says `heart` (and
`CommittedHeart`, and `card_suit: 'H'`) — storage. The code says `wild`
(`.track-code.wild`, `isWild()`, `SheetTrackProfile.wild`) — the concept, a
card with no track yet. The screen says **`ANY`** — the label. Each layer keeps
its own word on purpose (`adr/PROLOGUE_UX_ROUND3_PLAN.md`, decision 2); don't
rename one to match another. The dashed `.wild` class in particular belongs to
a family — `StandingStrip`'s `.dummy` rank slots, `DifficultyMeter`'s
`.seg.next` — that shares the dash, not the label.

**A code in a sentence needs different words than a code in a chip.** `ANY` is
a real English word, so in running prose it parses as the quantifier before it
parses as the label: "your ANY cards" is broken grammar on first read. In a
chip (`[ANY] 3 in hand`) it needs nothing. In prose, either mark it as a label
("a card marked ANY") or drop the qualifier where context already carries it
("the cards doing work lock in") — Round 3, decision 3.

## Errors

Decision history in `adr/ERROR_HANDLING_PLAN.md`. Four rules.

**1. A load error and an action error never share a variable.** They have
different lifetimes, and conflating them forces bad behaviour in both
directions — the bug that produced this section.

| | load error | action error |
|---|---|---|
| means | the screen has no data, or stale data | one control the player used failed |
| lifetime | sticky | until the player tries again |
| cleared by | a **successful load**, and nothing else | the next attempt at that action |
| renders | where the missing content would be (top of the view) | next to the control that raised it |

The clear goes at the **end of the success path**, not on entry: clearing on
entry blanks the message mid-flight, so a *failed* retry looks like it did
nothing. Worked examples: `loadGameState` / `toneError` in
`routes/table/[id]/+page.svelte`, `loadError` / `actionError` in
`PrologueView.svelte` and `ShakeUpView.svelte`.

**2. Put the message where the player is looking.** A message rendered under
the page header while a modal is open is painted *underneath* that modal —
every dialog in this app is a fixed, scrim-backed overlay. A control inside a
sheet, modal or panel needs an error slot inside that same surface. Existing
slots to reuse before adding one: `ResolvingCard`'s `error` prop (the plan
tree's slot, used by 10 of 11 panels) and the `setError: (msg) => void` prop
the `war/` sub-components take to lift their message into the parent panel.

**3. Render through `ErrorText`, never a raw `<p class="error-text">`.**
It carries `role="alert"`, which cannot come from CSS and which these
messages need — they are almost always *inserted* after an action. It draws
a danger notice, not bare red text: the danger mark sits beside the words, in
the danger colour, so a message is read as a notice even by someone who can't
tell the colour apart. Two variants, matching the two class namespaces (kept distinct because the
`plans/` tree has its own unscoped `.muted`; see `statusText.css`):

```svelte
<ErrorText message={loadError} />                    <!-- .error-text  -->
<ErrorText message={prepError} variant="panel" />    <!-- .res-error   -->
<ErrorText message={error} extra="inline" />         <!-- extra classes -->
```

Because the element belongs to `ErrorText`, a parent's scoped CSS no longer
reaches it — style `extra` classes through a scoped ancestor with
`:global()`, e.g. `.table-page :global(.error) { … }`.

**4. A swallowed error carries a comment saying why.** Silent `catch {}` is
often right — a failed background refresh should leave a working page alone
(`refreshTables` in `routes/profile/+page.svelte`), and WS-backed refetches
are eventually consistent anyway. But the reasoning has to be on the page, or
the next reader can't tell a decision from an oversight.
`designTokens.test.ts`-style guard: `errorHandling.test.ts` fails the unit
suite on an uncommented empty catch.

The message itself comes from the server wherever there is one — `apiFetch`
throws an `ApiError` whose `message` is the handler's `{"error": …}` string,
already written for players. Branch on `err.status`, never on the message
text. The `'Could not …'` fallback in each `catch` is only for the case where
the throw wasn't ours.

### Log marks

`LogMark.svelte` (`frontend/src/lib/components/`) holds 17 house SVG marks —
14 for the chat log, one per system-post family, plus three requested by name
and never reached through `markForCode`: `chat` (rule 5), `clock` (the waiting
glyph) and `war` (the barbute) — retiring the old Unicode
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
5. The set is where house marks are *drawn*, not only where log families are
   listed. `chat` (a single rounded bubble — the mobile chat bar and the
   profile card's unread chip) has no `system_code`, is never reached via
   `markForCode`, and is requested by name — it lives here so "where do the
   marks live" has one answer. A new mark still needs owner sign-off; this
   one got it 2026-07-25. Check the set before drawing: `scene` is a pair of
   quotation marks and `rumor` is two offset bubbles, so the speech-shaped
   corner of the set is crowded.
6. **Detail goes where there's room for it.** `chat` and `rumor` swapped
   shapes on 2026-07-25 for this reason. `rumor` renders in one place, the
   log slot, always at 16px, so it can afford the two-bubble shape — which
   also suits it, a rumor being a thing passed between voices. `chat` renders
   at 14px on the profile chip and 18px on the bar, i.e. it is the only mark
   that goes below 16px *and* it sits on the app's highest-traffic surface,
   so it holds the simplest shape in the set. Before assigning a shape, check
   the smallest size its slot will ever render at.

**Hostile-verb amendment.** Verb class has no channel of its own, and tearing
isn't a *severity* of writing — it's a different act on the same noun. So a
family carries a *second* mark when it contains a categorically hostile verb.
Exactly two qualify: `marginalia` (`torn` → torn sheet) and `asset`
(`taken`/`destroyed`/`leveraged` → crossed swords). The hostile mark is a
different object, not a damaged one — at 16px a crack or a nick is invisible.

**Borrowing a mark outside the log** is rule 5 working as intended, not an
exception to it: the prologue closing stage's "Shore up at-risk assets" row
renders `<LogMark family="tear">` at 16px, because those assets are one tear
from destruction and the torn sheet is already the house's word for that.
Borrow by name, size the box yourself (the component fills whatever box it is
given), and only when the mark's *noun* is the thing the slot is about — a
mark picked for its silhouette is a new mark, and that needs owner sign-off.

**One ranking mark, everywhere.** The `ranking` podium is *not* chat-only.
`PublicRecord.svelte` reuses the same `<LogMark family="ranking">` on its engrailed
dividers (`.engrailed-rank`, 16px), so the Public Record and the chat card share one
component and can't drift. Consequently `★` now means **only Main Character**
(the `AssetCardSelectable` badge) and `✶` **only the Shake-Up**, app-wide —
neither doubles as the ranking marker any longer. The Help "record" schematic
is the deliberate exception: its engrailed rows stay a heavier accent line,
no podium, because the 4-bar shape doesn't read at that scale.
