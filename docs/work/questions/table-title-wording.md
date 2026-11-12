---
title: "What should a table's title say in the states you haven't ruled?"
---

Four titles are ruled: "Row 6 · Seek Answers", "Prologue · Choosing", "Lobby" and
"Game over". The rest were proposed so the profile page and the table's title bar
have something to say in every state. One formatter, `frontend/src/lib/tableTitle.ts`,
builds all of them, so a change of wording is one edit and its test.

| State | Title | |
|---|---|---|
| Lobby | Lobby | ruled |
| Prologue, choosing | Prologue · Choosing | ruled |
| Prologue, a track's ranking (hearts being spent) | Prologue · Power ranking | proposed |
| Prologue, a track's set-asides being placed | Prologue · Power set-asides | proposed |
| Prologue, closing | Prologue · Closing | proposed |
| Main Event row with a plan | Row 6 · Seek Answers | ruled |
| Main Event row with a scene, or nothing yet | Row 6 | proposed |
| Main Event, the ending vote | Row 7 · Ending vote | proposed |
| Shake-Up, a category | Shake-Up · Esteem | proposed |
| Shake-Up, before its first category | Shake-Up | proposed |
| Game over | Game over | ruled |

Two states the formatter has a fallback for can't happen today, so they need no wording:
Main Event before row 1 (the row is set to 1 before the phase flips) and the retired tone-setting
phase (no code enters it, and a migration moved every game out of it).

Things behind the proposals that you may want to change:

- **Should a row with a scene read differently from a row with nothing yet?** Proposed: no,
  both are "Row 6". The title names the plan, and a scene is a step, which belongs to the play
  area. The alternative is "Row 6 · Scene" for a row mid-scene, which would change the title each
  turn of every row.
- **Which plan a row names.** The plan being resolved; while a resolved plan's follow-scene turn is
  being played, that plan, even if a second plan is waiting on the row; else the next plan waiting;
  else the last one resolved there. So with two plans on a row the title stays on the first through
  its scene and moves to the second when its setter passes focus, and a row keeps its name for the
  rest of its turns. It drops to a bare "Row 6" only when it never had a plan. The alternative is to
  drop the plan's name once it has resolved.
- **The Prologue's ranking stages name their track and the Shake-Up names its category,**
  where the ruling only fixed "Prologue · Choosing". The server sends the category for this
  (`shake_up_category`), which the ruling's list of what to send didn't include.
- **"Closing"** is the step's own name in the app and rules. It says little about what is
  left to do there (name a main character, fill marginalia, add an extra peer, set tones); a
  longer word would say more.
