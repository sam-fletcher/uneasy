---
name: writing-for-agents
description: Skills, CLAUDE.md and memory — the reference for writing anything an agent reads in this project. Use when creating, editing, reviewing or pruning a skill, a CLAUDE.md, a memory file or its MEMORY.md line.
---

# Writing for agents

An agent reads a skill, a `CLAUDE.md` or a brief the same way every
time it runs. The aim isn't identical output; it's the same **process**
every run. Each section below is a lever on that. When the document is a
skill, also read [SKILL-MECHANICS.md](SKILL-MECHANICS.md).

## What every session here reads

| Always loaded, every session | Reached through a pointer |
|---|---|
| `CLAUDE.md` (repo) and the private `CLAUDE.md` above it | A skill's body (behind its description) |
| `MEMORY.md`, the memory index (one line per memory) | A memory file (behind its index line) |
| Every model-invoked skill's `description` | `docs/DEVELOPMENT.md`, `docs/STYLE_GUIDE.md`, the tracker, notes |
| Hook output (`sync_main.sh` runs on every prompt) | Design principles, rules files, `adr/` (private) |

A brief to a work session or a subagent is read the same way: it is the
whole of what a fresh context knows.

- **Context load** is what always-loaded text costs the agent: tokens
  and attention on every turn, used or not.
- **Cognitive load** is what the owner carries: knowing which documents
  exist and when to reach for one. Spend it where the owner's judgement
  matters (a user-typed skill like `wait-what`), and nowhere else.

## Pointers

A **pointer** is the always-loaded line that names material elsewhere and
says when to fetch it: a skill's description, a `CLAUDE.md` bullet, a
`MEMORY.md` line. The pointer's *wording* decides whether the agent goes
and reads the material, so a weak pointer to a must-read file is a bug.
Sharpen the wording first; copy the material inline only if that fails.

A pointer says what the material is, then the **branches** that should
fire it — the distinct situations it covers. Because it is always loaded:

- Put the triggering word first.
- One trigger per branch. Synonyms for one branch are one branch.
- Cut what the material says about itself; the pointer only routes.

A `MEMORY.md` line is a pointer like any other: the hook (what the
memory is about) earns its place, and the outcome of finished work
belongs in the memory file, not the index line.

## Steps and reference

A document holds **steps** (actions in order) and **reference** (rules
and facts consulted as needed). Place each piece on a ladder by how soon
the agent needs it:

1. **Steps in the file** — what the agent does, in order.
2. **Reference in the file** — consulted on demand. A flat list of peers
   is fine (`work-session` §3, the house rules).
3. **Reference in another file**, behind a pointer — a sibling file, or a
   doc anywhere (`test-worktree` behind `work-session` §4).

The test for pushing something down: does **every** run need it? Inline
what every branch needs; disclose what only some branches reach.
Reference left inline buries the steps, and a buried step gets skipped.

Keep a concept's definition, rules and caveats under one heading, so
reading one part brings the rest. A document that is simply too long
thins attention even when every line is live: disclose, or split.

## Done when

A step whose end isn't obvious ends on a **Done when** line — the
condition that tells the agent it can move on. `retro`, `grilling`,
`commit` and `diagnosing-bugs` use the phrase; new skills should too.

- **Make it checkable.** "Understanding reached" lets the agent stop
  early, pulled by the steps it can see ahead. "Each moment has a quote
  or a concrete pointer" (`retro` §1) can't be fudged.
- **Make it demanding.** "Every changed handler accounted for" drives the
  digging that "list the changes" doesn't. A bar binds reference too:
  "every house rule checked against the diff".

If a bound stays fuzzy and you see sessions rushing it, split the later
steps out — but only across a real boundary (a separate session or a
subagent). Steps in the same context are still visible.

## When to split

- **By sequence:** a run of steps that tempts the agent to rush the one
  in front of it. `work-session` hands verification recipes to
  `test-worktree` and `drive-plan` this way. The reverse holds too:
  merging two sequences puts each one's later steps in view of the
  earlier ones, and invites the rush.
- **By invocation:** see [SKILL-MECHANICS.md](SKILL-MECHANICS.md).

Each split spends one of the two loads, so it has to earn it.

## Leading words

A **leading word** is one well-known word that carries a whole behaviour
the model already knows: *red* (a check that fails on the bug, in
`diagnosing-bugs`), *frontier* (the questions askable now, in `grilling`),
*draft* PR. Used consistently, it does in one token what a sentence does,
and the same word in prompts, docs and code helps the agent link them. In
a flat list of rules, it names the class of thing to look for. Prefer a
word the model already knows to a coined one; a coined word has to be
defined before it works.

Hunt for restatements: the same three-adjective phrase in three places,
a sentence gesturing at one idea. Collapse each into its word.

**State the target, not the ban.** A ban names the very thing it
forbids, and the agent half-reads it as an instruction. "No colour
literals" puts a literal in mind; "colour only through the tokens in
`app.css`" (`work-session` §3) says what to do. Keep a prohibition only
as a hard guardrail — "never merge" — and pair it with the target: "the
owner merges".

## Pruning

- **One home per meaning.** Here: workflow overview in `CLAUDE.md`;
  house rules in `work-session` §3; commands in `docs/DEVELOPMENT.md`;
  visual rules in `docs/STYLE_GUIDE.md`; work state in the tracker; design
  rulings in the private design principles; facts the repo can't show in
  memory. Elsewhere, point. Two copies drift apart (the workflow docs
  needed a whole commit, 11e622e, just to agree with each other again),
  and a meaning said twice reads as more important than it is.
- **Leave lookups to the environment.** The Makefile, `--help`, the file
  tree and the code are already the truth; restating them is a cache that
  goes stale. Write down what looking can't reveal: the reason behind a
  choice, the trap no config admits to.
- **Prefer a check to a sentence.** A rule a test or hook enforces holds
  every time; prose holds only when read (`retro` §2 ranks the options).
  Once the check exists, the prose shrinks to a pointer.
- **Cut no-ops.** A sentence the model would obey anyway costs load and
  says nothing. Delete the whole sentence. Whether a sentence is a no-op
  depends on the model's default, so settle a disagreement by running the
  document with and without it, not by argument. A word too weak to change
  the default ("be thorough") needs a stronger word, not more words.
- **Cut sediment.** A line that stopped bearing on the work — finished,
  stale, never relevant — stays because adding feels safe and deleting
  feels risky. Delete it.

## This project's extras

- **The repo is public.** `CLAUDE.md` owns the rule; for documents, it
  means naming private files by role ("the private `CLAUDE.md` gives the
  path").
- **The owner reads these too, and is learning.** Give the reason for a
  rule beside it; the reason is also what lets an agent apply the rule to
  a case it doesn't name.
- **Adapted from elsewhere?** Credit the source in a `CREDITS.md` beside
  the skill: what was kept, and what this version adds.
