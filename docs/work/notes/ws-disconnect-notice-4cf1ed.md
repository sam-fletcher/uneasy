# Connection notice: tell players when the live connection drops

## What changed and why
A dropped WebSocket was only logged to the console while `ws.ts` retried with
backoff (up to 30s), so a tab could sit open for hours on a board that had stopped
updating. The table page now shows a strip when the live connection has been down
for 5 seconds, and removes it when the connection is back and the board is current.

- **`lib/ws.ts`** tracks whether the table is *live*: the socket is open **and** its
  resync succeeded. `createConnection` takes an optional fourth argument,
  `{ onStatus }`, called with `'down'` after `DISCONNECT_NOTICE_DELAY_MS` without a
  live connection and with `'live'` when it returns. It also returns `retryNow()`.
  `onResync` now has to say whether the snapshot landed (`Promise<boolean>`). Its one
  caller, the table page, is the only code that changed to use any of this.
- **`lib/components/table/ConnectionNotice.svelte`** is the strip: a warning
  `Notice` ("Connection lost. This table isn't updating. Reconnecting…") and the
  house secondary button "Try now". The page renders it between the player row and
  the three panes, in the slot the load error already uses.
- **The threshold is 5 seconds**, measured from the drop and not from the latest
  failed retry. After a drop the retries fire at 1s and 3s, and a retry that works
  then needs another 1–2s for the handshake and resync. A recovery on the first
  retry lands at about 2–3s, well inside five. A recovery on the second retry lands
  at about 4–5s, right at the edge, so on production it can still show the notice for
  a moment. Whether to widen it to 6s is the owner's call (the question file); I did
  not change the constant. Not ruled on by the owner (the brief left it to me).

### A resync that fails does not count as live (fixed after review)
My first version marked the table live whenever the socket was open and the resync
promise *settled*. But `loadGameState` catches its own errors (so a background
refresh can never blank the board), so after the first load it never rejects. A
reconnect whose fetch failed therefore cleared the notice over a stale board, and
with the socket staying open nothing ever resynced again. That is the case
`loadGameState`'s own comment describes (the first fetch over a dead pooled connection
fails), and it is what a tab waking from sleep or a redeploy hits: the path this
feature exists for. Now:

- `loadGameState` returns `true` when the snapshot landed and `false` when it didn't
  (it still never rejects). `createConnection` treats `false` and a rejection alike.
- On failure the socket is **closed**, so the existing loop reconnects and resyncs
  again. The events held during the failed resync are discarded (they are newer than
  a snapshot that never came, and the next snapshot covers them).
- An open socket **no longer resets the backoff**. It resets only when a resync
  lands, so a server that accepts sockets but fails the fetch is retried at the
  growing delay, not every second.
- `ready` still settles after a failed first resync (the page shows its error). The
  old comment in the page ("let the next resync repair it") is now true: before, a
  failed *first* load with an open socket never retried.
- Every handler ignores a socket that is no longer the current one. A socket the
  browser reports CLOSED can fire `close` a moment late, and "Try now" can reconnect
  in that gap; without the guard that late `close` would schedule a retry beside the
  live socket (two sockets, every event twice). The same guard stops a replaced
  socket's resync flipping `syncing` under the new one's, which closes the shared
  `syncing` issue I noted in the first version.

### "The board is current when the notice goes"
`createConnection` runs `onResync` (the page's `loadGameState`) on every socket
open, not just the first, and the notice clears only after a resync *succeeds*, so
no notice means a current board. The e2e spec shows it: the game advances from Row 1
to Row 2 while the viewer is cut off, nothing reaches them, and when the notice
leaves the title reads Row 2. A second e2e test aborts the state fetch during the
reconnect and checks the notice stays up until one gets through.

What this does **not** cover: a socket the browser still reports open after the
network has silently gone (no `close` event). It is never marked down. See
`docs/work/next/ws-heartbeat.md`.

### "Try now"
It was cheap, so it is in. `retryNow()` makes the two moves `onVisibility` makes for
a returning player (reset the backoff, pre-empt a pending retry), and returns a
promise that settles when that attempt does (including its resync, which now closes
the socket on failure), so the button reads "Trying…" for exactly as long as the
attempt runs. It also restarts a retry loop that has *died*: the loop continues only
from the socket's `close` event, and a connection the browser blocks outright fires
`error` with no `close` (I hit this: the page's CSP only allows same-origin and
`wss:`, so my first test URL was blocked and the loop stopped). Without that, "Try
now" would read "Trying…" forever.

### Everything else in `ws.ts`, and how this meets `laws-rumors-seen-across-devices`
I read the whole file. **I did not touch `onVisibility`**: no line of it changed, so
the later piece can rewrite it without a conflict. Where the two meet:

- `onVisibility` calls `connect()` directly when a retry is pending. That path still
  clears the notice with no edit: `connect()` → open → resync → live. A unit test
  pins it ("reports live when the page becoming visible reconnects at once").
- `retryNow` repeats `onVisibility`'s two moves instead of calling it, so the later
  piece's marker re-read doesn't run when someone presses "Try now". If that piece
  wants one shared "reconnect now" helper, `retryNow`'s body is the thing to extract.
- Status is set only from the socket-open resync and from `close`. If the later piece
  calls `onResync()` from `onVisibility` to re-read markers on a live socket, that
  can't report 'live' for a socket that isn't: `markLive` is reached only from a
  resync that `connect()` itself started on a still-open socket.
- On waking a tab that slept for hours, the 5s timer is overdue and may fire before
  the `visibilitychange` event, so the notice can show for the 1–2s the immediate
  reconnect takes. That is true (the board is stale for that second), so I left it.

### Scope: the other pages
- **The Prologue** is not a separate page. It is a phase of the table page, on the
  same connection, so it already shows the notice (checked at 375px on a seeded
  Prologue table).
- **The profile page** has no WebSocket at all, by design (its own comment: hub
  presence means "has a table open"). It refetches when the player returns to the
  page, and swallows a failed refetch. So it can't *lose* a live connection, but it
  has a quieter form of the gap: a failed refresh leaves old rows on screen with no
  word of it. Not fixed.

## How it was verified
Against this worktree's own code, rebased on the current `main`: a server built here
on `:8167` with a private database. Nothing ran against `:8080` or `:8090`.

- `make check-fast`: passes (svelte-check 0 errors; Vitest 93 files, 1817 tests).
- `make test-integration` on a private database passed on the branch before the last
  rebase; `main`'s two newer commits (the duel and war prep mirror) touch no Go, and
  nothing I changed is Go, so it was not re-run.
- New unit tests: 32 in `ws.test.ts` (threshold, status, `retryNow`, a resync that
  doesn't land, a replaced socket) and 4 in `connectionNotice.render.test.ts`.
- New e2e `connection_notice.spec.ts` (5 tests): the notice is absent at 2s, present
  past 5s; the board is stale while cut and current after; a blip that heals never
  shows it; "Try now" works; a reconnect whose fetch fails keeps the notice up until
  one lands; and at 375px the button is 44px tall and nothing scrolls sideways.
- E2e specs covering what I touched, against this build: `table_shell`, `resilience`,
  `smoke`, `page_cache`, `back_dismiss`, `player_join`, `prologue_claim`, `chat`,
  `styleguide` and the new one. The rest of the e2e suite was not run.
- **The tests bite.** Each mutation below was applied to the code, the matching tests
  watched going red, then restored:
  - Unit, first version, 11 mutations (the timer not cancelled on recovery; threshold
    cut to 500ms; timer re-armed by every failed retry; live reported at socket
    open; a late resync reporting live; `close` not marking down; `retryNow` not
    resetting the backoff, not restarting a dead loop, or doubling an in-flight
    attempt; `disconnect()` leaving the timer or the waiters): each turned one to
    seven tests red.
  - Unit, the review's fix, 8 mutations: **the resync result ignored (a failed resync
    counts as landed) turns four tests red**; an open socket resetting the backoff, a
    failed resync not closing its socket, held events applied after a failure, `ready`
    not settling after a failed first resync, and each of the three stale-socket
    guards (`onclose`, `onopen`, the resync completion) removed: one to three tests
    red each.
  - E2e, the review's fix: with the resync result ignored, the new "reconnect whose
    fetch fails" test fails ("a failed resync is not a live table"); with the fix it
    passes. The first version's four e2e mutations (close not marking down, the page
    not wiring `onStatus`, a 60s threshold, the timer not cancelled) were each
    caught too; those ran before the review's change.
  - Component, 5 mutations: 4 caught (tone, `live`, `disabled` instead of
    `aria-disabled`, label). **Not caught:** removing the guard that ignores a tap
    while "Trying…". The render test can't click. It is harmless, because
    `retryNow()` doesn't double an in-flight attempt.
- Looked at by eye: 375px and 1280px with the notice, the "Trying…" state, the focus
  ring on the button, and the Prologue.
- Not verified: a real network drop (a laptop sleeping, a phone in a tunnel, a
  Railway redeploy). The e2e cuts the socket from inside the page and aborts the
  fetch with Playwright's routing; the browser's own close and refused-connection
  paths are real, the network failure is simulated.

## Behaviour changes to look at before merging
- A new strip appears under the player row when the connection has been down for 5s,
  and pushes the panes down while it shows. It isn't there in normal play.
- A reconnect whose resync fails now closes the socket and tries again at the growing
  backoff, instead of leaving a live socket over a board that never refreshed. That
  includes a failed *first* load, which used to just show its error and wait.
- On first load, a table that can't reach the server within 5s shows the strip too,
  over the cached snapshot if there is one.
- `createConnection`'s resync callback must now resolve a boolean, and the result gains
  `retryNow`. Nothing else in the app calls it.

## Open questions for the owner
- `docs/work/questions/connection-notice-look.md`: tint (like the update banner, which
  is also app chrome), place, tone, wording, the 5s vs 6s threshold, how long
  "Trying…" can last on a black-holed network, and a refused upgrade (signed out or
  not at the table) saying "Reconnecting…" forever.

## Noticed, not fixed
- **A half-open socket shows no notice**, and nothing reconnects it: the browser still
  reports the socket open after a network switch. Proposed in
  `docs/work/next/ws-heartbeat.md` (order 83), not built.
- **A refused upgrade retries forever.** `handler/ws.go` answers 401 or 403 before the
  upgrade, the handshake fails, and the loop retries at the capped backoff for as
  long as the tab is open. It was silent before; now the strip says "Reconnecting…".
  The browser's API can't tell a refusal from an outage, so it needs a server hint.
  Written up in the question file.
- **`resilience.spec.ts`'s "auto-reconnects after a network blip" probably doesn't
  exercise the reconnect.** Its comment says `setOffline(true)` severs the
  WebSocket. In a probe against this build the socket fired no `close` for 8s after
  going offline, and the spec finishes in 1.1s, less than its own 500ms wait plus the
  retry loop's 1s backoff. My inference, not shown: the socket stayed open and the
  queued message arrived when the network returned. The new spec cuts the socket
  another way.
- **`ws.ts` assumes `error` is always followed by `close`.** It isn't when the browser
  blocks a connection outright (a CSP block fired `error` and no `close`). Nothing
  restarts the loop then, except a player pressing "Try now". Production's CSP allows
  its own `wss:` origin, so I don't know of a way to reach it there.
