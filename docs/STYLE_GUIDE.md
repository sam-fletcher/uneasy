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
| `--color-chip-blue-*` | blue-950 | blue-700 | blue-200 |
| warning (= the orange chip) | orange-900 | orange-700 | orange-200 |

Family meanings (the role map — `adr/COLOR_ROLES_PLAN.md` rulings):

- **gold** — the brand: accent, active/selected states, warm borders.
- **parchment** — paper and body text; the warm ledger/sheet ground (never
  bright white).
- **neutral** — cool chrome: the elevation ladder, borders, plain text.
- **orange** — the one warning family: leveraged, pending-war, and every
  "careful now" signal.
- **red** — danger, which *includes* the at-risk game state (one concept),
  and war. (`--color-suit-red` retired 2026-08-04 with the last playing-card
  face — see **Motion & the deck** below.)
- **green** — success and tone-include.
- **blue** — attention *and* **opportunity**: `--color-highlight`
  (activity/prepare cue, and the prologue take chip on dark grounds),
  `--color-highlight-ink` (the same cue as a fill under light text),
  `--color-info` (calm informational fill). The take marking moved here from orange on
  2026-08-01 — a take is something the viewer stands to *gain*, so marking it
  with the warning family told the wrong story — and the family was rebuilt
  from two steps to six in the same pass (ADR-009 "Blue ramp, rev 2"). Use
  `--color-highlight-ink`, not `--color-info`, for blue against a parchment
  card face.
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
- The derivations above measure from the **raw viewport**, so the table
  route's `main.full-bleed` (`routes/+layout.svelte`) must add no
  horizontal padding and no auto inline margins — any gutter there comes
  straight out of the phase column, and a shrink-to-fit `main` collapses
  it entirely. Pad inside the phase views instead.
  `layoutTokens.test.ts` fails a gutter.

## Layout & interaction

- **Mobile-first.** Design and verify at a narrow viewport (360px) before
  desktop. Tap targets ≥ 44px (`min-height: 44px` on buttons/rows).
- Chat is a bottom strip below 790px, a docked right column at ≥ 790px.
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
  size. `shared/ChecklistRow.svelte` picks the mark from its `action` prop,
  so use it rather than hand-rolling a row.

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
| `actionButton.css` | the standard button (primary gold / secondary muted) |
| `choicePip.css` | the prologue choice pip — one gold disc with two homes (turn card ↔ category header), plus the spend animation |
| `cornerBadge.css` | corner count badge on tiles |
| `jumpPulse.css` | the "you landed here" accent flash after the app scrolls the reader somewhere (chat jump, prologue claim). Carries its own reduced-motion guard |
| `marginaliaTile.css` | the warm ledger tile for marginalia |
| `modalShell.css` | sheet/modal frame |
| `rankChip.css`, `rankStrip.css` | rank track pieces |
| `statusText.css` | status/annotation text conventions (incl. `.muted`) |
| `trackCode.css` | the prologue ranking track as three letters (`POW`/`KNO`/`EST`, and `ANY` dashed) — **the** way a track reaches the screen in a tight slot. Read the letters from `choosing.ts`'s `trackCode()`, never a literal |
| `ChecklistRow.svelte` | **the** checklist/disclosure row: glyph · title+subtitle · state chip · caret-or-arrow, with an optional body. Used by the lobby's *While you wait* and the prologue's closing stage; `action` picks the affordance (see the caret rule above) |
| `TableRoster.svelte` | who's at the table, as seats: colour dot, **You** for the current player, green ring for online, gold fill for waiting-on — and no role tags, see **Roles are named, not worn** below. Caller owns the heading, the count, per-row `trailing` content and the lobby's invite chair |
| `ErrorText.svelte` | **the** way an error reaches the screen — see **Errors** below |
| `HelpDisclosure.svelte` | the collapsible "? How X works" panel — a `ChecklistRow` specialisation. **The** way step-local rules reach a play screen — see **Local help** below |
| `WeightMeter.svelte` | a prologue card's value as the house segmented meter (1–4). Takes the raw card value, never a pre-computed weight |
| `LogMark.svelte`† | the house SVG marks (14 chat-log families + `chat`); also the ranking mark on the Public Record, the mobile chat bar's icon, and the closing stage's at-risk row — see **Log marks** below |

† `LogMark.svelte` lives in `components/`, not `shared/` — the name
undersells it, since it is reused outside the chat feed.

Plus `plans/shared/` (Buffet, DifficultyMeter) for plan flows and
`HelpContent` for the ?-panel/lobby help.

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

`HelpDisclosure` is now a **specialisation of `ChecklistRow`** (the help
glyph, an expanding caret, quiet by default) rather than its own frame:
`adr/LOBBY_AND_CHECKLIST_PLAN.md` D2. Its API is unchanged, plus `tone` and
`defaultOpen` pass-throughs. Style changes belong in `ChecklistRow`, so the
lobby's checklist and the prologue's local help can't drift apart.

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
messages need — they are almost always *inserted* after an action. Two
variants, matching the two class namespaces (kept distinct because the
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

`LogMark.svelte` (`frontend/src/lib/components/`) holds 15 house SVG marks —
14 for the chat log, one per system-post family, plus `chat` (see rule 5) —
retiring the old Unicode
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
`PublicRecord.svelte` reuses the same `<LogMark family="ranking">` on the rail
divider (`.rail-rank`, 14px) and the expanded engrailed dividers
(`.engrailed-rank`, 16px), so the Public Record and the chat card share one
component and can't drift. Consequently `★` now means **only Main Character**
(the `AssetCardSelectable` badge) and `✶` **only the Shake-Up**, app-wide —
neither doubles as the ranking marker any longer. The Help "record" schematic
is the deliberate exception: its engrailed rows stay a heavier accent line,
no podium, because the 4-bar shape doesn't read at that scale.
