---
title: "Redesign: the table page shell, at every width"
order: 30
---

**Starts after:**
- the shared pieces PR;
- the server data PR, for the title formatter and the Laws and Rumors "last seen";
- `table-error-boundary-silent` and `table-resync-races-roll-fetch`, which fix `+page.svelte` first.

This is the frame around the three panes, Public Record, Play and Chat. The panes' contents move in unchanged.

- **Header:** Home, the table's title from the shared formatter ("Row 6 · Seek Answers"), and Help.
  - The title's plan comes from the row state's `title_plan_type`. Don't pick a plan from the plan list: the server decides it once, so the title bar and the profile agree.
  - The phase badge goes.
  - The Tones, Laws, Rumors and War buttons go.
- **The player row** spans the full width under the header:
  - one line that scrolls sideways;
  - each player is a shape and a name;
  - a clock sits on whoever the game is waiting on, gold on you.
- **Below 744px**, one pane at a time, with a bottom tab bar: "Public Record", "Play" and "Chat".
  - Play carries the gold clock when it's your move.
  - Chat carries the pale dot when it has something new.
  - Public Record carries the dot whenever Laws or Rumors has one.
  - The table always opens on Play.
- **The record's sub-tabs** (Events, Laws and Rumors) are chips at the foot, above the tab bar. They show only while Public Record is open.
- **From 744px**, two columns:
  - Chat is docked on the right;
  - the left column switches between Public Record and Play, with two tabs at its foot.
- **From 1112px**, three columns. At either width:
  - each column is 360–440px and holds the phone screen unchanged;
  - each column's name, in small capitals, carries the same marks as its tab;
  - the title bar and the player row span the full width.
- **Sheets** rise over the column they came from: Tones over Chat, and header sheets over Play or the left column.
- **What goes:**
  - the rail and the chat strip;
  - the record-phase width tokens. `lib/breakpoints.ts` and `layoutTokens.test.ts` move to 744 and 1112.
- **The Tones sheet** moves into its own component unchanged, for the Chat piece to redesign.
- **Stopgaps the owner accepted:** the Waiting On bar and the way into wars move to the top of Play unchanged, until the Play and war pieces replace them.
- **Help's copy** changes wherever it describes something that moved.
- **Files migrated here:** `routes/table/[id]/+page.svelte`, `HelpButton.svelte`, `shared/rankChip.css`, `shared/rankStrip.css`.

**Done when:**
- the e2e suite passes, with its specs updated for the new layout;
- the PR has screenshots at 375, 744 and 1112px.
