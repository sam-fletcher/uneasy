---
title: "Is the connection notice's look, place and wording right, and should it be tinted like the update banner?"
---

When the table's live connection has been down for five seconds, the table page shows a strip
between the player row and the three panes: a warning `Notice` ("Connection lost. This table
isn't updating. Reconnecting…") with a "Try now" button. It was built from `Notice` and the style
guide with no mockup, so the look is a guess for you to judge from the PR's screenshots:

- **Tint.** It is untinted, like every other notice, because that was the brief. But the style
  guide keeps a tint for the update banner "since it is app chrome rather than game content", and
  this strip is app chrome too: it is about the software and the network, not a step. Should it
  take the banner's tint, or stay a plain notice?
- **Place.** Across the whole frame under the player row, so it moves the panes down while it
  shows. The other candidates are above the title bar (where the update banner sits) or inside the
  Play pane only. The strip is about the whole table, which is why it is not in one pane.
- **Tone and mark.** Orange warning with the signed-off triangle. Danger (red) was rejected
  because nothing is lost: the table catches up the moment the connection returns.
- **Wording.** "Connection lost. This table isn't updating. Reconnecting…", button "Try now"
  (reads "Trying…" while an attempt runs).
- **Threshold: 5 seconds, or 6?** After a drop the retries fire at 1s and 3s, and a retry that
  works needs another 1–2s for the handshake and the resync. A recovery on the first retry lands at
  about 2–3s, well inside five seconds. A recovery on the **second** retry lands at about 4–5s,
  right at the edge, so on production it can still show the notice for a moment and then take it
  away. Six seconds would let that one through quietly, at the cost of telling a player with a
  real outage one second later. Five is built; it is the one constant `DISCONNECT_NOTICE_DELAY_MS`
  in `lib/ws.ts`, and the reasoning is in its comment.
- **"Trying…" can last a while.** On a network that drops packets without refusing the connection
  (a black hole, not an error), the attempt lasts as long as the browser's own connect timeout,
  which I haven't measured, and the button reads "Trying…" and ignores taps for all of it. It is
  honest (an attempt is running) but may feel stuck. Showing the notice's own words as the status
  and keeping the button live is the alternative.
- **A refused upgrade says "Reconnecting…" forever.** If the server refuses the WebSocket
  (`handler/ws.go` answers 401 for a signed-out player and 403 for someone who isn't at the
  table), the handshake fails and the loop retries forever. Before the notice that was silent; now
  the strip says "Reconnecting…" and "Try now" does nothing useful. A player whose session expired
  would be better served by a sign-in prompt, but telling a refusal from an outage isn't possible
  from the browser's WebSocket API (both are a failed handshake), so it needs a server hint
  first. Not built.

If any of these change, `table/ConnectionNotice.svelte` and that constant are the only files.
