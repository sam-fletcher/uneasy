# Two rules the app read differently from the book

Branch `claude/book-rule-fixes-8e8635`. Item: `docs/work/next/book-rule-fixes.md` (deleted).

## What changed and why

The owner ruled on 2026-11-26 (from the rulebook's own pages) on two places where the app was
stricter or looser than the book.

**Propose Decree: the signatory's addendum is required, on a make and on a mar.** The book puts the
law in force "after the signatory adds an addendum" (p.47); the app let a blank one through.

- `set-addendum` (`handler/plan_propose_decree_routes.go`) now refuses a blank or whitespace-only
  addendum with a 400, and always requires the `and`/`but` connector (before, it was required only
  when there was text, so the connector check could never fire on a blank). The "left the law without
  an addendum" log line can no longer happen and is gone. A refusal changes and logs nothing.
- `ProposeDecreePanel.svelte`: "Place addendum" is disabled until there is text, like every other
  required text field; the prompt no longer says "(or leave it blank)" and the placeholder no longer
  says "(optional)". `setAddendum()` in `lib/api/plans.ts` takes a required connector.

**Spread Rumors: an open rumor may be left unwritten at preparation.** The book asks for the text at
preparation only "if you'd like to keep it secret for now" (p.59). A secret rumor still needs words.

- The block was in **two** places, not one: `srHandler.ValidatePreparation` (the brief's "requires
  preparation_notes") and the generic "preparation notes are required" check in
  `handler/plan_validation.go`, which applies to every plan. The generic one now skips Spread Rumors.
- A secret rumor's blank is refused where its secret is made: `stashSecretRumor`
  (`plan_spread_rumors_routes.go`), inside the prepare transaction, so the refusal prepares nothing
  and hides nothing. The old `KeptSecret` branch in `ValidatePreparation` was the only reader of
  `ValidationContext.KeptSecret`; see "Noticed, not fixed".
- `rumors/PrepForm.svelte`: "Prepare Plan" no longer waits for text unless "Keep it secret for now" is
  ticked (then it waits for text and a holder asset). The placeholder tells the preparer they can
  leave it until the roll.
- **Pre-roll needed no code change.** `sr-cast-roll` already refuses a blank rumor, and the field
  already starts empty when there is no draft (and shows no "starts from your preparation note" line).
  Tests and screenshots hold that.
- **Watcher's read-only prep screen:** it said "{preparer} is writing the rumor", which is no longer
  always true, so it now says "{preparer} is preparing the rumor". `docs/work/questions/kept-secret-line.md`
  (the owner's open question about those two lines) is updated to the new wording.
- **Make Demands stand-in:** nothing to change. Preparation is the preparer's alone, and the Spread
  Rumors pre-roll is preparer-only (perform_steps is post-roll only, `rumors/shared.ts`). The addendum
  prompt is neutral ("Add your rider…") and the stand-in case is covered by the existing perform-steps
  tests, which now place a real addendum.

## How it was verified

Against this branch's code, on a private database (now dropped) and a private server on `:8187`:

- `make check-fast`: pass (after `make build`, which the `cmd/server` tests need).
- `make test-integration`: the whole suite passes. (`cmd/server` failed once, because the run began
  while I was rebuilding the embedded frontend; re-run alone, it passes.)
- **New tests, each shown failing with the four source files reverted to `main` and passing with the
  fix:**
  - `TestProposeDecree_Addendum_BlankIsRefused` (make and mar; blank, connector-only, spaces-only and
    text-without-connector all 400; nothing placed or logged; a written addendum then enacts).
  - `TestSpreadRumors_OpenRumorMayBePreparedBlankAndIsStatedAtThePreRoll` (nil, empty and whitespace
    notes; pre-roll refuses a blank rumor; the stated rumor is what a make spreads).
  - `TestSpreadRumors_SecretRumorStillNeedsItsText` (blank secret refused, no plan, no secret). It was
    also shown failing with only `stashSecretRumor`'s check reverted, so it pins that check on its own.
  - Existing tests that used blank addenda now place a real one; `TestSpreadRumors_RejectsNoNotes`
    became `…AcceptsNoNotes`; `ValidatePreparation`'s "fresh prepare still needs the text" assertion
    flipped; `TestLog_ProposeDecree_BlankAddendumIsStillAnnounced` is deleted (that state is gone).
- e2e against `:8187`: `propose_decree`, `spread_rumors_secret_draft` (reworded line), `checkbox`, and
  the new `spread_rumors_open_blank.spec.ts` (prepare blank as an open rumor, secret blank keeps the
  button off, pre-roll starts empty and waits for the rumor) all pass. The decree spec gained a blank
  addendum check (API 400 and the button disabled).
- 375px screenshots (attached to the PR): the addendum step as the signatory and as a watcher; the
  rumor's preparation (open and blank, as the preparer and as a watcher; secret and blank); and the
  pre-roll with a blank draft, as the preparer.

Not run against this code: the full e2e suite (only the specs above); a 2-player or 4-player game for
the decree (3 players only); desktop-width screenshots.

## Behaviour changes to look at before merging

- Games already mid-resolution at the decree's addendum step now need text before the button works;
  nothing stored changes shape. A decree already enacted with a blank rider keeps it (`pdComposeAddendum`
  still treats a blank as "no rider", so old law rows read the same).
- A Spread Rumors plan already prepared with notes is unaffected. New ones can be prepared with an
  empty note: the "prepared Spread Rumors" log line then reads "there's a rumor brewing about X."
  with no quotation, and the Public Record has no rumor text until the pre-roll posts it.
- The watcher's mirror says "is preparing the rumor" (see the question file).

## Open questions for the owner

None new. The wording of the watcher's lines is still the open question in
`docs/work/questions/kept-secret-line.md`, now with "preparing".

## Noticed, not fixed

- `ValidationContext.KeptSecret` (`handler/plan_contract.go`) and its one setter
  (`mdReplayDeclared` in `plan_make_demands_leverage.go`) are now read by nothing: the only reader was
  Spread Rumors' blank-notes check. I left them alone because the setter is in the Make Demands files
  another session owns. Worth deleting once that lands.
- The generic "preparation notes are required" check in `plan_validation.go` now has a Spread Rumors
  exception; if more plans earn one, it should become a handler method rather than a growing `if`.
