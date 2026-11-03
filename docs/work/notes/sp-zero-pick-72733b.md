# Spread Propaganda: a zero-pick commit counts as choices made

## What changed and why
`SpreadPropagandaPanel.svelte` and `SpreadRumorsPanel.svelte` derived `choicesDone` from `existingChoices.length > 0`, so a mar
committed with nothing picked (server marker `choice_submitted`, empty `make_mar_choices`) looked
like "no choice yet" and the picker came back. They now use `hasCommittedChoices`, the panel's
existing `choicesCommitted(parseResolutionData(plan))` derivation, as Seek Answers does. Done-state
now comes from the server only.

## How it was verified
- New render tests `SpreadPropagandaPanel.test.ts` and a case in `SpreadRumorsPanel.test.ts`; each zero-pick case fails with its fix undone (picker rendered) and passes restored.
- `spread_propaganda.spec.ts` and `spread_rumors.spec.ts` (12 in all) passed against my own server on a private database (:8193).
  I added to the existing zero-pick e2e an assertion that the marker is set with no picks.
- `make check-fast`: passed.
- Integration suite not run: no Go changes.

## Behaviour changes to look at before merging
None for players in practice: the server auto-resolves a zero-pick mar, so the e2e passes with or
without the fix; the bug was a transient picker (and wrong follow-up state while `resolving`). The
render test is what bites.

## Open questions for the owner
None.

## Noticed, not fixed
Make Introductions still reads picked options (not a one-line swap: its mar is auto-applied with
empty choices and `marPending` covers for it) — `docs/work/next/make-introductions-choices-done-zero-pick.md`.
Exchange Courtiers has the same line; another session owns it. No positive UI assertion was added to
the Spread Propaganda/Rumors zero-pick e2es: the plan auto-resolves, so there is nothing visible to
assert beyond the absence checks already there.
