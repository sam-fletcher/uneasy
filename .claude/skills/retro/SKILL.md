---
name: retro
description: A retrospective on a finished Claude session — find what went wrong or slowly, and propose changes to the project's setup (checks, hooks, skills, CLAUDE.md, memory, dev tools) so it doesn't recur. Use when the owner asks for a retro or retrospective, or asks why a session went badly.
---

# Session retrospective

You are improving the **setup** future sessions work in, not the code a
session wrote. The question for every finding: what change to the
environment would have stopped this happening, or made it cheaper?

## 1. Gather the evidence

The session is the one the owner names; if they don't name one, it's this
one.

- **Transcript:** the app's session tools (search or export a transcript)
  when you have them; otherwise the JSONL files under
  `~/.claude/projects/` — a worktree session's folder name ends in
  `-claude-worktrees-` plus the worktree's folder name. Transcripts are
  long: have a general-purpose subagent read it in full and return the
  moments listed below, with quotes, rather than reading it whole yourself.
- **What it produced:** its PR (body, commits, review comments), its
  hand-over note, and the tracker.

Moments to find:

- The **owner corrected** the session, or had to say the same thing twice.
- The session **asked the owner** something it could have found itself.
- It reported something **verified that wasn't** (a check run against
  `main`, a viewpoint never looked at), or a reviewer found what it missed.
- It broke a **house rule** (`work-session` §3) or a principle it should
  have read.
- It spent a long time **finding** a file, fact or command.
- An **expensive or repeated** tool call: re-reading big files, rerunning
  slow suites, loops of the same failing command.
- A tool, endpoint or log it **needed and didn't have**.

**Done when** each moment has a quote or a concrete pointer (file, command,
turn). No finding without evidence.

## 2. Diagnose each moment

Pick the cheapest change that would have *reliably* prevented it. Prefer
them in this order — a check catches the mistake every time; an
instruction only works if it's read and followed:

1. **An automated check.** For a mechanical mistake (a banned pattern, a
   missing call pairing, a file in the wrong place), a guard test in the
   style the project already uses — `designTokens.test.ts` for colour
   literals, `layoutTokens.test.ts`, `handler/log_coverage_test.go`,
   `TestEverySQLCQueryIsCalled` — or a lint rule, or a `.claude/hooks/`
   hook for agent behaviour. First check whether a check already exists
   but wasn't run or isn't in `make check-fast` / CI: then that gap is the
   finding.
2. **A dev tool or more information**: a missing `/api/dev/*` endpoint, a
   `plandriver` or `shot.mjs` capability, a log the session couldn't see.
3. **A skill change**: a step a skill missed or got wrong, a description
   whose wording didn't fire when it should have, or a procedure sessions
   keep improvising that deserves a skill of its own.
4. **Memory**: a fact the session needed that memory lacked or had wrong; a
   correction the owner gave that should hold in future sessions. A
   correction about game *design* belongs in the design principles instead
   — propose it there, or a `grilling` session if it's unsettled.
5. **`CLAUDE.md`** (public, or the private one): only a pointer to where
   something lives. Everything in it is loaded every turn, so it also earns
   findings the other way — lines that change nothing (the session would
   behave the same without them) or that belong in a skill.
6. **The workflow**: a coordinator brief missing what the session needed,
   sessions colliding on files, the tracker or note drifting from the code.

## 3. Report

A list, **most severe first**. Severity is what it costs if it happens
again: wrong behaviour shipped or the owner misled, then the owner's time
spent, then session time or tokens spent.

```
### 1. <short title> — <severity>
**What happened:** <quote or pointer>
**Change:** <the concrete change, and where it goes>
**Size:** <small / medium / large>
```

Leave out findings with no clear fix, and anything you would only "keep an
eye on".

Don't make the changes in the retro. The owner picks; each accepted change
becomes either a quick edit (memory, a skill line) or a tracker item under
"Next — workflow and skills".

Transcripts quote the private rules and design principles, and the tracker,
skills and CLAUDE.md are public: anything you write there paraphrases, and
points at no private file.
