# redesign-war-7d6a89

The war band and the war's panel, rebuilt. Wave 5 of the visual redesign. A war no longer has a red "War"
button and a drawer: each live war has a one-line band at the top of Play, and pressing it opens the war's
panel as a sheet over Play, with the sides named after their leaders and joining in two steps.

## What changed and why

**1. A band for each live war** (`plans/war/WarBand.svelte`, `WarBands.svelte`). One 44px line at the top of
Play, above the open step's heading: the barbute, "otto’s war on Wren" in war red, "· under way" (or
"planned"), and the arrow that says it opens a sheet. Red is on the border and the name only, and the
name spells out "war", so the red is never the only cue. `WarBands` keeps which sheet is open as a *plan
id*, looked up in the live list each time, and clears it (`keepOpenWar`) when its war leaves the list:
the war list is emptied whenever its fetch fails, and without clearing, the next good fetch would find the
id again and reopen the sheet unasked. "…and others" is added when the declarer named more than one
enemy, and two wars at once are two bands. The wording is in `lib/warText.ts` (pure, tested).

**2. The war's panel is a sheet** (`WarSheet.svelte`). The shell's `RetinueSheet` (so Escape, the backdrop
and phone Back all close it, and from two columns up it covers only the Play column), with a title in war's
colour and the barbute, one line on when the war began and what being in it costs, and then the existing panel.

**3. Sides headed by their leaders; joining is a pick and then a commit** (`war/WarStatus.svelte`). The two sides are
"otto’s side" and "Wren’s side", listed as plain lines with each player's shape (nothing in them is pressed, so
nothing is boxed). A player outside the war gets three chips, "Join otto", "Join Wren", "Stay out", with no meaning
colour; a chip sends nothing, it shows its cost under the chips in ordinary secondary text, and the one main button,
labelled with what it will do ("Join otto’s side"), sends it. Until something is picked the button is greyed out
("Choose an option"). While the delay vote is open a join is free and puts you in the vote; from the moment it
closes, including the rows when the war is only planned, it owes an entry payment per opponent already in, named.
Both are priced from the same rule the server uses (`entryOpponents`).

**4. The panel's other parts moved onto the recipes** (`CostOfBattle`, `BattleCostForm`, `PeaceFlow`,
`SurrenderClaims`). The "How will you pay?" options are `<Chip>`s; Reject is a secondary button (it had no style of
its own); what is owed is a list of lines with shapes. The declarer's words are a finished-step line (`FinishedStep`),
as in every other plan. The panel draws no card and no title: the war is named by its band, by the sheet's title, or by
`war/WarName.svelte` where neither is on screen (the delay vote, a surrender claim left by an ended war, each panel when
two wars owe at once). The page's heading stays the only one (`MainEventView.test.ts` still passes).

**5. The stopgap and the drawer are gone.** `table/WarEntry.svelte`, its mount in `+page.svelte` and `lib/warDrawer.ts`
(`warDrawerOpen`, the two counts) are deleted. Nothing in `ws.ts` or `ws-handlers.ts` imported the drawer (they only name
the `war.entry_completed` event), so there was nothing to move.

**6. A peace vote is headed as a vote, whichever war the server names.** The row state while a peace proposal is open is
still `await_battle_cost`, and the step label is read off the kind alone, so the voters' heading said "Your move — pay the
cost of battle". Two wars can owe on one row; the server names one in `war_id` (picked from a Go map, so not a stable one)
while `acting_player_ids` covers both, so reading the war the row names made everyone's heading flip between two answers.
`peaceVoteLabel` (`lib/warGate.ts`) therefore ignores `war_id` and reads every war that is blocking the row: no proposal
open anywhere, the payment label stands; every blocking war at a vote, "vote on the peace proposal" for all; a mix, each
player gets their own business (a voter votes, the payer pays) and a player who is neither, or both, gets "settle the costs
of war". `lib/waitingOn.ts` is frozen this wave, so the fix is outside it (see below).

**7. During a takeover the sheet does not repeat the form.** While a war owes a cost or a claim, the play area shows its
panel, and the sheet opened from the band held a second live copy of the payment form, with the heading that says what is
asked hidden under it. I chose to point the sheet at the play area rather than give it an ask of its own: `WarBands` tells
the sheet which wars the play area is settling (`takeoverPlanIDs`), `WarSheet` passes that to the panel through a small
context (`war/sheetContext.ts`), and the panel then drops the payment form, the owed list, the vote and the claim form and
says "This war is holding up the row. Close this sheet to settle it in the play area." The sides, joining and a late
joiner's entry payment stay, since none of them is the takeover's business. The alternative (a one-line ask in the sheet)
would have had to repeat what the heading says, in a place where a player can't see the heading.

**Markers removed** (`tokens: not migrated`): `WarStatus`, `BattleCostForm`, `SurrenderClaims`. No tokens were added to
`app.css`. A "The war band" section is on `/dev/styleguide` (the real band, the real sheet over a fixture, the real
sides and join block), and a subsection of `docs/STYLE_GUIDE.md` sits before "The styleguide".

**Review round (this section was added when the first review came back):** the sheet could reopen by itself after a failed
war fetch (fixed, with a test of the rule and an e2e that fails the fetch); the peace label depended on a war the server
picks at random (fixed, see 6); "Joining is free until the war begins" read as free through the whole delay and the planned
rows (now "free while the war’s delay is still being voted"); the comments, the style guide and this note no longer track the
private wording; the "and others" e2e now tests what its name says, split into a still-open vote, two wars, a war ending, and
the sheet during a takeover; two source-text tests are replaced by pure ones.

**Files outside my list, kept small:** `tests/e2e/table_shell.spec.ts` (one test asserted the stopgap button and the
"Active Wars" title; it now asserts the band and the sheet), and the stale `WarEntry` mention in the style guide's shared
components table. `WarSheet` takes an optional body snippet that the app never passes, so the style guide page can show the
real sheet without fetching a game.

## How it was verified

Against this branch's own code (a private server on `:8147`, a private database, since dropped):

- **`make check-fast`:** passes (80 test files, 1338 tests, `svelte-check` clean, the guard test with the three
  `tokens: not migrated` markers gone, the git-guard tests, the tracker check).
- **e2e: the full suite, 209 specs, all pass** on the rebased build, after the review round too. The Make War specs read
  the old wording ("Side 1 — Declarer", "Join Side 1", "It is YOUR turn to pay cost.") and are rewritten. New tests:
  the band and the sheet (a bystander picks a side, reads the cost, joins and ends up owing entry; phone Back closes the
  sheet); a war still in its delay vote (named, no band, then the band appears when the last face closes it); two wars at
  once (a band each, each opening its own); the sheet while a cost is owed (the line, not a second form); and a war ending,
  or the war list failing, under an open sheet (the sheet closes and does not reopen). The peace test now also expects the
  vote heading.
- **Every new test bites.** Unit: 27 mutations in all, each turned red and restored (the side label, the entry rule, the lede's
  row, "and others", the free-join copy and its condition, the band's border, fill, arrow and height, bands with no war, the
  join block shown to a participant, the main button enabled with nothing picked, the peace label reading the war the row
  names, for a voter, for a payer, for a bystander, when all wars are at a vote, the sheet id not cleared, the takeover name
  too eager or too shy). e2e: one build with four mutations at once (the sheet id not cleared, no war name over the vote,
  only one band drawn, the sheet repeating the form), and each of the four new tests failed on its own assertion ("the sheet
  reopened by itself", the name not found, two bands expected and one found, the holding-up line not found).
- **Looked at, at 375px as the player in the war and as a bystander:** the band; the sheet with nothing picked, with a side
  picked (cost showing, main button naming it) and after Stay out; a war planned and under way; "and others"; two wars at once;
  the cost of battle as the payer and as a bystander; the delay vote; a peace vote; the sheet opened during a takeover; the sheet
  at 1112px (it covers only Play). Before and after pairs are in the PR.
- **Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari or Firefox; hover and focus
  states (described in CSS, not photographed); 344px and the two-column width; a surrender claim and a late joiner's entry
  payment as screenshots (both are covered by e2e specs, and I looked at the peace vote and the cost form, not at those two);
  five-player tables; two wars owing a cost at the same moment (the unit tests cover the label and the names; no e2e builds it).

## Behaviour changes to look at before merging

- **The War button and the drawer are gone.** A band for each war replaces them; the drawer listed every war in one sheet,
  and now each band opens its own war.
- **Joining is two taps** (pick, then the main button), as ruled. One tap used to join.
- **"Stay out" is still changeable on screen** (the chips stay, "Stay out" picked). Before, it collapsed the choices until a
  reload. This follows the sheet's own copy ("You can still join later"); see the question file.
- **The panel says less:** the "who pays" lines that repeat the heading, the "Your cost of battle" title, the "War — ongoing /
  Make War by otto" badge line and "No outstanding cost-of-battle this row." are gone. What is owed is a short list.
- **A peace vote's heading says "vote on the peace proposal"** (it said to pay), and "settle the costs of war" for a player
  waiting on a vote and a payment at once.
- **The sheet opened during a takeover** shows the sides and a line pointing to the play area, not a second payment form.
- **Wording:** "Side 1 — Declarer" is "otto’s side"; "(joining — owes entry)" is "joining, owes entry"; "(none yet)" is "No one yet".

## Open questions for the owner

- [war-stay-out-reversible](../questions/war-stay-out-reversible.md): should "Stay out" stay changeable on screen?
- [war-takeover-layout](../questions/war-takeover-layout.md): is "against" the right word in the owed-costs list?

## Noticed, not fixed

- **The row state can't say what a player is waiting for when two wars owe at once.** `lib/waitingOn.ts` (frozen this wave)
  reads the step label off the kind alone, and the server's `await_battle_cost` names one `war_id` picked from a Go map
  (`firstKey(outstanding)` in `handler/row_state.go`), so the client works the label out from the war list itself
  (`peaceVoteLabel`). Follow-up, Go-side, not done: have the row state say it, as an `await_peace_vote` kind or a per-war
  list of who is acting on what, and then drop the client's reading of the war list.
- **`DelayReveal` still says "Other players may join either side below."** and keeps its own "Vote for the war's delay" line
  under a heading that already says "the delay reveal". Same family as `plan-step-labels`.
- **`BattleCostForm` still picks the opponent with `PlayerChips`** (the interim `.player-chip` look from `planPanel.css`), and
  the picker for assets is `CardPicker`; both are shared with other panels and move in their pieces.
- **The war log's wording:** the server's join line says "joined the war on the declarers' side" / "the enemies' side"
  (`mwSideLabel`), which is the old declarer-and-enemies naming, and is now the one place left that doesn't name a side after
  its leader. Go-side, so not mine.
- **`PrepForm` (declaring a war)** was already on the recipes and is unchanged.
- **`WarSheet`'s `children` snippet exists only so the style guide page can show the real sheet** over a fixture; the app never passes it.
- **`.war-sheet :global(.chip)` and `.join-chips :global(.chip)` restyle the shared `Chip` from outside.** A prop or a context on `Chip` for "on a raised surface" would be cleaner, and would serve the other sheets too.
- **The shape-and-name line is drawn twice** (the `person` snippet in `CostOfBattle`, the member line in `WarStatus`); a shared piece would do, but this wave doesn't allow a new one.
