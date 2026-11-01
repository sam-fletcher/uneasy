# Exchange Courtiers regression spec (Phase 4)

## What changed and why

- `frontend/tests/e2e/exchange_courtiers.spec.ts` — seven live specs and one
  `test.fixme`, each pinning something the audit fixed:
  - **Preparation:** the peer is picked from the chosen player (switching
    player drops the pick), the Prepare button waits for peer + notes, the
    rules reference (Buffet) is there; the server refuses a peer that isn't the
    target player's. The log names the exact peer being angled for.
  - **Fair trade accepted:** the target proposes through the UI; the preparer is
    handed the decision live; both peers change hands; the log reads proposal →
    both transfers → resolution line (the order #29 set up); no "Complete plan"
    button for anyone.
  - **Mar by 1:** only the target is handed the choice, the margin is shown, the
    tiers beyond it are locked ("needs −2/−3"); "A Fair Trade" swaps both peers
    and logs both legs before the outcome.
  - **Mar by 2 (riposte):** the preparer becomes the actor, the target and a
    bystander see the wait line; surrendering gives the requested peer to the
    target and leaves the targeted peer where it was.
  - **Make (margin 0):** only Messy is open; the peer moves; the target owes a
    break and the others wait; the break closes the plan itself, log in order.
  - **Re-aim:** a peer destroyed between prepare and resolve pauses the plan —
    the preparer gets the re-aim panel with the reason, target and bystander
    only wait, every other route is refused, a non-preparer can't re-aim; after
    re-aiming the new target is asked to propose the trade and the log carries
    both the loss and the re-aim.
  - **`test.fixme`:** a bystander is shown "You asked for …" after the offer
    (second-person wording to a non-actor, audit Tier 6, still open). Item:
    `docs/work/next/exchange-courtiers-bystander-second-person.md`.
- `frontend/tests/e2e/helpers/exchange_courtiers.ts` — seeding, driving and
  log-reading helpers for this plan only. Shared `helpers.ts` is untouched.
- Product code is untouched.

Not pinned, on purpose: whether a re-aim is forced when the target only changes
hands (`docs/work/questions/exchange-courtiers-reaim.md`). The re-aim spec uses
a *destroyed* peer only.

## How it was verified

Against this worktree's own build, on a private database and port (`E2E_BASE_URL`):

- The spec: 7 passed + 1 skipped, three runs in a row, plus several earlier runs.
- **Each guard bites** (change made locally, rebuilt, reverted):
  stale-aim check disabled → re-aim spec fails; second transfer's log line
  dropped → fair-trade spec fails; resolution announced before the transfers →
  fair-trade spec fails; `AutoCompleteAfterChoice` returning false → the mar,
  riposte and make specs fail; mar choice given to the preparer in the panel →
  the mar and riposte specs fail.
- `make check-fast` passes.
- Not run: `make test-integration` (no Go code changed).

## Behaviour changes to look at before merging

None; tests and a tracker item only.

## Open questions for the owner

None new. The existing re-aim-on-handover question is untouched.

## Noticed, not fixed

- **Bystander second person** (above; tracked, pinned as `fixme`).
- **Bystander can keep "Dice roll in progress…" after a roll has resolved.** When
  the roll is created and force-rolled within milliseconds (the dev
  `force-roll`), a third player's open page sometimes keeps the in-progress
  stakes list and only corrects on reload (seen in ~1 of 3 runs). Real rolls pass
  through several stages over seconds, so this may be a force-roll artefact; I
  did not chase it, and the mar spec deliberately does not assert the
  bystander's line at that moment.
- The chat's plan group collapses once resolved; the spec expands it to read
  the entries. A shared helper for that (also useful to other plans' specs) would
  belong in `helpers.ts`, which I left alone as instructed.
