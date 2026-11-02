# spec-clandestinely-liaise — Playwright regression specs for Clandestinely Liaise

## What changed and why

`frontend/tests/e2e/clandestinely_liaise.spec.ts` keeps its existing hand-off test
and gains five more, with setup in a new `helpers/clandestinely_liaise.ts`
(three players at 375px: alice prepares, bob is the partner, cara is the
bystander; setup over the API, everything inside the liaison through the panel):

- **Preparation.** Seven bad prepare requests are each refused with a 400 and
  leave no plan behind (self, no partner, either meeting peer missing, the
  wrong player's peer, a third player's peer, a non-peer asset). Then the delay
  reveal: the bystander reads who is meeting whom and cannot submit a face
  (403), alice's side locks after her face, and the faces set the row to
  start + ceil(average).
- **Secret meeting, hidden information.** With cara's page and WebSocket already
  open, alice's kept secret is written. Its existence is public (the log line,
  the asset's secret count); its text is readable by its author only; which
  asset she picked is masked from partner and bystander until both have picked.
  After Things We Share, the "look" choice lets alice read bob's secret while bob
  still cannot read hers; the bystander's responses, plans, log, visible-secrets
  list, page and every WebSocket frame never contain the secret text.
- **Outcomes, UI path.** Break (with the destruction warning) and take-a-gift
  apply only once both picks are in; the gift picker omits peers; both parting
  with 0 resolves the plan as a make through `resolvePlan` with the custom
  resolution line, logged after the parting line, no follow-up scheduled.
- **Outcomes, update and follow-up.** Update rewrites the meeting peer's
  marginalia (submit stays shut until text is written); faces 1 and 2 schedule a
  pending second liaison two rows ahead and log it.
- **Guards**, with exact statuses: order of phases (409), preparer-only advance
  and non-participants (403), own assets only, duplicate keep-secret, every
  share-choice target rule (400/403), final picks after both are in, redelay.
- **Parting, one-sided cancel:** faces 3 and 0 end it with no follow-up and the
  "parted ways" line.
- **perform_steps** (a `test.fixme`, see below): a Make Demands winner fills the
  preparer's seat on every Liaise step and the preparer is locked out (403 and a
  "bob is nominating on your behalf" line), with the Waiting On bar naming the
  winner. The retarget wait is not asserted.
- **fixme** for the stale secret count (see below).

Review changes: the kept-secret mask is now checked at all three read sites (plan
list, one plan, the public record); the Waiting On bar is asserted after a pick;
the WebSocket check looks for the `liaise.keep_secret_submitted` frame itself; the
log-names-the-holder vs masked-pick tension is commented and has a question file.
A liaison always resolves as a make, so there is no mar to test.

## How it was verified

Against my own build of this branch on a private database (port 8161, unique
binary name), never `:8090`/`uneasy_test`:

- File run 3 times in a row: 7 passed, 2 skipped (the fixmes) each time.
- The fixme run un-skipped: fails as described.
- Mutations, each watched to fail and restored (nothing committed): picked-asset
  mask removed → hidden-information test; secret text appended to the public log
  line → hidden-information test; duplicate keep-secret guard removed → guards;
  the custom resolution line removed → both outcome tests; peer-ownership check
  removed → prep test; follow-up scheduling logged as not scheduled → follow-up test.
- Also: the record's mask removed, GetPlan's mask removed, the one-sided cancel
  broken, keep-secret's row-state broadcast removed: each fails its test. The
  perform_steps body, with the demand's difficulty clamped to 1 locally, passes and
  fails when the stand-in seat is removed (nothing committed).
- `make check-fast` passes. Not run: `make test-integration` (no Go change).

What does not bite: the WebSocket-frame check passed in the log mutation only
because the log leaked first; no mutation broadcast the text over the socket
alone. The 400 for each bad prepare request is pinned as a status, not by which
rule fired.

## Behaviour changes to look at before merging

None; tests and a tracker item only.

## Open questions for the owner

- `questions/liaise-secret-holder-public.md`: is the asset holding the meeting's secret public (the log names it) or hidden (the pick is masked)? The Make Demands retarget wait on Liaise
(`questions/make-demands-retarget-wait-duel-liaise.md`) is deliberately not
pinned either way.

## Noticed, not fixed

- `next/liaise-keep-secret-not-broadcast.md`: keep-secret (and Spread Rumors'
  stash and hide-source) create secrets without `secret.created`, so open pages
  keep the old secret count and "Look at partner's secrets" offers nothing until
  a reload. Pinned as a `test.fixme`; the hidden-information test reloads alice's
  page to get past it.
- `next/make-demands-on-liaise-never-rolls.md`: a Make Demands against a liaison
  gets difficulty 0, the roll insert fails and the demand sits resolving with no
  roll, so the perform_steps fix cannot be reached in play. Pinned as a fixme.
- Redelay overflow: faces that land past row 13 (for example 6 and 6 at row 9)
  log "parted ways with no future meeting planned" even though the pair agreed on a
  delay; the spec does not pin it.
- A player may change their Things We Share pick until the partner has
  submitted (the choice is an upsert). Looks deliberate; not pinned.
- `perform-steps-panels-gate-on-preparer.md` does not apply to this panel, which
  already handles the locked-out preparer (shown by the fixme body).
