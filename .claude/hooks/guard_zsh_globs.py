#!/usr/bin/env python3
"""PreToolUse hook: stop a grep whose --include= or --exclude= glob zsh expands.

The shell here is zsh, which expands an unquoted glob before the command
runs. `grep -r --include=*.ts foo src` therefore hands grep the names of the
.ts files in the current directory instead of the pattern, and grep, with no
--include matches left, searches nothing and prints nothing. That reads as
"not found". It happened about 18 times in one session.

This blocks any --include= or --exclude= value with an unquoted, unescaped
`*`, `?` or `[`. A quoted or escaped value passes, and so does a glob
elsewhere in the command (`ls *.md`), which zsh expands as meant. Heredoc
bodies and comments are data, so they are skipped.

It reads the command as the shell would, quotes respected, and fails open:
an input it cannot read is allowed.

Exit code 2 blocks the tool call and shows stderr to Claude.
"""

import json
import re
import sys

FLAG = re.compile(r"--(?:include|exclude)=")
GLOB_CHARS = "*?["
# A shell word ends at unquoted whitespace or an operator.
WORD_END = " \t\r\n;&|()<>"


class Blocked(Exception):
    pass


def block(reason):
    raise Blocked(reason)


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


def word_starts_at(command, i):
    """True when position i begins a shell word (after space or an operator)."""
    return i == 0 or command[i - 1] in WORD_END


def quoted_marks(command):
    """One flag per character: True when the shell reads it as quoted,
    escaped or part of a comment, so it cannot expand a glob there."""
    marks = []
    quote = None  # the open quote character, or None
    i, n = 0, len(command)
    while i < n:
        c = command[i]
        if quote is None and c == "#" and word_starts_at(command, i):
            end = command.find("\n", i)
            end = n if end == -1 else end
            marks.extend([True] * (end - i))
            i = end
        elif c == "\\" and quote != "'" and i + 1 < n:
            marks.extend([True, True])
            i += 2
        elif quote is None and c in "'\"":
            quote = c
            marks.append(True)
            i += 1
        elif quote is not None:
            marks.append(True)
            if c == quote:
                quote = None
            i += 1
        else:
            marks.append(False)
            i += 1
    return marks


def unquoted_glob_word(command):
    """The first --include=/--exclude= word with an unquoted glob, or None."""
    marks = quoted_marks(command)
    n = len(command)
    for m in FLAG.finditer(command):
        start = m.start()
        if marks[start] or not word_starts_at(command, start):
            continue
        end = m.end()
        while end < n and (marks[end] or command[end] not in WORD_END):
            end += 1
        for k in range(m.end(), end):
            if not marks[k] and command[k] in GLOB_CHARS:
                return command[start:end]
    return None


def main():
    try:
        event = json.load(sys.stdin)
        tool_input = event.get("tool_input") if isinstance(event, dict) else None
        command = tool_input.get("command") if isinstance(tool_input, dict) else None
        if not isinstance(command, str):
            return
        word = unquoted_glob_word(strip_heredocs(command))
        if word is not None:
            block(
                f"`{word}` has an unquoted glob, so zsh expands it before grep "
                "runs and grep can search nothing. Quote the value: --include='*.ts', "
                "or use git grep -n PATTERN -- '*.ts'."
            )
    except Blocked as reason:
        print(f"Blocked by .claude/hooks/guard_zsh_globs.py: {reason}", file=sys.stderr)
        sys.exit(2)
    except Exception:  # noqa: BLE001 — fail open
        return


if __name__ == "__main__":
    main()
