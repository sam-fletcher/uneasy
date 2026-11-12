---
title: "Redesign: the Chat pane and the Tones sheet"
order: 40
---

**Starts after** the table shell PR merges.

- **Header:**
  - "Tones · N set", as a fully rounded chip with no chevron;
  - the "Hide bookkeeping" switch;
  - no heading on a phone. In columns, the column's name starts the header.
- **Feed dividers** are centred, in three weights:
  - the day is faint;
  - the unread divider ("4 new") is main parchment;
  - a row boundary is deep gold.
- **A plan's resolution:**
  - a tappable header row: its mark, name, the preparer's shape, the status, and a chevron to fold it;
  - its lines indented behind a thin left rule, not a card.
- **War's lines** carry the barbute, in war red.
- **Composer:**
  - a "Speaking as" control above the message box;
  - Send greyed out until there's text;
  - the keyboard still opens only on a tap.
- **Chat's dot**, on the tab or the column name, clears once the new messages have been on screen.
- **Tones sheet:**
  - once tones lock, it lists only the topics someone set, grouped Never, Avoid detail and Include, with a line counting the rest;
  - before the Main Event, the tile grid stays for setting tones.
- **Files:**
  - `ChatPanel.svelte`;
  - the Tones sheet component that the shell extracted;
  - `LogMark.svelte`, where the barbute replaces the flag on war lines.
