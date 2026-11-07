# Exchange Courtiers: choicesDone reads the committed marker

## What changed and why

`ExchangeCourtiersPanel.svelte` decided "has the make/mar been committed?" from
`make_mar_choices` being non-empty; Seek Answers, Spread Propaganda, Spread Rumors and (in #59)
Make Introductions read `choicesCommitted()` from `plans/shared.ts`. Now `choicesDone =
choicesCommitted(rd)`, reusing the `rd` the block already parses, and the unused
`existingChoices` local is gone. Nothing else in the template changed.

**Can any outcome commit with no picks?** Not through the app. While a roll exists,
`ecHandler.ValidateChoices` demands exactly one pick, so every make and mar the UI can send
carries one; the plan has no `ForcedChoice`, so `choice_auto_applied` is never set. The one
route to `choice_submitted` with empty `make_mar_choices` is a direct API call with **no roll**
(the validators return early when there is no roll). I confirmed it with a throwaway integration
probe (since deleted): a rollless empty `make` returns 200, transfers the peer and resolves the
plan in the same request. That is a separate bug, not fixed here; #61 fixes it.

**Does the swap change what a viewer sees?** No. The two checks disagree only on a committed
row with no picks, and the only such row is the one above, which is already `resolved` in the
same request, so the resolving panel is never shown for it. For every reachable state the
picks and the marker agree. Hence no screenshots: no view changes.

`EndsNote` lines are untouched and true in every state: the accept note shows only before the
roll; the picker's note only once a pick is selected; the messy-break and riposte notes only
after the commit.

## How it was verified

Against this worktree's code:

- `make check-fast`: passed (665 Vitest tests; ran after `make build`, because the Go suite
  embeds the frontend build and a fresh worktree has none).
- `make test-integration` on a private database: passed. No Go code changed.
- **Render tests** (`ExchangeCourtiersPanel.test.ts`, `commit marker`, 10 new cases): before
  the commit (make picker for the preparer, mar picker for the target, dice wait for every seat
  while the roll is on screen) and after it (picks only, marker only, legacy picks without
  marker, messy break, riposte), each checking the `EndsNote` text where one shows.
- **Bite check:** putting the old picks-based line back fails the three marker-only cases
  (make, messy, riposte); restoring the fix passes all 19.
- Not run: e2e and 375px screenshots (no view changes, see above).

## Behaviour changes to look at before merging

None visible. A row with `choice_submitted` and no picks now reads as committed ("Resolving…"),
which is only reachable through the API hole above.

## Open questions for the owner

None.

## Noticed, not fixed

- Rollless `make-choice` on Exchange Courtiers (see above): fixed in #61, so no tracker item
  here. Other plans with a post-roll choice may share the early return in
  `checkChoiceMatchesRoll` / `enforceChoiceBudget`.
