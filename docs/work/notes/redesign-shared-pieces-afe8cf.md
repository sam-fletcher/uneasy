# redesign-shared-pieces-afe8cf

The pieces every later redesign PR builds from, the shared recipes, and `/dev/styleguide`, a
page that shows all of it. Wave 2 of the visual redesign; the tokens and guard test (wave 1)
are what this stands on.

## What changed and why

**1. The house marks, one component each** (`lib/components/shared/`):

| Component | What it is |
|---|---|
| `PlayerShape` | a player's shape in their colour: diamond, triangle, square, drop, gem, in seat order. `lib/playerShape.ts` holds the outlines; `seatIndex()` (new, in `playerColor.ts`) is the one place that maps a seat to a slot, so colour and shape can't disagree |
| `ClockMark` | the waiting glyph. `tone="you"` is bright gold, `"other"` is secondary |
| `BarbuteMark` | war's mark, in war red |
| `DieMark` | the die, for the 🎲 emoji |
| `NewDot` | the pale "new since you looked" dot |
| `RowMark` | the caret and the arrow at the end of a tappable row |

The clock, barbute and die are **`LogMark` families** (`clock`, `war`, and the existing `roll`)
wrapped by a component each. The house rule is that marks are drawn in one file; the wrapper adds
the size, the colour rule and the label. `PlayerShape` and `NewDot` are filled shapes at small
sizes, so they aren't line marks and have their own components.

**2. The shared recipes.** `actionButton.css` (main, secondary, greyed out), `chip.css` +
`Chip.svelte`, `tapRow.css`, `.status-word` in `statusText.css`, `Notice.svelte`, and the sheet in
`modalShell.css` + `RetinueSheet.svelte`. Each file's header says the rule behind it. The
STYLE_GUIDE has a new "Recipes" section that lists them.

**3. The migration.** The five CSS files and `RetinueSheet`, `ChecklistRow`, `TableRoster`,
`CrownGlyph` and `CharCounter` are off their markers and onto the tokens. Three of those are more
than a token swap, because the shapes and clock changed what they draw:
- **`TableRoster`**: a seat is a plain line with the seat's shape, the name in the player's colour,
  and a clock when the game waits on them (bright gold on you). The pill, the gold frame and the
  glow are gone. A tappable seat gets an arrow, since it opens a sheet.
- **`ChecklistRow`**: no box. It's a `.tap-row` with a rule beneath; a tone colours the title (and the
  state word), never a border or a wash; the state is a status word, not a chip.
- **`RetinueSheet`**: a raised surface, top corners 12px, a shadow, no border and no grabber (it never
  dragged), an SVG close button, and the title is deep gold at heading size.

**4. `/dev/styleguide`** (`routes/dev/styleguide/`). Every token and piece in each state. It imports
the real components and stylesheets, and every value and contrast ratio is **measured from the
browser** on mount, so it can't drift. `styleguide.test.ts` fails if a text, gap or radius token is
missing from the page, if anything in the app links to it, or if the `noindex` tag goes.

### Changes after the coordinator's review
- **A hovered selected chip kept a grey outer ring.** The hover rule (specificity 0,4,0) beat
  `.chip.selected` (0,2,0), and the generic disabled rule did the same to a locked, chosen chip, so
  both showed a grey 1px border around the gold inset pixel. Hover now excludes `.selected`, and the
  chosen-and-locked state is its own rule: **it keeps the bright gold border (it still shows which
  one was picked) and takes faint text, since it can't be pressed.**
- **Tests that bite on that.** `cssTestKit.ts` runs the cascade (specificity, then source order) over
  a stylesheet for an element in a given state, and `pieces.test.ts` asks it what a selected chip's
  border is at rest, hovered, focused and locked. `tests/e2e/styleguide.spec.ts` reads the real
  painted colours in Chromium. I put the old bug back to check: both fail on it (the e2e shows the
  hovered chip painting `rgb(140, 140, 140)` for the gold).
- **The "no box" tests read the CSS, not the source text.** They strip comments, so prose that names
  `border-radius` neither fails nor hides a real one (both are tested), and they now also refuse
  `box-shadow`, `outline` and `border-color`. `ChecklistRow` and `TableRoster` may draw only the
  rule beneath a row; `Notice` may draw none. `TableRoster` also checks one shape per seat.
- **Reworded** the sentences that followed the private rulings too closely: the update-banner note,
  the new dot's meaning, the tappable-row and border descriptions (guide, `tapRow.css`, the
  styleguide), the chip's no-fill reason, and the dice/circle/star passage.
- **One token for the hover border** (`--color-border-control-hover`, beside `--color-surface-hover`).
  `chip.css` and `actionButton.css` no longer carry the `color-mix` recipe; a test fails if they do.

### Decisions I made (flag any you'd rule differently)

- **Chip selected is `border` 1px + an inset 1px shadow in bright gold**, not a 2px border, so
  selecting a chip never shifts or resizes it or its neighbours. It renders as the ruled 2px border.
- **The die is only the mark.** The tile around a die (owner's colour on its edge, "For" and
  "Against" groups) is `DiceRollPanel`'s, which is the Play piece's file. The 🎲 swap is left there.
- **The barbute exists but war lines still show the flag.** `LogMark` now has a `war` family; routing
  the make-war codes to it is a one-line change in the Chat piece, which owns `markForCode`'s callers.
- **The D1 rule survives** (caret expands, arrow opens elsewhere) but both are drawn as SVG instead of
  typed `▾` and `›`. The caret is a filled triangle, the arrow a stroked chevron. (The mockups draw a
  chevron for both; I kept D1 because the rulings still say "caret or arrow".)
- **Greyed out is the raised-twice fill, faint text and no border**, not opacity (from the mockups'
  Send and Join). `aria-disabled` looks the same as `:disabled`.
- **The main button's label is weight 600** (the mockups). The guide said bold was only for counters;
  I added the main button to that sentence.
- **Checklist state colours:** on → green, off → orange, **warn → orange (was deep gold, from the
  old chip)**, risk → red, neutral → secondary. A row's tone colours its title: warn orange, risk red,
  primary and blocker deep gold. I changed `warn` so a deadline row is one colour: its title was the
  careful orange and its state word the game's gold, and the redesign marks "the game is waiting on
  you" with the clock, not with gold. `.status-word.accent` and `.faint` had no user left, so they are
  gone. If the owner wants the state word gold, it is one line in `ChecklistRow` and one in `statusText.css`.
- **`--size-tap` (44px) is a token**, so the touch floor is written once. The shared recipes use it,
  and `tapTarget.test.ts` fails if one of them retypes `44px`. Unmigrated screens still write it by hand;
  each piece moves onto the token when it migrates its file.
- **One mark size per row:** `RowMark` is a fixed 16px and the leading glyph slot in `ChecklistRow` is
  16px too (the help glyph was 18). I dropped `RowMark`'s `size` prop; nothing set it.
- **`corner-badge` digits went from 10px to the 12px floor**, and the badge from 15 to 16px.
- **`.sheet-title` is now 44px tall** (`min-height`) and clears the close button, so a long title can't
  run under it. Every sheet in the app picks that up.
- **Removed `margin: -1px`** from `TableRoster`'s screen-reader-only line. The guard rejects a negative
  literal, and an absolutely positioned 1px clipped box needs none.
- **Faint text is `#999689` and Sapphire `#4386fe`**, as `app.css` has them, not the older values in
  the decisions file.

## How it was verified

Against this worktree's code:
- `make test-integration` passed on a private database before the review round (no Go changed since,
  so it was not run again), and `make check-fast` passes: 59 test files, 905 tests, `svelte-check`
  clean (one existing warning in `HelpContent.svelte`), `go vet`, the Go unit tests, the git-guard
  hook tests, the tracker check.
- **The guard bites on real files**, and **each new test fails when its rule is undone**. The first
  nine mutations, all restored: a fill on `.chip.selected`, opacity on a disabled button, a gap token
  dropped from the page, `noindex` removed, a link to the styleguide in `HelpButton`, a shape named
  "circle", a typed `▾` in `RowMark`, a radius back in `ChecklistRow`, a dot back in `TableRoster`.
  After the review, 13 more: a hover rule out-ranking `.selected`, the chosen-and-locked rule removed,
  a `box-shadow` or `border-color` added to `ChecklistRow`, a `box-shadow` on a `TableRoster` seat, a
  `border` or `box-shadow` on `Notice`, a literal `44px` in `tapRow.css`, `.status-word.accent` back, the
  `color-mix` recipe copied into `actionButton.css`, and a warn state word back to gold. Two of the 13
  are comments naming `border-radius` and `background`, and they correctly pass.
- **e2e, against a private server** (`:8192`, a private database): the full suite, 163 tests (159 plus
  the four chip checks in `styleguide.spec.ts`), all pass.
  The specs that touch the changed screens (lobby, prologue claim, back-dismiss, tones, chat, smoke,
  the seeded main event) were also run on their own first.
- **Screenshots at 375px**, before (a build of `main` on `:8193`) and after (this branch on `:8192`),
  against one shared private database so each pair shows the same game: the lobby, a prologue closing
  stage with blockers, the Help sheet (and its corner-badge schematic), a retinue with the monarch's
  crown, the profile page, and the sign-up counter. And `/dev/styleguide` at 375 and 1280px.

**Not run against this code:**
- `jumpPulse.css`: its only change is `4px` to `--radius-piece`, the same 4px, so before and after are
  identical, and it is a 0.7s flash I can't freeze honestly. Not photographed.
- The full-screen variant of the sheet (`.modal-sheet`, used by the claim step's modal): migrated and
  covered by `prologue_claim.spec.ts`, but I didn't photograph it.
- Hover and focus states: they can't be frozen in a still. Checked in the stylesheets and described on
  the styleguide, not photographed.
- Real asset cards' corner badges: only the Help schematic that borrows `.corner-badge`. No seeded
  game has secrets.

## Behaviour changes to look at before merging

- **Every button in the app restyles** (the recipe CSS is global): main buttons are deep gold with
  heavier dark text and the control corner; secondary ones are raised-surface with a neutral border
  instead of gold text on grey; disabled ones are grey, not faded. The look is mixed until the screens
  migrate, as accepted.
- **The profile page now shows four "main" buttons at once** (Create a new table, both Edit buttons,
  Send feedback), against the one-main-button-per-step rule. The profile piece fixes it ("Create a new
  table" becomes secondary). Nothing is broken, but it's the roughest knock-on.
- **Lobby and closing stage**: the seats are lines with shapes instead of pills, the checklist rows
  have no boxes, and their state is small capitals. The header chips above still draw colour dots and
  gold rings, because the shell piece owns them.
- **Every sheet** (Help, Tones, Laws, Rumors, feedback, a retinue) is now a raised surface with a
  shadow and a gold title, and has lost its grabber.
- `HelpDisclosure` is a `ChecklistRow`, so it changed too: its title is now main-colour 16px instead of
  quiet secondary 14px. The pages piece owns that file.

## Open questions for the owner

- [notice-marks](../questions/notice-marks.md): the danger (octagon) and info (circled "i") notice marks
  are mine, not signed off, and whether an error should wear one.
- [styleguide-crawlers](../questions/styleguide-crawlers.md): the `noindex` tag is added by the page's
  code, so a crawler that doesn't run JavaScript never sees it. Add a header or a `robots.txt` as well?

## Noticed, not fixed

- `+layout.svelte` has a global `button` rule (padding, radius, size) that every recipe has to
  override. It belongs to the pages piece. The same file shows the site header on `/dev/styleguide`
  when you're logged in; harmless, but not ideal.
- `.action-btn.small` is 36px tall, under the 44px floor. It's kept, and labelled legacy on the
  styleguide, for the few spots that use it. **The cleanup piece should retire it** so the floor has no
  exceptions (the item file is another entry's, so I haven't edited it).
- `ErrorText` still renders plain red text with no mark (its piece is the pages piece).
- `DiceRollPanel` still draws the 🎲 emoji (the Play piece).
- `deadcode` prints `middleware/auth.go: unreachable func: AccountContext` on `main` too; it doesn't
  fail the check.
