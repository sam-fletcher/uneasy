# redesign-plan-panels-c-e37a80

The three plans with the longest sub-flows, moved onto the redesign: Make Demands (with the overlay it draws on
the plan it targets), Host Festivity and Propose Duel. Wave 5 of the visual redesign. Only how they look
changed; every route, guard, wait and string a player acts on is the same.

## What changed and why

**1. No boxes, no tints, no fades.** These panels were the last place a plan drew boxes of its own.

- **The demand overlay** (`plans/demand/TargetPlanDemandOverlay.svelte`) was a light parchment panel with
  dark ink, drawn at the top of every plan a demand targets. It is now a group on the page: a lead line
  ("Demand in effect from bob", with the demand's mark from the log), the effects as plain secondary-text
  rules, and the winner's forms below. It is shared by the other plans' panels, so it changes there too.
- **The duel's bouts** (`plans/duel/BoutsPhase.svelte`) drew each side as a sunken box with an accent border,
  the call as a tinted blue box and the tied dice on a wash. The sides are now sections (a shape, a name, some
  space); the call is an information notice; the set-aside tray keeps only its dashed antique-gold rim, with
  no fill, and each tied die in it takes its owner's colour; nothing is faded when a stake is resolved.
- **The festivity's guest list** keeps its thin rules between rows and loses its warm border colour.

**2. The shared pieces, instead of drawing our own.**

- Each die in the duel is `DieTile` (a 44px tile with the owner's colour on its edge); it replaces the small
  gold-filled numbers. A die nobody may read draws the die mark and no number, so the DOM carries no face
  the viewer must not see. The mark alone looks like a die that has not been rolled, so a stake whose die the
  viewer cannot read also says "hidden" in the status-word style ("called higher · hidden").
- The tray's dice know their owners: `setAsideDice` (`plans/duel/shared.ts`) shares the carry-over rule with
  `computeAccumulated` and reports each tied die with the player whose stake it came from.
- A stake you can play is the option card (`.choice-card`) with the shared tick, picked = the bright gold
  border. A stake that is not yours to play is a plain line with a status word ("called higher", "won",
  "lost", "tied").
- Chips are `<Chip>`: the festivity's option chips (the guest's and the host's), the duel's higher/lower
  and arms/wits.
- `.res-warning` is `<Notice tone="warning">` (the break-self warnings, the duel's "no assets to stake").
- Typed ✓ and ✗ are gone: option cards take `ToneMark`'s tick, and a result is the word with its colour
  beside it ("Result: Make").
- A Make Demands pick so far is a `FinishedStep` line with its picker's shape ("bob picked Control
  leverage"). The option cards give the title its own line and the effect the smaller line under it.
- "Send someone else" and "Cancel" in the duel's setup were underlined text links; they are now secondary
  buttons.

**3. Group gaps.** With the boxes gone, the groups inside a plan's step (the guest list, the host's Makes, an
insisted Mar; the duel's "who duels" and "stakes"; the demand's effects and forms) sat the same 12px apart
as the lines inside them, so they did not read as groups. Each panel now wraps its groups in a column with
24px (`--space-5`) between them and keeps the inner gaps. A recipe, so it is written here: **a plan body's
groups are `--space-5` apart; lines inside a group are `--space-2` to `--space-3`.** It is local to these
three panels because `planPanel.css` was frozen this wave; moving it into the frame is the tracker item
[plan-group-gap](../next/plan-group-gap.md), which also lists the other duplicates (below).

**4. A bug the redesign exposed.** The festivity's "Opt out" button had the class `action-btn` and none of
the three looks, so it drew as the browser's own white button. It is a secondary button now.

**Markers removed** (`tokens: not migrated`): `MakeDemandsPanel`, `TargetPlanDemandOverlay`, `HostChoosing`,
`GuestList`, `SocializingTurn`, `InsistFlow`, `BoutsPhase`, `SetupPhase`. Every file under the three folders
is on the scales. No tokens were added to `app.css`, and no shared file was touched.

**Scope against `docs/work/next/plan-step-labels.md`.** My panels no longer need its "move the notices" bullet
for `.res-warning` and `.chip-btn`: neither class is written in these folders any more. Still to do there:
`.choice-card` (Make Demands' draft and counter-demand, the festivity's mar picker, the duel's stake picks;
no shared option card exists yet), the `.player-chip` that `PlayerChips` still draws for the Make Demands
overlay, the festivity's chip pickers and the duel's setup, and the "Dice roll in progress…" lines (Make
Demands, the duel's roll phase; the festivity's "Rolling… resolve the dice above."). The heading hook and
"X is resolving…" lines are untouched. **Placement is for that item too:** "Targeting: Seek Answers by
alice (row 5)" in Make Demands, and the demand overlay's lead line, are facts about the plan like the card's
finished lines, and would sit above the heading if `ResolvingCard` took extra lines.

## How it was verified

Against this branch's own code (a private server on `:8178` and a private database; a baseline build of
`main` on `:8177` for the "before" shots):

- `make check-fast`: passes. 79 test files, 1367 tests, `svelte-check` clean (no warnings), `go vet`, the Go unit tests, the
  git-guard tests, the tracker check.
- **e2e: the full suite, 204 specs, all pass** (run again after the review changes below). `make_demands`, `host_festivity` and `propose_duel` also run alone first
  (38 specs). I edited two of the three specs: `host_festivity` reads the result word (`.outcome-make`)
  instead of "✓ Make", and `propose_duel` reads a die tile's face, or "?" for a hidden one, through one
  helper (`dieReads`) in place of the old `.die` text. The privacy checks it feeds (the opponent's and a
  bystander's dice carry no face in the DOM) are the same ones. That change bites: with a hidden die drawn
  as `face={v.die ?? 0}`, rebuilt, the "each viewer sees only their own dice" spec fails at the changed line;
  restored.
- **New unit tests** (`plans/planPanelsC.test.ts`, 70 tests, and `setAsideDice` in `plans/duel/shared.test.ts`): source rules for every file of the three plans
  (no `.res-warning`/`.chip-btn`/`.player-chip` markup, no marker, no inline style, no typed ✓/✗, no
  retiring token, no opacity), and what the panels render: the duel tracker (a player's own dice, nobody
  else's, a bystander sees none; picks only on your turn; the call as an info notice; won/lost words; tied
  dice dashed and in their owners' colours; "hidden" beside every die the viewer cannot read; sides unboxed,
  tray unfilled), the guest list, a guest's turn, the Make Demands draft and the
  overlay's CSS.
- **Every new test bites.** 34 mutations, each turned the two test files red and was restored: a
  `.res-warning` or `.chip-btn` back, a typed tick, an inline style, an opacity, a retired token, a hidden die
  drawn with a number, a stake pickable off your turn, a box on `.duel-side`, a fill on the tray, the call as
  a warning, a tied die drawn plain, a lost bout without its word, italics on the guest list, a guest row
  without its shape, a make no longer green, Opt out as a bare button, the glyph result, an option as a tab
  not a toggle, pick lines with the full description, picks no longer finished steps, a dud card without
  `aria-disabled`, a card without its effect line, a box or ink on the overlay, a coloured rule, narrow
  overlay buttons, the "hidden" word dropped, a tray die with a control's border or one side's colour, the
  declarer's tied die credited to the responder, a tray not emptied by a winning bout, and the dot between two
  status words dropped. Four were first not caught (two bad mutations, one harness gap, one assertion that
  matched the wrapper rather than the step); the test and the harness were fixed and rerun.
- **Looked at, at 375px, before and after:** Make Demands (preparing as the actor and a watcher; the draft
  as the first picker, the other picker and a bystander; two picks; the last pick; a mar's counter-demand;
  the overlay on the target plan as the winner of leverage and keep-or-change-target, the target's
  preparer and a bystander, and the retarget form open; perform steps as the winner), Host Festivity
  (preparing; the host, a guest and a bystander at the start; a guest's make choices, picking a rumor and a
  duel; the host's extra Makes and the insist button; the insisted Mar as the host, with a peer, a rumor and
  break-yourself picked; a duel challenge as the challenged, the challenger and a bystander; a mar's
  choices) and Propose Duel (preparing; setup as the challenger, the challenged and a bystander, with the
  peer picker and after staking; the bouts as declarer, responder and bystander, with a stake picked, an open
  call, a resolved bout and a tied bout with dice in the tray; the final roll's decision after a mar, to take
  the stake or spare). The before and after montages are in the PR.
- **Also looked at, at 1280px (three columns):** the duel's bouts, the festivity's insist flow and the demand
  overlay, each as every viewpoint (they fit their column; nothing is cut off).
- **Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari or Firefox;
  hover and focus states (not photographed); 344px and the two-column width; the festivity's
  "must accept" challenge and a guest's break-yourself pick as photographs (only e2e); the duel's claim after
  a make, where the winner picks which stakes to take (the picker runs in e2e); a table of
  four or five in the duel (three seats were used).

## Behaviour changes to look at before merging

- **The demand overlay has no light box on any plan it targets.** Its lines are secondary text on the page,
  and the winner's forms are full-width.
- **Make Demands' draft lines show the option's short name** ("bob picked Control leverage") where they
  showed the whole sentence; the card shows the effect under the title, and the overlay explains it once
  drafted. The "Draft complete" list is plain text.
- **The festivity's result reads "Result: Make" / "Result: Mar"** with the colour beside the word, where it
  read "✓ Make" / "✗ Mar".
- **The festivity's guest list shows a shape and a coloured name** per guest and no italics. It still reads
  "(you)" and "(host)" after the name, not "You": the frozen e2e helper `scoreRow` finds a row by the name.
- **The duel's dice are 44px tiles**, so the bouts screen is taller than before. A resolved stake no longer
  fades, and a die you cannot read says "hidden" beside it.
- **"Send someone else" and "Cancel" are buttons** in the duel's setup (they were small underlined links).
- **"Opt out" and the host's "Insist" are secondary buttons.** Only a step's required move is the main button.
  Insisting is optional ("pick your moment"), so I did not make it the main button.

## Open questions for the owner

- [duel-call-and-tray](../questions/duel-call-and-tray.md): the open call as an information notice, a
  set-aside die as a dashed tile, and whether a won die keeps its owner's colour or its winner's.

Two questions I first filed were answered by the existing rulings and are gone: the tray's antique-gold rim
stays (the style guide keeps it for the duel's die tray), and a demand's effects are plain secondary-text
rules (rules take no colour). The group gap is now work, not a question: the tracker item
[plan-group-gap](../next/plan-group-gap.md).

## Noticed, not fixed

- **`PlayerChips` still draws `.player-chip`** (shared by plans that are not mine), so the festivity, the
  duel's setup and the demand overlay still use that markup through it.
- **"Targeting: Seek Answers by alice (row 5)"** is drawn by Make Demands under the heading, though it is a
  fact about the plan like the card's finished lines above it. Moving it needs `ResolvingCard` to take extra
  lines, and "Waiting for bob to pick…" repeats the heading; both are for `plan-step-labels`.
- **`landingRow` in `MakeDemandsPanel` is computed and never shown**, though the file's header says the picker
  shows where the demand will land.
- **The Decline button's tooltip** ("You took accept_duels and cannot decline") names a stored key; phones
  show no tooltip anyway.
- **`DieTile` has no "set aside" state**, so a tied die borrows the dashed look of a repeated die (question
  above). It also draws the die mark for a face that is `null`, which means "not rolled yet", so the duel adds
  the word "hidden" for a die that has been rolled but is not the viewer's to read.
- **Duplicated CSS the freeze kept out of the shared files** (all in the tracker item
  [plan-group-gap](../next/plan-group-gap.md)): `.step-actions` in four files (`SetupPhase`, `RollPhase`,
  `SocializingTurn`, `ChallengeBanner`, all `2 1 10rem` / `1 1 6rem`; `DiceRollPanel` has its own), the
  overlay's `.demand-actions` (`12rem` both, so long labels stack full-width on a phone), `.taken-count` in
  `HostChoosing` and `HostPendingMar`, and the 24px group wrappers (`.fest-body`, `.setup-groups`,
  `.demand-banner`).
- **`.stake-line` rows carry two roles** (the option card when pickable, a plain line otherwise); a shared
  option card would let both go through one piece.
