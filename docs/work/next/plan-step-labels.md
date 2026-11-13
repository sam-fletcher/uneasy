---
title: "Let each plan panel name its own step in the heading"
order: 62
---

**Starts after** the `redesign-plan-panels-*` items merge, so no panel is edited twice.

The Play pane's heading says "Your move — the next step" for every plan's own steps, because the server
only knows a plan is resolving. Each panel knows more: "What have you learned?", "Result: Make", "Describe a
newcomer".

- **A hook for the frame:** a panel tells its card what its open step is called, and the card uses it in
  place of "the next step" in the heading (`plans/shared/stepFrame.ts`, `lib/stepHeading.ts`). The words
  after the dash stay a lower-case phrase.
- **Delete what the heading and the roll step now say:**
  - every "Dice roll in progress…" line, since the roll is the open step and has its own heading;
  - each panel's own "X is resolving…" line, where the heading already names X.
- **Move the notices:** `.res-warning` paragraphs become `<Notice tone="warning">` (a mark and coloured
  text), and the `.chip-btn`, `.player-chip` and `.choice-card` markup becomes `<Chip>` and the shared option
  card, so those class names leave `planPanel.css`.
- **Spread Rumors and Spread Propaganda** pass `showNotes={false}` so a secret rumor's draft is never shown.
  Keep that when the notes line moves into the frame: a test (`steps.test.ts`) guards it.
