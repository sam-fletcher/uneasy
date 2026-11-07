# E2E: wait for the commit, not the status (perform-steps race)

## What changed and why
- `frontend/tests/e2e/make_demands.spec.ts`, the perform-steps spec. After
  bob clicked Apply choices, the spec waited for `status === 'resolving'`. That
  is true from the roll onward, so the wait said nothing about the click. The
  two declare-truth posts could reach the server before the make-choice was
  saved. `saDeclareTruthHandler` then counted zero picks and answered 409
  "declare-truth already completed". The spec now waits for
  `resolution_data.choice_submitted === true`, checks the two `declare_truth`
  picks are saved, and only then posts the truths.
- `frontend/tests/e2e/helpers/make_demands.ts`: new `resolutionOf(f, id)`, which
  reads a plan's `resolution_data` from the detail endpoint. The plan list
  carries it as a JSON string, so `planOf` can't serve this.

No product code changed.

## How it was verified
Everything below ran against this worktree's own server (`make build`, then
`UNEASY_DEV=1 PORT=8192`, private database `uneasy_claude_great_benz_062751`).
The database was created and dropped with a small Go helper over the published
port, so no `uneasy-*` container was touched.

- **Bites before the fix.** A `page.route` on `**/api/plans/*/make-choice` that
  holds the request 1500 ms, placed before the Apply click, made the spec fail
  with the exact error from the report:
  `POST /api/plans/3/declare-truth failed (409): {"error":"declare-truth already completed for this plan"}`.
- **Passes after the fix, with the same delay.** Passed in 3.1 s.
- **No delay, after the fix:** `--repeat-each=12` on the perform-steps test:
  12 passed. Whole `make_demands.spec.ts`: 18 passed.
- **Full Playwright suite on the worktree server:** 159 passed (2.1 min), all 159 specs, against :8192.
- **`make check-fast`:** passes (exit 0): vet, deadcode, Go unit tests, svelte-check, Vitest (677/677), the guard-hook tests (61/61) and the tracker check.
- **Not run:** `make test-integration` (no Go code changed, so the integration
  suite has nothing new to exercise). The e2e suite ran only against my own
  server; none of it ran against `:8090` or `:8080`.

## Behaviour changes to look at before merging
None. Test-only: the spec now waits for the commit it was meant to wait for.

## Open questions for the owner
- [Should a Make Demands winner who still owes a decision be refused a ready?](../questions/make-demands-held-winner-ready.md)
  Found while sweeping (below): the server accepts a held winner's ready and
  resolves the roll without the decision. This is a product question, not a
  spec change, so the spec was left alone.

## Noticed, not fixed
- **Misleading 409.** `saDeclareTruthHandler` answers "declare-truth already
  completed for this plan" whenever zero picks are saved yet. With no picks
  the message is wrong: nothing has been completed. Product code, so not
  changed here.
- **Sweep: checked, left alone.** I looked for the same shape (click a submit,
  then a poll on something already true, then an API call). Each candidate
  below was either followed by a real wait or sits after one.
  - `make_demands.spec.ts` ~217–219 (leverage none): waits for the "Your
    leverage decision is in" text before the readies. Real wait.
  - `make_demands.spec.ts` ~341–343 (keep the target): polls `settled`, which
    only flips when the decision lands. Real wait.
  - `make_demands.spec.ts` ~497–498 and ~507–508 ("…form is gone" overlay
    specs): click, then readies with no wait on the click. Probed with the
    1500 ms recipe: the click's request never reached the server before the
    test ended, and the tests passed anyway, because the readies resolved the
    roll. So they do not depend on the click landing and show no failure. They
    do not prove the click landed first either. Left alone; the owner question
    above decides what they should assert.
  - `propose_duel.spec.ts` ~536, ~591, ~603, ~649: `status === 'resolving'`
    reads come after a UI wait for the claim controls, or after an awaited API
    call. Each click is followed by a poll for `resolved`, which is a real
    wait.
  - `spread_rumors.spec.ts` ~312: after the Disagree click, waits for the
    "declined" text before the status check. The Apply click is followed by a
    poll for `resolved`.
  - `seek_answers.spec.ts` ~457, ~529, ~609: synchronous reads after a real UI
    wait or an awaited API call.
  - `spread_propaganda.spec.ts` ~447 and ~524: ~447 reads after a UI wait. ~524
    waits for the pre-roll form to go, and the panel only drops it after
    `castSpreadPropagandaRoll` returns (`onPlansChanged()` runs after the
    await), so the roll is open when the spec reads it.
  - `exchange_courtiers.spec.ts` ~248: after a poll on the peer's owner, which
    is a real wait.
  - `clandestinely_liaise.spec.ts` ~590, its helper ~214, and
    `host_festivity.ts` ~81: `resolving` polls that follow awaited API calls
    only. No UI click precedes them, so there is no race to lose.
