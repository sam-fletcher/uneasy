---
title: "Scene setup: a watcher sees the focus player's picks in bright gold"
---

When the focus player sets a scene, everyone else watches the same form fill in. A picked chip or card
that is also locked keeps its bright gold border and takes faint text; that is how the shared chip and
asset card (`chip.css`, `AssetCardSelectable.svelte`) draw a chosen, locked thing, and the scene form
uses them unchanged. So a watcher sees the focus player's picks in bright gold.

Bright gold means "yours" everywhere else (your picks, your focus ring, your waiting clock). The Play
piece hit the same thing on the plan grid and drew a watcher's view of the focus player's pick in
neutral text colour instead (the third point of [plan-grid-redesign](plan-grid-redesign.md): a watcher's
mirrored pick in neutral rather than gold). That is the same question, so **one answer settles both.**

Doing it here would take a shared "mirrored" look for `Chip` and `AssetCardSelectable` (a "someone
else's pick" state beside selected and locked). Those two pieces are shared and frozen for this wave, so
I could not add it and left them as they are. If the answer is neutral, that look would be added to the
two shared pieces once, the plan grid's `.mirrored` rules could then use it in place of their own, and
the scene form's watcher would pick it up with no change of its own.
