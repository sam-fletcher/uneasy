#!/usr/bin/env python3
"""PreToolUse hook: keep Claude sessions off `main` and away from merging.

Sessions push their own branches and open PRs; the owner merges. This blocks
the commands that would bypass that: `gh pr merge` (auto-merge included), a
merge through `gh api`, any push that targets `main` — including a force
push or a delete, and pushes that send every branch (`--all`, a matching `:`
or wildcard refspec) — and a commit made while on `main`. Force pushes to
other branches are fine: sessions tidy their own branch history.

Some git options run a command of their own (`rebase --exec`,
`fetch --upload-pack`, `push --receive-pack`). Those commands are checked
too, because the commit-to-PR commands are allowlisted and run unprompted.

GitHub's branch protection on `main` is the real backstop for pushes — this
hook catches the mistake before it reaches GitHub and tells the session why.
It reads the command as a shell would (quotes, heredocs, `&&` chains), so
text that merely mentions a blocked command, in a commit message or a PR
body, isn't mistaken for one. It fails open: if it can't parse something it
allows it, and branch protection still applies.

Exit code 2 blocks the tool call and shows stderr to Claude.
"""

import json
import os
import re
import shlex
import subprocess
import sys

PROTECTED = "main"

# Push flags that send every branch, main included.
ALL_REFS_FLAGS = ("--all", "--branches", "--mirror")
# Push flags that take a separate value, so the next token isn't a refspec.
PUSH_VALUE_FLAGS = ("-o", "--push-option", "--repo", "--receive-pack", "--exec")
# Options whose value git runs as a command, per subcommand.
EMBEDDED_COMMAND_FLAGS = {
    "rebase": ("-x", "--exec"),
    "push": ("--receive-pack", "--exec"),
    "fetch": ("--upload-pack",),
    "pull": ("--upload-pack",),
    "clone": ("-u", "--upload-pack"),
    "ls-remote": ("--upload-pack",),
}
# git's global options that take a separate value, before the subcommand.
GIT_VALUE_FLAGS = ("-C", "-c", "--git-dir", "--work-tree", "--namespace",
                   "--config-env", "--super-prefix")
# Words that can precede a command without changing what runs.
PREFIX_WORDS = ("command", "builtin", "exec", "env", "nohup", "time", "then",
                "do", "else", "!", "{")
OPERATORS = set(";&|()\n")
SHELLS = ("bash", "sh", "zsh")


class Blocked(Exception):
    pass


def block(reason):
    raise Blocked(reason)


def current_branch(cwd):
    try:
        out = subprocess.run(
            ["git", "-C", cwd, "branch", "--show-current"],
            capture_output=True, text=True, timeout=5,
        )
        return out.stdout.strip()
    except (OSError, subprocess.SubprocessError):
        return ""


def strip_heredocs(command):
    """Drops heredoc bodies, which are data, not commands."""
    lines = command.split("\n")
    out = []
    i = 0
    while i < len(lines):
        line = lines[i]
        out.append(line)
        i += 1
        for _, delim in re.findall(r"<<-?\s*(['\"]?)([A-Za-z_][A-Za-z0-9_]*)\1", line):
            while i < len(lines) and lines[i].strip() != delim:
                i += 1
            i += 1
    return "\n".join(out)


def split_commands(command):
    """Yields each simple command as a token list, quotes respected."""
    lex = shlex.shlex(strip_heredocs(command), posix=True,
                      punctuation_chars=";&|()\n<>")
    lex.whitespace = " \t\r"
    lex.whitespace_split = True
    current = []
    skip_next = False
    for tok in lex:
        if skip_next:
            skip_next = False
            continue
        if tok and set(tok) <= OPERATORS:
            if current:
                yield current
            current = []
        elif tok and set(tok) <= set("<>&"):
            skip_next = True  # a redirection's target
        else:
            current.append(tok)
    if current:
        yield current


def embedded_commands(tokens, flags):
    """Yields the values of `flag value` and `flag=value` for the given flags."""
    for n, tok in enumerate(tokens):
        if tok in flags and n + 1 < len(tokens):
            yield tokens[n + 1]
        for flag in flags:
            if flag.startswith("--") and tok.startswith(flag + "="):
                yield tok[len(flag) + 1:]


def check_push(tokens, cwd):
    """tokens: everything after `push`."""
    positional = []
    skip_next = False
    for tok in tokens:
        if skip_next:
            skip_next = False
            continue
        if tok in ALL_REFS_FLAGS:
            block(f"`git push {tok}` pushes every branch, `main` included.")
        if tok in PUSH_VALUE_FLAGS:
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
        bare = spec.lstrip("+")
        if bare in ("", ":") or "*" in bare:
            block(f"`{spec}` pushes every matching branch, `main` included.")
        dest = bare.split(":")[-1]
        if dest in ("HEAD", "@") and ":" not in bare:
            dest = current_branch(cwd)
        for prefix in ("refs/heads/", "heads/"):
            if dest.startswith(prefix):
                dest = dest[len(prefix):]
        if dest == PROTECTED:
            block(f"`{spec}` pushes to `main`.")


def check_tokens(tokens, cwd):
    """Checks one simple command; returns the working directory after it."""
    while tokens and (tokens[0] in PREFIX_WORDS or re.match(r"^[A-Za-z_]\w*=", tokens[0])):
        tokens = tokens[1:]
    if not tokens:
        return cwd

    if tokens[0] == "cd":
        target = tokens[1] if len(tokens) > 1 else "~"
        if target == "-" or "$" in target:
            return cwd
        return os.path.join(cwd, os.path.expanduser(target))

    if tokens[0] in SHELLS and "-c" in tokens[1:-1]:
        check_command(tokens[tokens.index("-c") + 1], cwd)
        return cwd

    if tokens[0] == "gh":
        if tokens[1:3] == ["pr", "merge"]:
            block("`gh pr merge` is the owner's call.")
        if tokens[1:2] == ["api"] and any(re.search(r"/pulls/\d+/merge\b", t) for t in tokens):
            if any(t.upper() in ("PUT", "-XPUT", "--METHOD=PUT") for t in tokens):
                block("merging a PR through `gh api` is the owner's call.")
        return cwd

    if tokens[0] != "git":
        return cwd
    i = 1
    git_cwd = cwd
    while i < len(tokens) and tokens[i].startswith("-"):
        if tokens[i] in GIT_VALUE_FLAGS and i + 1 < len(tokens):
            if tokens[i] == "-C":
                git_cwd = os.path.join(git_cwd, os.path.expanduser(tokens[i + 1]))
            i += 2
        else:
            i += 1
    if i >= len(tokens):
        return cwd
    sub, rest = tokens[i], tokens[i + 1:]
    for embedded in embedded_commands(rest, EMBEDDED_COMMAND_FLAGS.get(sub, ())):
        check_command(embedded, git_cwd)
    if sub == "push":
        check_push(rest, git_cwd)
    if sub == "commit" and current_branch(git_cwd) == PROTECTED:
        block("this commit would land on `main` (you are on main). Work on a branch.")
    return cwd


def check_command(command, cwd):
    for tokens in split_commands(command):
        cwd = check_tokens(tokens, cwd)


def main():
    try:
        event = json.load(sys.stdin)
        tool_input = event.get("tool_input") if isinstance(event, dict) else None
        command = tool_input.get("command") if isinstance(tool_input, dict) else None
        cwd = event.get("cwd") or "."
        if not isinstance(command, str) or not isinstance(cwd, str):
            return
        check_command(command, cwd)
    except Blocked as reason:
        print(
            f"Blocked by .claude/hooks/guard_git.py: {reason} "
            "The owner merges, and main changes only through a reviewed PR. "
            "Push your own branch and open a PR (the `pr` skill).",
            file=sys.stderr,
        )
        sys.exit(2)
    except Exception:  # noqa: BLE001 — fail open; branch protection still applies
        return


if __name__ == "__main__":
    main()
