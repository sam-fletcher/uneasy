---
title: "Is catching up on return enough for the dots across devices, and should chat's divider move with the dot?"
---

The pale "new" dots on Laws, Rumors and Chat now agree across a player's devices by one rule: a
tab that comes back to the foreground, or a window that gains focus, asks the server for the
player's three markers and moves its dots to match (`onForeground` in `lib/ws.ts`, wired in the
table page; the read is `GET /tables/{id}/seen`). Three calls in it were mine, not yours:

- **Catching up on return, not pushing.** The case it can't cover is a second screen that stays
  in view while the first is read: a tablet propped beside a laptop, say. Nothing happens to the
  tablet when it is tapped, so its dot stays until it is next backgrounded and brought forward.
  Two windows side by side on one desktop are covered (the click into the other window is a
  `focus` event). A live push to the player's own sockets would cover the tablet too, but it needs
  a way to address one player's sockets in `hub/hub.go` and is the first event aimed at one player
  rather than the table, which is a new place for a hidden-information mistake to be made. Is the
  tablet case worth that?
- **Chat's "4 new" divider moves with the dot when Chat is off screen.** The divider is frozen at
  where the window loaded so it holds still under someone reading the unread run. When a return
  learns that the messages were read elsewhere and Chat isn't on screen (a phone on another tab),
  I moved the divider too, so that a dot saying "nothing new" isn't followed by a divider saying
  "1 new" when Chat is opened. With Chat on screen (a docked column) it stays where it is. The
  other reading is that the divider means "new since this device last had Chat open", which would
  leave it alone. It is one line in the table page (`rereadSeenMarkers`).
- **A docked Chat counts as "on screen" even when the window was hidden.** From two columns up, Chat
  is always a column, so `paneShown.chat` is true even while the whole window sat in the background.
  After a return that learns the messages were read elsewhere, the dot clears and the divider stays,
  the same mismatch this fixes on a phone. Gating the divider on "the page became visible" (rather
  than on "Chat is shown") may fit better: the reader of a docked Chat who is parked on the divider
  is, by definition, looking at the window. Not built, so the owner can say which reading is right.
