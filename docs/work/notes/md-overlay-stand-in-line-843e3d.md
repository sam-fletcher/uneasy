# make-demands-overlay-stand-in-line — hand-over note

Branch `claude/md-overlay-stand-in-line-843e3d`. Built from
`docs/work/next/make-demands-overlay-stand-in-line.md` (deleted).

## What changed and why

The target plan's banner said a Make Demands perform-steps winner "will submit this
plan's make/mar choices in <preparer>'s place", for every plan. On a Propose Duel that is
wrong: the preparer still plays the bouts, and on a mar the opponent chooses. The server
already matches the ruling (#136), so only the banner was out of step.

The sentence now has one source. `performStepsWording(planType)` in
`frontend/src/lib/plans/buffets.ts` returns a `lead` (a clause with no subject) and `notes`
(full sentences after it), both with `TARGET_SLOT` standing for the preparer's name.

- The option picker's line (`demandOptionLine`, imperative, no subject) is the lead plus
  notes, with the first letter capitalised and the name filled in as text. Its output is
  unchanged, so `buffets.test.ts` still passes as written.
- The banner (`TargetPlanDemandOverlay.svelte`) renders the same lead and notes with the
  winner in front: "You will …" for the Make Demands winner, the winner's `PlayerName` for
  everyone else. It fills `TARGET_SLOT` with a `PlayerName` for the preparer, so every name
  stays a `PlayerName`. "will" takes the base form for either person, so nothing is
  conjugated and the two persons cannot disagree about a verb.

Duel wording (book p.60; RULINGS, Make Demands): "Choose which staked assets a make takes, in
<preparer>'s place, once the duel's roll has landed. <preparer> still stakes and plays the bouts.
A mar's choice stays with the plan's own target." Liaison and festivity keep their wording without
"once <preparer> has rolled", since they have no roll of the preparer's own. Propose Decree keeps its
enact-and-addendum wording. Everything else reads "pick the make or mar options … once they have
rolled", plus the mar note for Exchange Courtiers and Spread Rumors.

## How it was verified

- `make check-fast`: exit 0 on the rebased branch (119 Vitest files, 2411 tests; svelte-check,
  lint, Go vet, deadcode, Go unit tests, hook guards, tracker check).
- Server-render tests in `TargetPlanDemandOverlay.test.ts` (`perform-steps banner`), on a
  four-player fixture: alice prepares the duel's target plan, bob is the Make Demands winner,
  carol is the duel's target, dan is a seated bystander. They check the duel line for the winner
  ("You will …"), the preparer, the target and the bystander (third person, named), the generic
  line for `spread_propaganda`, and no bare "winner" or "make/mar choices" in the banner.
- The names are pinned as `PlayerName` markup, so a plain-text name fails. Proved both ways:
  with the preparer's name as plain text, four tests fail; with the Make Demands winner's name as
  plain text, three fail. The old component fails all the new tests.
- `make test-integration` against a private database (dropped afterwards): exit 0.
- Playwright `make_demands.spec.ts` on my own server (`E2E_BASE_URL` at the worktree's build, on
  the rebased branch): 19/19. Two of its assertions were updated to the new generic line.
- 375px, on a Propose Duel with a bob (Make Demands winner) perform-steps demand, driven with the
  `make_demands` helper sequence over fetch:
  - the Make Demands winner's view: "You will choose which staked assets a make takes, in alice’s
    place, once the duel’s roll has landed. alice still stakes and plays the bouts. A mar’s choice
    stays with the plan’s own target."
  - the duel's target's view (carol, a third party to the winner's choice): "bob will choose …".
  The screenshots are the full 375px Play pane; the PR body carries them.

Not run against my code: the full `make check` e2e suite (only the Make Demands spec ran on this
rebase, and the Seek Answers and Re-aim specs ran on the earlier build), and no desktop-width check.

## Behaviour changes to look at before merging

- The banner's perform-steps sentence changes for every target plan. It now reads "You will …" /
  "<name> will …" in place of "… will submit this plan's make/mar choices in … place."
- `make_demands.spec.ts` asserts the new generic line in its perform-steps test (two assertions
  changed; no other spec text changed). `buffets.test.ts`, `planPanelsC.test.ts` and
  `MakeDemandsPanel.test.ts` also assert the picker's new text.

## Open questions for the owner

None. The duel wording follows the ruling as recorded (RULINGS, Make Demands, 2026-11-26).

## Noticed, not fixed

- The duel's stakes pane shows "alice's alice's main character" (and the same for carol) under
  FACES: the possessive is doubled. Pre-existing, in the duel's stake area, not this banner.
