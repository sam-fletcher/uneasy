---
title: "Spread Rumors: should a stale take lapse, or follow the asset?"
---

When the victim agrees to a take and one of the stashed assets has since been
destroyed or moved to someone else, the request now lapses: nothing transfers, it
is logged, and the aggressor chooses again (the option is not marked refused). I
chose this because the consent was given by one player for assets they owned at
the time, so it doesn't carry over to a new owner. The rulebook doesn't address
this edge explicitly, so it is not ruled; the alternative is to follow the asset
and ask its new owner.

Related: the victim's agreeing click gets a 409 even though they did nothing wrong.
It could be a 200 with `lapsed: true` instead; left as 409 for now so the client
sees the agreement did not take effect.
