---
name: diagnosing-bugs
description: A disciplined loop for finding the cause of a bug before fixing it — build a check that goes red on the bug, shrink it, rank hypotheses, probe, fix, keep the check as a regression test. Use when something is broken, stuck, wrong, throwing, frozen or slow, or a playtester reports a problem, and the cause isn't already obvious from one read of the code.
---

# Diagnosing a bug

The whole skill rests on one thing: a **red** check — a single command that
fails on *this* bug and will pass once it's fixed. With one, the cause falls
out of bisecting, probing and testing guesses against it. Without one,
reading code produces theories, not causes.

Bug fixes are still work sessions: follow `work-session` for scope,
verification and the PR. This skill covers getting from symptom to cause.

## 1. Pin the symptom

Write down, in one or two sentences, exactly what went wrong: which plan
or phase, which player saw it, what they expected, what happened. Every
later step checks against these words — fixing a nearby bug that isn't
this one is the commonest way to waste a session.

**Live reports** (from a playtester or the feedback form): you have no
access to the production database or its logs, and never try to get it.
Ask the owner for what you need — game phase and row, plan, player count,
what each player saw, roughly when — and reproduce on a dev game.

Rule out first: a "Failed to fetch" banner or a CSS MIME error after a
deploy is a tab left open across the redeploy, not a bug. And check what
"expected" means against the rulebook page: the reporter's expectation, or
the app's, may be the misreading — then it's a rules question for the
owner (the `grilling` skill), not a bug.

## 2. Build a red check

Spend most of your effort here. Pick the **first** of these that can reach
the bug — they get slower and flakier going down:

1. **Go unit test** in `game/` — pure rules and state logic, no database.
   `make test` or `go test -count=1 -run TestX ./game/`.
2. **Integration test** in `handler/` — the real handler, a real database,
   a game seeded straight into the right phase with `gametest` (`SeedMainEvent`,
   `SeedShakeUp` and their options). Seeds aren't always exactly what live
   play produces — `SeedShakeUp` leaves the game on row 13 where a live one
   is on row 14, and there is no seed for a finished game — so check the
   seeded state matches the reported one before trusting a green result. `make test-integration-run RUN=TestX
   PKG=./handler/...` on a **private** database (the `test-worktree` skill).
3. **Vitest** for frontend logic in `frontend/src/lib/` (`waitingOn.ts`,
   WebSocket handlers, stores), or a component rendered with
   `svelte/server`'s `render()`. `make test-frontend-unit`.
4. **curl against your own server**, using the dev endpoints: `/api/dev/seed`,
   `advance-row`, `force-roll` (`docs/DEVELOPMENT.md`, "Dev testing workflow").
5. **`cmd/plandriver`** to put a plan in the state, then **`shot.mjs
   --text-only`** as the player who saw the bug, asserting on the text
   (the `drive-plan` skill).
6. **A Playwright spec** for what only shows up in a browser —
   reactivity, overlays, two players acting at once. One browser context
   per player.

Steps 4–6 need your worktree's own server (`test-worktree`); the default
ports run `main`.

**Tighten it** once it exists. Make it faster: the narrowest seam, one test.
Make it sharper: assert on the reported symptom, not "no error". Make it
deterministic: force dice with `force-roll`, never rely on a real roll.

**Bugs that only happen sometimes** are usually races between a player's own
optimistic update and the WebSocket echo of it. Add latency to the socket,
or repeat the trigger many times, until it fails often enough to work with.

**When you can't build one,** stop and say so: list what you tried and ask
the owner for what would let you (a closer description, a screenshot, the
exact sequence of clicks). Don't move on to guessing.

**Done when** you can name one command, have run it, and its output shows
the reported symptom. Show the command and the relevant lines of output.

## 3. Shrink it

Cut players, assets, rows, steps and options **one at a time**, re-running
after each cut, until removing anything else makes the check pass. A small
repro leaves fewer suspects and becomes a clean regression test.

## 4. Rank hypotheses

Write **three to five** before testing any. Each one predicts something:
"if X is the cause, then changing Y makes the bug go away." A guess with no
prediction gets sharpened or dropped. Show the ranked list to the owner
before probing — they often know something that re-orders it — but keep
going if they're away.

Where bugs have hidden in this project before — check these against the
symptom first:

- **The game stalls: nobody can act.** The most damaging kind, because a
  stalled game can't be rescued from the UI — there's deliberately no
  escape hatch. Look for a completion or advance check gated on state that
  can never arrive (a result that's never set on one path, a hard-coded
  rank or player, a phase flag set only by a request that may never come),
  and for a waiting state that names nobody.
- **Frozen or duplicated UI:** a WebSocket broadcast of something the client
  already added optimistically, so a keyed `{#each}` holds two rows with the
  same id; or a WS handler *replacing* an object with a partial payload
  instead of merging, so a `$derived` throws.
- **Waiting On names the wrong players:** a new wait not wired into all
  three of `ComputeRowState`, the plan's `ResolvingWaitees`, and
  `frontend/src/lib/waitingOn.ts`.
- **A step resets or can be repeated after reload:** its done-state lives in
  client `$state` rather than in `resolution_data` behind a server guard.
- **An effect with no log line:** a raw effect helper used without its
  emitter, or a log post anchored somewhere other than `logRow(game)`.
- **Prepared plan acts on the wrong thing:** an asset or player id saved at
  preparation that changed hands, or broke, in the rows between.
- **Off by one rank:** rank 1 is the *highest* status, and dummy players
  hold rank 1 in 2–3 player games.
- **Hidden information visible:** secret text in a log post, or plan prep
  data returned to every player. The reverse too: an asset that changed
  hands without its new holder being granted sight of its secrets.
- **A player left without a main character:** an asset transfer clears the
  main-character flag, and play must wait until it's replaced.

## 5. Probe

Each probe tests one prediction. Change one thing at a time.

Prefer a test assertion or a debugger to logging. When you do log, tag every
line with one unique prefix (`[DEBUG-a4f2]`), so removing them is a single
grep. Never log everything and search afterwards.

**Slow, not broken:** measure before changing anything. The dev server
prints a `db trace` line per request — rank by `trips` — and `UNEASY_DEV=1`
mounts pprof.

## 6. Fix, then prove the test catches the bug

1. Turn the shrunk repro into a test at the seam where the bug actually
   happens. If the only available seam is too shallow to reproduce it
   honestly, say so in your hand-over note — that's a finding, not a reason
   to write a weaker test.
2. Watch it fail, apply the fix, watch it pass.
3. Undo the fix and watch it fail again (the `work-session` "prove it bites"
   step), then restore.
4. Re-run the original, unshrunk check from step 2.

## 7. Clean up

- `grep -rn "DEBUG-a4f2"` finds nothing.
- Scratch scripts and throwaway games are deleted.
- The cause — which hypothesis was right, and why — goes in the PR's
  Why section and the hand-over note, so the next session learns it. If it's
  a new kind of bug, add it to the list in step 4.
