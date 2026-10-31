#!/usr/bin/env python3
"""PreToolUse hook: keep Claude sessions off `main` and away from merging.

Sessions push their own branches and open PRs; the owner merges. This blocks
the commands that would bypass that: `gh pr merge` (auto-merge included), a
force or delete push, and any push that targets `main`. GitHub's branch
protection on `main` is the real backstop — this hook catches the mistake
before it reaches GitHub and tells the session why.

Exit code 2 blocks the tool call and shows stderr to Claude.
"""

import json
import os
import re
import shlex
import subprocess
import sys

PROTECTED = "main"

FORCE_FLAGS = ("-f", "--force", "--mirror", "--delete", "-d")
FORCE_PREFIXES = ("--force-with-lease", "--force-if-includes")
# Push flags that take a separate value, so the next token isn't a refspec.
VALUE_FLAGS = ("-o", "--push-option", "--repo", "--receive-pack", "--exec")


def block(reason):
    print(
        f"Blocked by .claude/hooks/guard_git.py: {reason} "
        "The owner merges, and main changes only through a reviewed PR. "
        "Push your own branch and open a PR (the `pr` skill).",
        file=sys.stderr,
    )
    sys.exit(2)


def current_branch(cwd):
    try:
        out = subprocess.run(
            ["git", "-C", cwd, "branch", "--show-current"],
            capture_output=True, text=True, timeout=5,
        )
        return out.stdout.strip()
    except (OSError, subprocess.SubprocessError):
        return ""


def check_push(tokens, cwd):
    """tokens: everything after `push`."""
    positional = []
    skip_next = False
    for tok in tokens:
        if skip_next:
            skip_next = False
            continue
        if tok in FORCE_FLAGS or tok.startswith(FORCE_PREFIXES):
            block(f"`git push {tok}` rewrites or deletes remote history.")
        if tok in VALUE_FLAGS:
            skip_next = True
            continue
        if tok.startswith("-"):
            continue
        positional.append(tok)

    refspecs = positional[1:]  # positional[0] is the remote
    if not refspecs:
        if current_branch(cwd) == PROTECTED:
            block("this push would update `main` (you are on main).")
        return
    for spec in refspecs:
        if spec.startswith("+"):
            block(f"`{spec}` is a force push.")
        dest = spec.split(":")[-1]
        if dest in ("HEAD", "@") and ":" not in spec:
            dest = current_branch(cwd)
        if dest.removeprefix("refs/heads/") == PROTECTED:
            block(f"`{spec}` pushes to `main`.")


def check_segment(segment, cwd):
    """Checks one simple command; returns the working directory after it."""
    try:
        tokens = shlex.split(segment)
    except ValueError:
        tokens = segment.split()
    if not tokens:
        return cwd

    if tokens[0] == "cd" and len(tokens) > 1:
        return os.path.join(cwd, os.path.expanduser(tokens[1]))

    if tokens[:3] == ["gh", "pr", "merge"]:
        block("`gh pr merge` is the owner's call.")

    if tokens[0] != "git":
        return cwd
    i = 1
    git_cwd = cwd
    while i < len(tokens) and tokens[i].startswith("-"):
        if tokens[i] == "-C" and i + 1 < len(tokens):
            git_cwd = os.path.join(cwd, tokens[i + 1])
            i += 2
        else:
            i += 1
    if i < len(tokens) and tokens[i] == "push":
        check_push(tokens[i + 1:], git_cwd)
    return cwd


def main():
    try:
        event = json.load(sys.stdin)
    except json.JSONDecodeError:
        return
    command = event.get("tool_input", {}).get("command", "")
    cwd = event.get("cwd", ".")
    for segment in re.split(r"&&|\|\||[;|\n]", command):
        cwd = check_segment(segment.strip(), cwd)


if __name__ == "__main__":
    main()
