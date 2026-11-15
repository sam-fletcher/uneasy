---
title: "Plans: sub-headings that read as headings, and step text that says each thing once"
order: 65
---

**Starts with a design round** the owner signs off: mockups, no code. The build starts after `plan-group-gap`,
since both edit `plans/planPanel.css` and most panels. If the round is settled before `plan-group-gap` starts,
fold it into that piece instead. One mockup round can also settle `plan-set-piece-line` and the open question
`plan-substep-lines`, since a sub-heading that reads as one may make a line between sub-steps unnecessary.

**The problem, as the owner saw it after the plan-panels-B PR.** Inside a plan's open step, a sub-heading and
the sentence under it look the same. Clandestinely Liaise's last sub-step:

```
Your move — the liaison                  18px, deep gold    the step heading
When will I see you again?               16px, main text    .choices-header
Reveal a die face (1–6) to choose …      16px, main text    .ft-prompt
Pick a row delay (or 0 to part ways):    16px, main text    .choices-header, in SimultaneousRevealInput
This is the last step: …                 14px, secondary    EndsNote
```

Three of the four lines under the heading are the same size, colour and weight, and two of them are styled as
headings. Before the redesign the sub-heading was 14px and the sentence under it was 13px in secondary text, so
the sentence stepped down and the heading stood out. The redesign moved both to the 16px body size, and the
panels moved those sentences from secondary to main text. `.choices-header` is used in 25 plan files, so every
plan has this.

**Two causes, possibly both** (the owner's reading):

- **Too few tiers.** The scale has 18px for the open step's heading and nothing between that and body text, so a
  sub-step's heading lands on 16px. Adding a size is the owner's call (`docs/STYLE_GUIDE.md`, "Type, gaps and
  corners").
- **Standardised the wrong way.** Explanatory text that was a bespoke 13–14px went up to body when it may belong
  at `--text-small` in secondary text, which would leave body for what the player must read to act.

The round should also settle whether a field's prompt ("Pick a row delay …") is a heading or a field label; the
style guide already gives labels the secondary `.ft-label` look.

**The copy in the same screens.** Propose Decree's council phase:

```
Your move — the next step
Council (1) · Signatory: alice
  • alice (signatory)
Proposed decree being debated:
"No private armies…"
Join the council?
Leverage one asset to join the debate — it becomes a die …
Leverage one asset to join:              the picker's label
```

- "the next step" names nothing; `plan-step-labels` already lets each panel name its step.
- The roster's heading carries a count, a dot separator and the signatory, then the list repeats "(signatory)".
- One screen mixes dashes, dots, bullets, colons, quotation marks and question marks: headings end in a colon or a
  question mark, and the decree in quotes sits at the same size as the heading over it.
- "Leverage one asset to join" is said twice, once in the sentence and again as the picker's label.

The fix may want a short rule in the style guide (how a heading ends, saying a role once, a picker label never
repeating the sentence above it), so the other plans can be checked against it. Liaise and Decree are the
examples; check every panel.

**Files:** `plans/planPanel.css`, `docs/STYLE_GUIDE.md`, `/dev/styleguide`, and the plan panels.
