---
title: "How should the duel's public call, tied dice and won dice read?"
---

The duel's bouts were never mocked up, so I applied the rulings to the pieces in them. Three are
judgement calls:

- **The open call** ("alice opens with Lady Merrow, calling higher. Pick one of your stakes to answer")
  was a blue box. It is now an information notice: the "i" mark and blue words, no box. It is
  information every player may read and the responder must read before choosing, so blue's "information"
  job fits. The other reading is plain step text (16px, no colour), as the roll step's sentence is. Which?
- **A die that is set aside** (a tie, waiting for the next bout's winner) is drawn as the shared die
  tile with a dashed edge and its owner's colour, the way a cancelled or repeated die is on the roll.
  `DieTile` has no "set aside" state, so I used its `duplicate` look and gave the tile its own accessible
  name. Should a set-aside die look different from a repeated one, which would be a change to `DieTile`?
- **The dice a side has won** ("Dice won for pool", "Dice won for interference") are drawn in the
  winning side's colour, though some of them came from the opponent: a bout's winner takes both dice, and
  any tied dice waiting in the tray. The edge says whose die it is everywhere else, so should each won
  die keep the colour of the player who rolled it instead?
