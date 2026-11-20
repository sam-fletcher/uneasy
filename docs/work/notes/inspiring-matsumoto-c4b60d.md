# Plan polish leftovers

Branch `claude/inspiring-matsumoto-c4b60d`. Item: `next/plan-polish-leftovers.md` (kept, reduced to what is
left). One commit per plan.

## What changed and why

The item's bullets dated from an audit on 2026-11-07, so each was re-checked against the code first. Five
were already fixed and are dropped (below). The rest are done.

**Exchange Courtiers**
- The riposte picker no longer offers the requested peer's last intact marginalia, which the server refuses.
  With one note or none left it says why in words and leaves "Surrender intact" as the one (main) button.
  `survivesABreak` in `plans/shared.ts` mirrors the server's check.
- The messy-break picker now shows the shared "this is the last marginalia" heads-up before the tear. The picker
  and its warning are a small `BreakPicker`, handed the picked note, so a unit test can render the warning.
- Two decisions now log under their own name, in the voice of the plan's existing lines:
  "X chose a riposte — …" and "X accepted the fair trade — the exchange goes ahead without a roll." (the accept
  line comes before the two transfers it licenses; a decline already logged). The riposte line says who may break
  the peer only when a break is possible: with one note or none it says the peer "can only be surrendered
  intact" (the server refuses any break), and under a Make Demands perform-steps win it names the winner, in the
  form the break and surrender lines use, since the winner decides.

**Host Festivity**
- The host's Make picker gives way to "Waiting for X to finish their roll…" while a guest's roll-and-choice is
  open (the server refuses every host make then). It returns by itself when the roller chooses. The pending-
  challenge half was already handled; a test pins it.
- A guest bound to accept duels now reads, on the page, "You must accept any duel challenge during this event,
  so you can't decline." The only explanation used to be a `title`, which named the stored key.

**Propose Duel**
- The duellists' names in the setup lines are marked as assets (`<em>`), like every other line that names one.
- The unreachable "you fight in person" branch is gone. Its one reachable case (a main character lost
  mid-resolution) reads "You have no peer to put forward in this duel.", never that the player fights.
- A player with no other peer to send in their stead is told so ("You have no other peer to send in your stead.")
  where "Send someone else" would be. Principle 3 decided this: a choice that can't be made now is fine when the
  reason is on screen, so a player learns what is possible; hiding or greying the button would leave it looking
  silently taken away. (This replaces my first version, which hid the button, and its question file.)

**Seek Answers**
- "You must also describe a flaw in (break) N of your own resource assets" shows only on a mar, with the number
  (the shortfall, capped at the resources the preparer can still flaw: `seekAnswersMarPenalty`, mirroring
  `game.SeekAnswersMarPenalty`). When the cap brings N below the shortfall it says why ("The plan marred by 3, but
  only 1 of your resources can still be flawed."); when it brings it to zero it says nothing is owed and why,
  rather than going quiet (the server logs "must break 0").
- "No resource can be broken" prints once: the forfeit note keeps only "This pick has no valid target."

**Spread Propaganda**
- The (a)–(d) lettering is gone from the picker, the reference tab and the "Choices applied" summary.
- Under the counter-propaganda option the picker names the top interferer with the tally ("Top interferer: bob,
  with 3 dice. Also interfered: mel, with 1 die."). The count comes from the roll's dice the way the server does it
  (`lib/plans/interference.ts`). A tie says what settled it: "ranks higher on esteem" only when esteem rank did,
  "goes first on the tie-break" between players with no rank (the server falls back to the player id), and the rest
  of the tally follows. The line is not shown on a counter-propaganda plan, where the option is shut.

**Make Demands**
- The target plan is a list of option cards ("Seek Answers by alice", "Row 5"), not a native `<select>`. The
  now-unused `.ft-select`/`.form-select` rules left `planPanel.css` with it.

### Bullets dropped as already fixed

- **Make War, "War — ongoing" before its row:** the redesign removed that badge line. The band now says
  "planned" until the war has a row, then "under way" (`warStatusWord`).
- **`RetinueView`'s red focus ring:** every focus ring in the file uses `--color-you` (gold), and the picker's red
  hover is gone, since `ccce2cd4`. Not exercised in a browser; I could not reach the main-character swap picker
  from a seeded game, so this is a read of the CSS.
- **Host Festivity, insisted mars not naming who insisted:** the host's choice already logs "X chose … in answer
  to Y's insistence", naming the guest; `TestHostFestivity_HostChoosesInsistedMar_AppliesToHost` pins it (the
  audit saw the earlier flow, where the guest picked the option). Nothing added.
- **Host Festivity, picker live during a pending challenge:** already hidden; only the guest's-roll half was open.
- **Propose Duel, "Send someone else" under 44px:** it is a plain `.action-btn`; measured 44px tall, 16px side
  padding at 375px on the unchanged build.

### Left on purpose

- Insisting as one tap and the scene panel's "Focus:" line are design calls (question files, bullets kept).
- The ~180 "you" lines audit is its own item.
- Sub-step heading sizes and how a step's text is punctuated (`plan-type-tiers` waits on mockups); no
  sub-heading was restyled.

## How it was verified

Against this branch's code, on a private database and a private server (port 8173 = the unchanged build for
"before"; 8174 = this branch for "after"):

- `make check-fast`: green (lint, Go unit, svelte-check, Vitest 1971 tests, hook tests, tracker).
- `make test-integration` on a private database (`…_it`, not `uneasy_test`): green, all seven packages (handler 213s), on the final code
- Each new test was run red first: the Go log tests failed on the unchanged code ("no log post contains…"; the
  one-note and stand-in riposte cases failed against the first version of the line too); the Vitest cases for the
  riposte picker, the festivity picker and decline note, the duel setup, the Seek Answers lines, the (a)–(d)
  labels and the Make Demands cards failed, then passed; the Seek Answers cases and the `BreakPicker` warning were
  also re-run with only the code under test reverted. The e2e assertions added to the Exchange Courtiers and Host
  Festivity specs failed against the unchanged server.
- e2e on the final build: `exchange_courtiers`, `propose_duel`, `spread_propaganda` and `seek_answers` (43 together)
  and `host_festivity` and `make_demands` (27 together), all passing.
- 375px before/after screenshots, as the player each change affects and a bystander where one sees it. They are
  in the PR.

Not run against this code: a real phone; Safari or Firefox; five-player tables; a Make Demands stand-in driving
the riposte, the host's make or the duel setup in a browser (the riposte stand-in is covered by render and
integration tests); the Spread Propaganda tally with a tie, with several interferers, or on a counter-propaganda
plan (unit tests only; the e2e has one interferer); the Host Festivity picker with a stand-in who is also the
rolling guest; `retinue_pieces.spec.ts` and the other plans' e2e specs (nothing in them changed).

## Behaviour changes to look at before merging

- A player with no other peer to send in a duel's setup sees a line where "Send someone else" would be.
- The host's Make picker disappears (replaced by a waiting line) while a guest is mid-roll, then returns.
- A riposte on a peer with one marginalia offers only "Surrender intact", as the main button, and its log line
  says the peer can only be surrendered.
- Two new log lines (the riposte choice; the accepted fair trade). Neither changes severity or who is waiting.
- The mar flaw line in Seek Answers is gone on a make, and now has a number on a mar (with the reason when the
  cap bites).
- Spread Propaganda mar options lose their letters; the e2e specs that located them by letter follow.
- Make Demands' target picker is a stack of cards: with many pending plans it is longer than the select was.

## Open questions for the owner

- [festivity-insist-two-step](../questions/festivity-insist-two-step.md): one tap or two to insist? (Recommend no
  change until a playtester mis-taps; a mis-tap costs only timing.)
- [scene-panel-focus-line](../questions/scene-panel-focus-line.md): what should "Focus:" say, or go? It means a
  different thing for a turn's scene and a plan's scene. (Recommend a label per kind.)
- [riposte-last-marginalia](../questions/riposte-last-marginalia.md): may a riposte tear the last note and destroy
  the peer? The server says no; I kept that. (Recommend keep: the option promises the target the peer.)
- [spread-propaganda-interferer-tie](../questions/spread-propaganda-interferer-tie.md): the server settles a tie
  for top interferer by esteem rank, which no ruling covers and the picker now shows. (Recommend keep.)

## Noticed, not fixed

Tracked as items in `docs/work/next/`:

- [festivity-pending-mar-roll-lock](../next/festivity-pending-mar-roll-lock.md): the host's pending-mar card has the
  gap the Make picker had (the roll lock refuses it, the card stays live).
- [seek-answers-empty-picker-notes](../next/seek-answers-empty-picker-notes.md): the other two Seek Answers
  sub-steps repeat their picker's empty message in the forfeit note, in different words; plus a trimmed space in
  the stepper's note ("asset— no valid targets").

Not tracked:

- Exchange Courtiers: if the preparer had no peer when the trade step ran (they roll with nothing offered) and the
  roll then mars at margin 2, riposte is pickable, and `riposte-break` answers 409 "no peer was requested". I did not
  reproduce a wedge (every player holds a main character, which is a peer, so the case needs a lapse mid-resolution),
  but forfeit handles a missing peer and riposte does not.
- `MakeDemandsPanel.svelte` derives `landingRow` and never reads it.
- `seekAnswersMarPenalty` is Seek Answers-only but lives in `plans/shared.ts` with the helpers every plan shares.
