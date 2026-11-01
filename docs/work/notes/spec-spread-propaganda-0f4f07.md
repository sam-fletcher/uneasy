# Spread Propaganda e2e regression specs

## What changed and why

Added `frontend/tests/e2e/spread_propaganda.spec.ts` (phase 4 of the UX audit:
one regression spec per plan). Three specs run, one is `test.fixme`:

- **mar flow** — the pre-roll step (preparer sees the form, bystander a wait line;
  a choice before the dice is refused; the stated message reaches the bystander),
  the Buffet previewable by a bystander before any roll (Make and Mar tabs), the
  shared Stepper (exact total enforced, a single-effect option capped, counter-
  propaganda greyed with its reason, 44px targets, + shuts at the budget), the
  committed state ("Waiting for alice to face the fallout…" for the bystander),
  break-yourself driven through the picker, and the plan resolving without a
  "Complete plan" click.
- **hidden information** (same spec) — once both tables are open, alice writes a secret on
  the asset she then breaks; bob's WebSocket frames are recorded too. Her own reads still return it; as bob, no response (assets, plans,
  plan, visible secrets, posts, per-asset secrets) and no rendered page contains
  the text, while the secret's *existence* (`secret_count`) stays public.
- **make flow** — no "Apply choices" gate, the artifact + the changes narration are
  one required step, bystander waits on it, the plan resolves on the artifact and
  the narration lands in the log.
- **zero-pick mar commit** — titled for what it proves: it auto-resolves, leaving nobody
  waiting.
- **wedged row** (audit finding 3) — with one peer, "a peer leaves" can't be picked past
  capacity (option shut with its reason); and a stale over-pick (two hand-overs, one
  peer) is carried to resolved by the forfeit hatch after the one peer is given.
- **fixme** — the artifact step appearing live on a forced make; see below.

Not pinned (waits on the owner): who learns a broken asset's secrets when a Make
Demands "perform steps" winner stands in for break-yourself.

## How it was verified

Against my own worktree build, on a private database, via `E2E_BASE_URL`
(port 8183); never `:8090` or `uneasy_test`.

- 3 consecutive runs: 5 passed, 1 skipped each (Playwright counts the fixme as skipped).
- Each guard shown to bite by breaking it locally and reverting:
  forced make not applying (make spec fails); `AutoCompleteAfterChoice` false
  (zero-pick and mar specs fail); a choice allowed before the cast (mar spec
  fails at the 409 check); `give_peer` cap raised, forfeit refused, secret text added
  to the `secret.created` broadcast (the cap, wedged-row and hidden-info specs fail); the Buffet removed from the panel (mar spec fails);
  Lay-Low cap raised (mar spec fails at the Stepper); `GetSecrets` returning every
  secret (hidden-information check fails). Mutations were not committed.
- `make check-fast` passes.
- Not run: `make test-integration` (no Go changed).

## Behaviour changes to look at before merging

None (test-only).

## Open questions for the owner

- `questions/spread-propaganda-lay-low-cap.md` — the picker caps "keep your head down"
  at one; the book lets options repeat. The spec marks it as a UI choice.
- The demand-winner break-yourself question stays open.

## Noticed, not fixed

- **An auto-applied make isn't pushed to open panels** (`next/auto-applied-choice-not-broadcast.md`,
  shared with Make Introductions' fixmes).
  The make spec reloads both pages after the server has applied the choice; the
  `test.fixme` spec pins the live behaviour.
- The zero-pick spec guards "the table isn't stranded"; since Spread Propaganda
  auto-completes a zero-pick mar, the choice-committed marker isn't observable on
  its own there (it matters for Spread Rumors, covered by Go tests).
- A 3-player track can give alice a dummy-shifted rank (rank 3 → difficulty 4), and
  the actor rolls only two dice, so a make needs rank 1. The spec reads the difficulty
  from the roll rather than assuming it.
