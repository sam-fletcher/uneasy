# redesign-plan-panels-a-89bb97

Plan panels A of the visual redesign: Spread Rumors, Seek Answers, Spread Propaganda and Chronicle
Histories. Their own styles move onto the tokens and the screen-anatomy rulings, and their markers go.
Markup and CSS only: no rule, prompt, option or API call changes, so no game behaves differently.

## What changed and why

**1. Warnings are a `<Notice>`** (all four). A destruction warning on a break was a coloured paragraph
with no mark. `Notice` gives it the warning mark and coloured words, the one way a notice is drawn. Six
call sites: Spread Rumors (1), Seek Answers (2), Spread Propaganda (1), Chronicle Histories (2).

**2. Chronicle Histories draws its options as `<Chip>`.** Its two option rows used the older `.chip-btn`
markup, with `class:active` and `class:unavailable`. Now each is the house chip: `selected` for the pick,
and `aria-disabled` for the break that has nothing to tear, with its hint still in `title`. The chip also
carries `aria-pressed`, which the old markup did not. Its inline `style=` attributes (two list indents and
one margin) move into its own `<style>` block.

**3. Spread Rumors: no boxes.** The take-asset consent was a framed box with an accent bar. It is now a
group: its "Consent needed" heading and some space, since the open step already says what is asked. The
"Keep it secret for now" row was a bordered, filled box. It is now the shared tap row (`tap-row`, from
`tapRow.css`), with the checkbox and its explanation inside it. The checkbox keeps the default look the
rest of the app uses. Its inline `display:flex;gap:…` on the Request consent and Back buttons becomes the
`form-actions start` class.

**4. Seek Answers and Spread Rumors: quoted text is plain.** The question put to a player (Seek Answers) and
the stated rumor (Spread Rumors) were a blockquote, which the browser indents, and the rumor was also set
in italic with a rule. The style guide's Typography section keeps italic for asset names, and says other
quoted player text "stays quoted, not styled". So both are now a paragraph in quotation marks, in the
body text, with no indent, italic or rule. The question's own style was missing altogether before this.

**5. Type on the scale.** The stated rumor was 15.2px, now body (16px). Its label was 11.2px, now the label
size (12px). The "how it's spreading" and "targeting" lines were 13.1px, now small (14px). Gaps move onto
the spacing tokens. Nothing goes below 12px.

**6. Markers.** `tokens: not migrated` is removed from `SpreadRumorsPanel.svelte` and
`ChronicleHistoriesPanel.svelte`. The other two had none.

**What was not changed on purpose:**
- `PlayerChips` (shared, used by Spread Rumors, Seek Answers and Spread Propaganda, and by the other plan
  panels) still emits `.player-chip`. Converting it to `<Chip>` changes every panel's chips at once and
  breaks selectors in five e2e specs, so it belongs to one shared-file change in `plan-step-labels`.
- `showNotes={false}` stays on Spread Rumors and Spread Propaganda (`steps.test.ts` guards it).
- No `STYLE_GUIDE.md` edits this wave, as briefed. The recipes I used are in this note below.

**Recipes used here (for the style guide later):**
- *Quoted player text* (a stated rumor, a question put to a player): a paragraph in quotation marks, in the
  body text. Not italic, not indented, no rule. The guide keeps italic for asset names.
- *Consent group* (a decision in an open step): heading, then text and buttons, with 8px between them. No
  frame, no fill, no accent bar.
- *Checkbox row*: the shared `tap-row` (at least 44px, a hairline under it, from `tapRow.css`). Its hover
  lift and focus ring are written in the panel. `tapRow.css` draws them only for `button.tap-row` and
  `a.tap-row`, and a `<label>` takes focus through the checkbox inside it, so `:focus-within` draws the
  ring. The checkbox keeps the default look.

**The next item, `plan-step-labels`.** Its bullets against this PR:
- *Covered for these four panels:* `.res-warning` becomes `<Notice>` (done here); `.chip-btn` becomes
  `<Chip>` (done for Chronicle Histories, which was the only panel here with it).
- *Still open in these four:* the `.player-chip` part (`PlayerChips`, see above); "Dice roll in progress…"
  (all four panels); the "X is resolving…" lines (Spread Rumors, Seek Answers, Spread Propaganda); the
  heading hook; and deleting the old class names from `planPanel.css`, which is frozen this wave.
- *Not needed here:* `.choice-card`. None of these four uses the option card.

## How it was verified

Against this branch's own code: a private server on `:8196` with its own database, built with `make build`
after the review fixes. A second server on `:8197` runs the base commit (`2cbbdc7`, `main` before these
panels), so the "before" pictures are real builds of both sides.

- **`make check-fast`: passes** (exit 0), re-run after the review fixes. Lint, `go vet`, Go unit tests,
  `svelte-check`, Vitest (79 files, 1314 tests), the git-guard tests and the tracker check.
- **The four plans' e2e specs: 43 pass**, against `:8196`, after the review fixes (`spread_rumors`,
  `seek_answers`, `spread_propaganda`, `chronicle_histories`).
- **The full e2e suite: 204 pass**, one worker, against `:8196`, on the commit before the review fixes. The
  fixes touch only these four panels and the Chronicle spec, so the rest of the suite is unchanged, but it
  has not been re-run in full since them.
- **Guard and panel tests.** `scaleTokens.test.ts` passes for the four panels with their markers removed,
  so they carry no literal sizes, gaps or radii. The existing panel tests pass (`SpreadRumorsPanel.test.ts`,
  `SpreadPropagandaPanel.test.ts`, `PerformStepsPanels.test.ts`, `shared.test.ts`, `steps.test.ts`).
- **New guards, `planPanelsA.test.ts`** (20 cases: the rules across the four panels, plus the chip and
  `showNotes` checks). Each rule was turned red by a deliberate mutation and then restored:
  - a `.res-warning` paragraph back in Spread Rumors: "draws its warnings as a Notice" fails;
  - a `background` and a `border:` in Spread Rumors' own CSS: "adds no box in its own CSS" fails. The box
    check reads resting rules only, so a control's hover or focus may paint. A resting background on the
    secret row was turned red by a mutation and restored;
  - an inline `style=` in Spread Propaganda: "draws no inline style attribute" fails;
  - a `chip-btn` in Chronicle Histories, plus the `tokens: not migrated` marker in Spread Propaganda: the
    marker test and both chip tests fail.
- **Drive-plan, 375px, before and after: 33 screenshots each side.** Per plan, the states are preparation
  (the actor, and a bystander seeing the live draft), the pre-roll step, the step after the roll (actor and
  bystander), and the new states: the warning `<Notice>` as the actor (Seek Answers, Chronicle Histories,
  Spread Propaganda's mar, Spread Rumors), Seek Answers' question to its target (wren), and Spread Rumors'
  take-asset consent to its victim (wren). Each picture is cropped to the play pane's content. The
  comparison images show before and after side by side, one per plan.
- **Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari and Firefox;
  hover and focus (described in CSS and tested by source guards, not photographed); 344px and the
  two-column width; the Propose Duel, Make Demands and Host Festivity panels, and the war and scene
  panels (not mine); a watcher's view of every sub-step.

## Behaviour changes to look at before merging

- **A destruction warning now has a mark** (the warning triangle), as a notice. It was a bare paragraph.
- **The Chronicle Histories options are chips with `aria-pressed`.** The look is the house chip; the
  unavailable break is still `aria-disabled` with its hint.
- **Spread Rumors' consent has no frame or accent bar.** The "Consent needed" heading is the only mark of
  the decision.
- **"Keep it secret for now" is a checkbox row with a hairline.** The checkbox keeps the default look.
- **The question put to a player and the stated rumor are plain text in quotation marks.** Before, the
  question was the browser's default blockquote, and the rumor was italic with a rule. Neither is italic now.
- **The stated rumor is body size** (16px, not 15.2px) and its label is 12px.

## Open questions for the owner

- [plan-substep-lines](../questions/plan-substep-lines.md): whether sub-steps inside one open step take a
  hairline between them.
- [keep-secret-toggle](../questions/keep-secret-toggle.md): whether "Keep it secret for now" stays a
  checkbox row or becomes a chip.

The quotation question was settled by the style guide (quoted text stays quoted, not styled), so its file
is gone.

## Noticed, not fixed

- **`PlayerChips` is shared.** It still emits `.player-chip`. Changing it to `<Chip>` is one file, and it
  moves every panel at once, so it is the coordinator's call (see the `plan-step-labels` note above).
- **`AssetCreationForm`** (used by Spread Propaganda's artifact step) is not in my files. I did not check
  whether it draws boxes.
- **`.res-warning` still exists in `planPanel.css`** for the panels that are not mine (Clandestinely Liaise,
  the duel setup). It goes when those panels move.
- **Spread Propaganda's message** is small secondary text in straight quotation marks. The rumor and the
  question use curly ones. Left as it is: it is quoted text, not styled, which the guide allows, but the
  two marks do not match.
