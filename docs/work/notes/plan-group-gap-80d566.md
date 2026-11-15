# plan-group-gap-80d566

(Named after the branch, not the worktree folder: the folder, `redesign-plan-panels-b-25b28c`, is the name of the
wave-5 session whose note is already in this directory, as in `plan-step-labels-d68aa8.md`.)

The plan panels stop writing the same recipes out again. The gap between the groups of an open step, the step's button
row, the count after a lead line, the option card and the player chips each have one definition, are on
`/dev/styleguide` and in `docs/STYLE_GUIDE.md`, and a test fails on a panel that writes one out again. The intended
visual change is the group gap; every other look is kept, and the few that changed are listed below.

## What changed and why

**1. One group gap, in the frame.** `planPanel.css` has `.step-groups`: a column with 24px (`--space-5`) between its
children, against the 12px between a group's own lines. `ResolvingCard` puts the panel's body in it, once, so every
panel's open step gets it. `.fest-body`, `.setup-groups` and `.demand-banner` are gone; those three still wrap the groups
they nest inside a group of their own (the festivity's body, the duel's setup, the overlay's forms) in the same class.

I chose the frame over wrapping the groups in each panel. A pass over the screenshots of every plan state (a script,
below) listed the sibling groups closer than 24px: Chronicle Histories in every state (the difficulty meter, the buffet,
the scene form and the outcome were all 12px apart), Seek Answers (the buffet and the form), Exchange Courtiers (the
buffet and the offer), Make Introductions (the peer count, the buffet, the naming form), Propose Decree (the council
roster and the decree form) and Spread Propaganda (the buffet and the form). They are siblings directly under the
card in every case, which is what one rule at the frame fixes, and eleven panels each remembering to wrap would be
the copy-paste this piece removes. After the change the same pass finds none under 24px, in any state it reached.
What it costs is that every direct child of the body is now a group: see "Behaviour changes".

**2. One step button row.** `.step-actions` is in `shared/actionButton.css` (every user already imports it): a wrapping
row, the main button `2 1 10rem`, the other `1 1 6rem`, both stretched to the row's height. `.stacking` is the demand
overlay's (both `12rem`, so two never fit a 344px column). It replaces the copies in `SetupPhase`, `RollPhase`,
`SocializingTurn`, `ChallengeBanner`, `DiceRollPanel` and `.demand-actions`. I did not make a `<StepActions>`
component: the buttons are each caller's own, with their own `disabled` conditions and labels, so a wrapper would only
hold a `div`.

**3. One `.taken-count`**, in `planPanel.css`, for `HostChoosing` and `HostPendingMar`.

**4. `PlayerChips` is a row of `<Chip>`.** The shape and the name in the player's colour are the same; the colour is a
`style:color` on the name, so the chip itself stays neutral (a pick is the border alone, as the recipe says). It owns its
own row layout (three declarations). `.player-chip`, `.chip-btn` (including `.used` and `.unavailable`, which had no users)
and `.player-chips` are out of `planPanel.css`. `shared/steps.test.ts` compared `.player-chip` to `chip.css` rule by
rule; that guard moved to the component, `plans/PlayerChips.test.ts`, which pins that it is a `.chip` and not a
lookalike, that a pick is `selected` with `aria-pressed`, that a locked chip is `disabled`, and that the colour is on the
name and never on the chip. The cascade for every chip state is already run on `chip.css` in `pieces.test.ts`.

**5. One option card, `shared/OptionCard.svelte` + `optionCard.css`**, beside `Chip`. Props: `title`, `selected`,
`badge` (a signed level), `description`, `reason` (why it is locked) and `aside` (what rides at the end of the row: the
duel's die). It replaces `.choice-card` in Make Demands (the draft and the counter-demand), `MakeMarPicker`, Re-aim, the
duel's bouts and the festivity's pending mar. Three decisions:

- *A card is locked by the attribute the caller sets,* `disabled` or `aria-disabled`, as a chip is, not by a class. Make
  Demands' cards are `aria-disabled` (a tap says why) and the other two are `disabled`; that choice stays with the caller,
  so no handler changed.
- *The lock reason goes under the sentence.* The make/mar picker and the festivity's mar picker had it at the right edge of
  the row in 12px; Make Demands had it as a line under the sentence. One recipe means one of them moves, and I moved the
  two. Question: [option-card-lock-reason](../questions/option-card-lock-reason.md).
- *A card that is both picked and locked still shows the hairline,* as before (the old `.locked` rule came after
  `.active`). A chip keeps its gold when locked; I did not carry that over to cards, since it is not the piece's brief.

`.choice-list` became `.option-list`, and sits in `optionCard.css`. The duel's stake card keeps `stake-line pick` as
classes with no style behind them: the e2e specs count the stakes and press a pick by them, and a comment in
`BoutsPhase` says so.

**6. The face chips are `<Chip>`.** `SimultaneousRevealInput` trims the chip's padding to the die with
`.face-row :global(.chip)`. `D6Face`'s comment says `<Chip>`.

**7. Dead code and docs.** `.res-warning` is gone from `planPanel.css`. The paragraph in `docs/STYLE_GUIDE.md` that said
`planPanel.css` still carried the older names is replaced by the rule for groups in a step and a paragraph saying what a
player presses in a step is a shared piece. The option card, the step row (both kinds), the group gap and `.taken-count`
are in the style guide's recipes and shared-components table, and on `/dev/styleguide`: the step row and the stacking row
under Buttons, player chips under Chips, a new Option cards section (rest, selected, locked, with a level, with a die),
and the group gap under Play steps.

**8. Specs.** Sixteen selectors in seven e2e specs moved from `.player-chip` / `.choice-card` to role selectors
(`getByRole('button', { name, exact: true })`, or `{ name: /^Defer/ }`). The duel specs' `.stake-line.pick` stays (see 5).

## How it was verified

Against this branch's own code (a private server on `:8162`, then `:8163` for the e2e run, each on its own private
database; the docker containers serve `main`, so nothing here ran against them).

- **`make check-fast` passes** (rerun on the final tree): 88 test files, 1743 tests, `svelte-check` 0 errors and 0
  warnings, `go vet`, `golangci-lint`, the Go unit tests, the git-guard tests and the tracker check.
- **New unit tests, each shown failing without its code.** I broke the code 17 ways and each went red at the assertion
  that guards it: `PlayerChips` drawn as a `.player-chip` button (2 failures) and its name colour moved onto the chip (2);
  `OptionCard` ticking when not selected (1), dropping `aria-pressed` (1); `optionCard.css` with a deep-gold pick (1), a fill
  on a pick (1), a locked card that is not a hairline (2), and a 40px height (1); the stacking row at `10rem` (1) and a
  row that does not wrap (1); `.step-groups` at 12px (1) and `ResolvingCard` without the column (2); a panel with its own
  `.step-actions` (1) or its own `.taken-count` (1) again; a hand-built `.choice-card` in Re-aim (2); a `.chip-btn` face
  chip (2); the demand overlay not stacking (2). Each was restored from a copy. The tests:
  `shared/pieces.test.ts` (`OptionCard`, `optionCard.css` with the cascade run per state, the step row),
  `plans/PlayerChips.test.ts`, `plans/sharedRecipes.test.ts` (one definition each, every user, the 24px against the 12px)
  and `shared/steps.test.ts` (the body is in the column, after the frame's own lines).
- **e2e: the full suite on this tree: 215 specs, all passed** (3.0 min), against a build of the final tree on `:8163` and a
  fresh private database, with the throwaway screenshot spec moved out of `tests/e2e/`.
- **Looked at, at 375px, before and after,** by a throwaway Playwright spec that drives each state with the e2e helpers
  and `cmd/plandriver` against a `main` build (`:8161`) and this one, as the actor, the other party and a bystander,
  and runs the gap audit above on each shot. About 138 shots per side: Host Festivity (host, guest, the challenged guest,
  a bystander; the extra Makes, a mar to settle, the challenge chips), the duel (prep, setup, the champion picker, bouts,
  the claim after a mar), Make Demands (the draft, the counter-demand, the leverage form, the retarget form), Exchange
  Courtiers (prep, the offer, the mar picker), Re-aim (Exchange Courtiers, Make Demands), Make War (prep, the delay
  reveal, the cost of battle), Clandestinely Liaise (the delay reveal, the re-delay, together at last, keep, share),
  and the pre-roll of all eleven other plans plus the make and mar of Chronicle Histories, Seek Answers, Spread Rumors
  and Spread Propaganda. All in the PR as contact sheets.
- **Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari or Firefox; 344px and the
  desktop width; the states that script did not reach: Make Introductions' arrivals and mars, Propose Decree after the
  council (the mar, the addendum, the enact), Exchange Courtiers after the mar, Make War after the cost of battle, the
  duel's final roll as the loser of a make, and the Spread Rumors consent step. Their spacing follows from the frame's
  rule, and their markup is unchanged.

## Behaviour changes to look at before merging

Every look that changed, and nothing that is not here:

- **24px between the groups of an open step in every plan** (12px before, except Host Festivity, the duel's setup and the
  demand overlay, which were already 24 inside themselves). Where it shows: Chronicle Histories (every step), Spread
  Rumors and Spread Propaganda (the rumor or message as stated, the result, the picks), Seek Answers, Make Introductions
  (pre-roll), Propose Decree (the council roster and the decree form), Exchange Courtiers' offer, and the neighbours of
  the demand overlay on every plan. About 12–36px taller in the longest states. The step's heading is still 12px from
  its first group.
- **A fact line above a plan's first group is 24px from it.** Make Demands' and Spread Rumors' "Targeting: …" and the
  duel's "alice vs bob · difficulty 1" read as a group of one. Question:
  [plan-lead-lines-group-gap](../questions/plan-lead-lines-group-gap.md), on the same ground as `plan-finished-facts`.
- **A locked option card's reason is under its sentence** in the make/mar picker (Exchange Courtiers' mar, "needs −2") and
  the festivity's mar picker ("You have no peers"); it was at the right edge, in 12px faint text. Now 14px, under the
  card's own sentence, so those cards are one line taller when locked.
- **Chips and cards that look the same:** the player chips (every user), the face chips, the duel's stake card and Make
  Demands' and Re-aim's cards are, by the shots, the same at rest, picked, locked and read-only. The one thing that changes
  without being a look: the face chips gain `aria-pressed`, like every `<Chip>`.

For `plan-type-tiers`, which asked to be told whether the gap helps or hurts a sub-heading: it helps the ones that open a
group (Propose Decree's "Finalize your decree" and "Join the council?", Chronicle Histories' "Invoked artifacts", the
"Result: Make" lines), which now have 24px above them and read as the start of something. It does nothing for the
problem the item describes, a sub-heading and the sentence under it, because those two are inside one group, 12px apart,
as before (Liaise's last step is one group). Nothing got worse.

## Open questions for the owner

- [option-card-lock-reason](../questions/option-card-lock-reason.md): where a locked card says why.
- [plan-lead-lines-group-gap](../questions/plan-lead-lines-group-gap.md): what a plan's fact line above its first group
  should be, now that the gap shows it.

## Noticed, not fixed

- **`docs/work/next/plan-type-tiers.md` lost its `after: next/plan-group-gap.md` line.** The brief says to edit no other
  entry's file, but `tracker.py --check` (and so `make check-fast`) fails on an `after:` that points at a file that is
  gone, which deleting this item's file made true. That front-matter line is the only change to it; its prose still says
  "builds after `plan-group-gap`".
- **A picked option card's level turns bright gold** (`.option-card.selected .option-badge`), which the recipe gives to a
  chip's text and not to a card (a card has the tick). Only the make/mar picker has a level, and I kept the look.
- **`.demand-banner-header`**, the overlay's lead line, keeps its name; it is `plan-finished-facts`'s to move.
- **`app.css:242`'s comment** on `--color-accent-bright` still names "active face-chip".
- **An empty body adds 12px.** `ResolvingCard`'s `.step-groups` is a flex child of the card whether or not the panel
  rendered anything into it, so a state where a panel draws nothing leaves a 12px gap below the heading. I did not find
  such a state; a `:has(> *)` guard would fix it if one turns up.
