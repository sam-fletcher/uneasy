# Hand-over: typed results (typed-results-09d165)

Tracker item `plan-typed-results`, deleted in this PR.

## What changed and why

A typed ✓ or ✗ is drawn in whatever font each phone supplies, so it looked
different from device to device. The house way is the word, "Result: Make" or
"Result: Mar", in the outcome's colour (`planPanel.css`), as `MakeMarPicker`
and Propose Decree already draw it. Four panels still typed the mark:

- `ChronicleHistoriesPanel.svelte`: the make line and the mar line.
- `SeekAnswersPanel.svelte`: the Result span.
- `SpreadRumorsPanel.svelte`: the Result span.
- `SpreadPropagandaPanel.svelte`: the mar line.

Each now shows the word alone. Nothing else in those panels changed.

`tests/e2e/chronicle_histories.spec.ts` read the typed text at 375px. It now
checks `Result: Make` / `Result: Mar` exactly, so a substring match cannot
hit "make option" elsewhere on the panel.

New guard: `frontend/src/lib/components/plans/typedResults.test.ts` reads every
`.svelte` file under `plans/` (54, sub-folders included) and fails on a literal
tick or cross, heavy or light, as a character or an HTML entity. HTML comments
are skipped, so a note that names the mark is fine. Shown failing first: four
failures, one per panel above, before the change. Passing after.

## What the brief named but did not need changing

- **Host Festivity's guest line.** No typed mark. `GuestList.svelte` shows
  plain words ("Made a good impression", "Slipped up"), coloured by
  `outcome-make` / `outcome-mar`. The "Result:" line in `SocializingTurn.svelte`
  already shows the word.
- **`host_festivity.spec.ts`.** Does not read typed text. Line 96 checks
  `.outcome-make` has text `Make`. Unchanged.
- **Make Demands' tick.** No typed mark. The tick is `ToneMark`'s drawn SVG
  path, inside the shared `OptionCard`. `MakeDemandsPanel.svelte` is not
  touched (another PR is open on it). It needs no change. The existing
  `planPanelsC.test.ts` already checks it for typed glyphs.

## How it was verified

Against this worktree's own code, not `main`:

- `make check-fast`: passed. Go tests, svelte-check, Vitest (97 files, 1890
  tests, including the guard), the guard-git hook (61 cases), tracker check.
- Guard: fails on the four panels before the change, passes after.
- E2E against the worktree's own server (`E2E_BASE_URL`, private database
  `uneasy_claude_typed_results_09d165`, port 8192):
  `chronicle_histories`, `host_festivity`, `seek_answers`, `spread_rumors`,
  `spread_propaganda`. Result: see the line below.
- 375px screenshots of each changed Result line: 11 before, 11 after. The
  before build is a clean copy of `HEAD`, the after build is this worktree.
  Each shot is the Result line only, not the whole panel. Taken by throwaway
  specs that reuse each real spec's own helpers; the throwaway files are not
  committed.

**E2E result:** 52 passed across the five specs, on the worktree's own server.

`make test-integration` on the private database: passed, every package `ok`.
No Go code changed; it ran because the verification bar asks for it.

Not run: the whole e2e suite, and any spec against `main`'s own server
(`:8090`). The screenshot specs were throwaway copies and are not committed.

## Behaviour changes to look at before merging

- The Result line has no glyph now. The word and its colour carry the result,
  as in `MakeMarPicker` and Propose Decree. No character is drawn differently
  from one device to the next. This is the only visible change; no logic,
  payload or stored state moved.

## Open questions for the owner

None.

## Noticed, not fixed

- **Prologue ticks.** Left alone, as the brief said: `PrologueView.svelte`
  ("Done ✓ (tap to undo)"), `prologue/ClosingStage.svelte` ("✓ ready", "Ready ✓"),
  `prologue/TurnCard.svelte` ("✓ Claimed"), `prologue/TrackBoard.svelte`
  (`col-tick` and `done-tick`), `ClaimChoiceModal.svelte` (`step-marker`).
  These belong to the Prologue redesign.
- **Guard scope.** The guard reads `plans/` only. A plan panel moved out of
  that folder would escape it.
- **`docs/work/notes/README.md`** says a note is named after its worktree
  folder. The `work-session` skill says the branch name. This note follows the
  skill (`typed-results-09d165`, since the folder is shared with an older
  session).
