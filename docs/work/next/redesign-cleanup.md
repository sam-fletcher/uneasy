---
title: "Redesign: delete what the migration retired"
order: 59
---

**Starts after** every other `redesign-*` item has merged.

- **Retired tokens leave `app.css`:**
  - the sunken surface;
  - the gold wash (`--color-surface-active`);
  - the violet ramp and its chip trio;
  - the warm-strong border;
  - any chip trio, primitive or alias left with no users.
- **Unused files go:** `PhaseBadge.svelte` and anything else the migration left unused.
- **The guard test** finds no markers left. Its marker support then goes, so a literal anywhere fails.
- **`docs/STYLE_GUIDE.md`** describes only what's left.
