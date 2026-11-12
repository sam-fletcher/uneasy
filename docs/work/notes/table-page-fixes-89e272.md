# Table page fixes: the silent error boundary, and the resync/roll-fetch race

## What changed and why

Two small fixes to the table page, made before the shell redesign rewrites
`+page.svelte`. The page's diff is deliberately small: the logic went into
`ws-handlers.ts` and one new module, and the page got shorter.

**1. The error boundary now says what it caught.**
- The `<svelte:boundary>` had a `failed` snippet but no `onerror`, so the
  player saw the fallback and nobody saw the error. It now has
  `onerror={(error) => logBoundaryError(…)}`.
- `logBoundaryError` (`src/lib/boundaryError.ts`) is one `console.error` line
  with a recognisable prefix, `[uneasy:boundary]`, the phase the table was in,
  and the error object, which the console prints with its own stack.
- It logs locally and sends nothing to the server. Reporting render crashes is
  still the owner's open question (`questions/render-crash-report.md`, left
  alone).
- **No component stack.** The brief asked for one, but Svelte 5.57.3 doesn't
  hand `onerror` one. Its "in Foo.svelte" trail is attached only to an error
  that escapes every boundary (`apply_adjustments` in its
  `error-handling.js`). So the line carries the error's own stack, which in a
  dev build names the throwing component's file, and the phase.
- A unit test reads every `.svelte` file's AST and fails for any
  `<svelte:boundary>` with no `onerror`. So the guard survives the shell
  rewrite, and covers any boundary added later. It also asserts it found at
  least one, so a renamed AST node can't make it pass for nothing.

**2. A resync and a roll fetch can no longer undo each other.**
- `loadGameState` wrote `activeRoll` (and dice, votes, participants) from
  `getActiveRollForGame` with no knowledge of the roll events and the
  `fetchRoll` counter in `ws-handlers.ts`. Two orderings went wrong:
  - a snapshot read before a roll resolved landed after `roll.resolved` and
    put the unresolved roll back;
  - a snapshot that read "no active roll" was undone by a roll fetch still out,
    which put the roll back.
- The resync now goes through one function, `syncActiveRoll(ctx, read, opts)`
  in `ws-handlers.ts`. The page wraps the read where it already sits inside each
  `Promise.all`. It takes its ticket in the caller's own tick, before the
  await, so an event that arrives while the read is out counts as newer. Its
  answer applies only if:
  - no roll event or fetch took a newer number from the shared counter
    (`seq`) meanwhile; and
  - no *later resync* started (`resyncGen`).

  An answer that applies takes a number itself, which drops a fetch still out.
  That covers the second ordering.
- **Overlapping resyncs** (the first review's must-fix). A reconnect can start a
  resync while another is still out. With only `seq`, both held the same
  ticket and the first to land won. If the older read predated a plan
  resolving, its roll stayed up, and the newer "no roll" was dropped. Now each
  resync takes the next generation as it starts, and only the newest may apply,
  whatever order they land in. The generation is separate from `seq` on
  purpose: a resync that fails must not cost a fetch still out its number.
- `resyncPending` means "the newest resync is still out". It is cleared in a
  `finally`, so a read that throws no longer leaves it set (a stale mark made a
  `roll.resolved` for a roll nobody was loading get installed, where the old
  code dropped it), and an older resync landing doesn't clear a newer one's.
- While the newest resync is out, a roll event treats it like an in-flight
  fetch of the roll on screen. It supersedes it with a fetch
  (`supersedeRollFetch`), so the resync's older answer isn't the last word *and*
  the roll still ends up complete. Dropping the resync outright would have lost
  whatever roll events the page missed while offline, and a reconnect is when
  this race happens. `roll.resolved` also installs from its own payload while a
  resync is out, and `plan.resolved` drops a pending resync if the roll on
  screen is that plan's.
- Every writer of the counter goes through one helper, `takeNumber`, which
  also clears the in-flight mark and the pending mark: a new number outdates
  both.
- **Timing change, on purpose.** The roll is now applied when its own read
  lands, not after the rest of the round (record, plans, scene, banked dice)
  has too. The page has no derived state that couples the roll to the plans,
  and the live events already deliver them independently, so a table that is
  already on screen (a reconnect, or a cached snapshot) can see the roll
  refresh a round trip before the plans do. With no cached snapshot nothing
  renders until the whole load finishes, so a first visit can't see it. In
  exchange the page can't call the glue in the wrong order, and the tests
  drive the real glue.
- The names differ from the tracker item's sketch (`rollSyncCurrent`): the
  ticket and the apply are internal to `syncActiveRoll`, so the page has nothing
  to call in the wrong order.

**Tracker.** Deleted `next/table-error-boundary-silent.md` and
`next/table-resync-races-roll-fetch.md`. Added
`next/e2e-fail-on-boundary-fallback.md` for the part of the first item I did
not do (below).

## How it was verified

Against this worktree's own code:
- `make check-fast`: green (svelte-check 0 errors, 843 frontend unit tests, Go
  unit tests, tracker check). In a fresh worktree it fails three `cmd/server`
  tests until `make frontend` has run, which has nothing to do with this diff.
- `make test-integration` on a private database, on a build of the final source: every
  package ok.
- e2e against my own server (private database, own port): `smoke`, `resilience`
  (reconnect, then resync), `main_event_seed`, `seek_answers` and
  `propose_duel` (a roll created and resolved in one request): 25 passed. The
  other 17 specs did not run. A throwaway spec that loaded a seeded shake-up
  table (the `votes: false` branch) rendered it with no console errors; it is
  not kept.
- The boundary in a real browser: I added a throwaway throw behind a `?boom`
  query flag, built, and opened a seeded table at 375px. The fallback showed, and
  the console got `[uneasy:boundary] table phase view (no game) Error: …` plus a
  stack. The throw is gone again (I rebuilt from clean source and checked the
  embedded build for it). The phase read `no game` because the throw was on the
  first render, before the game loaded. In a production build the stack is
  minified (`at Oo (…/chunks/CdqpdEyM.js:4:5916)`), so the message and the phase
  do most of the work there.

**Tests bite.** For the boundary, I removed the `onerror` from the page: the
AST test fails and names `routes/table/[id]/+page.svelte:987`. For the resync I
broke one part of the fix at a time and ran `ws-handlers.test.ts`:

| Broken | Fails |
|---|---|
| ignore the resync generation (the old behaviour on overlap) | "the older lands first: only the newer answer applies" |
| ignore the fetch `seq`, or take the ticket after the read lands | 7: both orderings, "fetch that starts after the resync", "roll event while the resync is out", the plan case, and two more |
| any landing clears the pending mark | "the older landing does not end the newer one" |
| a throwing read leaves the mark set | "a resync that threw is not left pending" |
| a new number doesn't clear the pending mark | "after one roll event has superseded the resync, a second does not fetch again" |
| a new number doesn't clear the fetch in flight | "after a resync has applied, a roll event does not fetch again" |
| an applied answer takes no number | "a snapshot with no roll also drops a roll fetch still out" (the second ordering), plus the line above |
| a resync takes a number as it starts | both "read that failed / threw … spares a fetch out" cases |
| events don't supersede a pending resync | 5 |
| `plan.resolved` doesn't drop a pending resync | "…cannot bring the plan's roll back" |
| `roll.resolved` ignores a pending resync | 2 |

The overlap is tested both ways round (older lands first, newer lands first),
plus the pending mark surviving the older landing.

## Behaviour changes to look at before merging

- A rare console line on a render crash: `[uneasy:boundary] table phase view
  (<phase>)` followed by the error. Nothing changes for players.
- During a resync (page load and every reconnect), a roll event for the roll on
  screen now costs one extra `GET /rolls/:id`. That's the price of keeping the
  roll complete after discarding the older snapshot. It only happens inside the
  resync's own window.
- A resync's roll answer can now be discarded (a newer roll event, fetch or
  resync started). Before, it always applied, whatever it overwrote.
- The roll is applied slightly earlier than the rest of a resync round (above).

## Open questions for the owner

None new. `questions/render-crash-report.md` is untouched and still waiting.

## Noticed, not fixed

- **The e2e guard** (fail a spec whose page shows the boundary's fallback) is
  not small, so it is not in this PR: see `next/e2e-fail-on-boundary-fallback.md`
  for why and a plan.
- `next/redesign-table-shell.md` still lists the two deleted items as things it
  starts after. Both are now done, so its "Starts after" line can drop them.
- If the *newer* of two overlapping resyncs fails, the older one's answer was
  already dropped, so the roll on screen stays as it was until the next event or
  resync. That is the price of "only the newest applies", and no worse than the
  state before this PR.
- `make deadcode` prints `middleware/auth.go:94:6: unreachable func:
  AccountContext` without failing the check.
- `svelte-check` warns about an unused `.type-note` selector in
  `HelpContent.svelte`.
- Two test files each carry an eight-line `.svelte` file walker
  (`boundaryError.test.ts` and `blockWhitespace.test.ts`). I left them: the
  AST walks differ, and sharing the file lister would mean editing a test that
  isn't part of this work.
