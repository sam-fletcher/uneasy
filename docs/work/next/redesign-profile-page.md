---
title: "Redesign: the profile page"
order: 30
---

**Starts after** the shared pieces and server data PRs merge.

- **Rows:**
  - each table is a tappable row with a caret, with lines between rows;
  - the page's sections lose their boxes.
- **First line:**
  - the table's title, from the shared formatter;
  - when there's unread chat, the pale dot and "N new" in plain text at its end.
- **Second line:**
  - the players, each as a shape and name in their colour;
  - a clock on whoever the game waits on: gold on you, grey on others;
  - no join code;
  - the reminder bell stays, as a 44px button beside the caret.
- **Groups:**
  - "Waiting on you", "Waiting on others" and "Finished";
  - a group with no tables shows no heading;
  - each group keeps today's order;
  - a live table waiting on no one goes under "Waiting on others";
  - finished tables are no longer faded.
- **Buttons:**
  - Join is greyed out until a code is typed;
  - "Create a new table" becomes a secondary button.
- **Files:** `routes/profile/+page.svelte`. It stops using `PhaseBadge`.

**Done when:**
- the PR has screenshots at 375px and at desktop width, with tables in every group;
- the profile's e2e specs are updated.
