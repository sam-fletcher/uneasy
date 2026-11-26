# Disclosure rows: one standard across the app (Round 7, 2026-11-25)

## What changed and why

The owner found "What can happen?" read as plain text, couldn't see where its open content ended, and saw that a shut ▸ pointed the same way as the › of a row that opens elsewhere. Round 7 ruled one standard; this builds it everywhere.

- **The mark** (`shared/RowMark.svelte`): the caret is now a stroked chevron (`M6 9l6 6 6-6`) that points down shut and flips up (a 180° turn) open. The arrow stays `›`. Both are 20px in secondary text (were 16px, faint). Down/up mean "here", right means "elsewhere".
- **The line moves** (`shared/tapRow.css`): a `.disclosure` / `.has-body` / `.disclosure-body` recipe. The row's own line goes transparent and the content carries the line, so a lone disclosure is a list of one and the line marks where its content ends. `ChecklistRow` already did this by hand and now uses the recipe.
- **One explainer** (`shared/HelpDisclosure.svelte`): now a thin `ChecklistRow` (`action='expand'`, `glyph='help'`), with the deep-gold "?" and the title in main text. Everything explainer-shaped goes through it: `Buffet` ("What can happen?", with the question mark added), the Prologue's two help rows, scene setup, the **lobby primer** (it was a `ChecklistRow` with a gold title; it now takes a `subtitle` prop on `HelpDisclosure`) and the **Shake-Up intro** (found by a new test: it asked `ChecklistRow` for the "?" directly).
- **Ad-hoc disclosures moved onto the recipe:**
  - `FinishedStep` (a finished line with a detail);
  - the chat's plan and scene folds (`ChatPanel`; the line moves to the foot of the span);
  - the Prologue's category rows (`PrologueView` `.sheet-accordion`: no box, no fill, a RowMark caret instead of the typed ▾);
  - `ClaimChoiceModal`'s steps (rows with a caret, no boxes);
  - `StandingStrip` (see below).
- **Rows that open elsewhere** (Public Record lines, `WarBand`, the profile `TableRow`, `TableRoster`, `ChecklistRow` navigate) already used `RowMark kind="arrow"` and take the new strength with no change.
- **Docs and styleguide:** `docs/STYLE_GUIDE.md` (the caret/arrow rule, a new disclosure rule, the component table, Local help, the folding span) and `/dev/styleguide` (Rows section: shut and open side by side, the two marks, the explainer, a finished line).
- **Tests:** `shared/disclosure.test.ts` pins the closing line moving under the body, the explainer's "?", one `HelpDisclosure` for every "?" row, and no fill/shadow on any open row (a scan of every component's stylesheet). `pieces.test.ts` pins the chevron paths, the 180° flip and the 20px secondary-colour mark.

## How it was verified

Against this branch's own build, on a private database and server (:8193):

- `make check-fast`: passes (Go, svelte-check 0 errors, 2360 unit tests, hook tests, tracker).
- Each new test bites: reverted the flip, the `has-body` flag on `ChecklistRow` and `FinishedStep`, and the "?" glyph, and added a fill on an open row. Eight tests failed; all restored.
- 375px before/after screenshots (in the PR): Lobby checklist, "What can happen?" shut and open, a finished line shut and open, the chat's fold shut and open, the Prologue's help rows, a category row open with the full board open, a claim's steps, and the Public Record (rows that open elsewhere). Mouse parked off the page so no hover fill shows.
- e2e: the whole suite against :8193, plus the new `disclosure_rows.spec.ts` (a claim step, a category row and the standing card press open and shut). After the coordinator's review, the suite was re-run.

Not run: `make test-integration` (no Go changed), a desktop-width look, and a real phone.

## Behaviour changes to look at before merging

- **The lobby primer's title is no longer gold.** It is an explainer, so it takes the shared row: "?" in deep gold, title in main text (ruling point 5).
- **"What can happen" now reads "What can happen?"** and is sentence case, no longer small gold capitals.
- **The Prologue lost three boxes** that `redesign-prologue` had planned to remove later: the category panels (filled, bordered, rounded), the claim modal's step boxes, and the "Your standing" card's border. They are disclosures, so the ruling covers them. The tile grid and the expanded-tile gold join are untouched (game pieces).
- **"Your standing" is the one odd fit.** Its row is at the top, the always-visible strip sits under it and the board opens at the foot. The line under the whole unit is the closing line, so it sits under the strip shut and under the board open. Its head is now a `.tap-row` (hover lift, the shared `--color-you` ring) on a `.disclosure` / `.has-body` unit; its own line is switched off because the strip sits between. See the question file.
- **A claim's open step now shuts when pressed** (`toggleStep`; "all shut" was already a state the form allowed). Before, pressing the open step did nothing yet showed an up chevron and a hover lift. Shut steps no longer carry `aria-controls`, and the chat spans, finished lines and steps are wired the same way (`aria-controls` only while the body exists).
- **`ChecklistRow` keeps `.disclosure` on every row**, including blockers and navigate rows: the class there means "a row with a body under it" (a blocker always shows its body), so the recipe's moving line serves them too. A condition that made it true only for expanding rows would break blockers.
- **Kept boxed, deliberately:** an asset card that expands (`AssetCardSelectable`: a game piece, it keeps its box and only its mark changed) and the chat's "Speaking as" picker (a control that opens a floating menu, not a row that reveals content; it takes the new chevron, which flips up as the menu opens above it).
- The span fold in the chat gets a closing line at its foot when open; the body's hanging left rule runs down and meets it. A folded span that shows only its outcome line keeps its line under the header.

## Open questions for the owner

Four filed in `docs/work/questions/`: `prologue-boxes-ahead-of-round`, `lobby-primer-gold-title`, `standing-strip-closing-line`, `chat-picker-chevron-direction`.

## Noticed, not fixed

- PR #128 (duel pairing line) also edits `Buffet.svelte` and was still open when this branch started. Whichever merges second rebases; this branch rewrote `Buffet`'s markup, so #128's plain-text look is replaced.
- `ChecklistRow`'s doc comment and a few component headers still say "caret" for the down chevron; the name `kind="caret"` stayed, since renaming it touches every caller for no behaviour.
