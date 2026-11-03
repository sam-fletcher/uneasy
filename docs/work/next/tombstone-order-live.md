---
title: "Tombstone order in the retinue differs live vs after a reload"
order: 95
---

`RetinueView` sorts destroyed assets by `destroyed_at`. The `asset.destroyed`
event carries only the id, so a live-destroyed asset has no timestamp until
the next reload and sorts as if it were oldest. Add `destroyed_at` to the
event payload (or sort by something the client already has) so the order
agrees.
