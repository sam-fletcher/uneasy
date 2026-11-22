---
title: "Step headings: a shorter \"You:\" / \"Ada:\" prefix (an idea, not yet decided)"
order: 85
---

**An idea from the owner (2026-11-22), not a decision.** It starts with the owner's call, perhaps
with a mockup, before any code.

The open step's heading says whose move it is, then the step: "Your move — aid or interfere",
"Waiting on otto — the roll". The idea is a shorter prefix: "You: aid or interfere", "Ada: the roll".

**What needs deciding first:**
- **The wording itself:** "You:" and "<name>:" in place of "Your move —" and "Waiting on <name> —".
- **The colour of "You:" headings.** Today every heading is the deep gold of the game asking
  (`--color-accent`). The owner wonders whether a heading that's yours should be the brighter gold
  (`--color-you`), but isn't sure. Read the "Screen anatomy" colour rulings in the decisions file
  before proposing: they say what each gold means.
- **Several players owed at once.** Today a viewer who is one of the players owed sees only "Your
  move", naming no one else, and beyond a cap other names become a count. The idea raises
  combinations such as "You, Ada:" and "You, Others:", which would name the other players too: a
  change of behaviour, not only of wording.
- **Screen readers.** "You:" read aloud says less than "Your move". Consider whether the heading
  needs fuller accessible text.

**Where it lives.** `lib/stepHeading.ts` is the one place the heading is worded (the chips' clocks
read the same derivation), and `shared/StepHeading.svelte` draws it. Its unit tests pin the wording,
and 16 e2e files match heading text, so a change touches many specs. Do it in one PR, alone.
