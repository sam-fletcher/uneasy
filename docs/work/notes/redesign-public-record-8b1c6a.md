# redesign-public-record-8b1c6a

The Public Record pane's contents, rebuilt to the signed-off mockups. Wave 4 of the visual redesign
(the frame around it, the tabs and the chips, merged in #81).

## What changed and why

**Events** (`PublicRecord.svelte`)

- **A line per plan or scene, drawn as a tappable row** (`tapRow.css`, 44px): the preparer's or
  author's shape, the name, at most one status word, then the arrow. It still opens Chat at that
  point through the callbacks the page already passed (`onPlanJump`, `onSceneJump`).
- **The end mark is the arrow, not the caret.** The tracker item says "caret", but `RowMark` keeps
  the caret for rows that expand in place and the arrow for rows that open somewhere else, and the
  mockups draw the arrow. A line opens Chat, so it is the arrow.
- **Row numbers and empty rows can't be tapped.** An empty row is only its number (no dash).
  The number's "Row " is spoken for a screen reader.
- **The current row** has a deep gold number and a 2px deep gold bar down its left edge (an inset
  shadow, no fill). Every other number is faint. A row the plan picker points at takes the info blue
  for the same two marks.
- **The status word** is decided in `lib/recordLines.ts` (`planStatusWord`): "Resolving" (plain),
  Make (green), Mar (red), "No effect" (plain: a resolved plan with a NULL result, which the server
  keeps for a plan that closed without a roll) and nothing on a future plan. **Make War and
  Clandestinely Liaise never show Make or Mar**, since the server stores "make" on both when they
  end. Pinned by unit tests and by an e2e that really resolves a war.
- **The Shake-Up row** draws the house burst (`LogMark` `shake_up`), not `✶`.
- **The ranking dividers** are drawn as the mockup shows them, a row of scallops either side of the
  podium, in the antique gold. They were a fading hairline.
- **The Prologue** (and the lobby, and tone setting) shows the same thirteen blank rows under one
  line saying when play starts.

**Laws and Rumors** (`LawsRumors.svelte`)

- Plain entries separated by hairlines, not tinted cards. Each list opens with its total in words
  ("2 laws"). Text at body size; a law's addendum under it in italics.
- **Bylines carry the shape** of each name. A hidden source reads "Unknown" in faint italics with no
  shape. A rumor gets an **"About" line** (the asset's name and the shape of its owner now), which the
  old list didn't have and the mockup does; it is left out when a rumor names no asset.
- **The "new" dot** (`NewDot`) starts an entry above the viewer's marker, from #76's `rev` and
  `seen_rev`. The page reports a list as seen the moment it is on screen, which moves its marker, so
  an entry would lose its dot the instant it was drawn. This component takes the marker **as it was
  when the list opened** and draws dots from that: see "How the dot works" below.
- **Edit** is a bordered 44px button (`action-btn secondary`) on your own entries.

**New file:** `lib/recordLines.ts` (+ test): the status word and the "Scene:" prefix, pure.

**The page** (`routes/table/[id]/+page.svelte`), kept small:

- removed `onRowJump={jumpToRow}` and the `jumpToRow` function (about ten lines): row numbers are
  no longer tappable, so nothing calls it;
- passes `{assets}`, `seenLaws={lawsSeenRev}` and `seenRumors={rumorsSeenRev}` to `LawsRumors`
  (three lines): for the "About" line and the dots;
- a comment says "plan or scene line" where it said "row/plan/scene".

**Specs outside my own**, because they tapped a row number and that no longer jumps:
`chat.spec.ts` (the history-mode jump now taps a plan line, after preparing a plan so there is a
`plan.prepared` post to land on) and `table_shell.spec.ts` (the phone jump test, likewise).

**Docs:** a "The Public Record's lists" section in `STYLE_GUIDE.md`, and one line in
`lib/README.md`. Both files' `tokens: not migrated` markers are gone.

### How the dot works

- `dotsFrom` is the marker for the list on screen. It is read from the prop when the component
  first renders, again when the pane switches between Laws and Rumors (an `$effect.pre`, so a switch
  never paints one list against the other's marker), and, if the marker hadn't loaded yet, as soon as
  it does. It never follows the prop afterwards. Leaving the list and coming back remounts it against
  the newer marker, so a seen entry has lost its dot.
- An entry is new when `rev > dotsFrom` (`isNewEntry`, unchanged) **and** it is not one this viewer
  just wrote through the Edit button. An edit draws a new revision, which is above the marker the list
  opened with, so without that clause you would see a dot on your own edit. The component remembers
  the revision the PATCH returned.

## How it was verified

Against this worktree's code: a private server on `:8164` (built with `make build`) and a private
database.

- `make check-fast`: passes (Go vet and lint and unit tests, `svelte-check` clean apart from the
  `.type-note` warning in `HelpContent.svelte` that was there before, the full Vitest suite, the
  git-guard hook tests and the tracker check). The token guard passes with both markers removed.
- **New unit tests** (`recordLines.test.ts`, `PublicRecord.test.ts`, `LawsRumors.test.ts`), each
  shown to bite by undoing its rule and watching it fail: the war/liaison exemption (7 tests fail
  without it), a row number turned into a button, the caret in place of the arrow, the typed star
  back on the Shake-Up row, the pips always drawn, the Prologue note removed, a fill on the current
  row, the current number not gold, the scene mark dropped, the dot at-or-below the marker, a shape on
  "Unknown", Edit for everyone, entries turned back into tinted cards, one list reading the other's
  marker, the About line without the owner's shape, Edit back to a text link. (One mutant, Edit for
  everyone on a law, survived my first tests; I added the missing test and it now fails.)
- **New e2e** (`public_record.spec.ts`, 8 tests, all against `:8164`): a plan is a 44px line with an
  arrow and a row number or empty row does nothing; **a war the server really stored as "make" shows
  no Make** (a seeded `make_war` is resolved through `/api/plans/{id}/resolve`, and the test reads the
  stored `result` back first); the plan being played says "Resolving"; the Prologue shows thirteen
  blank rows and the note; the note is gone once play starts; a dot survives the page marking the
  list seen and is gone on return and after a reload; your own edit is not new to you, only the writer
  gets a 44px bordered Edit, and a bystander sees the dot; Rumors are plain entries. Shown to bite by
  building with the war exemption removed (the war test fails), with the own-edit clause removed (the
  own-edit test fails), and with the marker following the prop (the dot tests fail).
- **Full e2e suite against `:8164`**: **192 passed**, one worker, with the 8 new tests and the two edited specs (`chat`, `table_shell`) in it. After the last edits (comments only) I rebuilt the committed tree and re-ran `public_record`, `table_shell`, `chat` and `propose_decree`: 38 passed. `make check-fast` ran again on the final tree: passes (68 test files, 1087 tests).
- **Looked at** at 375 (before and after, in the PR): Events in the Main Event (a resolving plan, a
  Make, a Mar, a No effect, a war line, two scenes, the current-row bar), Events in the Prologue,
  Laws with a new entry and your own Edit, Rumors with an "Unknown" source, an About line on every
  rumor, a new entry and your own Edit; also at 744 (two columns) and 1280 (three), and a scene
  summary long enough to truncate.

**Not run against this code:** `make test-integration` (no Go changed); a real phone, Safari or
Firefox (Chromium only: the scallop divider is an SVG pattern used from another `<svg>`, which I
expect to work everywhere but did not see); hover and focus states (the shared recipes, not
photographed); a table of five players with long names on the Events lines.

## Behaviour changes to look at before merging

- **A row number is no longer a button.** It used to jump Chat to the row's advance post. The only
  way into Chat from the record is now a plan or scene line, as ruled. A row with no plan or scene has
  nothing to open.
- **The Shake-Up row's pips show only once the Shake-Up is under way** (they used to show, empty and
  faded, from the start). See the question below.
- **A scene's summary is truncated to one line** with an ellipsis; the tap opens the whole thing in
  Chat.
- **The "About" line is new:** each rumor says which asset it is about, with its owner's shape.
- **The ranking divider is a scalloped line**, not a gradient hairline.
- **Edit is a button**, and it appears beside the byline, not as an underlined word.

## Open questions for the owner

- [record-war-liaise-resolving](../questions/record-war-liaise-resolving.md): does a war or liaison
  line say "Resolving" while it is being played? I show it, since it is where the table is rather
  than an outcome.
- [record-shakeup-pips](../questions/record-shakeup-pips.md): keep the Shake-Up row's E / K / P pips
  (the mockup omits them)? I kept them, only once the Shake-Up is under way.

## Noticed, not fixed

- **Stale comments** still name `onRowJump`: `lib/api/chat.ts` (the `AnchorRequest` doc comment) and
  `lib/chatFeed.ts` (the `resolveAnchor` doc comment, which says `jumpToRow(1)` falls back to the
  first post). The `row.advanced` anchor itself is still used by the chat's own row divider.
- **Your own entry written outside the Edit button shows a dot if the list is open.** The server moves
  the writer's marker, but this component can't tell that apart from the page's own report, so a
  rumor you spread while the Rumors list is open (three columns) is drawn with a dot until you leave
  and return. The tab dot is right (the page re-reads the marker). It is the edge of
  [own-writes-not-new](../questions/own-writes-not-new.md): the clean fix is for the server to say
  which entries the viewer wrote, and it stores nothing about who did.
- **`ChatPanel.svelte` and `textLimits.ts`** have comments naming the old `.row-num-pill` and
  `.entry-line` classes; both are gone.
- **`shared/tapTarget.test.ts`'s recipe list** doesn't include `PublicRecord.svelte` or
  `LawsRumors.svelte`; both use `var(--size-tap)` and my own tests check it, but the shared guard
  would catch a literal `44px` in them too.
