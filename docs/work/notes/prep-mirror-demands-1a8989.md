# Make Demands preparation mirror lists the watcher's plans, not the preparer's

## What changed and why
While the focus player prepares Make Demands, every other player sees a read-only copy of
the form. Its target `<select>` was filtered by the viewer's own seat
(`p.preparer_id !== currentPlayerID`). A watcher therefore lost their own plans from the
list and was offered the preparer's. When the preparer's draft aimed at a watcher's plan,
the watcher's select had no matching option and showed nothing selected.

- `plans/MakeDemandsPanel.svelte`: `targetablePlans` now excludes
  `draftPreparerID = readOnly ? focusPlayerID : currentPlayerID`, the pattern the Exchange
  Courtiers and Clandestinely Liaise fix (sam-fletcher/uneasy#97) and the Duel and War fix
  (sam-fletcher/uneasy#101) use. The preparer's own form takes the same code path as before.
- `plans/prepMirror.test.ts`: a `Make Demands: the target plans` block. The preparer's list
  is everyone's plans but their own; the target and the bystander are each shown that same
  list, which leaves out the preparer's plan and keeps their own.
- Not changed: the server's eligibility filter (`handler/plan_make_demands.go`), the
  `PlanContext` shape, and every other panel.

## How it was verified
- **Reproduced before changing code**, on this worktree's own server (port 8192, private
  database `uneasy_claude_prep_mirror_demands_1a8989`), 375px, three dev logins. Game 3 was
  the "before" run and game 4 the "after" run. In both, alice (focus) has Seek Answers on row
  5, bob has Spread Propaganda on row 5, carol has Chronicle Histories on row 8. alice has
  the Make Demands prep form open. A watcher's view was read from the page's target select.
  - Before, bob's list was `Seek Answers by alice (row 5)` and `Chronicle Histories by carol
    (row 8)`. bob's own plan was missing. With alice's draft on bob's plan, bob's select
    showed `(no option selected)`.
  - After, bob's list is `Spread Propaganda by bob (row 5)` and `Chronicle Histories by carol
    (row 8)`. With the same draft, bob's select shows Spread Propaganda by bob.
  - alice's own select was the same before and after, with and without the draft.
- **Tests fail, then pass.** Against `main`'s `MakeDemandsPanel.svelte` (the fix reverted,
  test file as it is now): `Tests 2 failed | 18 passed (20)`. The two failures are the watcher
  cases, for example:
  ```
  AssertionError: expected [ …(2) ] to deeply equal [ …(2) ]
  - "Spread Propaganda by bob (row 5)",
  + "Seek Answers by alice (row 5)",
  ```
  With the fix: `Tests 20 passed (20)`. The file has 20 tests, 3 of them new. After the rebase onto main (which added four Spread Rumors cases to the same file) the failing run is still the two watcher cases; the preparer case passes either way.
- **`make check-fast`** on this worktree: exit 0 (96 test files, 1838 tests), on the branch rebased onto main. Its `deadcode`
  step prints one finding that predates this branch (see below); it does not fail the build.
- **`make test-integration`** on the private database `uneasy_claude_prep_mirror_demands_1a8989_it`:
  passes (exit 0; every package `ok`, `handler` took 173 s). The change is frontend only.
- **e2e** against this worktree's server (`E2E_BASE_URL=http://localhost:8192`):
  `make_demands.spec.ts` and `reaim.spec.ts`, 20 passed. The rest of the e2e suite was not run.
- **Screenshots**, 375px, the before and after runs above, attached to the PR.

## Behaviour changes to look at before merging
- A watcher's Make Demands prep mirror now lists the watcher's own plans, not the preparer's,
  and shows the preparer's pick when it is one of those plans. The preparer's form is unchanged.
- Frontend only: no migration, no stored-state change, no WebSocket or API change. Reverting
  the PR restores the old mirror.

## Open questions for the owner
None for this change.

## Noticed, not fixed
- `plans/SpreadRumorsPanel.svelte:142` filters assets by `a.owner_id === currentPlayerID`.
  I did not check whether that list is also drawn for a watcher's mirror, and Spread Rumors
  belongs to another session's work, so I left it.
- I grepped the other plan prep forms for filters on the viewer's seat. The other hits are
  resolve-time or own-action checks (Propose Decree's amend turn, Chronicle Histories'
  own marginalia), not the prep mirror. Not checked in a browser.
- `make check-fast`'s `deadcode` step reports `middleware/auth.go:94:6: unreachable func:
  AccountContext`. It is not on this branch and does not fail the build.
