# Chronicle Histories UI fixes (chronicle-ui-1eb99c)

## What changed and why
- Buffet Mar tab now says every player, the preparer included, chooses one (the preparer already got a picker; the reference contradicted it). Un-fixme'd its pinned e2e.
- Take-control wording now says the affected player has the final say over how their character acts, and the chooser can unselect the option if they can't agree. Consent step still unbuilt, as ruled.
- First-timer list: meter reason says a lower rank is better; "Break an invoked artifact" is disabled (chips in make and mar) with a hint when no invoked artifact has marginalia left; pre-roll scene field says it goes to the chat; make-step Summary is now optional in the panel (the server always treated narration as optional).
- Server: `invoke_another` refuses a destroyed artifact with 409 (applies to make-step and mar). Integration test added.
- `.chip-btn:disabled` style (dimmed, not-allowed) in planPanel.css, since no disabled chip existed before.
- Marginalia-owner warning is not built: question file added.

## How it was verified
Against this worktree's server on a private DB: the chronicle_histories e2e spec (17 passed, including 4 new/un-fixme'd), `make test-integration` (full, passed), 375px screenshot of the make step. The new integration test fails with the server check removed. `make check-fast`: see PR.

## Behaviour changes to look at before merging
- Break option is disabled whenever no invoked artifact has an intact marginalia, including after all are torn mid-scene.
- Server returns 409 (not 400) for a destroyed artifact on invoke_another.
- Summary on make steps is optional; an empty summary adds nothing to the log.

## Open questions for the owner
- docs/work/questions/chronicle-warn-marginalia-owner.md

## Noticed, not fixed
- Nothing further.
