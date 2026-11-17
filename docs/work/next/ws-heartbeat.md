---
title: "Notice a half-open WebSocket: an app-level heartbeat the page can watch"
order: 83
---

The connection notice (`table/ConnectionNotice.svelte`, fed by `onStatus` in `lib/ws.ts`) only knows
the table is down when the socket fires `close`. A **half-open** socket never does: after a network
switch (wifi to cellular, a VPN toggling, a laptop changing networks) the browser can keep reporting
the old socket as open for a long time, while nothing arrives. That table shows no notice, no
reconnect starts, and the board goes stale. It is arguably the real "open for hours showing a board
that stopped updating" case; the notice as built covers the drops the browser reports.

The server already pings every 30s and drops a peer that doesn't answer within 10s (`hub.go`,
`pingInterval` and `pingTimeout`). That doesn't help the page: a browser answers a protocol ping on
its own and doesn't tell the page it saw one, so the page has nothing to watch.

Sketch, not built:

- The server sends a small app-level message on an interval (a `heartbeat` event, a few seconds
  above the current ping, nothing in it), or answers a `ping` the client sends.
- `ws.ts` records when it last heard anything on the socket (any message counts). If a whole
  interval and a margin pass with nothing, it closes the socket itself: `close` then fires, the
  existing loop reconnects, and the status goes down and live as it already does.
- Open questions to settle first: the interval (every message to every client costs the free-tier
  server CPU and egress, and an idle table is quiet), whether the page should only watch while the
  tab is visible (a hidden tab's timers are throttled and a player isn't looking), and whether to
  send it only to tables with someone connected, which the hub already knows.
- Touches `hub.go` (a ticker beside the ping one) and `ws.ts`. Add a unit test with the fake socket
  and fake timers like `ws.test.ts` already uses.
