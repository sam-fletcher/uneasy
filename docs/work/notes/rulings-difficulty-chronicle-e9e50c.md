# Rulings: difficulty 1–6, Chronicle make options repeatable

## What changed and why
- `game.ClampDifficulty` (game/difficulty.go) is the one place that confines a
  difficulty to 1–6 (owner ruling 2026-11-01). Every per-plan formula
  (exchange courtiers, introductions, propaganda, seek answers, rumours,
  chronicle, decree, duel, festivity, demands) returns through it, and so does
  the leverage-vote adjustment in `rolls_leverage.go`. A demand on a
  Clandestinely Liaise (difficulty 0) now rolls at 1 instead of soft-locking.
- Make options for Chronicle Histories are repeatable (same ruling date). The
  server and panel already allowed it; the handler comment now states the
  ruling, and tests pin it.
- Deleted both tracker entries this resolves.

## How it was verified
- `make check-fast`; `make test-integration` on a private DB (all green).
- e2e `clandestinely_liaise` + `chronicle_histories` against my own server,
  3 runs in a row, all green. The perform_steps test is un-fixme'd.
- Bite checks: removing the lower bound fails 18 game unit cases; making the
  handler refuse a repeated option fails the new Go test.
  The new e2e UI-repeat test was not bite-checked (the UI never hid used options).

## Behaviour changes to look at before merging
- Formulas that used to return 0 for a rank-0 input, or 7 for 5 introductions
  peers, now return 1 and 6; the old unit tests that pinned those edge values
  were updated. No reachable play state produced them except the Liaise demand.

## Open questions for the owner
None.

## Noticed, not fixed
- `make check-fast` needs `make frontend` first in a fresh worktree (embedded
  build missing otherwise).
