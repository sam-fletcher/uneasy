# Dice roll tally: count players by side, not dice

## What changed and why
- `frontend/src/lib/components/DiceRollPanel.svelte`, the footer tally under
  the roster ("N aid · N interfere · N of M ready"). It was spotted during a
  design review, reading "2 aid · 0 interfere" while otto had picked
  interfere. **The cause:** the first two numbers counted *dice* in each pool
  (`actorPool.length`, `intPool.length`), but the third counts *players*. Before
  anyone commits, the actor's two automatic base dice are the only dice. So
  the tally read "2 aid" whoever had picked aid. A player who had picked
  interfere but not yet committed a die added nothing. It wasn't a stale read
  or a WebSocket race, and the roster was right.
- Both numbers now count participants: aid = the actor (who always aids)
  plus everyone whose intent is `aid`. Interfere = every non-actor whose intent
  is `interfere`. Someone who hasn't picked a side counts on neither. The dice
  are still shown in the pools above, so no information is lost. The tally
  reads intents from the same `participants` list as the roster chips. A
  side is public only once its player presses Ready, so nothing new leaks.
- No restyle, no move: the panel is due to be redesigned as a play-area step.
- New `DiceRollPanel.test.ts` (svelte/server `render()`), three cases: a side
  picked before any die is committed; committed dice not inflating the count;
  the actor counted as aid and an undecided player on neither side.

## How it was verified
All against this worktree's own server (`make build`, `UNEASY_DEV=1
PORT=8147`, private database `uneasy_claude_heuristic_montalcini_7b4020`).
- **Reproduced:** `plandriver setup make_demands -players otto,wren,pia,quin`
  (wren acts), skip-vote as wren, then otto posted `interfere` and pia and quin
  posted `aid`. As wren, otto and pia, shot.mjs showed the roster "otto ·
  interfere", "pia · aid", "quin · aid" above "2 aid · 0 interfere · 0 of 4
  ready".
- **Fixed:** same game, rebuilt server. All three viewers, at 375px, now
  read "3 aid · 1 interfere · 0 of 4 ready". The viewers were wren (actor),
  otto (interferer) and quin (an aider who is neither). Before shots came from
  a binary built from `HEAD` on :8148 against the same game.
- **The test bites:** with the template put back to the pool lengths, all
  three cases fail. The first prints the reported string, `Received: "2 aid
  · 0 interfere · 0 of 4 ready"`.
- `make check-fast`: passes (Vitest 698/698).
- `make test-integration` on the private database: passes (handler 157 s).
  No Go code changed. The database was dropped afterwards.
- **Not run:** e2e. No spec reads the tally (`grep` for it under
  `frontend/tests/` finds nothing).
- Seeded games 1 and 2 were deleted with `plandriver cleanup`.

## Behaviour changes to look at before merging
- The tally's first two numbers are now **players**, not dice. With two aiders
  who each commit two dice, it reads "3 aid", not "6 aid". If dice counts were
  ever the intent, the fix is a wording change instead ("2 aid dice · …"),
  but then it sits beside a player count in the same line.

## Open questions for the owner
None.

## Noticed, not fixed
- At 375px the tally sits below the asset list, out of view on a phone with a
  few assets. That's for the planned play-area redesign, not this fix.
