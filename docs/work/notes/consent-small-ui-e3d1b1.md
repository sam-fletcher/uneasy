# consent-small-ui-e3d1b1

## What changed and why

Three small changes the owner approved on 2026-11-26 (item `consent-small-ui`, now deleted).

1. **Every checkbox is 24px.** `AssetCardSelectable.svelte` gave the marginalia rows an
   own-size box (`.m-tap`, 20px, with a 14px tick). I deleted that rule and the tick's
   `size={14}`, so a row's box is the shared `.check-box` from `shared/checkbox.css` and its tick
   is `ToneMark`'s default, exactly as in the card header. The `.m-tap` class stays: the row's
   44px tap area (`.m-tap::after`) stretches from it. The rows were already 44px.
2. **A scene's groups take the step sub-heading.** Prompt, Where, When and Who else is here
   (`SceneSetupForm.svelte`), and Prompt, Location and Present (`SceneDetailsPanel.svelte`), were
   a local `.label` (12px secondary capitals). They are now `<h3 class="choices-header">`, the
   class `planPanel.css` already uses for a plan's sub-headings (12px, deep gold, letter-spaced);
   both files import `planPanel.css` and no longer define a heading rule, so the scene can't drift
   from a plan. Neither scene has a field with its own prompt (the inputs carry an `aria-label`
   and a placeholder), so there was no 14px `.picker-label` to apply. `docs/STYLE_GUIDE.md`
   says so, and says the box is 24px everywhere.
3. **Comments that cited deleted question files** now state the answer: `lib/recordLines.ts`
   and its test (a war or liaison keeps "Resolving" while it is played) and `ChatPanel.svelte`
   (the Tones chip keeps "Tones · 0 set"). Only the comment in `ChatPanel.svelte` changed.

I also restyled the **Prompt** heading in both files, which the item doesn't name: it is the same
`.label` class as the others, and leaving it secondary would have put one grey heading in a run
of gold ones.

## How it was verified

- `make check-fast`: passed (svelte-check 0 errors, 2382 unit tests).
- New or changed tests bite: with the three `.svelte` files restored to `main`'s versions, six
  tests fail (24px box; the scene headings, in both files), and pass again with the change.
  - `shared/checkboxGuard.test.ts`: `.check-box` is 24px; the card sets no size on `.m-tap`
    or `.select-tap`; no `ToneMark` in the card has a `size`.
  - `SceneSetupForm.test.ts`, `SceneDetailsPanel.test.ts`: the groups render as
    `choices-header`, and neither file declares a heading rule.
- 375px screenshots before and after, on a private server and database: the marginalia picker
  in Make War's cost of battle (unticked and ticked; `.m-tap` measured 20×20 before, 24×24 after),
  the scene setup form, and the scene details panel.
- e2e against my own server: `make_war`, `scenes` and `exchange_courtiers` specs, 41 passed.
- **Not run:** `make test-integration` (no Go changed); a break in Exchange Courtiers by eye (its
  spec passes, and it draws the same card); a desktop-width look; a watcher's scene setup by eye
  (same markup, disabled controls).

## Behaviour changes to look at before merging

- The scene's group headings are now deep gold. On the setup form the first heading ("Prompt")
  sits just under the step's own gold heading, so two gold lines are close together; they differ in
  size (18px against 12px) and the sub-heading is capitals.
- The marginalia boxes in an open card are 4px larger, so a row's words sit 4px further right.

## Open questions for the owner

None.

## Noticed, not fixed

- Four more comments cite question files that no longer exist:
  `frontend/tests/e2e/propose_decree.spec.ts` (`propose-decree-make-autoresolve`),
  `frontend/tests/e2e/spread_propaganda.spec.ts` (`spread-propaganda-demand-secrets`),
  `frontend/tests/e2e/clandestinely_liaise.spec.ts` (`make-demands-retarget-wait-duel-liaise`), and
  `frontend/src/lib/boundaryError.ts` and `docs/work/tracker.py` (`render-crash-report`). They are
  outside this item, and two belong to files other sessions are editing.
- `docs/work/questions/scene-labels-and-fields.md` ends with a note pointing at
  `next/consent-small-ui.md`, which this PR deletes. Its remaining question (typed place or time as
  the answer) is still open; the note can be trimmed to say the other half is built.
