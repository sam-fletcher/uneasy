# Nightly Playwright e2e run on GitHub Actions

## What changed and why

CI gates PRs on `make check-fast` and the integration suite and never ran
the Playwright suite, so three specs went stale and sat failing with no
signal. The owner chose a non-blocking nightly run on `main`; this adds it.
Whether to also gate PRs stays open (see the question file below).

- **`.github/workflows/e2e-nightly.yml`** (new). Triggers: `schedule` (daily
  06:17 UTC), `workflow_dispatch`, and `pull_request` filtered to this file
  only (a self-test for the PR that edits it). Same conventions as `ci.yml`
  (`permissions`, `concurrency`, `timeout-minutes`, `go-version-file`, Node 24
  with npm cache, `postgres:16-alpine` service on `uneasy_test`, actions
  pinned by major tag). Two jobs:
  - `e2e` (read-only token): `make build` (production binary with the
    frontend embedded, no Vite), start `./server` on `:8090` with
    `UNEASY_DEV=1` against the service Postgres, wait with `curl --retry`
    until it answers (the new e2e preflight aborts without waiting), install
    Chromium, run `npm run test:e2e` with `CI=true`. Serial, `retries: 0`,
    kept that way on purpose to measure real flakiness. On failure it uploads
    `frontend/test-results` (traces plus the JSON report) and `server.log`
    for 14 days.
  - `report` (the only job with `issues: write`, plus `actions: read`; runs
    only when `github.event_name == 'schedule'`): downloads the artifact,
    then opens one issue labelled `nightly-e2e`, or comments on the open one,
    with the run link, commit, failed step, failing spec names (from the JSON
    report with `jq`) and a compare link from the last passing run on `main`.
    A passing scheduled run closes the open issue with a comment. It uses the
    preinstalled `gh` CLI and no third-party actions.
- **`frontend/playwright.config.ts`**: when `CI` is set the reporters are
  `list` + `json` (to `test-results/report.json`) + `github` (failure
  annotations); otherwise still just `list`. Nothing else in the config
  changed.
- **`docs/DEVELOPMENT.md`**, e2e section: what the nightly does, what a
  failure looks like, reading the artifacts (`npx playwright show-trace`),
  reproducing with the `test-worktree` recipe, running it by hand.
- **Tracker**: deleted `next/ci-runs-e2e.md` (the nightly satisfies it);
  rewrote `questions/ci-e2e-cadence.md` to record the owner's choice and the
  one open part, gating PRs.

- **The first self-test run failed, and the cause was in the specs and a
  client race.** On the real runner (2 vCPU) the suite went 157 passed, 2
  failed: `exchange_courtiers.spec.ts:201` and `spread_rumors.spec.ts:234`.
  Re-running the same commit failed `exchange_courtiers.spec.ts:201` again plus
  `exchange_courtiers.spec.ts:268`. In every case a page showed the state from
  before the roll resolved ("target responds", "Dice roll in progress…") while
  the server, the chat and the other pages had moved on. Cause: when a roll
  opens, each page hears `roll.created` and then fetches the roll; the specs
  called the dev `force-roll` a few milliseconds later. If `roll.resolved`
  reached the page before that fetch landed, the page ignored it (no
  `activeRoll` yet) and the late fetch installed the unresolved roll. A person
  can't settle a roll they can't see; the specs could. The traces showed it:
  bob's `GET /api/rolls/18` came back with the dice already forced and
  `result: null`.
  - **Fix (specs only):** new `frontend/tests/e2e/helpers/roll_open.ts`,
    `expectRollOpen(...pages)`, waits for the generic "Dice Roll" panel on every
    open page. Called before the force in: `declineAndForce` (all the Exchange
    Courtiers specs; `openAll` now remembers its pages), `castFromUI` in the
    Seek Answers and Spread Propaganda helpers, and the single spec sites in
    Chronicle Histories, Host Festivity, Propose Decree and Spread Rumors.
    No sleeps; `retries` stays 0.
  - **Product, not fixed:** the handler race is real and some real flows can
    hit it, so it is a tracker item
    (`next/roll-fetch-overwrites-resolved.md`), not something to hide in specs.

Decisions worth knowing: the Chromium download is not cached (a few seconds
against one more thing that can go stale; the comment in the file says so).
The `report` job also files for a `cancelled` result, because a job-level
timeout may show up that way; a manual cancel of a scheduled run would
therefore file an issue (rare, and arguably worth knowing about).

## How it was verified

Against this worktree's own code, on a private database and port `:8173`
(no `uneasy-*` container or `:8080`/`:8090` touched; server stopped and
database dropped afterwards):

- `actionlint` v1.7.7 (via `go run`, nothing installed) is clean on the new
  file and `ci.yml`. `shellcheck` is not installed here, so the embedded shell
  was not linted by actionlint's shellcheck pass.
- Each `run:` block was extracted from the YAML and executed: `make build`;
  start the server and the curl wait (it rode out a refused connection, then
  passed; against a dead port it gave up at the 90 s cap, printed the log
  and exited 1); `npx playwright install --with-deps chromium`; the suite
  with `CI=true`: **159 passed (2.1 min)** with the three reporters, and
  `report.json` survived Playwright's start-of-run `test-results` cleanup.
- A throwaway failing spec (since deleted, never committed) produced traces
  and a JSON report with the nested-describe titles; the `report` script ran
  against it with a stand-in `gh` that records calls: new-issue,
  comment-on-open-issue, close-on-pass, nothing-to-close, a truncated report
  with a `cancelled` result, and more than 40 failures. The read-only `gh`
  calls (`issue list --label`, `run list`, `run view --json jobs`) were run
  for real against the repo to check their syntax.
- With `CI` unset, a run writes no `report.json` (local behaviour unchanged).
- **The roll race, and the fix biting.** A throwaway proxy (not committed)
  forwarded to my private server and delayed only `GET /api/rolls/<id>` by 400
  ms. Before the fix 14 specs in 7 files failed, including the two from the
  runner and `exchange_courtiers.spec.ts:268`, the third spec to fail there.
  After: the same 7 files, 73 specs, all pass, and a full pass through the
  proxy passed 158 of 159. Without the proxy, a full `CI=true` run passed 158 of
  159.
- **The one that still fails, and why it is not a spec problem.** In both of
  those full runs a different Propose Duel spec failed once (`a tie on the
  final bout…` through the proxy; `mar: the target spares the loser…` without it,
  when the machine's load average was about 10). Nothing forces those rolls: the
  server creates and resolves the final bout's roll in one request. It is the
  same client race on a real flow, so the specs cannot wait their way out of it.
  See the tracker item. Expect an occasional red nightly there until it is
  fixed in the client.
- `make check-fast`: passes (after the spec changes too).

**Not verified:** the `report` job has never run on GitHub. It fires only
on a real *scheduled* failure, so the issue/label/close calls, the artifact
download layout and the job's `permissions` were checked by reading and with
the stand-in `gh`, not live. What the real (failed) self-test runs did prove:
the `e2e` job end to end on a runner, the failure artifact upload (traces,
`report.json` and `server.log` came down with `gh run download`), the `report`
job being skipped on a pull request, and the issue-filing script's `jq` and
body against the runner's own `report.json` (stand-in `gh` only). The run
tally is in the PR body. To prove the report path
live after merge: on a branch, temporarily add `workflow_dispatch` to the
`report` job's `if`, break a spec, dispatch the workflow on that branch, then
close the issue it opens.

## Behaviour changes to look at before merging

- After merge, a scheduled run starts at about 06:17 UTC each day. GitHub
  only runs schedules from the default branch, so nothing happens before
  the merge, and the first scheduled run follows within a day.
- A failing night opens a public issue with the label `nightly-e2e` (the
  label is created on first use). Failure artifacts (traces, server log) are
  downloadable by anyone signed in to GitHub, as with any public repo; they
  contain only dev-seeded game data.
- Runner minutes are free for a public repo, so this costs nothing.

## Open questions for the owner

- Should e2e also gate PRs? Recorded in
  `docs/work/questions/ci-e2e-cadence.md`: revisit after a couple of weeks of
  nightly results, perhaps path-filtered. I did not add a PR gate.

## Noticed, not fixed

- **A client race that can leave a page stale after a roll that opens and
  resolves in one request** (Propose Duel's final bout, a Host Festivity guest
  roll nobody can leverage). Tracker item `next/roll-fetch-overwrites-resolved.md`
  has the cause and the fix (give rolls the latest-wins guard plans already
  have). It is product code, so I did not change it; it is the likeliest source
  of an occasional red nightly run.

- `ci.yml`'s header comment still says "Playwright e2e is not here yet"
  and points only at `make test-e2e`. It should mention the nightly workflow;
  I was told not to edit `ci.yml`.
- GitHub disables scheduled workflows after 60 days with no repository
  activity (`db-backup.yml` notes the same). Not a concern while the repo is
  active.
