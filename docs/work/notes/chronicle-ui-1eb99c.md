# Chronicle Histories UI fixes (chronicle-ui-1eb99c)

## What changed and why
- Buffet Mar tab now says every player, the preparer included, chooses one (the preparer already got a picker; the reference contradicted it). Un-fixme'd its pinned e2e.
- Take-control wording now says the player whose character it is decides how they act, and the chooser drops the option if they can't agree (paraphrased). Consent step still unbuilt, as ruled.
- First-timer list: meter reason says a lower rank is better; "Break an invoked artifact" is disabled (chips in make and mar) with a hint when no invoked artifact has marginalia left; pre-roll scene field says it goes to the chat; make-step Summary is now optional in the panel (the server always treated narration as optional).
- Server: `invoke_another` (make-step and mar) and the pre-roll cast-roll invoke both refuse a destroyed artifact with 409 (well-formed id, board moved on; 400 stays for not-an-artifact). Integration tests for each, the pre-roll one with a live-artifact positive control.
- Break chip uses a scoped `.chip-btn.unavailable` + `aria-disabled` (not `:disabled`, which leaked into face chips and disabled-fieldset mirrors). The click is guarded, and a selected Break is cleared if the last marginalia goes. Hint reads "Needs an invoked artifact that still has a marginalia to tear."
- A refused make-step no longer re-posts "Chronicle Histories succeeded"; it's logged only after the step applies.
- Marginalia-owner warning is not built: question file added.

## How it was verified
Against this worktree's server on a private DB: the chronicle_histories e2e spec (19 passed, including the un-fixme'd test and 5 new ones (mar-side break chip, bystander chips not dimmed)), `make test-integration` (full, passed), 375px screenshot of the make step. The new integration tests fail with the server check removed. `make check-fast`: see PR.

## Behaviour changes to look at before merging
- Break option is disabled whenever no invoked artifact has an intact marginalia, including after all are torn mid-scene.
- Server returns 409 (not 400) for a destroyed artifact on invoke_another.
- Summary on make steps is optional; an empty summary adds nothing to the log.

## Open questions for the owner
- docs/work/questions/chronicle-warn-marginalia-owner.md

## Noticed, not fixed
- Nothing further.
