# Skill mechanics

What changes when the document is a skill. Everything else is in
[SKILL.md](SKILL.md).

## Shape

A skill is `.claude/skills/<name>/SKILL.md`; `name` in the frontmatter
matches the folder. Sibling files hold reference the skill points at
(this file is one). An adapted skill has a `CREDITS.md`.

## Who can invoke it

- **Model-invoked** (the default): the `description` is always loaded,
  so the agent can fire the skill by itself and other skills can name it.
  The owner can still type it. The price is the description's context
  load in every session, work sessions included, so write it as a
  pointer: what the skill is, then "Use when …" with one trigger per
  branch. Most skills here are this kind.
  A model-invoked skill that is all reference is also a home for
  reference several skills share, since any of them can invoke it
  (this skill is one).
- **User-invoked**: `disable-model-invocation: true`. Only the owner
  typing `/<name>` fires it, and no other skill can reach it. No context
  load; the owner has to remember it exists. The description becomes a
  one-line summary for the owner (`wait-what`).

Make a skill model-invoked only when the agent, or another skill, must
reach it unprompted. A skill only ever typed by hand is user-invoked.

Reference that two user-invoked skills share can live in neither, since
neither can fire the other: put it in a plain file both point at.

## Splitting by invocation

Split off a model-invoked skill when it has its own trigger — a word the
owner actually types, or a situation sessions really hit — or another
skill must reach it. Its description is a new permanent load, so the
independent reach has to be worth that.

If user-invoked skills pile up past what the owner can remember, one
user-invoked **router** skill can list them and when to reach for each.
It can only remind; it can't fire them.
