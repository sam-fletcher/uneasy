---
name: grilling
description: Interview the owner in rounds until a design decision, game-rule question or plan is fully settled, then record the rulings. Use for any design call the owner hasn't already ruled on, an ambiguous rule, a UX direction with real trade-offs, or when the owner says "grill me".
---

# Grilling the owner on a decision

Interview the owner until you share one understanding of the decision.
Design articulation is the owner's top priority, so the goal isn't only the
answer: it's the *reason*, written down so the next session can apply it
instead of asking again.

## 1. Read what's already settled

Before asking anything:

- The **design principles** (private; path in the private `CLAUDE.md`) and
  the memory index. A question an existing principle already answers is a
  **fact**: state it with the principle's number, don't ask it. If the case
  sits badly with a principle, *that* is a question worth asking.
- For a **game rule**, the rules files, then the rulebook page itself
  (rendered as an image, never extracted text). The book is the final word
  where they disagree, and most rules files are condensed summaries (the
  private `CLAUDE.md` says which are verbatim). Ask the owner only what the
  book leaves open, and cite the page you read.

## 2. Map the decision tree

Every decision branches into the decisions that depend on it. The
**frontier** is every open decision whose prerequisites are settled: the
questions you can ask now without guessing at an answer you haven't heard.

**Finding facts is your job, never the owner's.** When a frontier question
depends on how the code behaves today, find out yourself — read it, or
send an `Explore` subagent — and ask the rest of the frontier while it runs.
Only the questions that depend on that fact wait for it.

## 3. Ask in rounds

Ask the whole frontier in one round, numbered, each with your recommended
answer and the reason for it. Then wait.

```
**Q1 — <short title>**
<the question, with the options if there are several. Plain language: the
owner is learning web development, so explain a technical term the first
time it appears. Say what each option costs players, not only what it
costs the code.>

➡️ **Recommend:** <answer> — <why; cite the principle it follows, or say
it's a new call>

---

**Q2 — …**
```

Word each question so **"yes" accepts your recommendation.** The owner can
answer the whole round at once ("yes to all but Q2: …").

Each answer moves the frontier: settled decisions unblock the ones that
depended on them. Recompute it and ask the next round. A question whose
answer depends on another question in the same round belongs to a later
round.

**Done when** the frontier is empty — every branch visited, nothing
silently assumed — and the owner confirms the summary of rulings you give
back. Don't act on any ruling before that confirmation.

## 4. Record the rulings

Rulings — on design *and* on how the app reads a rule — live in the
private design principles file. Follow the shape it already uses, and show
the owner the wording first:

- **A new principle, or an amendment to one** → the tension, the
  resolution, and the test to apply next time.
- **A case a principle settled** → a dated *Worked case* paragraph under
  that principle: what the rule says (page cited), what was ruled, and why.
- **A rules-optional choice ("may") the app makes compulsory**, or the
  reverse → a row in the file's register under principle 6, as well as its
  worked case.
- **A tension left unresolved** → the file's "Open tensions" section.

Memory gets at most a one-line pointer to the principle, so future
sessions find it. Elsewhere:

- **Anything still waiting on the owner** → a file of its own in
  `docs/work/questions/`. An answered question's file is deleted.
- **What the current piece of work should now do** → its brief, or its
  hand-over note.

The repo is public: in the tracker, notes and PRs, paraphrase rulings and
the rulebook, and never copy text from the private files.
