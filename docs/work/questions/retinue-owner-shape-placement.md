---
title: "Should every tile on a retinue carry its owner's shape?"
---

An owner mark says whose something is, so a card in a picker (a list that mixes owners) wears the
owner's shape beside its name. The retinue sheet is different: every tile on it has the same owner,
and the title already says whose it is.

I put the shape **once, beside the sheet's title** (`RetinueView`), and none on the tiles. Repeating
it on every tile would be four or five identical marks on a screen about one player.

- **Is the title enough?** If you want a shape on each tile as well, it is one `<PlayerShape>` in the
  tile head of `RetinueView.svelte`, in front of the name.
- **Related, and not a question:** `AssetCardSelectable` still receives the owner's *colour*, not a
  Player, so it finds the seat from the colour (`seatForColor`). That works for every table today,
  because nothing sets `token_color`. If custom colours ever ship, the callers should pass the Player.
