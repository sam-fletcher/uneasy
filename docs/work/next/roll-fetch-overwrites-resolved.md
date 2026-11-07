---
title: "A roll resolved within one fetch of its creation leaves open pages on 'Dice roll in progress…'"
order: 62
---

The table page's `roll.created` handler (`ws-handlers.ts`) fetches the roll
and writes `activeRoll` when the fetch lands, with no check that anything newer
has happened. If `roll.resolved` arrives first, its handler skips it (it only
applies to an `activeRoll` that is already set, and the fetch hasn't set it
yet), and the older response then installs the unresolved roll. The page shows
"Dice roll in progress…", or the duel's choices never appear, until a reload.
Plans already have the guard for this exact shape (`planSeq` / `refreshPlan`: a
response applies only if it still holds the latest sequence number); rolls don't.

Two ways in:

- **The dev `force-roll` shortcut**, when a spec calls it right after a roll
  opens. This caused the first nightly e2e run's failures on a slow GitHub
  runner (fixed in the specs by waiting for the roll to be on screen first).
  Delaying `GET /api/rolls/<id>` by 400 ms reproduced 14 spec failures at will.
- **Real flows that create and resolve a roll in one request**: the final
  Propose Duel bout broadcasts `roll.created` and then resolves straight away
  (`plan_propose_duel_bouts.go`), and a Host Festivity guest roll does the same
  when nobody can leverage. Nothing forces these, so no spec can wait its way
  out. With the machine heavily loaded (load average about 10) the unforced spec
  "mar: the target spares the loser" in `propose_duel.spec.ts` failed once with
  bob's page never offering "Spare alice", and with the 400 ms delay "a tie on
  the final bout" failed once in two runs. A player on a real network is
  unlikely to hit it (their fetch usually reaches the server after it has
  finished), but a slow client or busy server can, and it will show up as an
  occasional red nightly run until fixed.

The fix is in the client: give rolls the same latest-wins sequence as plans, and
let `roll.resolved` apply even when the fetch hasn't landed. An earlier
hand-over note (`notes/spec-exchange-courtiers-83f4d5.md`) saw the symptom as
"stale until reload, maybe a force-roll artefact".
