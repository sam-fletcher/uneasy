# redesign-play-af72ed

The Play pane and the shared plan frame, rebuilt. Wave 4 of the visual redesign: the roll is one of a
plan's steps, the Waiting On bar is gone, and the boxes, the gold wash and the status chips that wrapped
every plan are gone with it.

## What changed and why

**1. The open step has a heading, and it replaces the Waiting On bar.** The first line of the open step
says the current ask: "Your move — aid or interfere" when the game waits on you, "Waiting on otto — the
roll" when it waits on someone else (`lib/stepHeading.ts`, drawn by `shared/StepHeading.svelte`). It reads
the same derivation as the clocks on the player row, so they cannot disagree. `WaitingOnBar.svelte` is
deleted. The step labels in `lib/waitingOn.ts` are now lower-case phrases that finish the sentence
("the roll", "pay the cost of battle", "prepare a plan, or refresh 2 assets") and never repeat the plan's
name; the old subtitle is folded into them.

**2. A plan reads as its steps in order** (`plans/ResolvingCard.svelte`, `PlanPanel.svelte`,
`phases/MainEventView.svelte`):

```text
⚑ otto prepared Seek Answers           finished steps, one line each (FinishedStep)
✎ otto: “who holds the holdfast?”       a long one opens in place (caret)
⚄ Rolled MAKE · 2 distinct faces …     the roll, once it is over (opens to show the dice)
Your move — the next step              the open step's heading
<the panel's own body>                 the open step
```

The roll is drawn by the card, between the finished lines and the open step, through a context
(`plans/shared/stepFrame.ts`) that `PlanPanel` provides. While the roll is open it is the open step and
carries the heading itself, so there is only ever one. `MainEventView` draws the roll alone only when no card
claims it (a scene's roll).

**The heading is drawn by the page by default, and a surface claims it** (`plans/shared/headingSlot.ts`),
the same pattern as the roll's claim. `MainEventView` draws it at the top of the play surface unless a plan's
card, the preparation step or an open roll has claimed it by drawing it at the top of its own open step.
So no state can be left without one: a plan pending before kick-off, a scene before it has loaded, Make War's
own panel (which draws no card), a war's cost with no war on screen, a plan panel still lazy-loading. (My first
version had cards and takeovers draw it and nothing else, which left those bare; the review caught it.) The
takeovers and scene panels keep the heading at the top: the card and the roll are handed none while one is up.

**The roll's draft lives outside the panel** (`lib/rollDraft.svelte.ts`, keyed by roll id). The roll is drawn
inside the card or alone on the page, and moves from one to the other when a plan panel that was still loading
when the roll opened arrives. A component's own state does not survive that, so the side and the dice a player
had picked were lost. The card also claims the roll once, on mount, instead of tracking the frame (which changes
with every heading and would have let go and re-claimed each time).

**3. The roll is a step** (`DiceRollPanel.svelte`, new `plans/shared/DieTile.svelte`). Open: heading, one
sentence on what is rolled, the dice in "For" and "Against" (labelled only when there is an Against), then the
stage's controls. Each die is a 44px tile whose edge is its owner's colour; the die mark replaces the 🎲
emoji; cancelled and repeated dice are dashed, never faded. Aid / interfere and easier / harder are chips
with no meaning colour, and the one main button names what it commits ("Commit aid", "Commit and roll"),
with "Pass" beside it. Resolved: one line that opens to show the dice. The API calls are unchanged.

**4. Boxes and fills go** (`plans/planPanel.css`, shared by all twelve panels). No border, fill or shadow
on the plan; no gold wash; no violet "Resolving" chip; notices are coloured words; status words are plain
text. The panels still draw their chips and option cards with the older class names (`.chip-btn`,
`.player-chip`, `.choice-card`), so those stay, built from the house chip's tokens, and a test holds them to
agreeing with `chip.css`. The read-only mirror is no longer faded: each control's own disabled look says it
is not yours, and a fade would take text under the contrast floor.

**5. The preparation grid** is rebuilt on the scales (nothing below 12px). It was never mocked up; see the
question file.

**6. Shared plan pieces move onto the tokens and recipes:** `MakeMarPicker`, `PlayerChips` (a shape and a name
in the player's colour), `ChoicesApplied`, `SimultaneousRevealInput`, `RowPill` (now plain text), the Buffet
(a disclosure row), `DifficultyMeter`, `Stepper`, `EndsNote`, `DelayThresholdNote` (a notice), and
`choicePip.css`. All their markers are gone.

**Markers removed** (`tokens: not migrated`): `MainEventView`, `DiceRollPanel`, `planPanel.css`,
`ChoicesApplied`, `RowPill`, `SimultaneousRevealInput`, `shared/choicePip.css`, and the five `plans/shared/*`
files that had one. `WaitingOnBar` went with its own.

**Files outside my list, kept small:**
- `routes/table/[id]/+page.svelte`: the stopgap line. The page still draws the same heading above the phases
  that have no steps yet (lobby, Prologue, Shake-Up, end), so those lose nothing.
- `lib/waitingOn.ts` and its test (the labels), `lib/stepHeading.ts` and its test, `shared/StepHeading.svelte`.
- `routes/dev/styleguide/+page.svelte`, `docs/STYLE_GUIDE.md` (a "Play steps" subsection, and two stale mentions
  of the bar), `lib/README.md`.
- e2e: the specs that read the bar now read the heading and the clocks (`helpers.ts`: `stepHeading`,
  `waitingOnNames`, `expectWaitingOn`).

No tokens were added to `app.css`.

## How it was verified

Against this branch's own code (a private server on `:8187`, a private database):

- `make check-fast`: passes. 69 test files, 1138 tests, `svelte-check` clean (one existing warning in
  `HelpContent.svelte`), `go vet`, the Go unit tests, the git-guard tests, the tracker check.
- **e2e: the full suite, 186 specs, all pass.** One new spec file, `play_steps.spec.ts`, has two tests. The first reads the
  DOM order of a Seek Answers game as each player: finished line, then the roll, then the plan's body; one heading; no
  emoji; the dice split into For and Against with each owner's colour on the edge; the finished roll line above the
  heading and opening in place; the clocks agreeing with the heading. The second holds back the plan panel's script, so a
  roll that is already open is drawn by the page first: it has one heading, a player drafts a side, the panel arrives, the
  roll moves into the card, and the draft is still there. The first full run had four failures, all specs that read the old
  bar's wording; each is fixed and the suite is rerun clean.
- **Headings in every state.** `MainEventView.test.ts` renders the play surface on the server (no effects run, so nothing has
  claimed the heading) for every state the review named that has no card, and expects exactly one heading. A typed record in
  `stepHeading.test.ts` lists every `RowStateKind`, so a state added on the server without a step phrase fails the build.
- **Every new test bites.** About thirty unit mutations (the note shown when a panel asked to hide it, the body drawn before
  the roll, the heading drawn while the roll is open, a fade or a border on the plan, the gold wash back, a chip
  drifting from `chip.css`, the For/Against label with no second group, a meaning colour on a choice, the footer
  counting dice again, the heading naming others on your move, and so on), each turned red and restored; the review round added the
  page no longer drawing the heading by default, a ready actor being asked for the roll, labels counting toward the player cap,
  a state losing its step phrase, the draft forgetting the same roll, and a native-disabled face chip looking live. Three e2e
  mutations (the roll after the body; the heading drawn twice; the draft store forgetting the same roll), each turned
  `play_steps` red. One unit test
  originally missed a border on `.plan-panel` (the rule reader folded the `@import` lines into the first selector);
  fixed and shown to bite.
- **Looked at, at 375px:** Make War waiting on its delay faces (as a player in the war and as a bystander) and a war's cost of
  battle (as the payer and as a bystander), with the heading showing in each; every plan once as its preparer (all twelve), the preparation grid as the focus player
  and as a watcher, a Seek Answers roll as actor, as an interfering player, as an aiding player and as a
  bystander, then resolved, then finished. A Shake-Up roll. The open roll at 1112px (three columns). The before and
  after screenshots are in the PR.
- **Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari or Firefox; hover and
  focus states (described in CSS, not photographed); 344px and the two-column width; a watcher's view of eleven of
  the twelve plans (I looked at each as its preparer, and at Seek Answers as every viewpoint); the Propose Duel
  and Host Festivity rolls beyond what their e2e specs cover; the war's panel and the scene panels (not mine; only
  the heading above them is).

## Behaviour changes to look at before merging

- **No Waiting On bar.** Everything it said is now the heading (who and what) and the chips' clocks (the full set
  of who is owed). When it is your move the heading names no one else. The mockup for the roll does the same.
- **The roll is not below the plan any more.** It is a step inside it, and after it resolves it is one line above
  the step that acts on it. Players who knew where "Dice Roll" sat will look for it in a new place.
- **A plan's "Resolving" badge and "by otto" tag are gone;** the first finished line ("otto prepared Seek Answers")
  and the title bar say it.
- **"Ready" is now "Pass", "Roll" or "Commit aid"** depending on who you are and what you have picked. Pass with a
  draft discards the draft and readies; it is the same call the old Ready made with nothing drafted.
- **The roll's footer sentence for a player who must wait on the roller is gone** ("Waiting for otto to decide
  about a difficulty vote…"): the heading says it.
- **Read-only mirrors are not faded.** A watcher sees a picker at full contrast, with each control in its disabled
  look.
- **The preparation grid is taller** (about 600px at 375px against about 400px), because its text is no longer
  below 12px.
- **The Lobby, the Prologue, the Shake-Up and the end** show the heading above them (a stopgap) where they showed the
  bar. The Prologue's own labels read doubled after the dash ("Your move — Prologue — 3 of 15 chosen").

## Open questions for the owner

- [plan-grid-redesign](../questions/plan-grid-redesign.md): the grid's layout (three columns at 12px, or one), the
  tick on a picked card, and a watcher's pick in neutral rather than gold.
- [step-heading-wording](../questions/step-heading-wording.md): "the next step" for a plan's own steps, and "Your
  move" naming no one else.

## Noticed, not fixed

- **Each plan panel still has a "Dice roll in progress…" line** and its own "X is resolving…" line, now that the roll
  and the heading say it. They are the panels' (`docs/work/next/plan-step-labels.md`, new). The same item lets a panel
  name its own step in place of "the next step".
- **`.res-warning` paragraphs are coloured words with no mark.** `<Notice>` adds the mark; each panel moves over in
  its own piece. Until then the words carry it.
- **Panels that still draw their own chips and option cards** (Make Introductions, Chronicle Histories, Make Demands,
  Host Festivity) take the recipe's look through `planPanel.css`; they move to `<Chip>` in their pieces.
- **`MakeWarPanel` still draws `.plan-header` / `.plan-badge`,** which I kept (as plain text) so it does not break
  before the war piece replaces it.
- **The Prologue's wait labels** ("Prologue — 3 of 15 chosen") need rewording as lower-case phrases.
- **`EndgameVotePanel` and `MainCharacterChoicePanel` have their own titles** under the heading I draw above them.
  Their pieces can drop one or the other.
- **The scene panels** (not mine) keep their prompt boxes and wash under the heading above them; the scenes piece
  restyles them.
- **My first private server never started:** the port I picked (8192) was already taken by another session's server,
  so for a few minutes my `plandriver` and `curl` calls went to it, and created a handful of dev games in that
  server's database (names alice, bob, carol). Nothing was deleted. `deadcode`'s `AccountContext` line is on `main`
  too.
