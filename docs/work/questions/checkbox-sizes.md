---
title: "Should the app's checkboxes share one size?"
---

Checkboxes now keep the browser's native size. The surrender checkbox on Make War's cost of battle
is the browser's default box (13px), set in a 44px tap row with its words beside it. The "Keep it
secret for now" checkbox in Spread Rumors is set to 20px by its own rule, and its comment says it is
"the default one the rest of the app uses", which is no longer true of the size.

Should every checkbox in the app be one size? The choice is 13px (the browser's, what the surrender
row shows now), or 20px (Spread Rumors' size, set in one place). It would be one rule in
`routes/+layout.svelte` or a shared class in `app.css`. The tap target is 44px either way, so this is
only about how big the box looks.
