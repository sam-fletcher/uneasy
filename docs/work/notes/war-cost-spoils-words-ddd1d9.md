# Make War: the cost is paid, and a surrender's spoils are taken

## What changed and why

The owner ruled on 2026-11-26 (RULINGS, "The cost of battle is paid, and nobody is owed it"):
the cost of battle is paid from the payer's own assets, once per opponent, and nobody is owed
it; a surrender's opponents each take one asset. The app said "owed" and "claim". This PR
changes the words a player reads and nothing else: identifiers, CSS classes and API names keep
their names.

- `CostOfBattle.svelte`: the list label is "To pay this row, in order"; its EndsNote now reads
  "Play goes on once every battle cost is paid and every surrender's spoils are taken." Its
  comments say "to pay" and "each payment is against", not "owed" or "owe", so the old word does
  not get copied back.
- `SurrenderClaims.svelte`: the section is "Spoils of war"; the button is "Take"; the empty
  message is "No assets left to take."; the error is "Could not take the asset."; its EndsNote
  matches the cost list's.
- `BattleCostForm.svelte`: the surrender toggle's description reads "After this payment you
  surrender: each opponent who hasn't surrendered takes one asset of their choice from you."
- `handler/plan_make_war_peace.go`: the rejected-peace log line reads "…the cost of battle still
  has to be paid."
- `phases/MainEventView.svelte`: the war-gate comment's blocked-row line reads "…blocked on a
  cost of battle, or on spoils of war still to be taken." (It sits in an HTML comment, so no
  player sees it; changed because the item lists it.)
- **Waiting On step label** (`waitingOn.ts`): `'surrender-asset claims'` is now
  `'take the spoils of war'`, matching `'pay the cost of battle'` above it. The bar reads
  "Waiting on alice. Take the spoils of war". Tests: `waitingOn.test.ts`,
  `MainEventView.test.ts`, and the `stepHeading` checks in `make_war.spec.ts`.
- **Joining note** (`WarStatus.svelte`): "joining, owes entry" is now "joining, entry to pay",
  because joining partway through means paying the cost of battle to enter, and the ruling's
  "paid, never owed" reaches this line. Tests: `WarStatus.test.ts`, `make_war.spec.ts`, and the
  style guide's example of the member lines (`docs/STYLE_GUIDE.md`), which quoted the old wording.
- Tests: `make_war.spec.ts` reads the new strings and selects the button by `exact: true` name
  "Take", and the section heading "Spoils of war" by an exact text match (the step label
  "Take the spoils of war" also contains it); `EndsNote.test.ts` expects the new EndsNote text.

## How it was verified

Against this branch's own code, on a private server and the private database
`uneasy_claude_war_cost_spoils_words_ddd1d9` (dropped afterwards). The server ran on `:8194`:
`:8192` was another session's server, and I did not stop it.

- `make check-fast`: passed after the Waiting On, joining-note and comment changes (vet,
  deadcode, Go unit, svelte-check, Vitest including `waitingOn` and `WarStatus`, hooks,
  tracker-check).
- `make test-integration`: passed on the private database (every package `ok`), from the first
  pass; the second pass changed only frontend strings and comments, so the Go side is unchanged.
- `frontend/tests/e2e/make_war.spec.ts`: 24 of 24 passed on the final run.
- A throwaway spec (not committed) took the 375px screenshots attached to the PR. They show
  "To pay this row, in order" and "Spoils of war", which did not change in the second pass; they
  do not show the Waiting On bar's new label, which no screenshot covers.
- No new tests were added: this change moves copy only, so there was nothing to make fail first.
- **Not run:** the Waiting On bar specs outside Make War; `make test-e2e` at its default port
  (that would test `main`).

## Behaviour changes to look at before merging

Copy only. No logic, routes, payloads or states change. Two sentences shift meaning:

- The surrender toggle's description now says the surrender takes effect "after this payment" and
  that each opponent "takes" an asset, where it used to say "gets to claim".
- The Waiting On bar now says "Take the spoils of war" where it said "Surrender-asset claims".

## Open questions for the owner

None open from this PR. The joining-note wording reads the ruling as covering entry payments too;
if the owner means only the cost of battle, the line goes back.

## Noticed, not fixed

- **Dev styleguide** (`frontend/src/routes/dev/styleguide/+page.svelte`): a cell label "a claim
  left by a war that has just ended" (~line 1227), and a comment "still owes entry" (~line 230).
  Dev page only.
- **Remaining grep hits**, none player-read: test names "still owed in words" (`WarStatus.test.ts`
  ~82), "the surrender claims" (`waitingOn.test.ts` ~274), "owed per opponent" (`make_war.spec.ts`
  ~224), "claims only what is true" (`EndsNote.test.ts`); a comment "owed and the payer's form"
  was reworded. Left as they are: they are test names, not in the item's list.
- The item file `docs/work/next/war-cost-and-spoils-words.md` lives only on PR #141's branch, not
  on `main` or this branch, so this PR does not delete it. Once #141 merges, rebase onto `main`
  and delete it (see the PR body).
