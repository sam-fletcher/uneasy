# Host Festivity: the host's pending-mar card waits while a guest rolls

Branch `claude/festivity-mar-lock-409b1a`. Item: `docs/work/next/festivity-pending-mar-roll-lock.md` (deleted here).

## What changed and why

- `festivity/HostPendingMar.svelte` takes `blockedByOtherRoll` and `activeRollerID`, as `HostChoosing`
  does. While blocked, the card keeps its header ("X insists you take a Mar") and shows
  "Waiting for X to finish their roll…" in place of its option list, sub-inputs and submit button.
- The card stays mounted through the wait, so its draft (the picked option, rumor text, marginalia
  and asset choices) survives the wait. The exception is a guest's challenge-duel make: that opens a
  challenge, the parent's `!fest.pendingChallenge` gate unmounts the card, and the draft is lost.
  `HostChoosing` has the same gap. It predates this change; see Noticed, not fixed.
- `HostFestivityPanel.svelte` passes the two props it already computed for `HostChoosing`.
- Why: the server refuses `resolve-host-mar` while another guest's roll-and-choice is open
  (`hfBlockOnActiveRoll`, `handler/plan_host_festivity_routes.go`). The card's picker could only fail,
  the same gap #111 closed for the Make picker.
- No Go change was needed.

### Audit of the host-only controls against `hfBlockOnActiveRoll`

| Control | Route | Server guard | Client during a roll | Result |
|---|---|---|---|---|
| Make picker | `host-choice` | yes | waits (since #111) | already matched |
| Pending-mar card | `resolve-host-mar` | yes | did not wait | **fixed here** |
| End the event | `end-event` | no (refused by `EventEndable`, since a roller has no outcome) | disabled (`festivityEndable` mirrors the rule) | no tap is refused, so unchanged; see follow-up |

Guest-side controls, not host-only, checked along the way: Roll and Opt out (`SocializingTurn`, already
gated on `blockedByOtherRoll`), Insist (`InsistFlow`, gated the same way; its route has the guard), and
challenge-duel (the guest make path; its route has the guard). The challenge response route has no roll
guard, and is not host-only; a pending challenge already pauses the festivity server-side. Neither is changed.

The End case is low priority, and a follow-up rather than a fix: `docs/work/next/festivity-end-event-roll-lock.md`
(order 79). End can't get through mid-roll at all, because `AllGuestsResolved` is false while a roller has no
outcome; only its refusal message differs. The fix is a Go change, so it is outside this piece.

## How it was verified

Everything below ran against this worktree's own code. Its server ran on a private database,
`uneasy_claude_festivity_mar_lock_409b1a`, on port 8131. Nothing ran against `:8080` or `:8090`.

- **Render tests** (`planPanelsC.test.ts`, "the host's pending Mar", three tests):
  - Blocked: the test failed before the fix (the picker rendered, with no waiting line) and passes now.
  - Unblocked: shows the picker. Passed before the fix as well, as a guard.
  - Roller exemption: passed before and after, as a guard.
  - The roller test names the host as the roller. The server never records the host as a roller,
    and the real case (a Make Demands stand-in who rolls and settles) can't render server-side:
    the stand-in comes from an effect in `TargetPlanDemandOverlay`, and effects don't run under
    `svelte/server`. So the stand-in path has no unit test.
- **Draft survival** is not a unit test: `svelte/server` renders once and can't type. The e2e test covers it.
- **e2e** (`tests/e2e/host_festivity.spec.ts`, run through `E2E_BASE_URL`):
  - Before the fix: 9 passed, and the new test failed at its waiting-line assertion.
  - After the fix: 10 of 10 pass.
  - The new test: alice's card waits while carol rolls. The picker and submit are gone, and the
    server refuses alice's settle. When carol concludes, the card returns with the typed rumor
    still in it, and her submit goes through.
- **375px screenshots** as the host (alice), before and after, mid-roll and once the roll ends.
  Before: game 12. After: game 23.
- **`make check-fast`:** exit 0. svelte-check reports 0 errors. Frontend unit tests: 1979 pass.
  Hook tests and the tracker check also pass.
- **`make test-integration`** on the private database: exit 0. Every package passed, `handler` included.
  No Go code changed; the run is here because the work-session checklist asks for it.
- **Not run:** the default-port e2e (`:8090`, which is `main`), and the full e2e suite. Only
  `host_festivity` was run, as the brief asked. No other plan renders `HostPendingMar`.

## Behaviour changes to look at before merging

- While a guest rolls, the host's pending-mar card names the wait and shows no options. Before, the
  options were live and the settle was refused with a 409.
- When the Make picker and the mar card are both showing, the same "Waiting for X to finish their roll…"
  line appears twice, once on each card. That is what the brief asked for, but it reads as a repeat
  at 375px.
- Nothing else changes. Once the roll ends, both cards return as before, except on the duel-challenge
  path, where the mar card's draft is lost (see Noticed, not fixed).

## Open questions for the owner

None.

## Noticed, not fixed

- `end-event` is refused during a roll by `EventEndable`, not by the roll lock, so its message differs.
  It can't get through mid-roll. Low priority. Follow-up: `docs/work/next/festivity-end-event-roll-lock.md`.
- **Draft lost on the duel-challenge path.** A guest's challenge-duel make opens a challenge, which unmounts
  the host's pending-mar card through the `!fest.pendingChallenge` gate, and the draft goes with it. The
  Make picker (`HostChoosing`) has the same gap. Both predate this change, and no test covers either.
  Follow-up: `docs/work/next/festivity-host-card-gaps.md` (order 84).
- **Stand-in addressed as "you".** The card's header and option copy address the viewer as "you", so a
  Make Demands stand-in reads the host's mar as their own choice. Predates this change. Same follow-up.
- The Make Demands stand-in who rolls and settles has no test at any level. The stand-in is set by an
  effect that `svelte/server` can't run, and no e2e covers it either.
