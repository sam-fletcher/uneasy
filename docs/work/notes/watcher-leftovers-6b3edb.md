# watcher-leftovers-6b3edb

The pickers the legibility pass left a watcher as a line (tracker item `plan-watcher-leftovers`, first bullet),
and Seek Answers' two empty-picker pairs (`seek-answers-empty-picker-notes`, deleted). Builds on
`notes/plan-legibility-78ff06.md`, whose mirror recipe (`.resolve-mirror-wrap` in `plans/planPanel.css`, a
card picker's `readOnly`) every mirror here uses.

## What a watcher sees now

| Step | Before | After |
|---|---|---|
| Make Demands' draft | heading and the finished-pick lines | the **four option cards, greyed and inert**: those still on offer with the per-target line, a taken one ticked and "Taken by <name>" |
| Propose Decree's join-the-council | nothing, or a line | **each pending joiner's picker, greyed and inert**: that player's own un-leveraged assets, under "<name> may join the council" |
| Exchange Courtiers' accept-or-decline | "<target> asked for X… Waiting for <preparer>'s decision…" | the same line, plus **what each answer does** (a sentence, no buttons) |
| Decree's amendment and addendum | a line | unchanged: text steps stay lines |

The actor's view of each step is unchanged: the screenshots before and after are byte-identical for the actor
of all three.

## What changed and why

- **Make Demands.** Everyone who is not the next picker gets the board inside the shared mirror (a disabled
  fieldset). The mirror lists **all four** cards where the picker lists only those left, because the brief asked
  for the taken ones to be marked. A taken card is the house "picked" look (tick) with "Taken by <name>" as its
  line; the other three carry `demandOptionLine`, the same sentence the picker and the "What can happen?" say.
  When nothing left does anything, a watcher reads "None of what's left does anything against this plan." without
  the "pick whichever" the picker gets. The finished-pick lines stay, outside the mirror, so each taken option
  is now said twice (the line gives the order, the card says who holds it).
- **Propose Decree.** A viewer who owes no join-or-decline decision sees one mirror per pending decider
  (`pendingDeciderIDs`, the same set the signatory's "Waiting on" line uses). A player who still owes the decision
  has the live picker and is shown nobody else's; a seat that has decided sees the pickers still open. The
  explanation ("They may leverage one asset to join the debate, or decline…") is said once, under the first.
  Joining is open from the start of the council, so the mirrors are there before the debate opens too. They sit
  above the signatory's "Waiting on…" line and the (disabled while any mirror shows) "End the debate" button.
- **Exchange Courtiers: a sentence, not greyed buttons.** The step is two buttons, not a pick. I chose a
  third-person sentence ("If ravi accepts, the peers swap and Exchange Courtiers resolves with no roll. If ravi
  declines, it goes on to the dice.") for the target and any bystander, because (1) the house convention is that a
  button only its actor can press is left out, not shown dead (a Make Demands "Pick option", Seek Answers'
  "Apply choices" are all hidden from watchers); (2) the actor's step is about 200px at 375px (two stacked buttons
  and a note), and a dead copy would cost most of that to say what a sentence says; (3) there is no picker to
  mirror. **The brief called this the target's decision; in this step it is the preparer's** (the target made the
  offer and waits), and the text says so. "The peers swap" is only true while the preparer is who receives the
  targeted peer: a Make Demands keep-assets winner takes that side in the preparer's place and the offered peer
  still goes to the target, so with one the sentence names who gets which peer. The actor's own note under the two
  buttons had the same flaw from before and says the same now (`acceptClause`, one string for both).
- **Seek Answers.** Each empty-picker pair now reads as one explanation, as the break-a-resource pair does: the
  picker says what is empty, the note says what that means for the pick.
  - Learn an asset's secrets: "No asset's secrets remain to learn." / "This pick has no valid target."
  - The mar penalty: "None of your resources can be broken." / "This penalty can't be applied."
  - The penalty's picker is drawn greyed for everyone else too, so its message says "your" only to the preparer
    (whose resources they are, though a Make Demands winner is the one acting) and names them for everyone else,
    the stand-in included ("None of ravi's resources can be broken."). It was "No eligible resources of your own
    remain." to a watcher.
  - The stepper's note ("— no valid targets", "— all targets picked") was glued to its label ("…of an
    asset— no valid targets"): a note's leading space is trimmed inside its own span, so the space is now outside it.

## How it was verified

Against this branch's own code: a private server (`:8175`) built from it, and the baseline (`main`, which this
branch started from) on `:8174`, both on a private database.

- **`make check-fast` passes** (114 test files and 2273 tests, from 2195; `svelte-check` clean, `go vet`, the Go unit tests, `deadcode`,
  the tracker check). `make test-integration` was not run: no Go changed.
- **e2e: the four specs pass against my build** (`make_demands`, `propose_decree`, `exchange_courtiers`,
  `seek_answers`: 50 specs; then `propose_decree` and `exchange_courtiers` again, 22, with the assertions I added
  afterwards). Since #116 a spec fails on an uncaught page error or an unexpected `console.error`; none did.
  After the review round the four ran again together against a fresh build: 50 specs, all pass.
- **Specs and tests I changed because they asserted the old view:** `make_demands.spec.ts` (a bystander and the
  other drafter now have four disabled cards, no "Pick option", and a taken card is marked), `seek_answers.spec.ts`
  (pins the new pair), `planPanelsC.test.ts` ("shows the player who is not up no option cards"). Added to
  `propose_decree.spec.ts` and `exchange_courtiers.spec.ts`: what the watchers see.
- **Every new unit test bites.** 33 mutations, each turned a test red and was restored (the working tree compared
  against a snapshot after): mirror cards not disabled, fieldset not disabled, fieldset without the mirror class,
  taken card unmarked or not saying who, the generic effect instead of the per-target line, "pick whichever" told to
  a watcher, only the remaining cards, the next picker losing the live picker; Decree: every asset offered
  (leveraged included), picker not `readOnly`, fieldset not disabled or not a mirror, second-person text, the
  mirror shown to the player who decides, the empty message dropped, a mirror for every player; Courtiers: the
  sentence missing for the bystander or the target, in the second person, a greyed Accept button added; Seek
  Answers: each old wording restored, "your" said to a stand-in, the stepper space removed. **One stayed green and
  the test was fixed:** `\bresolve-mirror\b` also matches `resolve-mirror-wrap` (a `-` is a word boundary), so a
  mirror that lost its class still passed. Review then asked for one matcher: `plans/mirrorMatchers.ts` matches
  the class as a whole token, and all five test files that look for a mirror (mine, and #114's Spread
  Propaganda, Make Introductions and Courtiers) use it, with its own test. Stripping the mirror class from
  `MakeMarPicker` now turns four of #114's Courtiers tests red; it turned none red before. Six more mutations,
  all caught: the keep-assets sentence always saying "swap", the preparer's note not following it, "your"
  said to a stand-in or to nobody, and the matcher losing the whole-token rule or its `disabled` check.
- **Inert, and no second-person word, per mirror:** Make Demands and Decree have a test that every control
  inside the mirror is disabled (a disabled fieldset, `aria-disabled="true"` and `tabindex="-1"` on every card
  row, nothing in the tab order, no input, textarea, select or link, no Pick, Join or Decline button) and that no
  `you` or `your` reaches a watcher. Courtiers' sentence has the same for the target and a bystander, and a test
  that the preparer still gets exactly the two buttons.
- **Hidden information.** Every mirror is built from data the watcher's client already holds: the draft's picks
  (public `resolution_data`) and the target plan (public record) for Make Demands; every player's retinue and each
  asset's leveraged flag (public) for Decree; `fair_trade_asset_id` (public `resolution_data`) for Courtiers. A
  test per mirror renders it for two different watchers and compares the markup. No payload was widened and no Go
  changed. The asset cards' secret eyes are the viewer's own, through the context every card already reads.
- **Looked at, at 375px, before and after, as the actor, the other party and a bystander** (screenshots in the PR):
  Make Demands (one pick taken; none taken), Propose Decree (two pending joiners; after one declines), Exchange
  Courtiers (the offer, as the preparer, target and bystander; and again under a keep-assets winner, as the
  preparer and the winner), Seek Answers (both pairs, as the preparer and a watcher).
- **Not run against this code:** a real phone; Safari or Firefox; 1280px; hover and focus; a screen reader;
  a table of four or five; a retinue bigger than four assets (the seed gives four); the "all targets picked"
  stepper note (a server render cannot reach it: the count is local state); the e2e assertions I added were not
  themselves mutation-checked (they would need a rebuild per mutation); a Make Demands draft where all four
  options are inert (unit-tested only).

## Behaviour changes to look at before merging

- **Make Demands: each taken option now appears twice for a watcher** (the finished-pick line and the ticked card).
  Dropping the cards' taken half, or the lines for watchers, is a few lines; I kept the lines as committed state
  and the cards because the brief asked for the taken ones marked.
- **Propose Decree: the page gets longer for anyone watching a join** (one picker per pending joiner; about
  400px more in a three-player seed). It does not cost the signatory a live button (it is disabled while any
  mirror shows), only pushes their waiting line down. Question below, with a recommendation.
- **Exchange Courtiers: a sentence, not buttons** (above). The target sees it too, under their own second-person
  line.
- **Seek Answers: the penalty picker now names the preparer to a watcher and to a stand-in** (it said "your"),
  and still says "your" to the preparer.
- **Exchange Courtiers, with a keep-assets winner:** the sentence under the offer, and the preparer's own note
  under their buttons, say who gets which peer instead of "the peers swap".
- **A mirrored card is a disabled control, so a screen reader still announces it, as unavailable.** That is how
  every earlier mirror (`MakeMarPicker`, `CardPicker`) works; hiding the mirrors from assistive tech would take
  the information from the people who most need it. I read "reachable by a screen reader" as "operable".

## Open questions for the owner

- [decree-join-mirror-length](../questions/decree-join-mirror-length.md): a full picker per pending joiner, or a
  line saying who is deciding? And should the signatory's own step sit above the mirrors?

## Noticed, not fixed

- **Seek Answers' penalty heading and header are second person to people they are not about.** The heading
  "Describe a flaw in your own resource assets" says "your" to every viewer, watchers included, and the line under
  it ("you must break 1 of your own resources") says "you" to a Make Demands stand-in and "the preparer" to the
  preparer when a stand-in acts (`canSubflow` without `isPreparer`; the option step above uses
  `canPick && isPreparer`). Part of the ~180-line "you" audit in `plan-polish-leftovers`.
- **`go run ./cmd/plandriver setup make_demands -force make` cannot reach a make with the default seed** (the
  demander ranks below the target's preparer, so the difficulty exceeds their two dice; "cannot reach a result of
  3"). I seeded through `/api/dev/seed` with `rankings` instead (power `[1,0,2]`).
- **Seek Answers' reveal list for a watcher differs from the preparer's** (existing; a code comment says why): a
  watcher sees any asset that holds a secret, the preparer only those with one they cannot read.
