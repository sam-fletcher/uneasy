# Preparation mirrors show the preparer's players and peers

## What changed and why
While the focus player prepares an Exchange Courtiers or Clandestinely Liaise, the
other players see a read-only mirror of their form. Both panels built that mirror
from the viewer's own seat: the target or partner chips were everyone except the
viewer, so a watcher saw the wrong chips. Liaise's "Your meeting peer" list also
showed the watcher's own peers under the preparer's label.

- `PlanContext` (`plans/types.ts`) now carries `focusPlayerID`, the focus player's id.
  `PlanPanel` takes it from `MainEventView` (`game.focus_player_id`) and passes it
  into every panel's context. `MainEventView`'s resolve-mode context carries it too,
  for type completeness; prep mirroring never reads it there.
- `ExchangeCourtiersPanel.svelte`: `draftPreparerID` is the focus player in a
  read-only mirror and the viewer otherwise. The partner chips come from it.
- `ClandestinelyLiaisePanel.svelte`: the same id drives the partner chips and the
  "meeting peer" list (`draftPreparerPeers`). In the mirror the label reads
  "<preparer>'s meeting peer", and the empty-state lines name the preparer or the
  partner. The preparer's own form keeps "Your meeting peer" and "Your partner".
- Six existing test fixtures gain `focusPlayerID: null`. They build `PlanContext`
  literals that don't exercise the mirror, and svelte-check needs the field.

Considered and not taken: reading the preparer from the draft's `player_id`. The
draft only exists once the preparer highlights a card, so a watcher would have no
preparer to draw from before then. Also considered: deriving the mirror's selected
chip straight from the draft in the template, which would be SSR-testable. That
rewrites more of both panels than the bug needs, so the selection is checked in the
browser instead (see below).

## How it was verified
- `make check-fast` on this worktree: passes (exit 0).
- Unit: `frontend/src/lib/components/plans/prepMirror.test.ts`, 7 tests. With the two
  panels reverted to main, 5 fail: the Exchange Courtiers watcher chips (target, and
  bystander), the Liaise watcher chips and meeting-peer label (partner, and
  bystander), and the Liaise empty-state line. The 2 preparer-view tests pass both
  ways, so the preparer's form is pinned as unchanged.
  `svelte/server` does not run `$effect`, so the picked chip is not in these renders.
- Full frontend suite: 89 files, 1750 tests pass. svelte-check: 0 errors, 0 warnings.
- e2e, against this worktree's own server on a private database: `exchange_courtiers.spec.ts`
  and `clandestinely_liaise.spec.ts`, 21 passed. Not run: the rest of the e2e suite.
- Screenshots, 375px, mid-draft and unsubmitted, as alice (preparer), bob (target or
  partner) and carol (bystander), both plans, before (the two panels at main) and
  after. The bob shots show the bug: before, bob's chip is missing and alice and
  carol are listed; after, bob is listed and selected. Attached to the PR.
- `make test-integration` on `uneasy_claude_mirror_target_b6f1af`: passes (exit 0;
  `handler` took 193 s). This branch has no Go changes, so this is a regression check.

## Behaviour changes to look at before merging
- A watcher in the prep step now sees the preparer's chips: everyone but the
  preparer, with the pick marked. Exchange Courtiers: bob and carol are listed for
  alice's plan, not alice and carol.
- Liaise: a watcher sees "alice's meeting peer" listing alice's peers, not their own.
- The preparer's own form is unchanged and pinned by the unit test.

## Hidden information
Nothing new reaches any viewer. The preparer's draft already goes to the whole table
(`prepare_plan.draft`, a `hub.BroadcastEvent`), and it carries the same target and
peer ids the mirror now renders. The asset list each viewer gets is game-wide
(`listAssets(gameID)`). The mirror reads only those two.

## Open questions for the owner
None. The duel and war preparation forms have the same bug shape; they are filed as
their own item, `docs/work/next/prep-mirror-duel-war.md`.

## Noticed, not fixed
- `plans/duel/PrepForm.svelte:35` and `plans/war/PrepForm.svelte:32` build
  `otherPlayers` from the viewer (`playersExcept(players, currentPlayerID)`), the same
  shape as this bug. Not reproduced; filed as the item above. The war form is outside
  this change's scope.
- `ownerIntactAssets` (`plans/shared.ts`) has a doc comment that says "un-leveraged",
  but the filter checks only `is_destroyed`. Not changed.
- The Liaise tip "Pick a partner to choose their meeting peer." is addressed to the
  preparer and shows to watchers too. Left as it is.
