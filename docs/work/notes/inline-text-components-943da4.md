# Inline text components

## What changed and why

Each kind of inline text now has one Svelte component that owns its look, and the
frontend is moved onto them. The owner's ruling (the "Each kind of inline text has
one component" bullet of the Round 6 decisions) was the spec; the table is in
`docs/STYLE_GUIDE.md` under **Inline text**.

- **`PlayerName`**: the name in the player's colour (`playerColor`), no shape in a
  sentence. `label` adds the shape first, drawn by the existing `PlayerShape` (no
  second shape renderer), for bylines, the who-line, chips and the player row.
  `fixed` keeps the colour inside a player chip, and a chip's `.unavailable` class (set from `isDisabled`) is what makes its name and shape faint. A player nobody can see or find
  (`player={null}`) is the unknown-player grey, upright, no shape, even as a label.
  `PlayerNames` is a comma-joined list of them.
- **`AssetName`**: an `<em class="asset-name">`, italic, the colour around it.
- **`Quote`**: curly marks, upright, never parsed.
- **One sheet, `shared/inlineText.css`**, holds the look. The colour reaches a name
  as a custom property (`--player-name-color`), not as `color`, so a control takes
  it back with a plain class rule (`color: inherit` inside `.action-btn`, and inside
  `.chip` unless `.fixed`, and in a chip that can't be picked). No caller passes a
  prop to say "I'm in a button". Read-only mirrors aren't faded: their chips are
  `disabled` for the browser, so the faint rule is keyed on `.chip.unavailable`
  (never bare `:disabled`), which only a chip that can't be picked gets, and that
  fades its shape too.
- **The log** (`logMarkup.ts`) now emits the same classes (`asset-name`,
  `player-name` + `--player-name-color`). The old `.player-mark` rule in
  `ChatPanel.svelte` is gone. A log name with an unusable colour now falls back to
  the unknown grey rather than the secondary text colour.
- **Migrated**: ~103 `{playerName(players, x)}` in plan panels and phase views, 47
  hand-written asset `<em>`, the `style:color` spans (guest list, war lists, duel
  sides, player row, Laws/Rumors bylines, scene focus line, profile table row, Ended
  view, Prologue/Shake-Up), the curly-quoted player text in plan panels, and the
  `LawsRumors` "Unknown" (now grey, upright). The two non-asset `<em>` in plan panels
  (plan names, "every", "(declined)") became plain text.
- **Italic means an asset and nothing else**: removed from `.empty-note` (card and
  retinue), `.lr-empty`, `.lr-addendum`, the Prologue pending chip, `.wild-note`,
  `.choice-note`, `.decided-by`, `.empty-slot`, `.title-desc`, `.scene-prompt`,
  `.m-text.destroys`, the suggestion picker, and every `<em>` in Help except the
  book's title. Nothing is italic outside `.asset-name` now (the guard's allow-list
  is `app.css` and `inlineText.css`): ClosingStage's blank-asset names go through
  `AssetName`, and the card form's "Unnamed" is faint and upright, like an empty
  state (it stands in for the card's own title).
- **Review round**: a read-only `PlayerChips` mirror keeps its names and shapes in
  colour (the faint rule is `.chip.unavailable`, not bare `:disabled`); an
  unavailable chip's shape goes faint too; `TableRoster` and the Retinue sheet title
  ("wren's Retinue", the name coloured) are on `PlayerName`; the guard also catches
  `style:--var=` from a player colour and the `font: italic` shorthand; the
  "an asset" fallback in Spread Rumors is plain, not italic.
- **Class collisions**: `RetinueView`'s own `.asset-name` (a sheet title) is now
  `.tile-name`, which also keeps it upright as the ruling says; `TableRow`'s local
  `.player-name` went when it moved onto `PlayerName`.
- **Guard test**, `shared/inlineText.test.ts`: fails a hand-written `<em>` (only the
  Help title is allowed), an inline `style:color` from a player colour outside the
  components and `PlayerChips`, a second definition of `.player-name`/`.asset-name`/
  `.quote`, and any new file declaring `font-style: italic`. It also checks the
  components' output and that the log uses the same classes.
- `/dev/styleguide` gained an **Inline text** section with each row of the table.

## How it was verified

- `make check-fast` passes (Go vet/lint/unit, svelte-check, 2326 Vitest tests,
  hook tests, tracker check).
- The guard test was shown failing: a hand-written `<em>Old Keep</em>` and a
  `style:color={playerColor(...)}` put into `rumors/TakeAssets.svelte` made both
  checks fail with the file named, and restoring it made them pass.
- 375px screenshots from a private server (port 8192, private database) against
  `main`'s dev server (port 8080) for before: an Exchange Courtiers step as the
  target (finished lines, names in a sentence, an asset in a sentence), the Rumors
  list with a hidden source, and the Help page. The main button, choice chips, a
  player chip, a disabled chip, a warning notice and the hidden source are shown from
  the styleguide's new section, because no panel puts a name inside those on `main`,
  so there is no "before" for them.
- The full Playwright suite ran against the private server: 255 passed, 1 failed
  (the Ended view's e2e selector used the old `.name` class); fixed, and that spec
  re-run and passed.
- **Not run**: `make test-integration` (no Go code changed); nothing checked on a
  real phone.

## Behaviour changes to look at before merging

- Names in plan panels are now coloured everywhere the old code wrote them plain,
  including the line "X to respond" on duel tracker and finished lines.
- A rumor's hidden source is the grey "Unknown", upright (was faint italics). A log
  name whose player can't be resolved is the unknown grey (was secondary text).
- Player names in the player row, byline and focus line keep colour and shape, but
  now come from `PlayerName`, so their gap and ellipsis are the component's: check
  the player row and profile table rows at 375px.
- Italic is gone from the empty states and notes listed above (and from the Help
  terms). A rumor's "About" asset name is now italic.
- Peace terms in Make War now show curly quotes (were straight).

## Open questions for the owner

None filed. One judgement call to confirm: removing italic from the suggestion
picker's example button and from `.title-desc`/`.scene-prompt` (all player or
example text, so "italic means an asset only").

## Noticed, not fixed

- Names inside strings stay plain (tracked as `docs/work/next/inline-text-names-in-strings.md`):
  `ownerLabel`/`title`/`label` props, `aria-label`s, `EndsNote` text,
  `DelayThresholdNote`, the Make Demands option lines, the Prologue/Shake-Up
  "Owned by" labels, `TurnCard`, `SimultaneousRevealInput` (display names only, no
  seat) and `warName`; `stepHeading.ts` is Round 6's.
- Chat bylines and the scene present-list still colour names their own way (bylines
  have the in-character mask-cast); they're out of this piece's table.
- The set piece's deep-gold peers and centred quotes are Round 6's, untouched.
- A chip lays its children out with a gap, so a mixed label (words, name, asset)
  needs one wrapping `<span>`; noted in the style guide, not enforced.
