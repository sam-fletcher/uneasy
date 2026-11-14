# plan-step-labels-d68aa8

(Named after the branch, not the worktree folder: the folder, `redesign-plan-panels-b-25b28c`, is the name of the
wave-5 session whose note is already in this directory.)

Each plan now names its open step in the heading, and the panels stop printing the lines the heading and the roll say.
The heading no longer reads "Your move — the next step" for a plan's own steps: it reads "Your move — place the
addendum", "Waiting on otto — choose the options". Behaviour is unchanged (same routes, events and rules); what
the table is told, and what the panels print under the heading, changed.

## What changed and why

**1. The heading names the open step.** The server only knows a plan is `plan_resolving`. The plan's own
resolution data knows which step. Each plan registers one pure function (`lib/plans/stepLabels/<plan>.ts`, listed in
`plans/registry.ts`, whose entry now requires one), and `mainEventWaitingOn` asks it through the row state's
`plan_id`, with the plan's own roll (`ownRoll` drops another plan's). I chose this over the tracker item's shape, a
hook through which the open panel tells its card, because the page draws the same heading before a panel's lazily
loaded chunk has arrived, so with a hook the two would have disagreed and read "the next step" in that moment; one
call for both, testable without mounting anything. The cost is that a label mirrors its panel's step conditions;
`lib/plans/stepLabels.test.ts` feeds each one the data of every step it names (and checks every label is a
lower-case phrase, never speaks to the reader and never names its plan).

It applies to `plan_resolving`, `plan_pending` and to four kinds that span several asks of one plan:
`liaise_resolving` (three phases, all "the liaison") and `await_courtier_response` (a fair-trade offer, a mar's choice
and a messy break, all "respond to the exchange"), and the two where one kind holds two different asks at once for two
different people: `await_question_answer` (the target's answer, and the actor's own picks) and
`await_introductions_marginalia` (an author's marginalia, and the actor's fates). Every other kind keeps its own label.
That is a small extension of the brief ("Liaise's four phases"); each falls back to its old label when the plan can't
say.

**Where one wait holds two asks, the heading names what they share** ("form the council", "the question and the picks",
"the newcomers"), never one of them: a phrase for either ask tells the other person to do the wrong thing. The review
found two of these that I had missed (the Seek Answers question and the Make Introductions mar; the deleted "Waiting
for X to answer Y's question" line used to correct the first for the actor). I chose to route their kinds through the
plan's own label and name what the asks share, over restoring such a line for the viewer it is wrong for, because a
heading that is wrong at the top of the screen for one of the two people it names is the fault, and a line under it
only patches it, in every panel, for ever. Each state is a test (`stepLabels.test.ts`, `waitingOn.test.ts`) and the
three are in one question file.

Each phrase finishes both "Your move — …" and "Waiting on otto — …". **new** = a phrase this piece adds (it replaces "the next step", or for Liaise and Exchange Courtiers' target, a vaguer kind label). *existing* = a step the server already names with its own kind, unchanged, listed so the table covers the whole plan.

| plan | step | phrase |
|---|---|---|
| **Chronicle Histories** | invoke artifacts and set the scene | **set the scene** |
| | the roll | *the roll* (existing) |
| | a make option, one at a time | **choose a make option** |
| | every make option made, or a mar everyone has answered | **wrap the scene** |
| | a mar's one choice each | *everyone chooses* (existing) |
| **Seek Answers** | the preparer says what was learned | **describe what was learned** |
| | the options, once the dice landed | **choose the options** |
| | one kind of pick left | **break a resource** · **learn an asset's secrets** · **declare a truth** · **ask a question** · **pay the mar penalty** |
| | several kinds left | **carry out the picks** |
| | a question put to a player (the target's) | *answer the question* (existing) |
| | a question in flight while the actor still owes picks (two asks) | **the question and the picks** |
| | nothing owed | **complete the plan** |
| **Spread Rumors** | state the rumor | **state the rumor** |
| | a mar opens with the counter-rumor | **write the counter-rumor** |
| | the options | **choose the options** |
| | one pick left / several | **break the target asset** · **hide the source** / **carry out the picks** |
| | a take's consent | *consent to the take* (existing) |
| | nothing owed | **complete the plan** |
| **Spread Propaganda** | state the message | **state the message** |
| | a mar's options | **choose the mar options** |
| | one effect owed | **create the artifact** · **give up a peer** · **break an asset** |
| | a peer and an asset together | **face the fallout** |
| | a counter-propaganda | *counter-propaganda* (existing) |
| | nothing owed | **complete the plan** |
| **Make Introductions** | name the peers (finalizing casts the roll) | **name the new peers** |
| | a make: an arrival form per peer | **describe each arrival** |
| | a delayed traveller | **describe the delayed arrival** |
| | a mar: a fate per peer | **settle each newcomer** |
| | only a newcomer's marginalia someone else owes | *describe a newcomer* (existing) |
| | fates still to settle and a marginalia still owed (two asks) | **the newcomers** |
| **Exchange Courtiers** | the target names a peer | **propose a fair trade** (was *respond to the exchange*) |
| | the preparer takes or leaves the offer | **accept or decline the trade** |
| | a make, after the dice | **say how the trade went** |
| | a mar, after the dice (the target's) | **choose how to respond to the mar** (was *respond to the exchange*) |
| | a riposte | **break or surrender the peer** |
| | a messy break (the target's) | **break a marginalia** (was *respond to the exchange*) |
| **Propose Decree** | the council forms (the preparer's text, others join: two asks) | **form the council** |
| | the debate is open, decisions owed | **join or decline the council** |
| | every decision in | **end the debate** |
| | a mar: each council member amends | **amend the law** |
| | the signatory's addendum | **place the addendum** |
| | enacting | **enact the law** |
| **Propose Duel** | the stakes | *stake for the duel* (existing) |
| | the bouts | *the bout* (existing) |
| | the claim after a make | **claim the stakes** |
| | the claim after a mar (may be spared) | **take the stakes or spare them** |
| **Host Festivity** | every step of the event | *the festivity* / *answer the challenge* (existing) |
| **Make Demands** | the four-option draft | *draft pick* (existing) |
| | the counter-demand | *the counter-demand* (existing) |
| | draft done or counter placed, before it completes itself | **complete the plan** |
| **Make War** | the delay vote, a cost of battle, a surrender claim, a peace vote | *the delay reveal* / *pay the cost of battle* / *surrender-asset claims* (existing) |
| **Clandestinely Liaise** | the four phases | **together at last** · **secrets we keep** · **things we share** · **when will I see you again?** (the last three were *the liaison*) |

Still "the next step" (the plan cannot tell, or the moment is too short to be a step a player waits in):

- a plan the page has not loaded yet, or whose id the row state doesn't carry;
- a plan still *pending*, before the server kicks it off;
- Make War resolving (it resolves in one go; its steps have their own kinds);
- the instant after the dice land and before the server records the result (Propose Decree, Make Demands before the draft);
- Make Introductions with every arrival described or every fate settled, and Exchange Courtiers or Propose Duel after their last step: the server completes the plan itself;
- Clandestinely Liaise before its first phase is written, and Propose Decree's law after it is enacted.


**2. Deleted what the heading and the roll now say.** Every "Dice roll in progress…" (Chronicle Histories, Exchange
Courtiers ×2, Make Demands, Make Introductions, Seek Answers, Spread Propaganda, Spread Rumors, Propose Decree with
"Recording the result…", the duel's "— resolve above", the festivity's "Rolling…"); every "X is resolving …" and its
kin ("is researching…", "is naming the new peers…", "is welcoming…", "is choosing options…", "is writing the rumor that
spreads instead…", "Waiting for X to enact the law…", "…to place the addendum…", "…to pick…", "…to place a
counter-demand…", "…to claim stakes…", "…to complete the plan…", "…to author the artifact…"); `PreRollStep`'s wait line
(and its `actorName`/`waitingVerb` props); Make Demands' "Your turn to pick."; and the sub-step titles the heading says
word for word: Propose Decree's "Amend the law (your turn)" and "Enact the law", Liaise's four phase titles, Make
Demands' "Counter-demand", the war delay vote's "Vote for the war's delay" (and its "Other players may join either
side below."). The test for each: would a watcher still learn what it said? A line that says more stays.

Kept on purpose, because each says more than the heading: consent's "Waiting for bob to agree to give up Dame Orr…"
(names the asset), Exchange Courtiers' offer lines (name the peer) and riposte line, the bout's "Waiting for X to
declare/respond", the delay reveal's "Waiting for participants to reveal their delay dice…" (the brief listed only the
other two lines of that block), the Liaise delay reveal's "X and Y are settling on a time…", the signatory's "Waiting
on bob, carol to decide if they want to join the council." (it explains a disabled button), Chronicle's "The scene stays
open until X wraps it" and Decree's "X is finalizing the decree…" (the heading before the debate is "form the council").
Propose Decree's "Proposed decree being debated:", "Finalize your decree" and "Join the council?" stay too: the heading
doesn't say them. `plans/headingSaysIt.test.ts` keeps the dice and "is resolving" lines from coming back.

**3. The class-name bullet moved to `plan-group-gap`** (`PlayerChips`→`<Chip>`, `.choice-card`'s shared option card,
`.chip-btn.face-chip`, the unused `.res-warning` rule, and the STYLE_GUIDE "older class names" paragraph). I edited that
tracker file for it, as the brief allowed, and deleted `plan-step-labels.md`. I did not touch `planPanel.css`'s
rules (only two comments that quoted the deleted lines) or the STYLE_GUIDE paragraph. `step-heading-wording.md` lost
its first bullet (the "the next step" stopgap question), and its title with it.

## How it was verified

Against this branch's own code (a private server on `:8173`, a private database, never `uneasy_test`; the docker
containers serve `main`).

- `make check-fast` passes (rerun on the final tree): 86 test files, 1732 tests, `svelte-check` with 0 errors and 0 warnings, `go vet`,
  `golangci-lint`, the Go unit tests, the git-guard tests and the tracker check.
- **New unit tests:** `lib/plans/stepLabels.test.ts` (every step of every plan, plus the grammar rules),
  `lib/waitingOn.test.ts` (the plan's own step through the row state, the acting set where one phase has two asks, the
  plan's own roll only, and the fallbacks), `components/plans/headingSaysIt.test.ts`. **They bite:** I broke the code
  ten ways and each went red at the assertion that guards it: `waitingOn` no longer asking the plan (3 failures),
  Decree's `end the debate` branch (2), a panel printing "Dice roll in progress…" again (`headingSaysIt`), `PreRollStep`'s
  wait line back (its test), the registry wiring Chronicle Histories to Seek Answers' function (5), and a pending plan
  reading its prepare-time draft as a step (1); after the review, the Seek Answers and Make Introductions two-ask states
  (2 each), Chronicle's `??` back for the panel's `||` (1) and the `await_question_answer` routing (1). Each was restored
  from a copy.
- **e2e: the full suite, 215 specs, on this tree.** The first run was 205 passed and 10 failed, all because a spec still
  waited for a line I had deleted: my search for them had been cut short. Ten specs now read the heading, and the six
  files that held them pass (60 tests). **The final run, on the committed tree rebuilt after the last change, passed
  all 215 specs again after the review's changes** (2.9 min), with the Seek Answers question, the Make Introductions mar
  and Decree's council now asserted in the specs that cover them.
- **Looked at, at 375px, before and after,** for a resolving step of every plan, as the actor and a bystander (and the
  target where there is one): the same dev-seeded states driven on `main` and on this branch by a throwaway script
  (`plandriver` plus the plans' own routes), all in the PR as contact sheets. The headings read from the live page on
  every one of those states are in the PR.
- **Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari or Firefox; 344px and the
  desktop width; the steps I could not reach through the routes (see below).

States photographed: Chronicle Histories (set the scene); Seek Answers (pre-roll, options); Spread Rumors (pre-roll,
options); Spread Propaganda (pre-roll, the artifact); Make Introductions (naming); Exchange Courtiers (the offer, as the
target, the preparer and a bystander); Propose Decree (the council); Propose Duel (the stakes, unchanged); Host Festivity
(unchanged); Make Demands (the counter-demand, from a forced mar: a forced make failed with "the actor has only 2 dice");
Make War (the delay vote); Clandestinely Liaise (the delay vote, together at last, secrets we keep).

**Not photographed** (the deleted line only shows mid-flow, and I could not reach it through the routes): the duel's
final roll (`— resolve above` and the claim), a guest's roll in the festivity, Decree's mar amendments, addendum and
enact, Exchange Courtiers after the roll, Make Introductions' arrivals and mars. Their labels are unit-tested and
several are read from the live page in e2e (Decree's addendum, enact and amend; the duel's claim after a make and a
mar; Exchange Courtiers' messy break and mar choice).

## Behaviour changes to look at before merging

- **The heading's words** (the table above). They are the owner's taste; three phrases rest on a judgement and have
  question files: Liaise's phase names (one has a "you" in it), the shared names where one wait holds two different asks
  ("form the council", "the question and the picks", "the newcomers"), and "carry out the picks" / "face the fallout"
  when several kinds of pick are open at once.
- **A watcher sees less under the heading.** The "X is resolving…" and "Waiting for X to…" lines are gone; the heading
  carries them. In Liaise, a bystander sees the heading and nothing else (the old "A clandestine liaison is underway."
  said less than the heading does).
- **Exchange Courtiers' target now reads "propose a fair trade", "choose how to respond to the mar" or "break a
  marginalia"** where it read "respond to the exchange" for all three.
- **Liaise's first phase waits on the preparer alone** ("Waiting on ana — together at last") while both players are in the
  scene. That is the server's `acting_player_ids`, unchanged; the heading just says it more plainly than "the next step".

## Open questions for the owner

- [step-label-liaise-phases](../questions/step-label-liaise-phases.md): the book's phase names as labels, and "when will
  I see you again?" with its "you".
- [step-label-two-asks](../questions/step-label-two-asks.md): where one wait holds two different asks (Decree's
  council, a Seek Answers question beside the actor's picks, a Make Introductions mar), the heading names what they
  share ("form the council", "the question and the picks", "the newcomers").
- [step-label-several-picks](../questions/step-label-several-picks.md): "carry out the picks" and "face the fallout".

## Noticed, not fixed

- **A preparer with no peer to trade is named as the wait's target.** In Exchange Courtiers' opening step the server
  names the target ("Waiting on bob — propose a fair trade") even when the preparer has no peers, in which case the
  panel gives the preparer a "Proceed to dice roll" button and the target nothing to do. `ResolvingWaitees` names the
  target whenever there is no offer. Pre-existing; the old heading said "respond to the exchange" for it.
- **Make Demands' "Targeting: …" line and the demand overlay's lead line** state facts about the plan among the open
  step's lines: tracker item `plan-finished-facts`.
- **The typed ✓ Make and ✗ Mar** in Seek Answers, Spread Rumors, Spread Propaganda, Chronicle Histories and the
  festivity (`host_festivity.spec.ts` and `chronicle_histories.spec.ts` read them), where the shared `MakeMarPicker`
  already says "Result: Make": tracker item `plan-typed-results`.
- **`plan-type-tiers`'s Liaise example is out of date:** its sub-step "When will I see you again?" is gone, and the
  heading reads "Your move — when will I see you again?". I did not edit its file (the brief allows two others).
- **Not every plan's wait names the plan's step when it is the target's** (e.g. Make Demands' draft is `draft pick`, the
  duel's bout is `the bout`): their own kinds, unchanged and unrefined.
- **A roll that belongs to another plan on the same row degrades every label that reads `roll`.** `ownRoll` drops a
  roll whose `plan_id` isn't the plan's, so when the table's one open roll is another plan's, Spread Rumors' mar shows
  "choose the options" before the counter-rumor is written, and Exchange Courtiers' roll phase says "the next step".
  The panel reads the same `ctx.activeRoll`, so it is not a label-only bug: it shows the same wrong step. Found in
  review; not fixed.
- **`stepLabels.test.ts` feeds hand-written data,** so a panel whose step conditions drift from its label fails no test.
  The label tests prove the labels against the data of each step; nothing checks the panel and the label agree on what
  each step is. The e2e specs that read the heading are the only cross-check, and they cover some steps.
- **A stray `make_demands` dev game** was left in the main dev database when `plandriver setup make_demands -force make`
  failed after creating it ("the actor has only 2 dice"); I could not name its id from the failed output.
- **`planPanelsB.test.ts` still says "the five panels"** in several names, and its `expectTitled` helper now serves four
  Decree titles; fine, noted for the cleanup that moves the next panels.
