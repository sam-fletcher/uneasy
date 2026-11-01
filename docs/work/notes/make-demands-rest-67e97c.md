# Make Demands: the draft picker shows options that would do nothing

## What changed and why

The brief listed four Make Demands audit items. Three were **already fixed on
`main`** when I looked, so I verified them in code instead of redoing them:

- **Retarget 500 / no wait state** — `/demand-retarget` treats "no roll yet" as
  a live window; `DemandRetargetFinalized`, `await_demand_retarget` in
  `ComputeRowState`, the demand blockers set and `waitingOn.ts` are all there,
  with an integration test walking the window.
- **Leverage nothing** — the overlay always renders the finalize button,
  labelled "Leverage none — let the roll fail".
- **Leverage offered where nothing rolls** — `mdTargetHasPreparerRoll` gates
  the unready seed, the `/demand-leverage` 409 and the host lockout; the
  overlay mirrors it.

What was genuinely open was the draft picker. It never *hid* options, but it
offered all four without saying that some do nothing against the chosen
target. Now `MakeDemandsPanel` shows such an option greyed (dashed card, same
`.locked` style) with the reason always visible under it, so touch users need
no tap to see it. The logic is two pure functions in `plans/shared.ts`
(`demandOptionInertReason`, `draftOptionViews`):

- control_leverage: inert on Host Festivity, Propose Duel, Clandestinely
  Liaise (mirrors `mdTargetHasPreparerRoll`).
- keep_or_change_target: live only on Exchange Courtiers, Spread Rumors,
  Propose Duel, Clandestinely Liaise.
- keep_assets: inert on Seek Answers and Chronicle Histories.
- **Never blocks the draft:** inert options are disabled only while a live one
  remains. The draft needs all four picked, so when only duds are left (the
  forced last pick included) they stay pickable, with a one-line note.

## How it was verified

- `make check-fast` passes (after `make build`; the Go server tests need the
  embedded frontend). Vitest: 556 pass.
- New `shared.test.ts` cases; with the disabling logic reverted the
  "disables the inert options while a live one remains" case fails, restored
  it passes.
- Private server (port 8187, own database), Make Demands against a Seek
  Answers, forced to a make: viewed at 375px as the first picker and a
  bystander (waiting text only). The picker shows keep-target and keep-assets
  greyed with reasons; leverage and perform-steps stay live. I did not
  separately screenshot the second picker; same component and data.
- Not run: `make test-integration` and e2e (no Go code changed).

## Behaviour changes to look at before merging

The picker's cards look different against targets where an option is inert
(dashed, dimmed, reason line). Nothing server-side changed.

## Open questions for the owner

- Should the server also reject picking an inert option while a live one
  remains? Recommendation: no, it could only block the forced last pick.
- On Propose Duel and Clandestinely Liaise the retarget winner gets no wait
  state (`pendingDemandRetargetChooser` returns 0 without a preparer roll),
  though the picker shows keep_or_change_target as live there. Block on the
  winner, or treat the option as inert? Not built either way.

## Noticed, not fixed

- The inert tables exist in TS only (plus Go's `mdTargetHasPreparerRoll`). If
  a plan gains a target or stops awarding assets, `shared.ts` needs updating
  by hand.
- The tracker item's wording ("D2, D5–D7") used different numbers than the
  audit, which already lists D2, D5, D6, D7 as fixed.
