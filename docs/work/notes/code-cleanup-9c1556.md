# Hand-over: code and test cleanups (code-cleanup-9c1556)

Branch `claude/code-cleanup-9c1556`. Works through the six items in
[next/code-cleanup-leftovers.md](../next/code-cleanup-leftovers.md) that were
not left for the owner. Two bullets stay in that file (`PendingHostMars`, and
the plan panels over 400 lines).

## What changed and why

- **`middleware.AccountContext` removed** (`20050acd`). It was only called
  from tests, and `make deadcode` flagged it. The tests that used it now log in
  through a real session: `serveAs` in `handler/integration_harness_test.go`
  creates a sessions row, sends the cookie, and runs `EnsureSession`. Two
  logged-in validation test groups moved from the unit suite to the integration
  suite, because the login gate needs the database (see below). No test of the
  function itself existed.
- **`deadcode` pinned to v0.52.0** (`4a386e56`). It was `@latest`. v0.52.0
  was released 2026-10-10; v0.53.0 landed the day of this change. On this tree
  it reports nothing now.
- **`NextDeclarer` dropped from `ResolveBout`** (`c2959332`). The bout-respond
  handler never read it. It sets the next initiative itself, to the other
  duellist (`pduelOpponentID`). The rulebook's Propose Duel section swaps
  initiative after a match and after a decided bout alike, so the two always
  agreed. No one declares out of turn, so this is not a rules question and no
  question file was written. The duel test
  `TestProposeDuelHTTP_FinalBoutTie_SaysTheDiceAreLost` now asserts that
  initiative passes after a tied bout. The other duel tests read initiative
  afresh each bout, so they would not catch a missed swap.
- **Shake-Up seeded at row 14** (`0d23f394`). `SeedShakeUp` defaulted to 13.
  Live play enters the Shake-Up at 14 (`advanceRowInner` moves past row 13).
  The default is now 14. Each seeded phase is bounded by its own row: the Main
  Event by 13 (`lastRecordRow`), the Shake-Up by 14 (`shakeUpRow`), so a Main
  Event seed cannot land on the Shake-Up's row (`gametest/options_test.go`).
  The seed test asserts row 14. Nothing else depended on 13. Comments in
  `handler/assets_chat_log_integration_test.go` and the `diagnosing-bugs`
  skill are updated.
- **e2e: a Shake-Up board load check** (`50b324b6`).
  The item said no e2e spec seeds a Shake-Up board. That was stale:
  `frontend/tests/e2e/shake_up.spec.ts` already seeds three. I added one test
  to that file. It seeds the default board, the rolling step with the first
  roll open, which live play reaches at row 14. It opens the table as each of
  the three seats and waits for the Players bar to name the first roller. The
  bar reads the roller only after the view has fetched the Shake-Up state, so
  that is the sign the board has drawn. A view that throws shows the fallback
  instead, and the wait ends on it. The test then checks the fallback is
  absent. The file's header comment describes it.
- **Raw `t.Errorf` replaced in 15 test files** (`6e0eb553`). The
  item said eleven; the count is fifteen. Each call became a testify
  assertion. The structural guards that scan the source tree (log coverage,
  mark literals, sqlc dead queries, the plan-write scan) now collect their
  offenders and report them through one assertion. No Clandestinely Liaise
  test file was skipped: none of the fifteen is one. Two of them name Liaise in
  passing and were converted: `handler/secrets_helper_test.go` (an allowlist
  entry, left unchanged) and `game/scenes_test.go` (a plan type in a list).
- **`tone_setting` removed from the frontend** (`8e8e59d5`). Migration 023
  moved every game out of the phase, and no Go code uses the value; the Go
  mentions are comments. The phase type, the table title's case, the phase
  badge label and the before-play check named it. They are removed. A stray
  row now reads "Tone setting" through the title's fallback for an unknown
  phase, and the badge shows the raw name. The database CHECK still allows the
  value. Whether a migration drops it is the owner's call:
  [questions/tone-setting-stray-rows.md](../questions/tone-setting-stray-rows.md).

## How it was verified

Against this branch's own code, on the final tree:

- `make check-fast`: passed. It includes `go vet` with and without the
  `integration` tag, golangci-lint, the Go unit tests, svelte-check (0 errors
  across 726 files), and the Vitest suites (52 of 52 in the two frontend
  files this change touched).
- `make test-integration` on the private database
  `uneasy_claude_code_cleanup_9c1556`: exit 0; every package `ok`, handler in
  192.7s.
- e2e: `frontend/tests/e2e/shake_up.spec.ts`, 4 of 4, against this worktree's
  server on `:8192` with `E2E_BASE_URL` set, not the `:8090` container. Run on
  the final frontend build.
- Bite checks, each restored afterwards:
  - Shake-Up seed default at 13: `TestSeedShakeUp_Default_MirrorsBeginShakeUp`
    fails (expected 14, got 13).
  - Initiative swap removed from `handler/plan_propose_duel_bouts.go`:
    `TestProposeDuelHTTP_FinalBoutTie_SaysTheDiceAreLost` fails at the new
    assertion ("initiative should swap after a tied bout…").
  - Per-phase bound ignored in `gametest/options.go`:
    `TestCurrentRowIsBoundByPhase` fails.
  - A deliberate throw in the rolling section of `ShakeUpView.svelte`: the
    e2e check fails at the fallback assertion (`Expected: 0, Received: 1`).

Not run:

- Other e2e specs. None of them seeds a Shake-Up board; I did not run the
  full suite.
- The guard refactor (collect, then assert once) has no bite check. The guards
  pass on the tree; the change is structural.
- The t.Errorf conversions are mechanical. The converted tests run and pass in
  the packages they live in.

## Behaviour changes to look at before merging

- **Two validation test groups moved to the integration tier.** The push
  subscription body checks (`push_subscriptions_integration_test.go`) and the
  `UpdateMe` username/password caps (`accounts_me_length_integration_test.go`)
  need a logged-in account, so they need the database. `make test` no longer
  runs them. `make test-integration` does, and CI runs it. The logged-out checks
  and the `CreateAccount` caps stay in the unit suite.
- **A seeded Shake-Up now sits at row 14.** Posts a seeded Shake-Up writes now
  get a nil row anchor, as in live play. Any local fixture built on the old
  default will show that difference.
- **Seeds are bounded by their phase's row.** A Main Event seed above 13 is now
  rejected, where it was accepted before.
- **A stray `tone_setting` row** would show "Tone setting" as its title and the
  raw `tone_setting` as its phase badge. No code path produces one.

## Open questions for the owner

- [Should the database CHECK drop the retired tone_setting value?](../questions/tone-setting-stray-rows.md)

## Noticed, not fixed

- The `deadcode` comment in the Makefile describes `-test` and
  `-tags integration` flags. The command passes neither. Passing them would
  change what the tool reports, so I left it.
- `handler/prologue.go`'s header comments still list `tone_setting` in the
  phase flow.
- About 55 `t.Fatalf` calls remain across the test suite. Out of scope here.
- Three integration test files on `main` are not gofmt-clean:
  `handler/wait_state_integration_test.go`, `handler/plan_make_war_integration_test.go`,
  `handler/push_notifications_integration_test.go`. I did not touch them.
- `handler/secrets_helper_test.go` still names `plan_clandestinely_liaise_phases.go`
  in its allowlist. Left as it is.
- `make check-fast` runs `go fix ./...` and `golangci-lint run --fix`, which
  rewrite source files in place. On this branch they changed test files
  (assertion forms only). Anyone running it should expect `git status` to
  change afterwards.
- The item `e2e-fail-on-boundary-fallback` is a larger change: it would fail
  every spec whose page shows the fallback. The check I added is local to the
  Shake-Up spec.
