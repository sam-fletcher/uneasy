---
title: "A fact line above a plan's first group is now 24px from it: does it join the group, or become a finished line?"
---

Now that every plan's open step is a column of groups 24px apart (`plan-group-gap`), the lines that state a fact
about the plan and sit above its first group, as a line of their own, are 24px from that group where they were
12px:

- Make Demands' "Targeting: Seek Answers by alice (row 5)", and the same line in Spread Rumors
  ("Targeting: bob's artifact"), above the draft or the rumor form;
- Propose Duel's "alice vs bob · difficulty 1", above the stakes and the bouts.

`plan-finished-facts` already asks whether such a line becomes a finished line of the card or joins the first group;
this is the same question for two more lines, with the new gap making the answer visible. Until it is answered they
read as a group of one.
