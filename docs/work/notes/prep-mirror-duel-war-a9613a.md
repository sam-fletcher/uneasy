# Duel and war preparation mirrors show the watcher's players

## What changed and why
While the focus player prepares Propose Duel or Make War, everyone else sees a read-only
mirror of the form. Its player chips were built from each viewer's own seat
(`playersExcept(players, currentPlayerID)`), so a watcher's own chip was missing and the
preparer was listed as a choice. Exchange Courtiers and Clandestinely Liaise had the same
bug until #97 fixed them.

- `plans/duel/PrepForm.svelte` and `plans/war/PrepForm.svelte` build the chips from
  `draftPreparerID = readOnly ? focusPlayerID : currentPlayerID`, the pattern #97 used. The
  preparer's own form takes the same code path as before.
- The tests are in `plans/prepMirror.test.ts`, which gains two `describe` blocks: Propose
  Duel's challenger chips and Make War's enemy chips. The preparer's list is everyone but
  the preparer, and each watcher is given that same list.
- `PlanContext.focusPlayerID` already exists (from #97), so nothing else changes: not the
  context, `PlanPanel`, `MainEventView`, `ProposeDuelPanel` or `MakeWarPanel`.

## How it was verified
- **Reproduced before changing code**, on this worktree's own server (port 8131, private
  database `uneasy_claude_prep_mirror_duel_war_a9613a`), at 375px with three browser
  contexts. Game 1 for Propose Duel and game 2 for Make War: alice (focus) picked bob.
  Before the fix, alice saw bob picked and carol listed; bob saw alice and carol, with his
  own chip missing; carol saw alice and bob. Both plans, same result.
- **Tests fail, then pass.** The six Duel and War tests were first in their own files, and
  moved into `prepMirror.test.ts` afterwards. Before the fix they gave `Tests 4 failed | 2 passed (6)`.
  The 4 are the two watcher cases in each form, e.g.
  `expected [ 'alice', 'carol' ] to deeply equal [ 'bob', 'carol' ]`. The 2 that pass are the
  preparer's own view in each form. After the fix: `Tests 6 passed (6)`. In `prepMirror.test.ts`
  on `main`'s two forms: `Tests 4 failed | 9 passed (13)`; with the fix: `13 passed`.
- **`make check-fast`** on this worktree: exit 0 (92 test files, 1781 tests).
- **`make test-integration`** on the private database `uneasy_claude_prep_mirror_duel_war_a9613a_it`:
  passes: every package `ok` (exit 0); `handler` took 251 s.
- **e2e** against this worktree's server (`E2E_BASE_URL=http://localhost:8131`):
  `propose_duel.spec.ts` and `make_war.spec.ts`, 35 passed. The rest of the e2e suite was not run.
- **Screenshots**, 375px, the same two runs before and after the fix, as alice (preparer)
  and bob (watcher, the challenger or enemy). Attached to the PR.

## Behaviour changes to look at before merging
- A watcher in a Duel or War prep step now sees the preparer's list: everyone but the
  preparer, with the pick marked. Before, their own chip was missing and the preparer's was
  listed.
- The preparer's own form is unchanged.

## Open questions for the owner
None for this change.

## Noticed, not fixed
- **Reload mid-draft is intended, not a bug.** A watcher who reloads after the preparer's
  pick sees the plan list but no mirror form. The live draft is deliberately not kept across
  a reload (ADR-006).
- `plans/MakeDemandsPanel.svelte` (`targetablePlans`, around line 70) filters
  `p.preparer_id !== currentPlayerID`, the viewer, into the target `<select>` of the prep
  form. A watcher's mirror may list the preparer's plans and drop the watcher's own. Filed as
  `docs/work/next/prep-mirror-make-demands.md` (order 82); not fixed here.
- `festivity/PrepForm.svelte` builds no player chip list from the viewer, so this bug shape
  does not apply there (checked by grep).
