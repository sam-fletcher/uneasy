# Laws, Rumors and Chat dots: agree across a player's devices

## What changed and why
Opening Laws or Rumors, or reading chat, saves a per-player marker on the server and clears
the pale dot on *that* device. The player's other open tabs and phones kept the dot until they
reloaded or reconnected, because the marker is private to the player and nothing is pushed.

**The fix is the item's way (a): catch up when the player comes back.** A tab that returns to
the foreground, or a window that gains focus, while its connection is live asks the server for
the player's three markers and moves its dots to match.

- **`GET /api/tables/{id}/seen`** (`handler/seen_markers.go`, one route line in `cmd/server/main.go`)
  returns `{laws, rumors, last_read_post_id}`: the caller's own markers, 44 bytes, from two
  trivial queries (the seat lookup every table route makes, and the existing
  `GetLawsRumorsSeen`). It is new because the markers were only reachable through `GET /state`
  (the whole board) or the full Laws and Rumors lists. The item said "no server change", but
  the brief said not to fetch the whole board on every foreground, and this is the smallest read
  that does that. Additive: an old tab never calls it.
- **`lib/ws.ts`** gains `ConnectionOptions.onForeground` and `FOREGROUND_MIN_GAP_MS` (2s). The
  callback fires from `onVisibility` (page became visible) and from a new window `focus`
  listener (a window that was already visible: two windows side by side never change visibility).
  It is called **only when the connection is live**: a socket that is down, connecting or
  mid-resync is about to run a resync that re-reads everything, so a second read would only
  race it. It is folded to one call per 2s (one return raises both events within milliseconds,
  and a player flicking between apps shouldn't cost a request per flick), and a throw or a
  rejection is swallowed (best effort). ws.ts knows nothing about markers: the page supplies
  the callback.
- **`lib/seenMarkers.ts`** is the pure merge (`mergeSeenMarkers`): each marker only moves
  forward, so a slow reply from before the player read something here can't bring a dot back.
  `getSeenMarkers` is in `lib/api/laws.ts` beside `markLawsSeen`; its reply type
  `SeenMarkersReply` is there too, so `lib/api/types.ts` is untouched.
- **The table page** wires `onForeground: rereadSeenMarkers`, which merges the reply into
  `lawsSeenRev`, `rumorsSeenRev` and `chatLastReadPostID`. The dots are already derived from
  those, so nothing else changes.
- **It skips the request when it can't change anything.** A read only ever moves markers
  forward, and a dot is "something above my marker", so a read can only clear dots. With no
  Laws, Rumors or unread-chat dot lit, and no marker still null (a marker that hasn't loaded
  yet only a read can fill), `rereadCouldClearADot` says no and nothing is sent. That makes
  most returns to a phone free; the page's activity ping is throttled to five minutes for the
  same reason.
- **Chat's "N new" divider** also moves, by its own rule (`dividerMarkerAfterReread`): only when
  the *server's* chat marker is beyond this device's own, and Chat is not on screen (a phone on
  another tab). The divider is frozen at where the window loaded so it holds still under a
  reader. Without this the Chat tab's dot would clear and opening Chat would still say "1 new
  message". The comparison is against this device's own chat marker, not the merged one,
  because this device's reading is already in its marker: a merged comparison would swallow a
  divider this device left behind whenever only Laws or Rumors moved elsewhere. With Chat on
  screen it stays put. **My call, not ruled on by the owner**: see
  `docs/work/questions/seen-across-devices-choices.md`, which also records a case this does not
  cover (a docked Chat counts as on screen even while the window was hidden).

### How it meets the connection notice (#103)
I read `docs/work/notes/ws-disconnect-notice-4cf1ed.md` first. `onVisibility` changed in two
small ways only: after its existing "retry pending, so reconnect now" block it now calls
`notifyForeground()`, and `onFocus` is new beside it. Everything #103 pinned is untouched
(`retryNow`, the status timer, the stale-socket guards) and its tests pass unmodified apart
from one hoisted variable in the test harness (`win`, so the new tests can fire `focus`).

- A reconnect through `onVisibility` still resyncs and re-reads the markers through
  `loadGameState`; the live gate makes `notifyForeground` a no-op there. (A pending retry always
  means "not live", because a retry is only scheduled from `close`.)
- "Try now" does not call the re-read; it reconnects and the resync covers it.
- `notifyForeground` never marks the table live or down: only the socket-open resync does, as
  #103 needed.

### Why (a) and not (b), the player-targeted event
Both fix the dot that hurts: reading on the laptop, then picking up the phone. (a) covers every
case where the second device is not in front of the player's eyes when the first is read, plus
two windows on one desktop (the `focus` event). It cannot cover a second screen that is
*already in view* with no event when touched, such as a tablet propped beside a laptop: it
catches up the next time it is backgrounded and brought forward.

(b) would also cover that, and also be instant. It costs more than the case is worth, and its
risks are the kind this project guards:
- `hub/hub.go` has no way to address one player: `clients` is owned by `run()`, so it needs a
  new channel and a loop, not a helper.
- Every path that moves a marker has to send it: the three handlers, and the author's
  automatic catch-up inside the create and edit queries, which have no handler hook.
- It is the first event aimed at one player rather than the table. A mistake in the addressing
  broadcasts a player's marker to the table; migration 058 keeps the Laws and Rumors markers in
  their own table precisely because a marker can name the hidden writer of a rumor.
- A new WebSocket payload is a one-way door for tabs left open across a deploy; (a)'s read is
  additive and nothing in it can reach another player.
- A socket that is down misses it, and then needs the re-read anyway.

### What it costs on a phone
**Most returns cost nothing**: with no dot lit and every marker loaded, nothing is sent. With
a dot lit, on each return to a live tab and at most once per 2 seconds, it is **one GET with a
44-byte body**, and two small queries on the server. Measured against this build on a seeded
table (the sizes are small because the table is, but `/seen`'s is constant): `/seen` 44 bytes,
10ms locally. A full resync is about eleven requests (`/state`, assets, secrets, posts,
record, active roll, plans, plan tokens, active scene, banked dice, wars) that grow with the
game; production's 205–480ms per-request floor (the prod latency note in memory) means
`/seen` is one such round trip. The phone is already awake because the player just opened it.
A tab whose socket has really closed takes the reconnect path, whose resync reads the markers,
and the live gate keeps the re-read from firing beside it. **A half-open socket is the
exception**: the browser still reports it open after the network has gone, so `ws.ts` counts it
as live, the GET fires and fails quietly, and the reconnect happens after (the heartbeat in
`docs/work/next/ws-heartbeat.md` is what would catch it first). The cost I did *not* measure:
real radio and battery on a phone.

## How it was verified
Against this worktree's own code: a server built here on `:8174` with the private database
`uneasy_claude_seen_across_devices_0e4e2c`. Nothing ran against `:8080` or `:8090`.

- `make check-fast`: passes (Vitest 97 files, 1863 tests; svelte-check 0 errors).
- `make test-integration` on the private database: passes.
- New tests: 11 in `ws.test.ts` (live gate, first resync, pending reconnect, burst fold,
  focus, hidden, disconnect, a throwing and a rejecting callback), 14 in `seenMarkers.test.ts`
  (the merge, when a read is worth a request, where the divider goes), 5 Go integration tests in `handler/seen_markers_integration_test.go` (zeros for an unmarked
  seat; follows a marker moved elsewhere; agrees with `/laws`, `/rumors` and `/state`; only the
  caller's own, exactly three keys; non-member and signed-out refused), and 7 e2e tests in
  `seen_across_devices.spec.ts`: Laws then Rumors on a phone after a laptop read (and that
  nothing pushes the news first), a second window gaining focus, bob's reading never clearing
  alice's dot, chat's tab dot and its divider, a message that arrives after the laptop read
  keeping the phone's dot, a return with no dot lit making no request (and one with a dot lit
  making it), and a divider this device's own reading left behind surviving a return that only
  moved Laws.
- **The tests bite.** Mutations applied, tests watched going red, then restored:
  - Go, 4: chat marker dropped, laws and rumors swapped, another seat's markers, an extra
    `player_id` key: each red in 1–3 tests.
  - `ws.ts`, 7: no live gate (2 red), no burst fold (2), focus not listened to (1), focus
    listener leaking after `disconnect()` (1), a rejection escaping the callback (1: the test
    listens for `unhandledRejection` itself, because vitest otherwise only marks the run, not
    the test, failed), a sync throw escaping (1), hidden counting as foreground (2). One
    mutation (`onVisibility` still notifying after its reconnect) turned out to change nothing,
    because the live gate already covers it; I removed the redundant `return` rather than leave
    dead code.
  - `seenMarkers.ts`, 6: always worth a request, null markers ignored, an unread-chat dot
    ignored, the divider moving when only Laws moved, the divider moving with Chat on screen,
    the divider able to move back: one red each.
  - e2e, second round: never skipping the request turns "no dot lit makes no request" red; the
    first version of the divider rule (follow the merged marker) turns "a divider this
    device's own reading left behind" red.
  - e2e, first round: not wiring `onForeground` in the page turns four of the five red; leaving
    the divider alone turns the chat test red; "clear the dot by marking the newest post read, ignoring the
    server's marker" turns the fifth red.
- e2e against this build, 50 of 50 passing: `seen_across_devices` (7, and 14 of 14 over two
  repeats), `chat` (16), `public_record` (8), `table_shell` (10), `connection_notice` (5),
  `resilience` (2), `page_cache` (1), `back_dismiss` (1). The rest of the e2e suite was not run.
- 375px screenshots of the second device before and after are in the PR.
- **Not verified:** a real phone or iOS Safari. The e2e "comes back" step dispatches
  `visibilitychange` (hidden, then visible) and `focus` from inside the page, because Playwright's
  pages are always visible. That drives the real handler in `ws.ts`; it does not show the browser
  raises the event (it does, on every browser the app supports). Production latency and battery
  were not measured. One race in my own first e2e draft is worth knowing: the page already
  re-reads the Laws and Rumors markers once, half a second after loading (see below), and on a
  fast local server the laptop's read landed inside that half second and cleared the phone's dot
  for the wrong reason. The spec waits that look out first (`openPhone`).

## Behaviour changes to look at before merging
- A tab you come back to loses a dot you cleared on another device, within about a round trip.
  A return with no dot lit sends no request.
- Chat's "N new" divider can be gone when you open Chat on a device you hadn't had it open on,
  if you read those messages elsewhere. Before, the dot stayed and so did the divider.
- A window that gains focus now triggers the same read (one request, at most once per 2s).
- New route `GET /api/tables/{id}/seen`. Additive; nothing existing changes shape.

## Open questions for the owner
- `docs/work/questions/seen-across-devices-choices.md`: is catching up on return enough (the
  propped-tablet case is the gap), should chat's divider move with the dot, and should a docked
  Chat's divider follow when the window was hidden (it doesn't now).

## Noticed, not fixed
- **The page's own post-load marker re-read is heavier than it needs to be.** The `markerRereads`
  effect in the table page fetches the whole Laws list and the whole Rumors list half a second
  after load, for the one number each carries (`seen_rev`), whenever either list has an entry
  above the marker, including other people's. `getSeenMarkers` would do it in 44 bytes. Left as
  it is: it is not this piece, and it works.
- **A half-open socket** (the browser still reports it open after the network went) counts as
  live to `ws.ts`, so a return on it makes a read that fails quietly. The existing
  `docs/work/next/ws-heartbeat.md` covers the cause.
- `make check-fast` prints `middleware/auth.go:94:6: unreachable func: AccountContext` from the
  deadcode step and still exits 0. Not mine and not touched.
