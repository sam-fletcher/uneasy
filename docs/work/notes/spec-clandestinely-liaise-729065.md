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
- **fixme** (new bug, see below).

## How it was verified

Against my own build of this branch on a private database (port 8161, unique
binary name), never `:8090`/`uneasy_test`:

- File run 3 times in a row: 6 passed, 1 skipped (the fixme) each time.
- The fixme run un-skipped: fails as described.
- Mutations, each watched to fail and restored (nothing committed): picked-asset
  mask removed → hidden-information test; secret text appended to the public log
  line → hidden-information test; duplicate keep-secret guard removed → guards;
  the custom resolution line removed → both outcome tests; peer-ownership check
  removed → prep test; follow-up scheduling logged as not scheduled → follow-up test.
- `make check-fast` passes. Not run: `make test-integration` (no Go change).

What does not bite: the WebSocket-frame check passed in the log mutation only
because the log leaked first; no mutation broadcast the text over the socket
alone. The 400 for each bad prepare request is pinned as a status, not by which
rule fired.

## Behaviour changes to look at before merging

None; tests and a tracker item only.

## Open questions for the owner

None new. The Make Demands retarget wait on Liaise
(`questions/make-demands-retarget-wait-duel-liaise.md`) is deliberately not
pinned either way.

## Noticed, not fixed

- `next/liaise-keep-secret-not-broadcast.md`: keep-secret broadcasts no
  `secret.created`, so the partner's open page keeps the old secret count and
  "Look at partner's secrets" offers nothing until a reload. Pinned as a
  `test.fixme`; the hidden-information test reloads alice's page to get past it.
- A player may change their Things We Share pick until the partner has
  submitted (the choice is an upsert). Looks deliberate; not pinned.
- The Make Demands `perform_steps` panel gating (`perform-steps-panels-gate-on-preparer.md`)
  does not apply to this panel, which already handles the locked-out preparer.
