---
title: "A page resync (loadGameState) can race the roll fetch it doesn't know about"
order: 77
---

The table page's `loadGameState` resync writes `activeRoll` (and its dice,
votes and participants) from `getActiveRollForGame`, in `+page.svelte`. The
live roll fetch that `roll.created` starts, and its sequence number, live in
`ws-handlers.ts` (`fetchRoll`), and the resync cannot see them. Two ways they
disagree, both narrow:

- A resync snapshot read before a roll resolved can land after `roll.resolved`
  was applied, and put the unresolved roll back.
- A resync that read "no active roll" (the plan has finished resolving) can be
  undone by a roll fetch still out for that roll, which puts the roll back.

Both leave the stale roll panel up until the next reload. Likeliest after a
reconnect, when the page resyncs while roll events are still arriving.

A fix would let the resync take a sequence number from the same counter
(export a small `startRollSync(ctx)` / `rollSyncCurrent(ctx, seq)` pair from
`ws-handlers.ts`) so that its answer applies only if no newer roll event or fetch
started meanwhile, and so that a resync dropping the roll also drops a fetch
still out. Test it with `getActiveRollForGame` mocked, as `ws-handlers.test.ts`
mocks `getRoll`.
