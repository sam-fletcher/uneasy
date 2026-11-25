# Round 6: inside an open step

Branch `claude/round6-inside-a-step-bf0b94`. Builds the owner's Round 6 rulings on what is inside an open plan
step (the "Inside a step" block of the private decisions file, which is the spec), across every plan panel.

## What changed and why

**The step heading is two lines.** `lib/stepHeading.ts` now returns the step alone (`label`, capitalised) and
a structured who-line (`who`, `more`, `yours`), plus a plain `text` for tests and assistive tech.
`shared/StepHeading.svelte` draws it as one `<h2>`: a 14px who-line ("Your move" in bright gold, or faint
"Waiting on", then each owed player as a labelled `PlayerName`; at most two names, then "and others"; "Your move
· also ◆ alice" when you are one of several) over the 18px deep-gold step. It reads aloud as "Waiting on alice and
bob. The delay reveal." The callers (DiceRollPanel, PlanPanel, MainEventView, stepFrame, ResolvingCard, the table
page) pass the heading through unchanged, so only types and comments moved. A step label no longer has to finish
"Your move — …": `plans/stepLabel.ts` and `waitingOn.ts` now say it names the step alone. I left every label's
wording as it was.

**Type tiers.** A sub-heading (`.choices-header`, in about 35 plan files) is a 12px uppercase letter-spaced
deep-gold label. Every heading was rewritten to name its part once, with no colon, question mark or count (counts
became a plain note under it: "2 remaining."). `.picker-label` is the 14px secondary field prompt;
`FormField` no longer appends a colon. Sub-steps are separated by space only (the one hairline in an open step,
Make War's way-in rule, went). `.taken-count` is gone. `plans/subHeadings.test.ts` pins the look and fails a
sub-heading written the old way.

**Copy pass, every panel.** "Result: Make/Mar" headings became "Make options"/"Mar options" (the roll's finished
line already says the result); the Decree roster reads "The council" with "alice · signatory" (the "(preparer)"
tag went: the preparation line says who prepared); pickers no longer repeat the sentence above them ("Your
un-leveraged assets"); and "Waiting for X to …" lines that only repeated the who-line and the step label were
dropped or reworded to say only what the heading does not (Liaise, Decree, Duel, Exchange Courtiers, Chronicle,
Seek, Spread Rumors, Make War's delay reveal, Re-aim).

**The set piece.** New `plans/shared/SetPiece.svelte` and an `AssetName setPiece` modifier (italic deep gold,
18px, styled in `inlineText.css`). Used for Clandestinely Liaise's meeting (delay reveal; the scene is quoted
above it) and Propose Duel's pairing (setup step; "faces"). It has no difficulty line.

**Centred player-written text.** New `plans/shared/QuotedText.svelte`: a small deep-gold label with the player's
name in colour, then 16px text in `Quote`, at most 320px wide. Used for the decree in debate ("alice proposes"),
the law being written ("The law"), the rumor, the backfired rumor and the counter-rumor ("The rumor …"), each
with the optional "How it's spreading" note. Typed text stays left-aligned in its field; finished steps are
unchanged.

**The preparation line carries facts fixed at preparation.** `ResolvingCard` takes an `against` snippet:
"bob prepared Make Demands against alice's Seek Answers (row 5)", "alice prepared Propose Duel against bob",
"alice prepared Spread Rumors about bob's holding". The line wraps (`FinishedStep wrap`). Make Demands'
"Targeting:", Spread Rumors' "Targeting:" and Propose Duel's "vs" lead lines are gone; the open step still says
everything a player needs to act (the draft cards say what each option does to this target). The demand
overlay's banner became a sub-heading ("Demand from bob"); it is a standing effect, not a preparation fact.

**Docs and tracker.** `docs/STYLE_GUIDE.md` (type scale, Play steps, Inline text) and `/dev/styleguide`
(the six who-line states, sub-heading and field label, set piece, quoted text, preparation line). Tracker:
deleted the questions `plan-substep-lines`, `plan-lead-lines-group-gap`, `step-heading-wording` and the items
`plan-type-tiers`, `plan-set-piece-line`, `plan-finished-facts`, `step-heading-short-prefix` (and their lines in
`LEGACY_QUESTIONS`). `inline-text-names-in-strings` lost its "step heading wording" bullet: the who-line colours
names now. New item `set-piece-candidates`.

**After the coordinator's review.** Facts that had been moved are now kept: the duel type ("against bob · a duel of
arms") and Exchange Courtiers' subject ("for bob's Bob Courtier") are in the preparation line; the Make Demands
draft keeps "N of 4 picked." as a note; the reveal widget keeps "N of M submitted." as body text (the who-line names
at most two players). When a plan's aim has moved since preparation (a re-aim, or a demand winner settled it) the
line says "now against …": the data holds only the current aim, and a kept target can't be told from a changed one,
so it can't say what was prepared. A mar's Spread Rumors line keeps "about <asset>": it is a public preparation
fact, and the old "Targeting" line was hidden in a mar only because the rumor that spreads is then about the
preparer's main character. For each waiting line removed I checked that its verb survives in the step label; the
bout was the one that did not, so its label now says "declare the bout" or "respond to the bout" (`duelBoutLabel`,
which falls back to "the bout" when it can't tell, e.g. a stand-in). Every field label lost its colon and a source
scan holds it; the sub-heading scan now matches to the header's own closing tag (it had stopped at the first nested
`</span>` and never checked the "Make options" headings). The roll step's "Banked dice" is a sub-heading with its
count as a note; Spread Propaganda's message uses `QuotedText`; a law and its addendum are two quotes with the
app's "and"/"but" outside both; `StepHeading.text` is gone (tests read the rendered heading).

## How it was verified

Against this branch's own build (a private server on port 8192 and a private database):

- `make check-fast` passes.
- The full Playwright suite passes: 256 tests. About 60 e2e assertions had to follow the new heading text
  ("Waiting on alice. Set the scene") and the reworded lines; the first full run had 25 failures, all of that kind.
- 375px before/after shots, as the acting player and as a watcher, from one database (the same parked games under a
  build of the branch's start and under this branch): Liaise's delay reveal and last step, Propose Duel's pairing,
  Propose Decree in debate, a rumor (the form and the roll), Make Demands' draft, and the who-line in each state
  (you alone, one other, two others, you plus one, more than two others, you plus others; the last two from a
  five-player game). They are in the PR.
- `plans/subHeadings.test.ts` bites: writing "The council (2):" as a heading fails three cases.

Not run: `make test-integration` (no Go code changed).

## Behaviour changes to look at before merging

- **A heading's text is read as "Waiting on alice. The step"** (the full stop is screen-reader only). Every e2e
  assertion on heading text moved to that shape; `expectWaitingOn` now expects "and others" past two names.
- **Names in labels are upper-cased by the sub-heading style** ("ALICE PROPOSES"), as on the signed-off mockup.
- **The sub-heading look reaches the "What can happen" disclosure** (it was a `.choices-header`): it is now a gold
  label without the question mark.
- **`FormField` prints its label as written**, so labels that relied on the automatic colon lose it everywhere.
- **Removed lines**: the Make Demands/Spread Rumors "Targeting:" and Duel "vs" leads, Decree "(preparer)", the
  waiting lines listed above, the post-submit "Waiting on N others: …" in the reveal widget, and the Duel
  non-participant's "Facing …" line. A watcher of a duel's setup now has only the set piece and the stakes note.
- **The Duel pairing shows once, in the setup step.** Later steps (bouts, roll) name who they mean and draw no
  pairing; I judged repeating a staged moment in every step to cheapen it.
- **The Liaise delay reveal no longer has a "settling on a time…" line** for non-participants: the who-line says it.
- **Seed peers read oddly** ("alice's alice's main character") because the dev seed names a peer after its owner;
  real peers don't.

## Open questions for the owner

Two, each also a file in `docs/work/questions/`: whether the Duel pairing should show in later steps
(`duel-pairing-later-steps`), and whether the "What can happen" disclosure may wear the sub-heading look
(`disclosure-in-the-sub-heading-look`). Otherwise nothing new. The three `step-label-*` questions still stand, and the heading's new shape changes what they ask
only a little: they show the old "Your move — …" examples, but what they ask is the label's wording, which this
piece did not touch (the heading now shows the label alone, capitalised, under "Waiting on …"). The "you" in
Liaise's last label ("When will I see you again?") now stands alone as a heading rather than after a dash, so
the "label never speaks to the reader" point is, if anything, easier to see.

## Noticed, not fixed

- Make War's declaration and Make Introductions' newcomer as set pieces: filed as `set-piece-candidates`.
- Names built into strings are still plain (the existing `inline-text-names-in-strings` list); the who-line was
  the last one of those that is done.
- `ShakeUpView.svelte` still hand-writes a colon in a field label ("Marginalia text (optional — …):"), and
  `BattleCostForm`/`BoutsPhase` keep question-form field labels. Neither is a sub-heading.
- The Prologue's step labels ("Prologue — 3 of 15 chosen", "Initial Rankings: …") are written in a different
  shape from the plans' lower-case phrases. They read acceptably as a heading now, but are still unlike the rest.
- Prologue/Lobby/Shake-Up/ended still draw the heading from the table page (they have no step of their own).
