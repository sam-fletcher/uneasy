---
name: pr
description: Open or update a pull request in this repo — push the branch, write the body (why, summary, evidence, merge danger), attach screenshots. Use when a piece of work has passed its verification and is ready for review, and whenever writing or revising a PR body.
---

# Opening a pull request

A work session opens its PR **as a draft** once the `work-session`
verification bar is met. The coordinator reviews the diff and marks it ready
(`gh pr ready <n>`); the owner reviews and merges. Nobody else merges, and
nobody enables auto-merge — a hook blocks `gh pr merge` and pushes to `main`.

## 1. Push

- Your work is committed on your own branch and `git status` is clean. The
  tracker row and hand-over note are in those commits.
- `git push -u origin <branch>`.
- `gh pr create --help` lists `--attach` (gh 2.103 or newer). If it
  doesn't, stop and ask the owner to run `brew upgrade gh` — the evidence
  section depends on it.

## 2. Write the body

Write the body to `pr-body.md` in your scratchpad, with any screenshots
beside it, then fill in this template:

```markdown
## Why

<one or two sentences: the problem, in players' or developers' terms>

## Summary

<the smallest visual that makes the change clear>

## Evidence

<before and after>

## Merge danger

**Door:** <one-way or two-way> — <why>
**Blast radius:** <one or two words> — <what could break, for whom>

## For the owner

<behaviour changes to look at, open questions, what wasn't verified;
link the hand-over note for the full record>
```

Skip preambles and keep prose brief. Use the game's own terms (plan, row,
make, mar, the Waiting On bar), not invented ones.

### Why

The owner is learning web development from these PRs, so lead with the
reason for the change, not a list of files. One or two sentences.

### Summary

Pick the smallest view that makes the key point clear. Usually one; several
only when the change has distinct parts.

- **Call tree** for server control flow:
  ```text
  ComputeRowState
    miHandler.ResolvingWaitees   # now adds the demanded winner
  EmitSystemPost
  ```
- **Component tree** for UI structure, with the state that matters:
  ```text
  <PlanPanel>
    <MakeIntroductionsPanel>   isPreparer || isDemandWinner
      <ArrivalForm>
  ```
- **Pseudocode** for a rule or algorithm.
- **Shallow file tree** for a refactor or a new set of files.
- **Mermaid sequence diagram** for a flow between browser, server and other
  players over WebSocket.
- **`diff`-fenced sketch** when the shape already exists and the point is
  what changed — of a call tree, component tree or file tree, not raw code:
  ```diff
   <PlanPanel>
     <MakeIntroductionsPanel>
  -    <CompleteButton>
  +    <ArrivalForm>   shown to the preparer or the demanded winner
  ```

Put each visual beside the sentence it supports. Show only the calls,
components, files and states the reviewer needs.

### Evidence

Show **before and after**.

- **Screenshots** are best when the change is visible. Take them at 375px
  (the `drive-plan` skill), as each viewpoint that changed: the actor, the
  other party, a bystander. Reference them in the body as
  `![Make War, defender's view, after](./after-defender.png)` and attach
  each with `--attach` (step 3); gh uploads them and rewrites the
  references. Never commit screenshots to the branch.
- **Tests** next. Name the test, and show it failing without the fix and
  passing with it — the `work-session` "prove it bites" step already
  produced this. A short excerpt of the output, not the whole log.
- Say plainly what did **not** run against this code (an e2e spec, a
  viewpoint, a player count).

### Merge danger

`main` deploys straight to live games, so judge this against games that are
mid-play when the deploy lands.

**Door.** A two-way door can be walked back by reverting the PR. A one-way
door can't. One-way doors in this project:

- A **database migration**. It applies on server start, and a revert
  doesn't undo it — a new migration does.
- A change to the shape of stored state (`resolution_data`, plan prep
  data) that games already hold.
- A change to a **WebSocket payload or API response** that a tab left open
  across the redeploy will still receive.
- **Hidden information** reaching a player who shouldn't see it. It can't
  be un-seen.

**Blast radius.** Which plans or phases, which player counts, every game or
only games in a particular state, mobile layout, the action log.

### For the owner

Behaviour changes worth a look before merging, open questions (these also
go in the tracker), and anything noticed but not fixed. Link the hand-over
note (`docs/work/notes/<session-name>.md`) rather than repeating it.

## 3. Check, then create

**This repo is public.** Before creating, read the body once more for:
rulebook text quoted rather than paraphrased; anything from the private
folder (rules files, design principles, ADRs, private notes); real player
names or emails in screenshots. Dev-seeded games only.

```bash
gh pr create --draft --base main \
  --title "<what changes, under 70 characters>" \
  --body-file <scratchpad>/pr-body.md \
  --attach '<scratchpad>/after-defender.png#Make War, defender view, after'
```

The title says what changes, in plain words — never the branch name.
End the body with the attribution line your instructions give.

To revise later: `gh pr edit <n> --body-file … [--attach …]`. Re-attaching
uploads the file again, so attach only new or changed images.

## 4. Report

Put the PR URL in your final message, with whether it is still a draft.
