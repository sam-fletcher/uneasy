# Redesign: the retinue and the game pieces

Wave 3 of the visual redesign, running beside the table shell and the profile page on disjoint
files. This screen was never mocked up, so every choice below follows the written rulings; the ones
the rulings don't settle each have a question file.

## What changed and why

**1. A game piece is one look.** An asset card, an asset tile on the retinue, a marginalia tile and
the asset-creation preview now share it: the warm piece border (about 3:1 against the page), the
piece corner (4px) and the raised surface. A marginalia tile sits inside a card, which is as deep
as boxes nest. `shared/marginaliaTile.css` is the one place the tile is drawn (the retinue, the
Help sheet's replica and the creation preview all take it).
- A **torn** note is faint words with a line through them, the line in red (the colour of a loss).
  It used to fade the whole tile with opacity, which takes the text under the contrast floor.
- An **empty slot** is a dashed outline with no fill, since it isn't a piece yet.
- Tile text is 14px (it was 12.5px) and clamps to **three** lines (it was two), because two lines at
  14px cut off the notes players actually write.

**2. Selected is the bright gold border and a tick, with no fill.** `AssetCardSelectable`'s selected
card uses the chip's trick: a 1px border plus an inset 1px shadow, which draws as 2px but never moves
the card or its neighbours. The tick is the card's existing checkbox, now a neutral control box that
turns gold and shows an SVG tick. In marginalia-pick mode the card holding the picked line takes the
same border, and the picked line is bright gold text. The owner's colour is kept for the owner's shape; gold alone says
"picked".

**3. Owner marks are shapes.** A card wears its owner's shape beside its name; the coloured 3px left
edge is gone. The card is handed a colour, not a Player, so it finds the seat from the colour
(`seatForColor` in `playerColor.ts`, five colours, case-insensitive). The grey "no owner" fallback
finds no seat and so draws no shape, which is right for an unowned card. Nothing sets `token_color`
yet, so the lookup is exact today; the prop doc and `seatForColor`'s comment say to pass the Player
if custom colours ever ship. No caller changed.

**3b. The retinue sheet** (`RetinueView`):
- The title is the shared sheet title with the **owner's shape** in front, once. (Every tile on the
  sheet has one owner; see `questions/retinue-owner-shape-placement.md`.)
- **The at-risk count lives here**, as a danger notice in words with the house tear mark: to its
  owner, "2 assets need marginalia. Fill an empty slot, or one break could destroy them."; to anyone
  else, "2 assets have too few marginalia. otto can still shore them up." It counts the same thing
  the player row used to (`isNeedlesslyAtRisk` over live assets). The slot to fill still carries its
  red rim on the tile.
- **Status is words under the name, not badges.** The type chip ("PEER") is plain small capitals; a
  leveraged asset says "LEVERAGED" in orange beside the die; a destroyed one says "DESTROYED" in red.
  The header's orange pill is the same orange word with a count ("1 leveraged asset"), and "Sets next
  scene" is a plain status word (it was a filled gold pill).
- **Every control is 44px** and drawn as a control: the outline-star "make main character" toggle and
  the secrets eye (with its "+") are bordered squares. The passive ones (the filled star, the struck
  eye) are plain glyphs with no box. The old ones were 18px glyphs with a ~26px hit area.
- The tombstone loses its opacity and greyscale and keeps a dashed neutral outline and faint words.
  **The big cross over it is gone** (`questions/tombstone-cross.md`).
- Secrets are plain entries separated by lines, not boxed rows. A red hover on the "break which
  marginalia" picker is gone: that is a choice, and a choice wears no meaning colour.
- Inputs are controls: the control corner, a neutral border, the raised-twice fill, 16px text.
- The presence dot stays (it is a status dot, the one circle on the sheet) with its words.

**3c. Small pieces.**
- `AssetCardSelectable`: "Leveraged" is an orange word under the name beside the die. The leverage
  toggle is now **a chip** (`chip.css`: gold border when picked, its own locked look), with the house
  die instead of the 🎲 emoji. The caret is `RowMark`, the main-character star is `StarMark`, the torn
  mark is the house tear. A disabled card drops to the hairline and faint text, not opacity.
- `SuggestionPicker`: inputs 16px (they were 14.4px, so iPhone Safari zoomed the page on focus), the
  control corner, the raised-twice fill and a neutral border; the reroll is a plain secondary button in
  main text (it was gold on a warm border). The dashed example is kept: it is an offer, not a choice.
  The fit-to-width floor is 12px (it was 11px).
- `AssetCreationForm`: the preview is a piece; the name on it is plain text (the dashed and gold name
  box is gone); the type is words with its icon.
- `MainCharacterChoicePanel`: no box (the play area already frames it, and boxes are for things a
  player presses). The conscript cost is a **warning notice** (orange: a cost you are choosing) instead of a red
  tinted box. Flagged below.

**4. Typed glyphs are gone from these files:** `★`, `▾`, `▸`, `✗` and 🎲 each came from a different
font on every platform. New `shared/StarMark.svelte` is the star (solid, or an outline for the
control); the rest use `RowMark`, the tear and `DieMark`.
- **No second tick.** All four of `Notice`'s tone marks (warning, danger, success, info) are lifted into
  `shared/ToneMark.svelte`, and `Notice` draws its default mark from it. The selected card's tick is the
  success tone's tick, so the two share one drawing; the table shell's at-risk warning mark can take
  the warning triangle from the same place. (`questions/notice-marks.md` from the shared-pieces PR says
  `Notice.svelte` is the one file to change for the danger and info marks; it is now `ToneMark.svelte`.) `StarMark` is the one genuinely new drawing, and it is
  the star the retinue already used; `questions/star-mark.md` asks for its sign-off.
- The card's checkbox takes the control corner (`--radius-control`), as the 44px buttons do.
- The retinue's at-risk count comes from `atRiskCountByPlayer`, the helper the player row uses.

**5. `/dev/styleguide` and the guide.** The styleguide gains a StarMark cell and a **Game pieces**
section: tiles (filled, torn, titled, empty) and cards in each state (not picked, selected, leveraged,
disabled, torn, at-risk, picking a note; the first one is tappable). `STYLE_GUIDE.md` has a Game pieces
recipe and StarMark and the tile in its tables.

The `tokens: not migrated` markers are gone from `RetinueView`, `AssetCardSelectable`,
`AssetCreationForm`, `MainCharacterChoicePanel`, `SuggestionPicker` and `marginaliaTile.css`. No new
token was needed.

## How it was verified

Against this worktree's own build, on a private database (`:8184`; the "before" build of `main` on
`:8183`, same database, so each pair shows the same game):
- `make check-fast`: passes (61 test files, 958 tests, `svelte-check` clean apart from the existing
  `HelpContent` warning, `go vet`, the Go unit tests, the git-guard tests, the tracker check).
- **New unit tests** (`retinuePieces.test.ts`, 30): what the components emit, rendered through
  `svelte/server`, and what the stylesheets say. **Each bites**: I put back, one at a time, a gold wash
  on a selected card, the owner colour on the border, opacity on a disabled card, the old warm tile
  border, opacity on a torn tile, the type chip, a missing at-risk notice, the tombstone cross, the
  🎲 emoji, a typed ★, a dropped owner shape and a 30px control. All 12 fail the suite; each file was
  restored.
- **New e2e spec** (`retinue_pieces.spec.ts`, 4 tests): reads the colours the browser painted on a seeded
  table (piece border and corner, the gold selected state with no fill and no movement, the orange
  "Leveraged" word, the at-risk sentence for you and for another player, 44px controls, 16px inputs).
  It **fails 4 of 4 against the old build** and passes 4 of 4 on this one.
- **The whole e2e suite**, 167 tests (the 163 existing plus these 4), on the final build against the
  private server: all pass.
- Screenshots at 375px, before and after: your retinue, another player's, an asset tile, an at-risk
  tile, a selected and an unselected card, the main-character choice, the asset-creation form and the
  add-marginalia editor (in the PR).

**Not run against this code:**
- `make test-integration`: no Go changed.
- **Hover and focus** can't be frozen in a still: described in the stylesheets and checked in the
  unit tests, not photographed.
- The **Prologue claim step** (`ClaimChoiceModal`, which uses `AssetCreationForm` and
  `SuggestionPicker` inside a full-screen sheet) passes `prologue_claim.spec.ts` but I didn't
  photograph it.
- **Marginalia-pick mode** in a real plan's break flow: covered by the e2e suite and shown on the
  styleguide, not photographed in a plan.
- Nothing on a real phone.

## Behaviour changes to look at before merging

- **The "break which marginalia" picker in the retinue** has no red hover any more (a choice wears no
  meaning colour).
- **The conscript warning is orange, not red.** It is a cost the player chooses to pay (all their
  assets become leveraged), which is what orange says. Say if it should stay red.
- **A destroyed asset has no cross over it** (see the question).
- **Marginalia tiles are bigger** (14px, three lines) so a retinue is taller than before.
- **The at-risk count is now in two places until the shell piece merges**: the retinue's notice and
  the player row's red badge, which the shell piece removes.
- **A leveraged card in a list shows "LEVERAGED" under its name** and has a taller header than before.
- **Every caller of `AssetCardSelectable`** (the dice roll's leverage list, the plan pickers, scene
  setup and details, the Shake-Up) gets the new card with no change of its own.

## Open questions for the owner

- [retinue-owner-shape-placement](../questions/retinue-owner-shape-placement.md): shape on the title
  only, or on every tile?
- [tombstone-cross](../questions/tombstone-cross.md): does a destroyed asset keep its big cross?
- [rename-an-asset-affordance](../questions/rename-an-asset-affordance.md): how should an owner see
  that a name can be renamed?
- [star-mark](../questions/star-mark.md): sign-off for the star's drawing.

## Noticed, not fixed

- **The tombstone's dashed neutral border** is my call: no ruling covers how a destroyed asset is
  outlined. It stands for "no longer a live piece" (the warm solid border is for live ones).
- **`seatForColor` is a stopgap** that goes when the callers of `AssetCardSelectable` pass a Player.
- **The at-risk count shows in two places until the table-shell PR (#81) lands**: the retinue's notice
  and the player row's red badge, which #81 removes.
- **`rankStrip.css`** (the Power / Knowledge / Esteem strip at the top of the retinue) is still the old
  sunken boxed look: it is the shell piece's file. The retinue will look right once that lands.
- **`HelpContent.svelte`** draws a static replica of an asset card with the old type chip
  (`.ex-asset-type`) and old head. It picks up the new tile from `marginaliaTile.css` for free, but its
  card and chip are the pages piece's to redraw.
- **The five callers of `AssetCardSelectable`** hand it a colour and so lose nothing, but each could pass
  the Player when its piece migrates; then `seatForColor` can go.
- **A note longer than three lines is cut off**, and only its owner can open the tile (to edit) to read
  it all. Another player's retinue shows the cut-off text with no way to expand it. It was already
  true at two lines.
- **`plans/CardPicker`'s label and `FormField`, `EndsNote`** are still on the old text sizes (other
  pieces' files).
- **Two Playwright runs at once wipe each other's `frontend/test-results`**: a second
  `npx playwright test` started while the full suite was running deleted the first run's trace files and
  failed one unrelated test with `ENOENT`. Run them one at a time (or give each its own `--output`).
