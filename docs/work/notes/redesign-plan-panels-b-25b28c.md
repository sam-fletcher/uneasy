# redesign-plan-panels-b-25b28c

Five plan panels moved onto the new look: Clandestinely Liaise, Propose Decree, Make Introductions,
Exchange Courtiers and Re-aim. Wave 5 of the visual redesign. None of them was mocked up on its own, so each
was built from the signed-off step layout (a heading, the step's own sentence at body size, the controls, a
notice, one main button) and the written rulings. Behaviour is unchanged: the same routes, events and rules.
Only how the panels look, and the markup that draws it, moved.

## What changed and why

**Four files left `tokens: not migrated`** (`ClandestinelyLiaisePanel`, `ProposeDecreePanel`,
`ExchangeCourtiersPanel`, `ReAimPanel`) and now sit on the type, gap and radius scales. Make Introductions
had no marker already; it only needed its chips and a missing gap. The guard test
(`scaleTokens.test.ts`) is what proves no literal is left in any of the five.

| panel | what moved |
|---|---|
| **Clandestinely Liaise** | The five Things We Share options are `<Chip>`; the warning about tearing a peer's last marginalia is a `<Notice tone="warning">`. Each phase's own sentence ("Each of you nominates…", "Pick one option…") is step text, not a small secondary note. The delay reveal (drawn by the table page, with no card around it) is no longer a centred stack: the scene note is a quoted line like the other steps' and "alice's X meets bob's Y" is one line of step text. |
| **Propose Decree** | The addendum's "and" / "but" are `<Chip>`. The inline margins (literal rems stacked on the plan's own step gap) are gone, and a heading with the text it introduces (the council, the decree under debate) sits closer than the step's groups do. "Result: ✓ Make" is "Result: Make" in its colour, as the shared make/mar picker draws it. The law under debate and the law as it stands are step text. |
| **Make Introductions** | The peer-count and mar-outcome choices are `<Chip>`. An outcome already given to another peer stays tappable (`aria-disabled`, so a tap says why) and is struck through as well as faint. The naming form and the mar's fates were bare divs with no spacing, so the recap lines ran into the next prompt; they are columns with the step's gap now. The recap's typed ✓ is gone. |
| **Exchange Courtiers** | Literal sizes and margins gone. The riposte step's "Break, then surrender" is the main button, with "Surrender intact" beside it; a preparer with no peer to trade gets "Proceed to dice roll" as the main button. Both follow the style guide's button rule; one point of taste is left in the question file. |
| **Re-aim** | Literal sizes gone. The reason line is no longer italic (italic is for asset names). Its player field uses the shared `FormField` label. The Make Demands option cards gain the tick every other picked card wears. |

Why a few of these calls, since each is one a reviewer may want to undo:

- **Step text at 16px.** The step layout in the signed-off Play mockups has the step's own sentence at body size
  in the main colour, and hints and waits smaller and secondary. The panels had both at 14px secondary, so the
  one sentence that says what to do didn't stand out from the footnotes. Hints, waits and "this ends the plan"
  lines are unchanged. Cost: the screens are a little taller.
- **A heading and its text sit closer than groups do.** Taking out Propose Decree's inline margins made every line
  equidistant, so "Proposed decree being debated:" read as one more line of the roster above it, and putting each
  phase's sentence at 16px left the panels' own sub-step titles ("Join the council?", "Secrets We Keep") the same
  size and colour as the sentence beneath them. Gaps are the only grouping now that the boxes are gone, so the
  council and the decree under debate are 4px inside and 12px apart, and a title and its sentence are 4px apart.
  One mechanism does it everywhere: the two sit in a `.titled` block (a column with the small gap), one rule in each
  of Liaise and Propose Decree. A negative margin pulling the sentence up was tried first and dropped: it closed the
  gap by an amount that depended on what else set one, and next to a wrapper's own gap it overlapped. I did not drop the sentences back to 14px under a title:
  that would make a step's text size depend on whether a title happens to be there, and the titles go when the
  frame's heading can name the step.
- **No typed ✓ or ✗.** A typed symbol is drawn in whatever font each device supplies, so it looks different from
  one phone to the next (the dice emoji were replaced for the same reason). The word carries the meaning in
  both places I removed one. (Panels outside this batch
  still draw `✓ Make` / `✗ Mar`: see Noticed.)
- **`.choice-card` stays.** There is no shared option card yet, so Re-aim's Make Demands cards keep the recipe in
  `planPanel.css`.

### Scope against `plan-step-labels`

That item's bullets, for these five panels:

- **"Move the notices" is done** for `.res-warning` (Liaise had the only one) and `.chip-btn` (Liaise, Make
  Introductions, Propose Decree). **Two things are left:** `.choice-card` (Re-aim's cards, until a shared option
  card exists) and `.player-chip`. The latter is drawn by `plans/PlayerChips.svelte`, one component used by
  Liaise's and Exchange Courtiers' prep, Make Introductions' mar and Re-aim (and by panels in the other pieces),
  so converting it to `<Chip>` is one edit best made once. `steps.test.ts` pins `.player-chip` to the house chip
  until it is.
- **"Delete what the heading says" is untouched:** every "Dice roll in progress…" and "X is resolving…" line is
  still in the five panels, and existing tests pin several of them.
- **The heading hook is untouched.** The sub-step titles ("Together at Last", "Council (2) · Signatory: ravi",
  "Finalize your decree") still sit under the frame's "the next step". Two things find panels by those titles
  and will need to follow when they go: `helpers/clandestinely_liaise.ts` (`section(page, 'Things we share')`, a
  frozen file this wave) and the `debated` locator in `propose_decree.spec.ts`.

## How it was verified

Against this branch's own code (a private server on `:8165`, a private database, never `uneasy_test`),
rebased on `main` at #85.

- `make check-fast` passes: 79 test files, 1310 tests, `svelte-check` with 0 errors and 0 warnings, `go vet`, the
  Go unit tests, the git-guard tests and the tracker check.
- **e2e: the full suite passes, 206 specs**, including the two new ones in `reaim.spec.ts`. I ran it three times:
  206 passed before the last two commits, 205 passed and one failed after the spacing commit, and 206 passed on
  the final code. The one failure was `propose_decree`'s "must not be blank", a race in the spec and not in
  anything I changed: the server commits the plan and only then passes focus, and the spec has bob post a blank
  decree (expecting 400) as soon as the plan is visible, so now and then bob is still refused with 403. It failed once in about ten runs; the check now
  polls for the status (a blank decree never creates a plan, so asking again is harmless) and passed six runs in a row.
- **28 new unit tests** (`plans/planPanelsB.test.ts`): the five panels' markup holds none of `chip-btn`,
  `player-chip` or `res-warning`, and none closes a gap with a negative margin; and, rendered, Make Introductions'
  count chips (one selected), used outcomes (greyed, struck, still tappable) and its settled-peer recap (one tight
  list, and no empty block when nothing is settled), Propose Decree's connector chips, result line, plain addendum
  and quoted decree text, Exchange Courtiers' riposte and no-peer buttons (main and secondary), the Liaise delay
  reveal's single line and quotes, and that every title and its sentence share a `.titled` block (five in Propose
  Decree, four in Liaise). **They bite:** I broke the source twenty-six ways and each went red: a legacy class back,
  a used chip losing `aria-disabled` or its strike, a chip never selected, each typed symbol back, the decree in the
  wrong class, an inline margin back, each main button demoted, the old stacked meeting line, straight quotes, and,
  for the spacing, a negative margin back in either panel, each title unwrapped in turn, the italic addendum back
  and the recap ungrouped.
- **New e2e** (`reaim.spec.ts`): the Spread Rumors branch of Re-aim as a real pause (the subject destroyed over
  the dev route; the preparer is asked, the others wait, a pick moves, the pause clears for everyone), and the
  Make Demands branch. **The server can't reach that pause through any route** (it is a database state), so the
  second test gives the page the paused plan and the options by intercepting the two requests that carry them. It
  checks what the page does with them: the card picks with a tick, the button follows the pick, the chosen option
  is sent back verbatim, and a bystander gets only the wait line. The two selectors the other specs used
  (`.res-warning`, the decree locator) moved with the markup.
- **e2e mutations.** Four more breaks, each built into the app and run against its matching spec, each red at the
  assertion it guards: the picked Make Demands card without its tick (`reaim.spec.ts`: "Picked" not visible), the
  Re-aim button no longer waiting for a pick (both Re-aim specs: not disabled), the Liaise break warning as a bare
  paragraph (`clandestinely_liaise.spec.ts`: no `.notice`) and the decree under debate in the old class
  (`propose_decree.spec.ts`: the locator finds nothing). My first mutation build was empty: the script truncated the
  files before reading them, so every panel rendered nothing and the specs failed at their first line. I noticed
  because they all failed there and not where I had broken something, fixed the script and rebuilt; the sources
  were restored each time (`git status` clean).
- **Looked at, at 375px, before and after** (the "before" build is `main` as this branch started, at #86; "after" is
  this branch, rebased on #85), as every viewpoint that matters. All 101 states below were captured on both builds
  by a throwaway script that drives each plan over the API and screenshots it as each player; the PR holds a sheet
  for the 20 that changed most, with "Before" above "After".

  | plan | states | viewpoints |
  |---|---|---|
  | Clandestinely Liaise | preparation; delay reveal; Together at Last; Secrets We Keep (picked, and after submitting); Things We Share (idle, break with the warning, update, gift, after a submit); When Will I See You Again | preparer, partner, bystander |
  | Propose Decree | preparation; finalize; debate (pending, join, seated, declined, end the debate); a make (addendum, enact); a mar (amend, addendum, enact) | preparer and signatory, a player who may join, bystander |
  | Make Introductions | preparation; naming; a make (arrival); a mar (each fate, outcomes used, the authors' forms) | preparer, other players, bystander |
  | Exchange Courtiers | preparation; the trade (propose, offer, waiting); the roll; a make (picker, messy break); a mar (picker, riposte) | preparer, target, bystander |
  | Re-aim | an Exchange Courtiers pause; a Spread Rumors pause; a Make Demands pause (stubbed) | preparer, the plan's old target, bystander |

- **Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari or Firefox; hover and
  focus states (described in CSS, not photographed); 344px and the two-column width; the Make Demands Re-aim
  branch against a real pause (only the stubbed one, above); Liaise's perform_steps stand-in forms (existing
  specs cover them; I did not photograph them).

## Behaviour changes to look at before merging

None in what a panel does. In what it looks like, beyond what the rulings dictate:

- **The riposte's main button** (and "Proceed to dice roll" with nothing to trade): the style guide's button rule
  decides both; the question below is about how the riposte reads.
- **Screens are a little taller:** each step's own sentence is 16px now, where it was 14px.
- **Propose Decree's and Make Introductions' check and cross symbols are gone** (`Result: Make`, `Lady Vane —
  Another retinue`).
- **The Liaise delay reveal's peer names lose their deep-gold italics.** The game's colour is for headings; asset
  names in this app are plain italics elsewhere.
- **Re-aim's "Target player" gains a colon** with the shared field label, like every other picker's label, and the
  plan's name in its first line is no longer bold.

## Open questions for the owner

- [plan-panels-b-main-button](../questions/plan-panels-b-main-button.md): with "Break, then surrender" as the
  riposte's main button, does breaking now look like the default choice?

## Noticed, not fixed

- **`PlayerChips.svelte` still draws `.player-chip`** (see the `plan-step-labels` section). One shared file, used
  by panels in other pieces; best converted to `<Chip>` once.
- **A delay reveal is drawn by the table page with no plan card,** so the open step's heading sits above the
  quoted note instead of after the finished lines, unlike every other plan state. Drawing it inside a card
  needs `MainEventView` and Make War's panel too, which belong to other pieces.
- **Typed ✓ and ✗ are still drawn** by Spread Rumors, Spread Propaganda, Seek Answers, Chronicle Histories,
  Host Festivity and Propose Duel (as `✓ Make` / `✗ Mar`) and by Make Demands (as the tick on its cards), and
  `host_festivity.spec.ts` and `chronicle_histories.spec.ts` read the first. The shared `MakeMarPicker` already
  says "Result: Make".
- **The sub-step titles repeat the frame's heading's job** (see the `plan-step-labels` section).
- **Two preparation mirrors show the watcher's players and peers, not the preparer's.** Exchange Courtiers and
  Clandestinely Liaise build their player chips (and Liaise its "Your meeting peer" list) from the viewer, so a
  watcher who is the target or partner sees chips with nothing selected, and under Liaise's "Your meeting peer" the
  watcher's own peers. It was there before this piece; it is in the "before" sheets too. Tracked in
  `docs/work/next/exchange-courtiers-mirror-target.md`.
- **`planPanelsB.test.ts` has its own `ctxFor`,** the same shape as the one in `EndsNote.test.ts`; a shared test
  helper is a cleanup for later.
- **Screenshot 20's Make Demands pause is a stub.** No route makes a demand's target stop being pending (advancing
  onto a row always starts the demand first, never its target), so the page is given the paused plan and options,
  and told there is no roll, since a real pause comes before the demand's roll.
- **`D6Face.svelte`'s header comment still names `.chip-btn`** as how a face is selected; it is the shared
  reveal input's own markup (`SimultaneousRevealInput.svelte`), which is another piece's.
