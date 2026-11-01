# Make War regression specs

## What changed and why
`frontend/tests/e2e/make_war.spec.ts` (+ `helpers/make_war.ts`): 16 specs and
one `test.fixme`, pinning what the 12-plan UX audit fixed in Make War. No
product code.

- Declaring: enemies + casus belli both required (UI), server refuses an empty,
  duplicate, self or outside-the-game enemy list (400), the declaration is logged.
- Delay vote: sides and join/stay-out choices for a bystander (and not for
  participants), a join is free and enrols the joiner (second join 409, bad side
  400), "stay out" enlists nobody, delay = average of faces rounded up.
- Cost of battle: owed per opponent in reverse power order; shown in the play
  area to every seat (audit #2); the bar names the one payer; break tears the
  marginalia and hands the turn on; out-of-turn and repeat payments 409;
  leverage two; row unblocked once all are paid.
- Peace (audit #8): the bar names the voters, a rejection is announced and
  returns the gate to the payer, unanimous accept ends the war.
- Surrender: pay-then-surrender via the form; when it empties a side the war
  ends with the claim open (fa13795, f6d17fd): claimant sees the form,
  surrendered player and bystander see the "X is choosing…" note, bar names the
  claimant; claimant takes an asset (owner changes, only that one, claim spent);
  non-claimants 404, someone else's asset 403; a side with an ally left keeps
  the war active and the surrendered player stops paying.

## How it was verified
Against this worktree's own build (`./server_makewar_spec` on port 8187, private
database, since dropped), never `:8090`. 16 passed, 1 skipped, three runs in a row.
Bite checks (not committed): reverting the ended-war listing in `ListWars` and
the peace-voters branch of the waitees override failed the peace and surrender
specs; hiding the panel on an ended war regardless of claims, and dropping the
bystander claim note, failed the surrender specs. The fixme fails when enabled.
`make check-fast` passes. Integration suite not run (no Go changed).

Setup note: the war plan has to be advanced to its own row first (so it is
`resolved`) before jumping to the cost row; otherwise the panel-hide rule is
never exercised.

## Behaviour changes to look at before merging
None (tests only).

## Open questions for the owner
- A proposer can vote on their own peace terms (including reject them) through
  the API; the UI hides the buttons. Harmless as far as I can tell; not pinned.

## Noticed, not fixed
- A join during the delay vote isn't pushed to other players' Waiting On bar
  (stale until reload): `docs/work/next/make-war-join-not-pushed-to-waiting-bar.md`,
  pinned by the `test.fixme`.
