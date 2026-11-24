# ws-heartbeat — hand-over note

Branch `claude/ws-heartbeat-b156f1`. Built from `docs/work/next/ws-heartbeat.md`
(deleted), with the choices the owner approved on 2026-11-23.

## What changed and why

**The problem.** After a network switch (wifi to cellular, a VPN toggling) the
browser can keep calling a WebSocket "open" while nothing arrives. `close` never
fires, so the connection notice never appears and the board silently goes stale.
The server's protocol ping can't help: the browser answers it itself and never
tells the page.

**Server** (`hub/hub.go`, `model/ws.go`). The hub's `run()` loop has a ticker
(`heartbeatInterval`, 30s) that puts `{"type":"heartbeat"}` on every connected
client's send channel. Memory only, no database. A client whose buffer is full is
skipped, not dropped (only real broadcasts decide that). The hub only runs while
it has clients, so an empty table costs nothing. The interval lives on the `Hub`
(`heartbeatEvery`) so the test can shorten it without a global.

**Page** (`frontend/src/lib/ws.ts`).
- `lastHeardAt` is set on open and on **every** message.
- A 5s `setInterval` (`HEARTBEAT_CHECK_MS`) runs `checkSilence()`: if the tab is
  visible, the socket claims to be open, and `HEARTBEAT_SILENCE_MS` (45s) have
  passed, it closes the socket.
- Closing goes through `dropCurrent`, which calls `sock.close()` **and** runs the
  close handling itself. A half-open socket's closing handshake has no peer to
  answer it, so the browser's own `close` event can take minutes; waiting for it
  would defeat the point. `handleClose` is once-per-socket (`closeHandled`), so the
  late real `close` does nothing.
- A hidden tab isn't checked. `onVisibility` calls `checkSilence()` first, so a
  dead socket is closed the moment the player returns, and the pending-retry code
  right after it reconnects at once with the backoff reset.
- The heartbeat is dropped inside `onmessage`, before the resync buffer and
  before `onMessage`. The table page's `handleWSMessage` therefore never sees it:
  no refetch, no reactivity. (Its dispatcher already ignores unknown types, but
  buffering a heartbeat during a resync would have been pointless work.)
  `EventTypes.Heartbeat` names it.

## How it was verified

- `go test ./hub -race`: `TestHubSendsHeartbeatToEveryClient` (three beats to each
  of two clients) and `TestHeartbeatSkipsAFullClientWithoutDroppingIt`. Bite
  checked: removing the `pushHeartbeat()` call fails the first.
- `ws.test.ts`, nine new tests: silent socket closed then reconnected and notice
  raised; a message restarts the count; heartbeats keep a socket up and aren't
  passed on; not buffered during a resync; not closed while hidden; checked and
  reconnected at once on becoming visible; left alone on return if something was
  heard while hidden; no watch after `disconnect()`; a late real `close` after the
  watch dropped the socket doesn't schedule a second retry. Bite checked by
  mutation: removing the visibility guard, the `lastHeardAt` update, the heartbeat
  filter, the check on visible, and `closeHandled` each fails at least one test.
- `make check-fast` passes.
- Manual, against this worktree's own server (:8167, private database, both torn
  down after): seeded a main-event game, opened `/table/1`, and from the page opened
  a second socket to the same endpoint. Frames: `presence.snapshot` at 0s, then
  `{"type":"heartbeat"}` at 28s and 58s. The table stayed rendered with no connection
  notice. (The built-in browser's network pane doesn't list WebSocket frames, so a
  probe socket in the page stood in for it.)
- **Did not run:** `make test-integration` (no Go code outside `hub/` and one
  constant in `model/`), e2e, and a *real* half-open socket (that needs the network
  to be cut under a live tab; the unit tests stand in with a fake socket that goes
  quiet). Not seen at 375px: no UI changed.

## Behaviour changes to look at before merging

- An existing test pair that left an open socket silent for 60s now sends
  heartbeats as a real server would (`advanceWithHeartbeats` in `ws.test.ts`),
  because the watch rightly closes such a socket. Tests written later that wait
  more than 45s on a socket meant to stay up need the same.
- Returning to a tab that was frozen by the OS (phone lock) with nothing heard for
  45s+ closes the socket and reconnects, even if the socket was actually fine and its
  queued heartbeat was about to be delivered. The cost is one resync, only when
  someone is looking. I kept the brief's "check right away".
- Worst-case detection is 50s (45s limit + one 5s check), plus the notice's 5s delay.
- Egress: one ~20-byte frame per client per 30s.

## Open questions for the owner

None.

## Noticed, not fixed

- The older hand-over notes (`seen-across-devices-0e4e2c`, `ws-disconnect-notice-4cf1ed`)
  still point at `docs/work/next/ws-heartbeat.md`; they're history, so I left them.
