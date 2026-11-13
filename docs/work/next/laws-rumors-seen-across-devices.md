---
title: "Laws and Rumors dots: update on a player's other devices"
order: 70
---

Opening the Laws or Rumors list marks it seen on the server (`PUT /api/tables/{id}/seen/laws|rumors`),
and the table page clears the pale "new" dot from the chip and from the Public Record tab. The marker
is saved, but it isn't pushed: nothing tells the player's *other* open tabs or devices, so each keeps
its dot until it reloads or reconnects (a reconnect re-runs `loadGameState`, which reads the markers).
Chat's read marker has the same gap.

Two ways to close it, smallest first:

- **Re-read the markers when a tab comes back to the foreground.** Today a tab becoming visible only
  reconnects if a retry is pending (`onVisibility` in `ws.ts`), so a tab with a live socket never
  looks again. The Laws and Rumors markers are on `GET /state` (`laws_rumors_seen`); chat's comes
  with the post window. No server change.
- **A player-targeted event** (for example `seen.updated`) sent to only that player's own sockets
  when a marker moves. It is live, but it may need a way to address one player's sockets rather than
  the whole table's, and it must carry nothing about any other player.

Do this for chat's marker in the same change, so the two dots agree on every device.
