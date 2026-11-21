# plan-legibility-78ff06

Difficulty, "What can happen?" and the watcher's view, across the twelve plans. It was the tracker item
`plan-legibility-pass`, whose "wait for the redesign" line is met.

## Inventory, before the pass → after it

Checked against the code, not against the item's bullets (which date from before the redesign). Nothing the
item listed had already closed, apart from one half-gap: Make Demands' draft cards already say why an
option is inert (from the redesign), so only the live per-target lines and the Buffet were missing.

| Plan | Difficulty meter | "What can happen?" | What a watcher sees | How its pre-roll is built |
|---|---|---|---|---|
| Make Demands | none (a bare figure in the dice line) → **shared meter, sum spelled out** | none → **per target** (the draft's cards say the same) | draft: actor's cards only, others finished-pick lines (unchanged) | none: the roll opens at once (open question `make-demands-pre-roll`) |
| Propose Decree | none until the dice panel → **shared meter** | none → **make and mar** | council roster and the law text for all (committed state); join, amend, addendum are the actor's only (unchanged) | the council meeting: finalize text, open the debate, join or decline, end the debate; not `PreRollStep` |
| Exchange Courtiers | none → **shared meter** | yes | a waiting line → **greyed mirror** (the trade, the make, the mar, the messy break, the riposte's notes); the accept-or-decline is two buttons, still a line | the fair-trade step (target offers, preparer accepts or declines); no narration |
| Make War | n/a (no dice) | none → **cost of battle, surrender, peace** | sides and forms; the payer's form is the payer's only (unchanged) | the simultaneous delay reveal |
| Make Introductions | yes (own copy) → **shared piece, same words** | yes | a line → **greyed mirror of the mar's fates**; naming, arrivals and marginalia are text (not mirrored) | the peer-naming form (names only, as printed); not `PreRollStep` |
| Seek Answers | none → **shared meter** | yes | greyed mirror (pickers, sub-steps); the pre-roll was a greyed empty textarea → **nothing** (see below) | bespoke fieldset with a bare "Submit" → **`PreRollStep`** |
| Chronicle Histories | yes, gone at the cast → **stays until the dice land** | yes | greyed mirror (unchanged) | bespoke: tick artifacts and set the scene, one submit; stays its own (it sets the difficulty) |
| Clandestinely Liaise | n/a (no dice) | none → **the five shares, and meeting again** | a bystander is given nothing, by an earlier choice (unchanged) | none: four steps with no dice |
| Spread Propaganda | none → **shared meter** | yes | a line → **greyed mirror** (the mar picker; a peer handed over; an asset broken); the artifact form is text | `PreRollStep` |
| Spread Rumors | none → **shared meter, both branches** | yes | greyed mirror (unchanged) | `PreRollStep` |
| Propose Duel | a bare "· difficulty N" on the "ravi vs sol" line → **shared meter** | none → **make and mar** | the stakes and bouts for all, dice masked (unchanged) | none: a setup phase (who duels, stakes) |
| Host Festivity | none: the number is only on each guest's Roll button (unchanged, see the leftovers item) | yes | the guest list live (unchanged) | none: guests roll in any order |

## What changed and why

- **One place works out a difficulty** (`lib/plans/difficulty.ts`), mirroring the server's formulas, and one
  meter shows it (`shared/PlanDifficulty.svelte`, on the existing `DifficultyMeter`). Before the dice land it
  reads the rankings every client holds; once a roll exists for the plan its own value wins, so the client
  can only be wrong before the roll. The reason line is viewer-neutral and names the branch where it
  matters: Spread Rumors on a main character is its owner's esteem *status* (6 minus rank), on any other
  asset the preparer's own *rank*; Make Demands says "Seek Answers is difficulty 2; bea outranks ada on
  power by 3, which adds 3". Every rank-based plan says "a lower rank is better", the Chronicle wording
  from the first-timer pass. **After the difficulty vote the meter shows the vote's outcome** and says the vote
  moved it, so it never disagrees with the dice panel; the server writes the adjusted value only when the last
  vote lands (a Go integration assertion now pins that it is absent from the roll payload until then). A plan
  shows the meter and its Buffet until *its own* dice land (`diceNotYetLanded`), not whenever the page's
  `rollOutcome` is null, which another plan's roll (a counter-propaganda) would reset.
- **Seek Answers' pre-roll is `PreRollStep`** (prompt from the book, a button that says "Post & open the
  roll", the table-reads-it note), and shows its difficulty. I chose the label over splitting the step in
  two because that is what Spread Propaganda's and Spread Rumors' read as.
- **Chronicle's meter stays through the cast and the roll**; the invoke label no longer says every artifact
  raises the difficulty (it is the higher of the rank and the number invoked, so it is untrue until there
  are more artifacts than the rank): "more than the knowledge rank raises the difficulty".
- **Watcher mirrors** in Spread Propaganda, Exchange Courtiers and Make Introductions, on the shared
  recipe (`.resolve-mirror-wrap`): the picker greyed and inert, no button, rows out of the tab order, in
  the third person (`PlanChoiceOption.neutralDesc` for Exchange Courtiers' second-person descriptions, and
  prompts that say what the actor is doing, not what the viewer should do). A test per plan pins that no
  second-person word reaches a watcher. Exchange Courtiers' Messy tier now says the target *will* break
  (it is compulsory), and its make header names who the peer really goes to (a Make Demands winner holding
  keep-assets takes it instead of the preparer).
  Two shared fixes first: `CardPicker`'s marginalia rows now honour `readOnly` (they were operable from the
  keyboard in every existing mirror), and `MakeMarPicker` has a read-only mode.
- **"What can happen?"** for Decree, Duel, War, Liaise and Make Demands (`lib/plans/buffets.ts`), from the
  book. War and Liaise have no make or mar, so theirs describe their own options. Make Demands' is built
  per target by `demandOptionLine`, and **the draft's cards use the same line**, so the reference before the
  dice and the cards where players choose cannot disagree (the cards had said only the generic effect).
  Control leverage says the winner chooses *how many* assets go onto the roll; on Propose Duel the stand-in
  plays the preparer's side of the bouts and claims after a make; on Propose Decree (no options) it enacts
  the law, and places the addendum when the preparer is the signatory.

## How it was verified

Against this branch's own code (a private server on `:8172` and a private database; a baseline build of
`main` on `:8171` and its own database, for the "before"):

- `make check-fast` passes: 113 test files, 2195 tests (it was 105 and 2011), `svelte-check` clean, `go vet`,
  the Go unit tests, the git-guard tests, the tracker check. **`make test-integration` passes on a private
  database** (the review round added one Go assertion, which fails if the server writes the adjusted
  difficulty before the reveal: checked by making it do so).
- **e2e: 140 specs across the eleven touched plans, all pass (run again after the review round)** (`seek_answers`, `spread_propaganda`,
  `spread_rumors` (+ its secret-draft spec), `exchange_courtiers`, `make_introductions`,
  `chronicle_histories`, `make_demands`, `propose_decree`, `propose_duel`, `make_war`,
  `clandestinely_liaise` (+ its secret-text spec)). I edited five specs because they asserted the old
  views (a watcher gets no picker; Seek's disabled textarea; "Submit"; Chronicle's old reason line and the
  meter gone at the cast).
- **Every new unit test bites.** About 60 mutations, each turned a test red and was restored: the meter
  removed, shown for ever, or gone at the cast; a mirror not marked inert, or its rows operable, or showing a
  button; a watcher given the second-person wording; the status read as a rank; Chronicle as a sum, not a
  max; a Spread Rumors main character read off the preparer's rank; the roll ignored; the Buffet missing,
  or shown to a bystander. Four were first not caught (a regex that could not match, an assertion that
  could not fail, a mutation that did not apply, and a state the server render cannot reach, the preparer's
  live tick count in Chronicle, which only the e2e spec sees) and the tests were fixed.
- **Hidden information:** every mirror is built from the public retinue, revealed dice and the plan's own
  data, never from a per-viewer record. A test per plan pins that two watchers read the same page, that no
  mirror holds a text field or a submit button, and that every count is zero (an uncommitted pick is local
  state; no resolution draft channel exists). No payload was widened and no Go changed.
- **Looked at, at 375px, before and after, as the actor, the other party and a bystander** (screenshots in the
  PR): Seek Answers, Spread Propaganda (pre-roll, mar picker, the picks owed), Spread Rumors, Exchange
  Courtiers (the offer, a mar), Make Introductions (pre-roll, the mar's fates), Make Demands, Propose Decree,
  Propose Duel, Make War, Clandestinely Liaise and Chronicle Histories (before and after the cast).
- **Pinned only by e2e:** Spread Propaganda's dice fetch for watchers is an `$effect`, which the server render
  the unit tests use does not run.
- **Not run against this code:** a real phone; Safari or Firefox;
  1280px; hover and focus states; a table of four or five; Exchange Courtiers' make as a screenshot when the
  target's difficulty exceeds the actor's two dice (the e2e spec drives it); the messy break and riposte
  mirrors as screenshots (unit and e2e only); Make Introductions' mirror after one fate is already struck
  through (unit test only).

## Behaviour changes to look at before merging

- **Seek Answers: the pre-roll button reads "Post & open the roll"** (it read "Submit" while also casting
  the dice). **A watcher no longer sees an empty greyed textarea** at that step: `PreRollStep` gives
  non-actors nothing, and the heading says whose move it is. The difficulty is there instead.
- **A watcher now sees the picker, greyed, in Spread Propaganda, Exchange Courtiers and Make
  Introductions.** Every other plan with a mirror already did.
- **The meter and the Buffet go when the dice land** (Chronicle's now stays through the cast and the roll).
  Whether they should stay longer: [plan-meter-after-the-dice](../questions/plan-meter-after-the-dice.md).
- **Propose Duel's bare "· difficulty N" is gone from the "ravi vs sol" line**; the meter carries it.
  "ravi vs sol" is unchanged (its placement is an open call, `plan-lead-lines-group-gap`).
- **Every player fetches the roll's dice on a Spread Propaganda mar** (the route is open to the table, every
  die is revealed by then); the counter-propaganda row needs it to grey the same options for a watcher.
- **Exchange Courtiers' make and mar headers** speak of the preparer to a watcher ("ravi beat the difficulty
  by 1 … transferred to ravi"); the mar's says only how far it marred, since the heading says who responds.
- **Chronicle's label** reads "(more than the knowledge rank raises the difficulty)".
- **The meter follows the vote**: after a difficulty vote it shows the new number, with "The vote moved it from
  3 to 5."
- **Make Demands' draft cards** say what each option does against this target (or "No effect here: …", once),
  where they had said the generic effect.
- **Exchange Courtiers:** Messy reads "will break"; the make header names the real recipient of the peer.
- The five new Buffets' wording beyond the book: the questions below.

## Open questions for the owner

One file each:

- [decree-buffet-wording](../questions/decree-buffet-wording.md): does the mar's lead line say more than the
  book?
- [war-buffet-wording](../questions/war-buffet-wording.md): where the war's Buffet sits.
- [liaise-buffet-wording](../questions/liaise-buffet-wording.md): for the two players only, in the first two
  steps.
- [plan-meter-after-the-dice](../questions/plan-meter-after-the-dice.md)

**Settled by the existing rulings, so not questions** (I had filed some and the review showed they were
answered):

- *Make Demands' per-target lines:* keep-assets is mechanical (the winner receives what the preparer would
  gain; on an exchange only the incoming side), perform-steps replaces the preparer's role one for one and is
  post-roll only (a mar chosen by the plan's own target stays with it), keep-or-change changes who and never
  what, and a dud shows its reason on the card. The lines are readings of those, and the cards share them.
- *Propose Duel's mar:* the opponent may take exactly the difficulty or spare the loser, all or nothing.
- *Text steps are not mirrored:* a watcher is shown no live draft of a resolution step, so a mirror of a text
  box could only ever be empty. Exchange Courtiers' accept-or-decline is not a text step; it is in
  `plan-watcher-leftovers`.

## Noticed, not fixed

- **Make Demands' draft picker has no watcher mirror**, nor do Decree's join-the-council picker, amendment
  and addendum, nor Exchange Courtiers' accept-or-decline. They were not in the item; the new tracker item
  `plan-watcher-leftovers` lists them with Host Festivity's difficulty (a number on the Roll button).
- **Exchange Courtiers' and Chronicle's older Buffets are second person** ("You take the targeted peer"); the
  five new ones are in the third. A pass over the old ones would make them agree.
- **Spread Propaganda refetches the roll's dice** though `MainEventView` already holds them from the same
  endpoint; passing them through the plan context would drop the duplicate fetch, at the cost of widening the
  context for every panel. Left as is.
- **`statusOf` exists twice** (`lib/plans/difficulty.ts` and `ProposeDuelPanel.svelte`, for the stake limit);
  left alone, the duel's clamps differently.
- **`make-demands-pre-roll`** (no narration step for a demand) is still open; with a pre-roll the meter
  would sit above a real step instead of under the "Targeting" line.
- **The Spread Propaganda `forfeit` route's "Skip" buttons** are the actor's only, as before; a watcher sees
  the "nothing left to give" note but not the button.
- **A Svelte trimmed leading space glued "decline" to its dash** in Spread Propaganda's counter-propaganda row;
  fixed there (explicit space, with a test that strips tags without adding spaces, which is what hid it from
  the first version). Seek Answers has the same trap (`seek-answers-empty-picker-notes`).
- **`make check-fast` needs `make frontend` first in a fresh worktree** (the embedded build is missing
  otherwise), as an earlier note said.
