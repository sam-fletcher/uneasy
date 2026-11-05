# Transition rulings: say what ended and what is next

## What changed and why

Removed clicks had taken away the beat that told the table a stage had ended. The
owner's 2026-11-03 rulings answer with sentences. Three pieces:

1. **The closing post says what comes next and who acts.** `EmitPlanResolved` now
   appends "Next: <who> to <what>." read from `ComputeRowState`, the same wait state
   that drives the Waiting On bar (`handler/plan_aftermath.go`). It is not written
   per plan, so a Make Demands stand-in, a parent plan's owed step or a replacement
   main character come out right by construction. It also names a lost main
   character. Only player names and fixed phrases go in, so nothing gated can leak.
   A fall-through (`cancelled`) is left alone: it already says what is next.
   The Make Introductions mar and the "Main character lost" screen are covered: the
   post names the loss and who replaces them, and the screen now says why and who
   the table waits on, for the chooser and for everyone else.
   *Fix passed on from #56:* "came to nothing" is gone. A re-aim that finds nothing
   reads "closed with no effect". A declined counter-propaganda says the
   counter-propaganda is over and that the Spread Propaganda it answered is not
   ended by this.
2. **A line under every ending submit** (`plans/shared/EndsNote.svelte`). Sweep
   tables below.
3. **A break of another player's asset logs at Important** through one helper,
   `breakSeverity(owner, actor)`. An owner breaking their own stays Default. The
   destruction line follows, so a blank asset destroyed by a rival (which has no
   tear post) also reaches its owner.

Also checked: Host Festivity's "earns a make" lines were already Default (the
opt-out line and the guest-mar line), and Make Demands draft picks landed at
Default in #57. Nothing to raise.

## Sweep: ruling 2 (lines under ending submits)

Changed (panel: submit → line):

- Spread Propaganda: last of create artifact / give peer / break self, and their
  "Skip" when it clears everything owed; Apply choices on a mar with no step left;
  **Decline** on counter-propaganda (it ends, the plan it answers carries on).
- Seek Answers: last of break / reveal / declare / mar flaw, their Skips, and the
  question's Answer when it is the last owed.
- Spread Rumors: last tear / hide-source and Skips; Apply choices when no
  sub-step follows; the victim's Agree (and that Disagree sends the preparer back).
- Exchange Courtiers: Accept (swap, no roll) vs Decline; Apply choices unless
  Messy or Riposte was picked; Messy break; both Riposte buttons.
- Make Introductions: last newcomer's Resolve (when no author is owed); an author's
  Write marginalia when they are the last owed. The last arrival was already done.
- Propose Decree: End the debate (opens the roll); the last amender's Submit /
  Leave unchanged (ends the amendments); Place addendum (names who enacts, which is
  the Make Demands stand-in when one holds the plan); Enact the law.
- Propose Duel: Take / Spare; last bout's Respond; stakes commit ("bouts begin when
  both have committed": the opponent's commit is masked from the client, so a static
  sentence, not "last").
- Make Demands: last draft pick; Place counter-demand; the held plan's keep/re-aim
  (only while it holds the roll or the plan) and leverage (until finalised).
- Make War: a battle cost and a surrender claim get a sentence that stays true from
  one war's view ("play goes on once every cost and claim at the table is settled",
  since the gate spans all wars); the last peace vote when everyone else accepted.
- Main-character choice: "play goes on once every player who lost one has chosen".
- Host Festivity: End the event. Chronicle Histories: both Wrap the scene buttons;
  the last Submit choice (the scene stays open). Clandestinely Liaise: Advance,
  Keep secret, Submit share, and the final reveal. Re-aim: Re-aim this plan.

Checked and left (reason):

- Every prep form (Prepare): ends nothing in play.
- Pre-roll casts (Spread Propaganda/Rumors, Seek Answers, Chronicle): open the roll
  rather than end anything; the next step is visible at once.
- Make/mar pickers where the plan always owes a step after them (Seek Answers,
  Spread Propaganda make, Chronicle, Spread Rumors with a take): not the last step.
- Seek Answers "Ask question" and "Veto": the answer is still owed, so neither ends.
- Propose Decree join/decline and amend/skip: they pass the turn along.
- Host Festivity guest Roll / Opt out / Submit choice / host Take: the event stays
  open until the host ends it. Insist: makes no step.
- Propose Duel Confirm champion: changes who fights, ends nothing.
- Make War delay reveal, entry payment: no end of a plan or gate that the player
  can tell from their own submit.
- Dice roll panel (leverage, Ready): not a plan panel; the outcome shows at once.
- Make War Join / Stay out, Propose peace (a vote opens, nothing ends) and Surrender
  (a checkbox that removes only that player; the claims that follow are visible).
- Festivity challenge Accept / Decline and host "Take this Mar": responses inside a
  festivity that stays open until the host ends it.
- The fallback "Complete plan" buttons (Spread Propaganda, Seek Answers, Spread
  Rumors, Make Demands): reachable only in the brief window before auto-complete,
  and already named for what they do.
- War delay reveal and Clandestinely Liaise's first delay reveal: they set a row, so
  neither ends a plan; Liaise's *final* reveal does, and has its line.

## Ruling 3: every break

Now Important when the breaker isn't the owner: Chronicle Histories (break
artifact), Clandestinely Liaise (already Important), Exchange Courtiers (messy
break, riposte), Seek Answers (break resource), Spread Rumors, shake-up break, the dev tear route, plus the destruction line.
Also: the main-character swap (`assets_main_character.go`, tear then destroy) goes
through `EmitMarginaliaTorn` / `EmitAssetDestroyed`, so it follows the same rule
(it is the owner's own break, so Default). Make Introductions' broken journey now
uses `breakSeverity` on the recipient, so a Make Demands `keep_assets` winner
actor against another owner is Important.
Left at Default: an owner's own breaks (Spread Propaganda "break yourself" by the
preparer; a Make Demands stand-in doing it to the preparer's asset *is* Important),
Host Festivity's own self-break and its abandoned disagreement peer broken at the
end of the event (the game breaks it, with the owner as actor; owner question
filed), and Make War's cost (the route refuses non-owners, so it is always the
owner's own).

## How it was verified

- `make check-fast` passes (the frontend must be built first for `cmd/server` tests).
- `make test-integration` on a private database: all packages ok.
- New Go tests (`plan_aftermath_integration_test.go`) each fail with the fix
  removed (checked by disabling the aftermath and `breakSeverity`): counter
  decline with the parent's step owed, Make Demands stand-in, lost main character
  (both wait-state cases), who acts next, break severity. One existing test
  asserted the old Default tear severity; updated.
- New Vitest file `EndsNote.test.ts`, with false states as well as true ones for Spread Propaganda, Seek Answers, Make Introductions, Exchange Courtiers, Make Demands, the peace vote and surrender claims, the Decline line (parent resolving / resolved / unknown) and the main-character screen. Host Festivity, Propose Decree, Clandestinely Liaise and the Make Demands overlay conditions have no render test (their fixtures are large); they were type-checked only.
- 375px screenshots against a server on this worktree: the Make Introductions mar
  (the last author) and the main-character-lost screen as the chooser and as a
  bystander.
- Not run: Playwright e2e specs; the counter-propaganda Decline line was checked
  by the render test, not by screenshot.

## Behaviour changes to look at before merging

- Every plan's closing post gains a "Next: …" sentence. It reflects the real wait
  state at that instant, so the last plan on a row says "set the scene" for the
  current focus player.
- A break of another player's asset is Important for everyone, not only the owner
  (the unread marker is a table-wide severity, not per owner).
- The "Next:" phrases live in one map (`nextPhrases`); a new wait kind needs an
  entry or the exhaustive lint fails the build.
- "Has no main character" in the closing post means the players who currently lack
  one, which is nearly always this plan's doing but is not recorded per plan; a
  nested plan's post can repeat a loss from just before it.
- A take or destruction of an asset that *was* a main character now says "That was
  X's main character." in its own log line (read from the asset before the change,
  since a transfer clears the flag; not from whether the owner is short). The Make War
  surrender-claim take logs through `mwLog`, not `EmitAssetTaken`, so it carries the
  sentence itself; a battle-cost destruction goes through the shared destruction
  line. Prologue takes pass false (no main characters exist yet).

## Open questions for the owner

Each has a file in `docs/work/questions/`:

- Should the Make Introductions mar post also list each newcomer's fate?
- Important is table-wide, so every player's marker lights on a break; the ruling
  said "its owner". Owner-only, or fine?
- Should Host Festivity's abandoned disagreement peer, broken by the game with the
  owner as actor, log at Important?

## Noticed, not fixed

- Idea, not built: a server `ends_on_submit` hint on a plan would replace the
  client's re-derivation of `CanComplete` in each panel and could not drift.
- `make check-fast` prints a deadcode note for `middleware.AccountContext`; it
  predates this branch and does not fail the target.
