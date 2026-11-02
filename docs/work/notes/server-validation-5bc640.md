# server-validation-5bc640

## What changed and why
- **Seek Answers / Chronicle Histories pre-roll gate.** Both gained a `CheckChoice`
  (the `ChoiceGate` hook Spread Propaganda/Rumors already use) that refuses
  make-choice with a 409 while there is no roll. Before, a make committed with nothing
  rolled and a mar 500ed. The generic route is untouched, so plans that legitimately
  have no roll are unaffected.
- **Propose Duel non-staked claim.** `ApplyChoice` now returns a 422 `httpError`, and
  `MakeChoice` honours an `*httpError` from `ApplyChoice` (new `respondApplyChoiceErr`)
  instead of flattening every error to a 500. Validation already ran before any change.
- **Spread Rumors take-consent.** `respond-take-consent` (agree path) re-checks each
  stashed asset: still in the game, still owned by the named victim, not destroyed.
  If any fails, the pending request is cleared, an action-log post says it lapsed,
  and the response is 409. `TakeAssetDenied` is left false because nobody refused, so
  the aggressor can choose again; the plan does not wedge. I did not follow the asset
  to its new owner: consent was given by one specific owner, so it doesn't carry over.

## How it was verified
- `make check-fast` green; `make test-integration` green on a private DB (the run
  before the final small refactor into `respondApplyChoiceErr`; the handler package
  was re-run via check-fast unit tests and the targeted integration tests afterwards).
- Each new/tightened test failed with its fix removed (Seek Answers, Chronicle, both
  stale-consent cases, and the duel test now asserting 422).
- e2e `seek_answers`, `propose_duel`, `chronicle_histories`, `spread_rumors` against
  my own server: 37 passed, 3 skipped (pre-existing fixmes). Un-fixmed the two specs
  that pointed at these items.
- Not run: the full e2e suite; no UI changes.

## Behaviour changes to look at before merging
- Stale take consent returns 409 and drops the request (see above). The victim's UI
  will show an error toast for that click; the row state moves on via the broadcast.

## Open questions for the owner
(Also filed in `docs/work/questions/spread-rumors-stale-take-lapse.md`.)
- Should a take whose asset changed hands follow the asset to its new owner (asking
  them) rather than lapse? I chose lapse as the safe reading; the rulebook wasn't
  consulted for this edge.

## Review follow-ups
- Only a missing row counts as stale; other DB errors return 500. The lapse check covers the stashed assets, not whether the victim is still the target player. Chronicle has a Go integration test but no e2e (Seek Answers has both). The agreeing click's 409 is kept and listed in the question.

## Noticed, not fixed
- `cmd/server` unit tests need a built frontend in `cmd/server/frontend_dist`
  (they fail on a fresh worktree until `make build`).
