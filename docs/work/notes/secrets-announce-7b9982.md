# Secrets are announced the same way everywhere

## What changed and why

Owner ruling (2026-11-01): how many secrets an asset hides, and which asset
holds them, is public; only the text is hidden. So every new secret now goes
through one door, `writeSecret` in `handler/secrets.go`: it stores the secret,
broadcasts `secret.created` (ids only, no text) and posts one log line naming
the author and the asset ("X wrote a secret on **A**"). Hide-source hides the
spreader from the characters, not the players, so its line names the author too.

- All four creation sites route through it: the plain write-secret route,
  Liaise keep-secret, Spread Rumors hide-source, and the Spread Rumors prepare
  stash (which creates inside the prepare transaction via `createSecret` and
  announces after commit, so clients never refetch before the commit).
  Plan-sourced lines are anchored to the plan.
- Removed the custom lines: Liaise's "entrusted the meeting's secret to …" and
  the "discoverable from …" half of hide-source's line. Hide-source keeps its
  player-level "hid themselves as the source — no one at court knows" line, which
  says something the generic line doesn't. The Spread Rumors prepared line no longer names the holder (the generic
  line right after it does), so the holder isn't said twice.
- Liaise: dropped `sanitizeLiaiseKeptSecretsForViewer` and its three read sites;
  the pick is public the moment it lands. Text stays gated.
- Liaise keep-secret and hide-source store the secret and their own state in one
  transaction and announce after the commit (a failed save announces nothing and
  a retry writes nothing twice); the stash does the same, and `createPlanInTx`
  returns the stashed asset so `plans.go` doesn't re-derive the condition.
- Guard: `TestSecretsAreCreatedThroughTheHelper` fails if any handler file
  calls `CreateSecret`, or calls the store-only `createSecret` outside the two
  files that pair it with `announceSecret`. `hub.Client.Outbox` lets tests read
  broadcast frames.
- Tracker: deleted `next/liaise-keep-secret-not-broadcast.md` and
  `questions/liaise-secret-holder-public.md`. Memory note
  `reference_hidden_information` states the ruling.

The other ruling-implied sites the brief listed (Seek Answers, Propaganda,
Make War) don't create secrets; `grep CreateSecret` finds only the four above.

## How it was verified

All against this worktree's code and a private database:
- `make check-fast`, `make test-integration` (full suite), full Playwright e2e
  on a private server (125 passed, 24 skipped as before).
- Bite checks: a stray `CreateSecret(` in a scratch handler file fails the guard;
  removing the broadcast from the helper fails the un-fixme'd e2e test.
- Integration tests updated/added: Liaise pick public on the single-plan and
  plans-list reads (text still unreadable to the partner) and exactly one
  `secret.written` line per keep-secret; Spread Rumors stash, hide-source and
  keep-secret broadcast `secret.created` (hub frames); a refused stash announces
  nothing.
- 375px: the log line checked as author and bystander (screenshots on the PR).
- Not run: the record read site has no integration test (the code that
  masked it is deleted, so there is nothing left to regress).

## Behaviour changes to look at before merging

- Spread Rumors prepare with a secret rumor now posts a second line (the secret
  line) after "prepared Spread Rumors …"; the prepared line says "a secret
  rumor" and no longer names the holder.
- Liaise: the first picker's asset is visible to the partner and bystanders
  immediately (UI and API), not after both have picked.

## Open questions for the owner

None.

## Noticed, not fixed

- `gofmt -l handler` flags `plan_make_war_integration_test.go`,
  `push_notifications_integration_test.go`, `wait_state_integration_test.go`
  (already unformatted on main).
