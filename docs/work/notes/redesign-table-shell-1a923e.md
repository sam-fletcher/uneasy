# redesign-table-shell-1a923e

The frame of the table page, rebuilt at every width. Wave 3 of the visual redesign. The panes'
own contents (Public Record, Play, Chat) moved in as they were; the later pieces restyle them.

## What changed and why

The table page is a **title bar**, a **player row** (both the full width) and three **panes**,
Public Record, Play and Chat. A wide window shows them as columns; a narrow one swaps them with tabs:

| width | what shows |
|---|---|
| under 744 | one pane at a time, a three-tab bar at the foot, opening on Play |
| 744 to 1111 | Chat docked on the right; the left column is Public Record or Play, with two tabs at its foot |
| 1112 and up | three columns, no tabs, each starting with its name |

**New files** (`frontend/src/lib/components/table/` unless noted):

| file | job |
|---|---|
| `TableTitleBar` | Home, the title, Help. The title is `tableTitle()` fed by `rowState.title_plan_type` |
| `PlayerRow` | a shape and a name per player, one line that scrolls sideways; the clock on whoever the game waits on (gold on you); a warning mark, no number, for at-risk assets |
| `ShellColumn` | a pane's column. A pane that isn't showing is `visibility: hidden`, not unmounted, so the chat keeps its place. It provides itself as the thing a sheet covers |
| `PaneTabs`, `RecordTabs` | the tab bar, and the Events / Laws / Rumors chips at the foot of the record |
| `PaneName`, `PaneMarks` | a column's name in small capitals, with the same marks as its tab |
| `panes.ts` | pure: which panes show, which tabs, which marks. Unit-tested |
| `TonesSheet` | the Tones sheet, moved out of the page as it was |
| `WarEntry` | the stopgap: the old War button, now at the top of Play |
| `lib/tableLayout.svelte.ts` | the one place the breakpoints are read (`mode`: phone, two, three) |
| `lib/sheetHost.ts` | context that says which column a sheet covers |

**Marks.** The gold clock on the Play tab/name means it is *your* move, and only yours. The pale
dot is "new since you looked": on Chat (unread messages) and on Public Record (whenever Laws or
Rumors has something new, even while you're on it). Marks carry no counts.

**Laws and Rumors "last seen"** (the client half of #76, which left it unconsumed): the page holds
the server's two markers (`laws_rumors_seen` on `/state`) and lights a chip's dot while its list
has an entry above the marker. Opening a list with the record on screen reports it
(`PUT .../seen/laws|rumors`, monotonic, once per newest rev). **An edit counts as new**, and your
own writing doesn't: the server moves the writer's marker with the write, but the client only hears
about the entry, so when a list gains something above my marker the page asks for the marker again
after half a second (once per newest rev, so someone else's entry can't start a loop).

**Sheets open over their column.** `RetinueSheet` finds the column through context, measures it
before it first renders, and covers only that: Tones over Chat, the war's panel over Play, and Help or a
player's retinue over Play (or the left column when there are two). On a phone, one column, so the
screen. With no column (profile, landing) it is the old viewport sheet, a centred dialog from 744
instead of 790. It is drawn at the end of `<body>` ("Behaviour changes" has why that matters).

**Files outside my list I had to touch, kept small** (later pieces restyle them):
- `ChatPanel.svelte`: the strip, scrim, expanded sheet, `isDesktop` and Back handling are gone; it
  fills its column and takes `visible` (is the pane on screen: gates the read marker and the
  landing on the unread divider), `showName`, and a stopgap header with the **"Tones · N set" chip**
  the redesign puts there plus the old bookkeeping toggle. The Chat piece owns the header from here.
- `PublicRecord.svelte`: the rail and the overlay are gone; the row list is a pane that fills its
  column. Row content is untouched. It now renders in every phase (rows 1 to 13 blank in the
  lobby and Prologue), since all three tabs show everywhere.
- `RetinueSheet.svelte`, `shared/modalShell.css`: the 790 query became 744, and the sheet's
  placement (above).
- `HelpContent.svelte`: the Rankings tab's picture of the old chip is now the real new chip (real
  `chip.css`, `PlayerShape`, `ClockMark`), with new copy; the Public Record tab says where Laws and
  Rumors went; the panel's phone query is 743.
- `lib/README.md`, `docs/STYLE_GUIDE.md` (Layout widths, State channels, the shared-components table).

**Deleted:** the rail, the chat strip, the record-phase width tokens, the header's phase badge and
Tones / Laws / Rumors / War buttons, the Laws and Rumors sheets, the rank line on the chips and
`shared/rankChip.css` (nothing drew it any more).

**Migrated off `tokens: not migrated`:** the page, `HelpButton`, `shared/rankStrip.css`. New files
are on the tokens from the start, including `TonesSheet` (its sizes now use the type, gap and
corner scales; its look is otherwise the old one).

**Layout tokens in `app.css`**: `--layout-column-min/-max`, `--layout-title-bar`,
`--layout-column-head`, `--layout-tab-bar`. They are `--layout-*`, not `--size-*`, because the
styleguide test requires every `--size-*` token to appear on `/dev/styleguide`.

## How it was verified

Against this worktree's code (a private server on `:8175`, a private database):

- `make check-fast`: passes. 955 unit tests, `svelte-check` clean apart from the `.type-note`
  warning in `HelpContent.svelte` that was there before, `go vet`, the Go unit tests, the git-guard
  hook tests and the tracker check.
- **e2e: the full suite passes, 173 tests**: the 163 that were there plus 10 new ones in
  `table_shell.spec.ts`, after the specs the layout broke were updated (below). An earlier full run
  failed nine: seven because my player chip reused the class `.player-chip` that `PlayerChips` owns
  (specs find chips by it; renamed `.row-chip`), and two because my first `openPane` helper looked
  for a tab before the page had loaded. The next full run passed, and the last one is on the
  final build.
- **New unit tests** (`panes.test.ts`, `shell.test.ts`, and the layout guard in
  `layoutTokens.test.ts`), each shown to bite by undoing its rule: two columns not showing Chat, a
  phone Chat choice not falling back to Play, a tab bar at three columns, Play lit by anything but
  your move, a count printed in the at-risk mark, the clock gold for everyone, a dot on Events, a
  tab losing `aria-current`, a hidden pane losing its marker, the two-column threshold off by one,
  an old 790 literal in a component, a breakpoint constant that no longer fits two or three
  columns, and `app.css`'s column band drifting from the constants.
- **New e2e** (`table_shell.spec.ts`): the header at 375; the clock and "You"; the dots and their
  clearing, for good, across a reload; an edit is new to everyone but the editor, live; a record
  line opens Chat on a phone; two columns (no Chat tab, Help covers only the left column); three
  columns (names, marks, Tones covering only Chat, a retinue covering only Play); the war entry at
  the top of Play and its drawer as a sheet over Play, at 375 and 1280; and a sheet opening while
  its pane is another tab (shown to fail without the portal).
- **Looked at** at 375, 344, 744, 1112 and 1280: the lobby, Prologue closing, a main-event row, the
  Shake-Up, five players with a long name, and every sheet.

Updated specs: `back_dismiss` (rewritten: a tab is not a history entry; sheets are), `chat`
(`.strip` becomes the Chat tab; the unread badge becomes a dot that clears), and the `phase-badge`,
`Open tones` and `Open laws` locators in `main_event_seed`, `page_cache`, `start_prologue`,
`prologue_claim`, `propose_decree` and `tone_coalescing`. `helpers.ts` gains `openPane`,
`openRecordTab`, `openTones` and `tableTitle`. `openPane` decides from the viewport width, so one
spec reads the same at a phone's width and a desktop's.

**Not run against this code:** `make test-integration` (no Go changed); a real phone or a real
iPad (the viewports are Chromium's); Safari or Firefox; hover and focus states (described in CSS,
not photographed); the war's panel *content* (unchanged, only its frame moved); the Make Demands
overlay and the other plan flows beyond what the full e2e suite covers.

## Behaviour changes to look at before merging

- **Everything in the header is gone but Home, the title and Help.** The phase badge, Tones, Laws,
  Rumors and War. Tones is the chip in the chat header, Laws and Rumors are chips at the foot of
  the Public Record, War is a button at the top of Play (a stopgap).
- **The player chip lost its rank line and its gold ring**, and its at-risk number became a red
  warning mark. Both are in `docs/work/questions/player-row-details.md`.
- **Back no longer dismisses the chat or the Public Record**: they are panes now, not overlays. It
  still dismisses sheets. Choosing a tab adds no history entry.
- **Play is wider on a phone.** The rail and its 44px are gone, so at 375 the Play column is 375,
  not 331, and the views' own padding is unchanged. A plan panel that was tuned to the old 300 to
  380 box now sits in 360 to 440.
- **A sheet is no longer a centred dialog on desktop.** On the table page it is as wide as a column
  and covers one. Elsewhere (profile, landing) it is still centred, from 744 instead of 790.
- **The Public Record shows in the lobby and Prologue** as thirteen blank rows, and the Laws and
  Rumors lists are reachable there. The Prologue's Laws and Rumors checklist rows now open those
  lists rather than a sheet.
- **Tabs move keyboard focus.** The tab bar is drawn in the foot of the pane that is showing, so
  the pressed tab is gone once the pane changes; the page puts focus on the same tab in the new
  bar (an e2e fails without it).
- **A sheet is drawn at the end of `<body>`.** Without that, the Tones sheet that the lobby's
  checklist opens while a phone shows Play would be invisible: it is declared in the Chat column,
  which is hidden, and visibility is inherited.
- **Player-row chips centre from 744 up** (the mockups left-align); on a phone they start at the
  left and scroll, as drawn. Also in the questions file.
- **The retinue sheet and Help are 360 wide on a tablet**, not 440, as they are as wide as a column.

## Open questions for the owner

- [player-row-details](../questions/player-row-details.md): the rank line, the at-risk mark's
  drawing and colour, "You" against the name, and centring.
- [tones-chip-wording](../questions/tones-chip-wording.md): "Tones · 0 set" until something is set.

## Noticed, not fixed

- **The Laws and Rumors entries' own "new" dots** are the Public Record piece's. The page reports a
  list as seen the moment it is on screen, so `lawsSeenRev` / `rumorsSeenRev` have already moved by
  the time the list renders. That piece needs the marker *as it was when the list opened* to keep an
  entry's dot while you read it (chat's unread divider does the same). It can capture
  `lawsSeenRev` in the page before the report effect runs, or have the page pass it down at open.
  `lawsRumorsSeen.ts` already says so.
- **`layoutTokens.test.ts` has a transitional allowance** for `routes/profile/+page.svelte`'s old
  790 query (`PENDING_MIGRATION`), so this PR doesn't edit a file the profile piece is rewriting. It
  stays harmless once the file stops using 790; the cleanup piece can delete the map. If the
  profile piece ends up with a 744 query it needs no change.
- **`ClaimChoiceModal`** (the Prologue's claim step) is still a page-level full-screen sheet that
  becomes a centred card from 744. It is the Prologue piece's, and may want to be a column's sheet.
- **`+layout.svelte`'s comments** still say "dock" and 790 / 1070 (comments only; the pages piece).
- **`PhaseBadge`** has no table-page user now, only the profile; the cleanup piece deletes it with
  the profile's.
- **`ErrorText` under the player row** gets the page's inline padding via a `:global(.error)` rule
  in the page; the pages piece owns the component.
- `svelte-check` still warns about `.type-note` in `HelpContent.svelte` (there before this PR).
- **The at-risk triangle in `PlayerRow.svelte` is a copy of `Notice`'s warning mark** (the same
  path; the comment says so). The retinue piece is lifting `Notice`'s marks into a shared
  component. **Follow-up: when that lands, `PlayerRow` imports it and the copy goes.**
- **`WarEntry`'s red and orange fills are a stopgap.** It is the old War button moved to the top of
  Play unchanged; the Play and war pieces replace it with the one-line band and delete the file.
- **The Laws and Rumors dots don't update on a second device.** "Seen" is saved on the server but
  isn't pushed over the WebSocket, so another tab or device keeps its dot until it reloads or
  reconnects. Chat's read marker behaves the same way. Tracked in
  `docs/work/next/laws-rumors-seen-across-devices.md`.
