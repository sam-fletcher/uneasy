---
title: "Should the database CHECK drop the retired tone_setting value?"
---

Tone Setting stopped being a phase, and migration 023 moved every game out of
it. The games table's phase CHECK, from migration 002, still allows the value,
so the database would accept a row that holds it.

The client no longer names the phase: it is gone from the phase type, and a row
that somehow held it would show its name, "Tone setting", from the client's
fallback for an unknown phase.

Dropping the value from the CHECK would need a migration, and would make such a
row impossible to save. Should that migration happen, or should the CHECK keep
the value?
