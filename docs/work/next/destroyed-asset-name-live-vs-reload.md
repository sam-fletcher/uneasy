---
title: "A destroyed asset's name shows after a reload but not live"
order: 90
---

Found while removing reload workarounds from the Seek Answers spec. When a plan
destroys an asset (breaking its last marginalia), the client's `asset.destroyed`
handler drops it from `ctx.assets`, so a panel that looks the asset up by id
(Seek Answers' "Resolved so far: Broke bob's resource") falls back to "an
asset". After a reload the name is back, so the live and reloaded views
disagree. The spec asserts only `Broke ...` for that line until this is
settled. Likely fix: keep destroyed assets in the client list with
`is_destroyed` set (as the retinue display does), or have the panels fall back
to the name recorded in the log.
