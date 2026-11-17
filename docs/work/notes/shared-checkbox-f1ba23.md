# One checkbox: the asset card's drawn box, shared

## What changed and why
The owner ruled that the app has no plain browser checkboxes, and that the
one every checkbox wears is the box the asset cards already draw. There were
two plain ones left (Make War's "Surrender after this payment" and Spread
Rumors' "Keep it secret for now"), each at a different size and each explained
badly: the surrender checkbox's explanation lived only in a `title`, which a
phone can't show.

- **`shared/checkbox.css`** is the box's one recipe: 24px, the control border,
  the raised-twice fill, the control corner; ticked is the 2px bright gold
  border (inset shadow, no fill) with `ToneMark`'s success tick; locked is a
  hairline; focus is the gold ring. Each rule answers to two spellings: the
  asset card's `aria-checked` / `aria-disabled` attributes, and a real input's
  `:checked` / `:disabled` / `:focus-visible` beside the drawn box. The file
  also holds the row (`.check-row` on a `.tap-row`).
- **`shared/Checkbox.svelte`** is the whole row: box, `label`, optional
  `description` (secondary text) and a `reason` that shows only while
  `disabled`. Under the drawn box is a real `<input type="checkbox">`, visually
  hidden (clipped, not removed), so `bind:checked`, Tab, Space and screen
  readers work natively; the label names it and the description and reason
  describe it. `title` is not accepted on it, on purpose.
  - *Rejected:* a `<span role="checkbox">` with hand-written key handling, as
    the asset card has. It would have re-implemented the browser's keyboard and
    form behaviour; the card only does it because its box sits inside a header
    `<button>`, which can't hold an input.
- **`AssetCardSelectable.svelte`** keeps its markup and behaviour. Its three
  boxes gained the `check-box` class and lost their own copy of the look; what
  stays in the card is the marginalia rows' smaller box (`.m-tap`, 20px) and the
  header's 44px tap area.
- **Make War** (`BattleCostForm`): the explanation is now words on the screen;
  with "Propose peace" picked the row is locked and says "Doesn't apply when
  proposing peace." Both `title`s are gone. **Spread Rumors**: same two-line
  explanation, its local checkbox CSS deleted. The preparer's and a watcher's
  views, the `fieldset disabled` mirror and the secret-text gating are
  untouched.
- **Guard** `shared/checkboxGuard.test.ts`: fails if any `.svelte` file but
  `Checkbox.svelte` renders `<input type="checkbox">` or `type="radio"`. The
  chat's `role="switch"` input is listed with its reason, and an entry fails if
  its switch goes away. A second test holds the box to one definition
  (`.check-box` declared only in `checkbox.css`; the card's boxes wear it and
  declare no fill, border or corner of their own).
- **Docs:** a "Checkboxes" recipe in `docs/STYLE_GUIDE.md` (when a checkbox,
  when a switch, when a chip), a row in its shared-components table, and a
  "Checkboxes" section on `/dev/styleguide`.
- **Tracker:** deleted `questions/keep-secret-toggle.md` and
  `questions/checkbox-sizes.md` (answered by the ruling) and the Make War
  surrender bullet in `next/plan-polish-leftovers.md`. Added
  `questions/marginalia-checkbox-size.md` and
  `next/spread-rumors-secret-draft-mirror.md` (see below).
- **e2e** `checkbox.spec.ts`, new: see below.
- Two small test edits that follow from the move: `retinuePieces.test.ts`
  reads the box's corner from `checkbox.css` now, and `styleguide.test.ts`
  expects the new section.

## How it was verified
Everything ran against this worktree's own code: a server built with
`make build`, on `:8191` with a private database
(`uneasy_claude_shared_checkbox_f1ba23`, dropped afterwards). "Before" shots
came from `main`'s e2e server on `:8090`, which was at the same commit
(`d053afe`).

- **`make check-fast`:** passes (Go vet and unit, svelte-check 0 errors, 92
  Vitest files / 1,775 tests, hook tests, tracker check).
- **Guard, red first:** run before the migration, it failed naming
  `SpreadRumorsPanel.svelte` and `BattleCostForm.svelte` as plain checkboxes, and
  the asset card as still drawing its own box. Green after.
- **Each new test bites** (mutate, run, restore): the box wrapped so it is not
  the input's next sibling; the reason shown when not locked; the reason never
  shown; the explanation put in a `title`; `aria-labelledby` dropped; the
  ticked rule moved above the locked one; the locked row faded with opacity; a
  card box drawing its own fill; a card box without `check-box`; a plain
  checkbox, a radio, and an unlisted switch added. Each fails the test written
  for it. One mutation survived at first (a vacuous assertion on an apostrophe
  the renderer doesn't escape); fixed and re-run.
- **Measured in a browser (375px):** on `/dev/styleguide`, computed styles of
  every state: ticked border `--color-you` with the inset ring, locked border
  `--color-border`, locked and ticked stays gold, locked words
  `--color-text-faint`, opacity 1, cursor `not-allowed`, the box 24×24 centred
  on the first line of the words, the hidden input 1×1. By keyboard: Tab shows
  the 2px gold ring (offset 2px) on the box, Space toggles, a tap on the row
  toggles, a tap on a locked row does nothing. The accessible name is the label
  and the description (and the reason, while locked) describe it. This caught a
  real bug the text-level tests could not: my first markup wrapped the box in
  an extra span, so `input:checked + .check-box` never matched and no box ever
  went gold. There is now a test for that.
- **Screenshots at 375px, before and after:** Make War's cost of battle as the
  payer (unticked, ticked, locked with Propose peace picked); Spread Rumors'
  preparation as the preparer and in a watcher's mirror; Propose Decree's
  "Leverage one asset to join" picker; the new `/dev/styleguide` section. They
  are attached to the PR.
- **e2e, full suite on `:8191`:** 216 passed (3.0 min), including
  `make_war.spec.ts`'s "the surrender checkbox is a box with its words beside
  it on a phone" (the hidden input is 1×1, so it stays narrow and the words stay
  on the phone), and the styleguide spec. Run before the new spec below was
  added.
- **New e2e `checkbox.spec.ts`** (375px): the surrender row (explanation on
  screen and no `title`; a tap on the words ticks it and the box takes the gold
  ring and tick; Space toggles; "Propose peace" locks it, clears it and shows the
  reason in words, faint and not faded; back to a payment unlocks it) and Spread
  Rumors' toggle (the preparer ticks it by its words; a watcher's mirror shows
  it ticked and doesn't respond). On the branch: 2 passed. On `main`'s e2e
  server the surrender test fails at its first assertion (the explanation isn't
  on screen); the Spread Rumors one passes there too, since ticking and the
  inert mirror already worked, so it guards the migration and doesn't prove it.

What did **not** run: `make test-integration` (no Go code changed), anything
at a width other than 375px, and the real WebSocket path between two players on
a phone (the watcher's view was two Playwright contexts on a laptop).

## Behaviour changes to look at before merging
- **A locked asset card's box is a hairline now.** The recipe has one locked
  look, so a card's box that can't be picked (the unpicked cards once Propose
  Decree's single pick is made; every card in a watcher's read-only mirror)
  drops from the control border to the hairline. It matches the card's own
  border and words, which already drop to the hairline when locked. A picked and
  locked card still keeps its gold box. The enabled box is unchanged. The
  alternative is to scope the hairline to `Checkbox` only, at the cost of the
  two looks being able to drift, which is what the ruling was against.
- **A ticked, locked checkbox keeps its gold border** (so a watcher can see
  what the preparer ticked), as a locked chip and a locked card do.
- **Every checkbox row now has a hairline under it** (it is a tap row) and
  highlights on hover. Spread Rumors' row already had both.
- The surrender explanation reads: "After this payment is recorded you are
  marked surrendered, and each opposing opponent who hasn't surrendered gets to
  claim one of your assets." It is the old `title`, in the second person.

## Open questions for the owner
- Should the marginalia boxes inside an open asset card (20px, with a 14px
  tick) grow to the house 24px? They sit in 44px rows, so 24px would fit. I left
  them as they were because changing them is a design call, not covered by the
  ruling. File: `docs/work/questions/marginalia-checkbox-size.md`. Until it is
  answered, "one size" is true of every box except these.

## Noticed, not fixed
- **A watcher sees the draft text of a secret rumor.** While the preparer has
  "Keep it secret for now" ticked, the watcher's read-only mirror still fills in
  the preparer's typed text (`emitPrepDraft` sends `notes` regardless of
  `keep_secret`). It happens on `main` too. The panel tells the preparer "Others
  won't see the text until you spread it", so this is hidden information
  reaching the table. I changed nothing in the gating, as briefed. Filed as
  `docs/work/next/spread-rumors-secret-draft-mirror.md`; the coordinator may want
  to rank it above the polish items.
- The guard reads markup, so it can't see a type set through an expression
  (`type={kind}`). No input does that today.
