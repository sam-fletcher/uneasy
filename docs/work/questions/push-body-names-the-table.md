---
title: "What should a reminder notification name, so a player at several tables can tell them apart?"
---

Every reminder reads "The game is waiting on you!", so a player at more than one table can't tell
from the lock screen which one it means. An early notification plan had the body name the table by its
join code. I didn't build that: the profile page later dropped the code from its table rows, because
the players' names tell tables apart better and the code only matters for inviting. A player mid-game
may not recognise it, and on a lock screen anyone nearby can read it (before the Prologue starts, a
code lets them join). So what should the body name?

- **The other players** ("Waiting on you at the table with otto and wren"), the same cue the profile
  rows use. Long at five players.
- **The table's title** ("Row 6 · Seek Answers"), the words the table's own title bar shows.
- **The join code**, as the early plan had it.
- **Nothing more.** Keep the generic line, and let the title and tag keep reminders from stacking.
