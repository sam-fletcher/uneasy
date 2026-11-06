# make-choice needs a settled roll

## What changed and why

`POST /api/plans/{id}/make-choice` accepted a make or mar for a plan that had
no roll at all. `checkChoiceMatchesRoll` and `enforceChoiceBudget` both return
early with no roll, so nothing validated the pick, and only four plans
(Seek Answers, Chronicle Histories, Spread Propaganda, Spread Rumors) had a
`ChoiceGate` that happened to refuse it. Exchange Courtiers (#60's finding)
did not: the preparer's `{"result":"make"}` before any roll took the peer and
resolved the plan.

The fix is at the shared seam: `checkChoiceMatchesRoll` now answers 409
(`errNoRollYet`) when the plan has no roll, for every plan. No opt-in, so a
plan added later is covered by default. The `ChoiceGate` comment and the
function comment now say a gate can rely on a settled roll existing.

### Audit: every plan, make-choice with no roll

Run against the original code by parking a resolving plan of each type with no
roll and posting make and mar as its preparer (`TestMakeChoice_NoRoll_AllPlans`).

| Plan | Before the fix | Legitimate with no roll? |
|---|---|---|
| Exchange Courtiers | **200, took the peer, resolved the plan** | No. Accepted fair trade resolves through `/fair-trade` and is untouched |
| Make Introductions | **200, resolved the plan with a result** | No. Its roll comes after `/finalize-peers`; the UI never calls make-choice |
| Propose Decree | 200, recorded the choice | No. Roll comes from call-roll after the council; the outcome auto-applies on the roll |
| Propose Duel | 200, recorded the choice | No. The roll comes after the bouts; the UI calls it only from the roll phase |
| Make Demands | 200, recorded the choice | No. Rolls at kickoff; a plan held for re-aim has none yet |
| Make War | 200, recorded the choice (state is unreachable: it resolves in OnResolve) | No. Never has a roll or a choice |
| Host Festivity | 200, recorded the choice | No. Guests roll; everything runs through extra routes |
| Clandestinely Liaise | 200, recorded the choice | No. No standard roll; everything runs through extra routes |
| Seek Answers | 409 (ChoiceGate) | No |
| Chronicle Histories | 409 (ChoiceGate) | No |
| Spread Propaganda | 409 (ChoiceGate) | No |
| Spread Rumors | 409 (ChoiceGate) | No |

"Recorded the choice" means `ChoiceSubmitted` and the picks were written, with
`ApplyChoice` run, which for a plan that rolls later can pre-empt or corrupt
its real flow. Verdict: **no plan legitimately reaches make-choice without a
roll.** The frontend confirms it: the only callers of `makeChoice` are the
Exchange Courtiers, Seek Answers, Spread Rumors, Spread Propaganda and duel roll
panels, all after the dice. No game-rule call was needed.

## How it was verified

- `make check-fast`: passes (needed `make build` first in a fresh worktree:
  the `cmd/server` tests want the embedded frontend build).
- `make test-integration` on a private database: passes, all packages.
- Each new test bites: with the fix reverted, `TestMakeChoice_NoRoll_AllPlans`
  fails for the eight plans in the table above and
  `TestExchangeCourtiers_MakeChoice_NoRollRefused` fails; restored, they pass.
- Legitimate no-roll paths: `TestExchangeCourtiers_NoRollPaths_StillWork`
  (accepted fair trade resolves, no roll row; declined → roll → settled
  make-choice works) and the existing Make War, Host Festivity and Clandestinely
  Liaise integration suites, all green.
- Not run: browser/e2e. No frontend or UI code changed.

## Behaviour changes to look at before merging

A make-choice with no roll now returns 409 "the dice haven't been rolled yet".
Nothing in the app sends one. The `nil` branches in the four existing
`CheckChoice` gates are now unreachable through this route; I left them as a
second line of defence rather than widen the diff.

## Open questions for the owner

None.

## Noticed, not fixed

- Tracker: `docs/work/next/exchange-courtiers-rollless-make-choice.md` is added
  by #60, which was still open (not on `main`) when this finished, so I did not
  delete it. Delete it when #60 and this PR are both in (or in whichever lands
  second).
- Only `make-choice` was audited. The per-plan make/mar routes that take a
  result (e.g. Spread Rumors' `request-take-consent`, which already refuses with
  no roll) were read but not exhaustively tested for the same gap.
