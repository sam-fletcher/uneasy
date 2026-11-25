---
title: "May the 'What can happen' disclosure wear the sub-heading look?"
checked: "Style guide 'Play steps' and the Round 6 ruling (a sub-heading is a 12px uppercase deep-gold label) read; the 'Screen anatomy' rule that controls are objects and headings are not is the tension. Nothing rules on a disclosure's own look."
---

The shared "What can happen" reference is a pressable row (`plans/shared/Buffet.svelte`) whose text was a
`.choices-header`. When the sub-heading became a small deep-gold uppercase label, the disclosure took the same
look: "WHAT CAN HAPPEN" with a caret. The question mark went with the rule that a sub-heading has none.

The house rule is that a control is an object and a heading is not, so a pressable row that looks like a
sub-heading may read as a label rather than as something to press (the caret and the 44px row are what say
otherwise).

- **Keep** the look (what is built): it matches the other labels, and the caret says it opens.
- **Give the disclosure its own look**, for example secondary-colour text at body size with the caret, so it reads
  as a control.

The one place is `Buffet.svelte` (`.buffet-heading`, which carries the `choices-header` class).
