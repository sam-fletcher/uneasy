# Checkbox width: the global input rule leaves checkboxes alone

## What changed and why
The global text-field rule in `routes/+layout.svelte` was written for every
`<input>`, checkboxes and radios included. Its `width: 100%` stretched a
checkbox across its whole row. In the surrender row on Make War's cost of
battle the box sat in the middle and the words were pushed to x = 375, the
phone's edge, so the row showed a bare box and no text (see the before
screenshots in the PR).

- The rule is now `:global(input:not([type='checkbox']):not([type='radio']))`.
  Every other input (text, email, password, and the five with no `type`, which
  mean text) keeps the same look.
- The `:focus` outline rule is unchanged, so a checkbox or radio still gets the
  gold focus ring for keyboard users.
- Local overrides left alone, neither redundant:
  - Spread Rumors' `.sr-secret-toggle input[type='checkbox']` sets a 20px box;
    the browser default is 13px.
  - The chat's `.bookkeeping-toggle input` is drawn as a switch.
  - `.surrender-label input { flex: none; }` is also unchanged.
- `docs/STYLE_GUIDE.md` says "inputs" means the text-like types, so the rule
  is written down where the input size rule already is.
- New guard `src/lib/inputTypes.test.ts`: reads `routes/+layout.svelte` and
  fails if any global input rule that sets size, padding, border or font size
  reaches checkboxes or radios.
- New e2e assertion in `make_war.spec.ts` ("the surrender checkbox is a box
  with its words beside it on a phone"): the checkbox is at most 24px wide, its
  words sit to its right, and the words end inside the 375px width.

## How it was verified
Everything below ran against this worktree's own code, on a server at
`:8192` with a private database. Nothing ran against `:8080` or `:8090`.

- **Red first.**
  - `inputTypes.test.ts` on the old rule: fails, naming `:global(input)`
    as the offender.
  - The e2e assertion on the old build (`main`'s code): checkbox 359px wide,
    words at x = 375 with width 62, so they start at the edge of the phone.
- **Green after the fix.**
  - Checkbox 13 × 13, row 44px tall, words at x = 29 (width 178), inside 375.
  - Spread Rumors toggle: the screenshot is byte-identical before and after.
  - Chat switch: byte-identical before and after (40 × 24).
  - Text input on `/profile` ("Join code"): the same 271 × 44 box. The PNGs
    differ at the byte level only; they look the same.
- `make check-fast` on this worktree: passes (svelte-check 0 errors; Vitest 89
  files, 1745 tests; lint, vet, deadcode, guard_git and tracker check).
- `make test-integration` on the private database
  `uneasy_claude_checkbox_width_af6bae_it`: every package passes (cmd/server,
  db, game, handler, hub, middleware). No Go code changed; this is the
  verification bar, not a check on the CSS.
- Full e2e suite (216 tests, `npx playwright test`) against
  `E2E_BASE_URL=http://localhost:8192` on the private database
  `uneasy_claude_checkbox_width_af6bae`: 216 passed, 0 failed.
- Not run: the e2e suite against any other build; the other viewports (only
  375px was checked for the look, plus the desktop tests that the suite runs).

## Behaviour changes to look at before merging
- Checkboxes and radios no longer get the global border, fill, padding, radius
  or font size. Only the surrender checkbox changes look, becoming the browser's
  13px box in its 44px row. The Spread Rumors toggle and the chat switch look the
  same because they set their own size and appearance.
- Nothing else changes: the other 24 inputs (text, email, password, and the
  five with no `type`) keep the global look.

## Open questions for the owner
- Should every checkbox be one size? See `docs/work/questions/checkbox-sizes.md`.

## Noticed, not fixed
- The comment on Spread Rumors' "Keep it secret" toggle says it uses "the
  default one the rest of the app uses", but its own rule sets a 20px box. Left
  for the question above to settle.
- The throwaway evidence spec used for the before/after screenshots was not
  committed.
