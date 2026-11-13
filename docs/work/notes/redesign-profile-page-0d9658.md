# Redesign: the profile page's table list

## What changed and why

The profile's table list is rebuilt to the signed-off design, and the page is off the
`tokens: not migrated` marker.

- **Rows, not cards.** Each table is a tappable row with an arrow at its end and a hairline
  between rows. No box, fill or radius, and a finished table is no longer faded to 55%.
  `lib/components/profile/TableRow.svelte` holds one row.
  - First line: the table's own title from `tableTitle()`, the same words its title bar says. At
    the line's end, when chat is unread, the pale `NewDot` and "N new" in plain text (capped at
    "99+ new", as the old chip was; still hidden on a finished table, as before).
  - Second line: each player as `PlayerShape` and their name in their colour, no pill. `ClockMark`
    on whoever the game waits on: bright gold on you, grey on others. Your own name stays your
    name, not "You" (your seat, and so your shape and colour, can differ from table to table).
  - No join code. No `PhaseBadge`.
  - The reminder bell stays: a 44px button beside the arrow, still only on a table waiting on you.
- **Three groups.** `lib/tableGroups.ts` splits the list into "Waiting on you", "Waiting on
  others" and "Finished", drops an empty group and its heading, keeps each group's server order
  (most recently joined first), and puts a live table waiting on no one under "Waiting on others".
- **Sections lose their boxes.** Join, Notifications and Account are a heading and space; the only
  boxed things left are controls (inputs, selects, buttons). The `.card`, the old two-up tile grid
  and the 790px width rule are gone.
- **Buttons.** Join is greyed until a code is typed (its disabled style is the shared one now);
  "Create a new table" is secondary. See the first behaviour change below for the others.
- **Tokens.** Every font size, gap and radius on the page is a token, so the marker is gone and
  `scaleTokens.test.ts` now holds the page to them.
- **The bell's accessible name** (`lib/tableBell.ts`) now names the table by its title instead of
  its join code. The code is no longer on screen, so a screen-reader user couldn't match it to
  anything.
- **Server: no change.** `ListMyTables` already sent everything the row needs (title fields,
  `unread_count`, `waiting_on_player_ids`, the roster with seats), in the one request. Nothing to
  add, so no Go changed.

### Why one `<li>` holds a link, a button and an arrow

A `<button>` can't sit inside an `<a>`, so the bell and the link are siblings. The link stretches
over the whole row through a `::after` overlay so the full row stays one tap target. The bell is
positioned above that overlay. The arrow sits beneath it, so tapping the arrow opens the table too.
This is the old card's trick, kept, because it is what makes a row with a link and a button both
tappable at 44px.

The `<li>` wears the shared `.tap-row` class (`shared/tapRow.css`), which supplies the layout, the 44px
minimum and the hairline. That recipe's hover and focus rules are written for an `<a>` or `<button>`
that holds the row's whole content, which this link can't (it can't hold the bell), so the `<li>`
carries the hover and the link's `::after` carries the focus ring.

### Why an arrow and not a caret

The item says "caret", but `RowMark` has two marks with two meanings: a filled caret that expands in
place, and a stroked arrow that opens something elsewhere. A row that opens a table is the arrow, and
that is the chevron the mockup draws.

## How it was verified

All against this worktree's own build, on a private database (`uneasy_claude_redesign_profile_page_0d9658`)
and a private server on :8192. Both were dropped/stopped afterwards.

- `make check-fast`: passes (Go vet, lint, deadcode and unit tests, svelte-check, 951 Vitest tests,
  guard hook tests, tracker check). `deadcode` prints `middleware/auth.go: unreachable func:
  AccountContext`, which is old and not in this PR; it exits 0.
- **Did not run:** `make test-integration`. No Go changed.
- New unit tests: `tableGroups.test.ts` (6) and `components/profile/TableRow.test.ts` (17, rendered
  through `svelte/server`, including that the row has no box and no join code), plus the changed
  label in `tableBell.test.ts`.
- New e2e spec `profile_tables.spec.ts` (7) at 375px against real seeded games: grouping, an empty
  group's heading, title and players and no join code, "N new" on the right row only, the bell
  not opening the table while the rest of the row does, Join grey until a code is typed, no
  sideways scroll at 344px. **The Finished group is only tested on a rewritten response:** the dev
  API can't finish a game, so the spec seeds a live game and rewrites its entry in the list
  response to `ended` in the browser. A genuinely finished game was never driven through the page.
  (The "Join is grey until a code is typed" test guards behaviour that was already on `main`, so
  it is a regression guard for this rewrite, not evidence of something new.)
- Existing specs that touch the profile still pass: `smoke`, `page_cache` (its selector moved from
  `a.table-card` to `a.table-link`), `back_dismiss`, `player_join`, `styleguide`.
- **Each new test bites.** Mutating the code made the matching test fail, one mutation at a time:
  finished counted as your move; empty groups kept; group order reversed; a waiting-on-no-one table
  dropped; join code shown; unread shown on a finished table; clock on a finished table; clock
  always grey; "99+" cap removed; a border and radius on the row; a 36px bell; the bell's label
  back to the join code; Create made main; and the bell unpositioned (under the overlay). (Join
  greyed until a code is typed was already on `main`; making it never disabled does fail the e2e
  test, but that guards existing behaviour, not something this PR adds.) One mutation survived: removing only `z-index: 1` from the bell changes nothing, since
  a positioned element later in the page already paints above an earlier one. It stays as the
  explicit statement of intent; removing `position` as well fails the e2e test.
- Looked at 375px and 1280px against the dev data, and at 344px with a long title, "99+ new" and a
  very long name (no overflow). Keyboard: the focus ring traces the whole row, and the bell takes
  focus after its link.

## Behaviour changes to look at before merging

- **Other buttons are secondary now** (Edit, Turn on/off, Send feedback), and the Edit buttons are
  44px, not 36px. The item only named Join and Create; this leaves Join as the only filled button. Asked
  in `questions/profile-other-buttons.md`.
- **One column at every width.** The old page widened to two phone-widths and laid cards two
  across at 790px+. Rows read top to bottom, so they stay in one 440px column. Asked in
  `questions/profile-desktop-width.md`.
- **Gutter 16px**, as in the mockup (it was 12px, matching the phase views' inner gutter).
- **A success notice** ("Password updated.") is a `Notice` with a tick, not a bare gold line.
- **The bell's accessible name** names the table by its title now.
- The old card's *online* state was already undrawn; unchanged.

## Open questions for the owner

- `docs/work/questions/profile-desktop-width.md`
- `docs/work/questions/profile-other-buttons.md`
- `docs/work/questions/profile-finished-unread.md`

## Noticed, not fixed

- `docs/STYLE_GUIDE.md` "State channels on a player row" still says the profile's table-card pills
  "follow in their own pieces". That section is rewritten by the pieces for the header chips and
  the profile, and the shell piece is editing the same paragraph, so left alone to avoid a conflict.
  `TableRoster.svelte`'s header comment also says "the profile page's table-card pills".
- `app.css` token needed: none. The page used only existing tokens. (`--text-small` stands in for the
  wordmark's "the", which the mockup sets at 0.6em, about 13px, a size the scale doesn't have.)
- `frontend/src/routes/+layout.svelte`'s `main.flush` comment still mentions the profile's "888
  two-up cap", and its global `button`/`input` rules are untokenised. The shell piece owns that
  file; the profile overrides the input rules locally.
- `frontend/tests/e2e/page_cache.spec.ts` ends by asserting the *table page's* `.phase-badge`. That
  belongs to the shell piece, which removes the badge from the table header; its spec will need the
  new title bar's selector.
- A finished table hides its unread count, as the old card did; the item doesn't say to. Asked in
  `questions/profile-finished-unread.md`.
