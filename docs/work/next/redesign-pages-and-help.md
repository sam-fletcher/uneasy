---
title: "Redesign: the landing and sign-in pages, Help, and the app frame"
order: 40
---

**Starts after** the table shell PR merges, because the shell edits Help's copy.

- **The update banner** stays the one tinted strip in the app. It's about the app, not the game.
- **Everything else** follows the rulings: sizes, gaps, radii, boxes only on controls, and notices as a mark and text.

This screen was never mocked up. Apply the rulings as written, put before-and-after screenshots at 375px in the PR, and file a question in `docs/work/questions/` for anything the rulings don't settle, rather than guessing.

**Files:**
- `routes/+page.svelte`, `routes/+layout.svelte`;
- the `login`, `signup`, `reset-password` and `locked-out` routes;
- `HelpContent.svelte`, `FeedbackForm.svelte`, `UpdateBanner.svelte`;
- `shared/PushBlockedHelp.svelte`, `shared/HelpDisclosure.svelte`, `shared/ErrorText.svelte`.
