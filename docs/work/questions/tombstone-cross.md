---
title: "Should a destroyed asset keep the big cross drawn over it?"
---

A destroyed asset used to be greyed, outlined with a dashed border, labelled "Destroyed" in a small
box, and crossed out with two lines corner to corner. The redesign takes away the greying (it was an
opacity and a filter, which pull text under the contrast floor), turns the label into a word in red
(the colour of a loss), and keeps the dashed outline.

I **dropped the cross**. At the new sizes it ran straight through the word "Destroyed" and the
tile's notes, which are the things a player opens a tombstone to read, and the red word, the dashed
outline and the faint text already say it.

- **Do you want the cross back?** It was an absolutely positioned SVG over the tile
  (`.tombstone-x`). If so, it should sit behind the text and be faint enough not to strike out the
  word. Say so and I'll restore it that way.
