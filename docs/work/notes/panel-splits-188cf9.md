# panel-splits-188cf9

The "plan panels over about 400 lines" bullet of `next/code-cleanup-leftovers.md`: the three longest
plan panels, split into a frame and one component per step, the shape `plans/duel/`, `festivity/`,
`war/` and `demand/` already have. **A pure refactor: nothing a player sees or does changes** (markup,
wording, what a watcher's greyed mirror shows, the requests and their order).

## What changed and why

| Panel | Before | Frame now | Step components |
|---|---|---|---|
| `SpreadRumorsPanel` | 1,115 | 190 | `rumors/`: PrepForm, StateRumor (pre-roll), CounterRumor, PickOptions, TakeAssets, TakeConsent, AfterChoices (+ ResolvedSoFar) |
| `ClandestinelyLiaisePanel` | 854 | 96 | `liaise/`: PrepForm, DelayReveal, TogetherAtLast, SecretsWeKeep, ThingsWeShare, WhenWillISeeYouAgain |
| `SeekAnswersPanel` | 881 | 96 | `seek/`: PrepForm, PreRoll, PickOptions, AnswerQuestion, AfterChoices (BreakResource, RevealSecret, DeclareTruth, AskQuestion, MarPenalty, ResolvedSoFar) |

The three folders also hold a `shared.ts` each and, for Spread Rumors and Seek Answers, one small state
class. Total lines go **up**, 2,850 → 3,695: every step repeats its imports, its props and the comment
saying what it is, which is what a file per step costs. Per file (lines):

| Spread Rumors | | Liaise | | Seek Answers | |
|---|---|---|---|---|---|
| `SpreadRumorsPanel` (was 1,115) | 190 | `ClandestinelyLiaisePanel` (was 854) | 96 | `SeekAnswersPanel` (was 881) | 96 |
| `rumors/AfterChoices` | 248 | `liaise/ThingsWeShare` | 261 | `seek/shared.ts` | 229 |
| `rumors/PrepForm` | 225 | `liaise/SecretsWeKeep` | 197 | `seek/MarPenalty` | 96 |
| `rumors/shared.ts` | 168 | `liaise/PrepForm` | 185 | `seek/AskQuestion` | 95 |
| `rumors/PickOptions` | 129 | `liaise/shared.ts` | 109 | `seek/AfterChoices` | 95 |
| `rumors/TakeConsent` | 122 | `liaise/DelayReveal` | 73 | `seek/AnswerQuestion` | 91 |
| `rumors/StateRumor` | 86 | `liaise/TogetherAtLast` | 72 | `seek/BreakResource` | 88 |
| `rumors/TakeAssets` | 84 | `liaise/WhenWillISeeYouAgain` | 43 | `seek/RevealSecret` | 78 |
| `rumors/ResolvedSoFar` | 69 | | | `seek/PrepForm` | 69 |
| `rumors/CounterRumor` | 63 | | | `seek/PickOptions` | 110 |
| `rumors/picks.svelte.ts` | 28 | | | `seek/DeclareTruth` | 60 |
| | | | | `seek/PreRoll` | 56 |
| | | | | `seek/ResolvedSoFar` | 44 |
| | | | | `seek/drafts.svelte.ts` | 40 |

No file is over 261 lines.

**What each frame keeps, and why.** A `$state` moved into a child that an `{#if}` can unmount is lost
when it unmounts (#115 found a draft lost that way), so state moved only where its owner is on screen for
the whole of its own life:

- **Spread Rumors.** The picks (counts, the take-asset step, the chosen assets, the busy flag) are one
  small class made by the panel (`rumors/picks.svelte.ts`): the picker is *off screen* during the
  take-asset screen and the consent wait, and the counts must be there when it comes back. The error line
  stays the panel's (ResolvingCard draws it); each step writes it through a bound prop. The counter-rumor
  text, the pre-roll's secret-draft fetch and the sub-step picks moved into the step that owns them.
  The rumor's text still never enters a live draft: `PrepForm` builds it with `spreadRumorsPrepDraft`.
- **Liaise.** The meeting's text (`keepText`) is local to `SecretsWeKeep` and still never reaches a draft.
  Its reset effect still keys on a derived string (`"<plan id>:<seat>"`), never on the plan object, so a
  refetch does not wipe what is typed (`clandestinely_liaise_secret_text.spec.ts` pins it and passes).
  Which SEAT the viewer fills decides what every step shows, so `liaise/shared.ts` works it out once
  (`liaiseSeats`) and hands it down; effects read one seat number through their own `$derived`, never the
  whole object (a new object is built on every change anywhere at the table).
- **Seek Answers.** Every pick and typed text (counts, chosen marginalia and assets, the declared truth,
  the question, the answer) stays in one class made by the panel (`seek/drafts.svelte.ts`): the picker
  gives way to the sub-steps, each sub-step leaves when nothing is owed on it, and a player who is *asked*
  a question is shown the answer form in place of the whole sub-step area (a Make Demands winner can ask
  themselves), so a draft owned by a step would be lost when its step left. The three "Skip" buttons still
  share one busy flag.

Facts several steps read (whose turn it is, the rumor's subject, the option names, what is still owed)
are worked out once per panel (`rumors/shared.ts`, `liaise/shared.ts`, `seek/shared.ts`) and handed to each
step as `scene` / `seats`. Steps otherwise take the plan context (`ctx`) and the plan.

**Two guard tests moved with the code they read.** `planPanelsA.test.ts` and `planPanelsB.test.ts` read
a panel's source for "no legacy chip / `.res-warning` / inline style / box in its own CSS / negative
margin"; after the move the frame holds almost none of the markup, so they would have passed over
nothing. They now read the frame and its step folder (`STEP_FOLDERS`), and `planPanelsA` joins every
`<style>` block instead of the first. No assertion changed. I mutated one step file per rule (an old chip
class, an inline `style=`, a `background`, `player-chip`, a negative margin): each turned the matching
test red, and was restored.

Also: `frontend/src/lib/README.md` lists the three new folders; the item file lists the panels still over
about 400 lines (see below); `OPTION_KEYS` in `SpreadRumorsPanel` was never read and went with the move.

## How it was verified

Against this branch's own code: a build of it on `:8187`, and a build of `main` (which the branch started
from) on `:8186`, **both on one private database**, so a game parked in a state is the same game on both.

- **`make check-fast` passes** (`svelte-check` 0 errors, 114 test files, 2,298 tests, `go vet`, Go unit tests,
  `deadcode`, the hooks, the tracker check). **No unit test file was added or removed and no assertion was
  changed**; the two source guards above changed only what they read. The count is higher than `main`'s
  because the per-file source guards now run over the new components.
  `make test-integration` was not run: no Go changed.
- **e2e, against the build of this branch:** `spread_rumors` and `spread_rumors_secret_draft` (7 tests),
  `clandestinely_liaise` and `clandestinely_liaise_secret_text` (12 tests), `seek_answers` (10 tests). All
  pass, with #116's guard on (a spec fails on an uncaught error or an unexpected `console.error`).
- **Screenshots at 375px, `main` against the branch: 115 shots, 113 identical to the byte, the other two
  identical in the panel and in markup.** Every shot is the same game state on both builds, as the actor and
  as every other seat (a throwaway Playwright spec, not committed: a game parked in each state over the API,
  then snapped by each build). Per panel (markup is compared too: the Play pane's HTML with class hashes
  and comments stripped, identical in every one of the 115):

  | Panel | States | Shots | Identical to the byte |
  |---|---|---|---|
  | Spread Rumors | pre-roll (open and secret rumor); make: picker, counts, the take-asset screen and a chosen asset, consent pending, a consent refused, after the picks (break + hide, leverage + break, leverage + hide); mar: counter-rumor (empty and typed), picker, consent pending, after the picks; the live prep mirror (empty, typed, kept secret) | 47 | 47 |
  | Clandestinely Liaise | delay reveal; Together at Last; Secrets We Keep (empty, asset and text picked, one kept); Things We Share (each of the five options, one submitted); When Will I See You Again?; the live prep mirror | 31 | 29 |
  | Seek Answers | pre-roll; make and mar pickers (and with counts); sub-steps (break + declare, reveal + ask, text typed); a question pending as the target (answerable and vetoable) and as the asker; the mar penalty; resolved so far; the live prep mirror | 37 | 37 |

  The two that differ are Liaise shots, and only at the fade on the right edge of the header's player strip
  (outside the panel): it differs by a few hundred pixels between two snaps of `main` itself (see Noticed).
  Contact sheets are in the PR.

## Behaviour changes to look at before merging

None intended. One difference in *lifetime*, in flows I could not drive:

- **Spread Rumors and Liaise: a step's own pick or typed text now lives in the step.** Before, the panel
  kept it if the step was swapped out and back within one phase. What can swap a step out: in Liaise, the
  "waiting for the Make Demands winner to settle the partner" line and the bystander branch (a viewer whose
  seat changes mid-phase); in Spread Rumors, the same kind of change for the counter-rumor box and the
  break-target / hide-source picks. In every flow I could drive the text is empty by then (a submit clears
  it). The picks where a swap *is* part of the normal flow (Spread Rumors' counts and take-asset step,
  every Seek Answers pick) stayed in the panel.

## Open questions for the owner

None.

## Noticed, not fixed

- **The brief's list of the panels still over about 400 lines was missing two.** Recounted, six are over:
  Propose Decree (751), **Make Introductions (709)**, Exchange Courtiers (691), Spread Propaganda (687),
  **Chronicle Histories (676)** and Make Demands (506). Two *step components* are about as long
  (`demand/TargetPlanDemandOverlay.svelte` 505, `duel/BoutsPhase.svelte` 401). The item lists all of them.
- **Spread Rumors' picks are not cleared when the panel is handed another plan of the same type.** The
  plan-change effect resets the take-asset step and the chosen assets, not the counts; that is how it was
  and how it is. Whether a panel instance can see two plans in a row (rather than being remounted between
  them) I did not check.
- **A header difference between two snaps of the same build.** The fade on the right edge of the header's
  player strip (x 330–375, y 52–110 at 375px) differs by a few hundred pixels between two snaps of `main`
  itself (a few shots in each run). It is outside the plan panel and is why the comparison reports it
  separately.
