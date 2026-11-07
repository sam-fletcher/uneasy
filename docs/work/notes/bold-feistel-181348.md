# Make Introductions: choicesDone reads the committed marker

## What changed and why

`MakeIntroductionsPanel.svelte` decided "has the outcome been committed?" from
`make_mar_choices` being non-empty; the other plans that have this question read
`choicesCommitted()` (the `choice_submitted` / `choice_auto_applied` markers, with the
non-empty picks as a fallback). It worked only because of `ForcedChoice`: the server applies
the roll the moment the dice land, and the mar applies with **no picks**, so the old
`choicesDone` was false for the whole mar and two template conditions added `&& !marPending`
to cover for it.

Now `choicesDone = choicesCommitted(parseResolutionData(plan))`, and the two `!marPending`
guards are gone (the pre-roll difficulty meter/buffet, and the "Dice roll in progress…" wait).
Nothing else in the template changed. The mar branch's own `{#if !marPending}` ("<actor> is
resolving the marred introductions…") stays: it answers "has the server recorded the mar
yet", which is a different question. The `EndsNote` lines from #58 live inside the mar branch
and key off `unresolvedDrafts` / `outcomesAwaitingWriting`, so they are untouched and true in
every state.

**Was the old behaviour wrong?** No. The guards and the marker agree on every state the server
can write, so each viewer (actor, bystander, marginalia author, delayed-arrival traveller)
sees exactly what they saw. The only difference is for a plan stored before the markers
existed (before 2026-10-30) that is mid-mar: it has `mar_pending` but no marker, so it would
show the dice wait while a roll is on screen. I judged that not worth a fallback (a fallback
on `marPending` keeps the very line the item removes); say if you want one.

## How it was verified

Against this worktree's code:

- `make check-fast`: passed (669 Vitest tests).
- `make test-integration` on a private database: passed. No Go code changed.
- **Render tests** (`MakeIntroductionsPanel.test.ts`, 12 new): make and mar as actor,
  bystander and marginalia author, committed (roll on screen or gone) and **not** committed
  (dice landing for make and mar, roll gone with nothing applied, pre-roll meter), plus a
  picks-without-marker plan.
- **Bite check:** swapping the panel back to the picks-based `choicesDone` while keeping
  the guards removed (the naive half of the change) fails the two "roll still on screen"
  mar tests. Restoring the original panel, guards and all, passes all of them: the new tests
  pin behaviour across the refactor, they do not distinguish old from new, as expected from a
  no-behaviour-change refactor.
- **e2e** `make_introductions.spec.ts` against my own server: 8/8 passed with `--workers=1`.
  The mar test now also asserts `choice_auto_applied` with empty `make_mar_choices`, and that
  no one is left on the dice wait or the difficulty meter. (With Playwright's default parallel
  workers the first run had 5 setup failures from `/api/dev` calls; serial passes. Not
  investigated; it looks like shared-server contention, not this change.)
- **375px screenshots** driven with `plandriver` + `shot.mjs` on my own server: pre-roll
  (preparer, bystander), dice wait (both), mar just landed (actor, two bystanders),
  mar mid-way (actor, marginalia author, other bystander), make (actor, bystander).

Not verified: a plan row stored before the markers existed; a delayed-arrival traveller plan
visually (unchanged by this edit: it never reads `choicesDone` beyond the `!isDelayedArrival`
guard).

## Behaviour changes to look at before merging

None intended. See the legacy-row note above.

## Open questions for the owner

None.

## Noticed, not fixed

- **Exchange Courtiers has the same line**, and `main` still reads
  `existingChoices.length > 0` at `ExchangeCourtiersPanel.svelte:350`. The item said another
  session covered it, but nothing on `main` or in a tracker file did. It is fixed in #60, so
  no item is added here.
- Seek Answers reads `existingChoices.length > 0 || hasCommittedChoices`, which is equivalent
  because the marker function includes the picks fallback; harmless, could be simplified.
