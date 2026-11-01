---
title: "Spread Rumors: re-validate the stashed take at consent time"
order: 20
---

`respond-take-consent` commits the asset ids stashed when the take was
requested, without re-checking owner or destroyed state, so an asset that
changed hands in between still transfers. Re-validate at commit time.
Noticed during #25.
