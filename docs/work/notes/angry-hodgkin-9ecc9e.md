# Make War: the claimant of an ended war's surrender claim can finally take it

## What changed and why

A surrender that empties a side ends the war in the same call that opens the
surrender claims. Every 1-v-1 war works this way. The open claims keep the row
on `await_surrender_claim`, but nothing on the client could discharge them:
`GET /tables/{id}/wars` returned only active wars, `warGatePlans` skipped
non-active wars, and `MakeWarPanel` hid itself once its plan was resolved and
its war ended. The server's claim route worked; only the UI was unreachable.

- **Server, list:** `ListWars` also returns an ended war while it has a
  fulfillable open claim (new query `ListEndedWarsWithOpenClaimsByGame`).
  Active and ended wars go through one `buildWarState` loop; an ended war with
  no fulfillable claim is dropped.
- **Server, row gate (review fix):** the row state (`warConflictGate`) read raw
  unfulfilled claims, while the advance gate and the war-state view filtered
  to *fulfillable* ones (the surrendered player still owns a non-destroyed
  asset). So a claim with nothing to seize could keep the row on
  `await_surrender_claim` naming a claimant no screen could serve. The row
  gate now uses the same `mwFulfillableSurrenderClaims`, so all three agree.
  My first version of this note wrongly said the list filter matched the row
  gate; it matched the other two.
- **Client:** `warGatePlans` surfaces an ended war with open claims (an ended
  war owes no cost of battle, so only claims count). `activeWarPlans` requires
  `status === 'active'`, because being in the list no longer means "under
  way". The panel's hide rule moved to `warPanelHidden` in `warGate.ts`
  and stays false while a claim is open.
- Claim flow untouched: each opponent takes one asset of their choice from the
  surrendered player, which is what the route already does.

Judgement calls from review, and what I did:
- **Bystander note:** `WarStatus` shows everyone but the claimant "X is
  choosing one of Y's assets to take." while a claim is open. Whether a
  bystander should see this much is an owner question (tracker).
- **End reason in words:** "(surrender)" now reads "(a side surrendered)";
  "(peace)" reads "(peace was agreed)". Unknown values still print raw.
- Updated the two comments that said the list was active-only
  (`api/plans.ts`, `MainEventView.svelte`); renamed the test helper to
  `listedWarClaims`.

## How it was verified

- `make check-fast` and `make test-integration` (private database): all pass.
- Tests bite, each shown failing with its fix reverted:
  - Row gate: `TestMakeWarHTTP_SurrenderWithNoClaimableAssets_RowCanAdvance`
    (now also asserts `ComputeRowState` and the war list) and the new
    `..._SurrenderClaimsShareOneAsset_LaterClaimDropsOutOfRowState` both fail
    on "must not hold the row state on await_surrender_claim".
  - List: `..._SurrenderEndedWar_ClaimStaysListed` fails (expected the war with
    one claim, got an empty list). `..._EndedWarWithoutClaims_NotListed` is a
    guard that passes either way.
  - Client: reverting `warGatePlans`/`activeWarPlans` fails two `warGate.test.ts`
    cases; dropping the `open_claims` exception from `warPanelHidden` fails
    "stays up while an ended war still has an open surrender claim";
    reverting `WarStatus` fails the bystander-note and end-reason tests.
- **Deviation from the review's ask:** the panel-hide test is a unit test of
  `warPanelHidden` plus a source-wiring check, not a `svelte/server` render of
  the panel. The panel gets its war from an `onMount` fetch, which the server
  renderer never runs, so a render test can't put it in the ended-war state.
  `WarStatus`, which takes the war as a prop, is render-tested.
- Scenario B (two claimants, one asset) inserts the second claim directly
  instead of seating a third combatant, which would mean a late join and entry
  payment that isn't under test.
- Hand-driven at 375px against a private server of this worktree: 1-v-1 war,
  enemy paid and surrendered. The claimant saw the form and completed it by
  tapping through the UI (artifact moved, row went to the scene step, war left
  the list). The bystander saw the war panel, the new one-line note and the
  Waiting On bar naming the claimant.
- Not run: Playwright e2e (specs hard-code `:8090`, which serves `main`); a
  3+-player war where a surrender leaves the war active (unchanged path).

## Behaviour changes to look at before merging

- `GET /tables/{id}/wars` can now contain `status: "ended"` entries. A tab left
  open across the deploy runs the old `activeWarPlans`, which counts any listed
  war, so its header chip would light for an ended war with a pending claim
  and open an empty drawer until reload.
- The row state no longer reports `await_surrender_claim` for a claim with
  nothing left to seize; that row can advance (it already could via the
  advance gate, which is what the earlier soft-lock fix targeted).
- Bystanders see the ended war's panel, with the note, while a claim is pending.

## Open questions for the owner

- Is the bystander's panel plus one-line note the right amount to show while a
  surrender claim is pending? (Also in the tracker.)

## Noticed, not fixed

- `make check-fast` prints a deadcode note for `middleware.AccountContext`
  (unreachable); it doesn't fail the run and isn't from this change.
