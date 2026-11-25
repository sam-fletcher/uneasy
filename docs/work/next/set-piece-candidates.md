---
title: "Set pieces: Make War's declaration and Make Introductions' newcomer"
order: 69
---

The Round 6 piece built the set piece (`plans/shared/SetPiece.svelte`, `AssetName setPiece`, the "Play steps"
section of `docs/STYLE_GUIDE.md`) and used it for Clandestinely Liaise's meeting and Propose Duel's pairing. The
owner named two more places that may earn it, to be looked at once the look is settled:

- **Make War's declaration:** who declares war on whom. It pairs players rather than peers, so `SetPiece`'s
  `peer` is optional (a side can be a name alone); the question is whether it reads as the same kind of moment.
- **Make Introductions' newcomer arriving:** one named peer arriving into a retinue, not a pairing, so it may
  want a variant with a single side.

Each is a design call before it is code: a line earns a set piece only when the game itself brings two named
pieces face to face, so it stays rare enough to mean something. Draw each against the signed-off pieces on the
private design canvas first.
