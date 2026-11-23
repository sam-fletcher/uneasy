#!/usr/bin/env python3
"""Tests for guard_zsh_globs.py: what it must block, and what it must let through.

Run with `make test-hooks` (part of `make check-fast`). Each case is a Bash
command as Claude would send it; the hook either blocks it (exit 2) or lets
it through (exit 0).
"""

import json
import os
import subprocess
import sys

HOOK = os.path.join(os.path.dirname(os.path.abspath(__file__)), "guard_zsh_globs.py")
BLOCK, ALLOW = 2, 0
PREFIX = "Blocked by .claude/hooks/guard_zsh_globs.py:"


def cases():
    return [
        # An unquoted glob in a --include= or --exclude= value is blocked:
        # zsh would expand it before grep sees the flag.
        ("grep -r --include=*.ts foo src", BLOCK),
        ("grep -rn --exclude=*.test.ts foo src", BLOCK),
        ("grep -r --include=a?.ts foo src", BLOCK),
        ("grep -r --include=[ab].ts foo src", BLOCK),
        ("grep -r --exclude=[ foo src", BLOCK),
        ("grep -r --include=*.ts --exclude=*_test.go foo", BLOCK),
        ("grep -r --include=*'.ts' foo src", BLOCK),
        ('grep -r --include="x"*.ts foo', BLOCK),
        ("grep -r --include=*.ts foo src | head -20", BLOCK),
        ("cd web && grep -r --include=*.svelte foo src", BLOCK),
        ("echo ok; grep -r --include=*.go foo .", BLOCK),
        ("(grep -r --include=*.go foo .)", BLOCK),
        ("git status\ngrep -r --include=*.ts foo src", BLOCK),
        ("grep -r \\\n  --include=*.ts foo src", BLOCK),
        # Quoted or escaped values are what the shell passes through unchanged.
        ("grep -r --include='*.ts' foo src", ALLOW),
        ('grep -r --include="*.ts" foo src', ALLOW),
        (r"grep -r --include=\*.ts foo src", ALLOW),
        ("grep -r --exclude='*.test.ts' foo src", ALLOW),
        ("grep -r --include='*'.ts foo src", ALLOW),
        ("grep -r --include='[ab].ts' foo src", ALLOW),
        ("grep -r --include=ts foo src", ALLOW),
        ("grep -r --include= foo src", ALLOW),
        ("grep -r --includes=*.ts foo src", ALLOW),  # not a flag this hook checks
        # Globs elsewhere in a command are zsh's to expand, as meant.
        ("ls *.md", ALLOW),
        ("wc -l src/*.ts && grep -r --include='*.ts' foo src", ALLOW),
        ("grep -r foo src/*.ts", ALLOW),
        ("git grep -n PATTERN -- '*.ts'", ALLOW),
        # Text that only mentions the flag is data: quoted, a heredoc or a comment.
        ('echo "grep --include=*.ts"', ALLOW),
        ("echo 'grep --include=*.ts'", ALLOW),
        ('git commit -m "tidy: --include=*.ts is blocked"', ALLOW),
        ("git commit -F - <<'EOF'\nTitle\n\ngrep --include=*.ts is blocked\nEOF", ALLOW),
        ("cat <<EOF\ngrep -r --include=*.ts foo\nEOF\nls *.md", ALLOW),
        ("# grep -r --include=*.ts foo\nls", ALLOW),
        ("ls # grep --include=*.ts", ALLOW),
        ('echo "a" # --include=*.ts', ALLOW),
        # Ordinary commands.
        ("", ALLOW),
        ("make check-fast 2>&1 | tail -20", ALLOW),
    ]


def run(stdin):
    return subprocess.run([sys.executable, "-I", HOOK], input=stdin,
                          capture_output=True, text=True)


def main():
    failures = []
    all_cases = cases()
    for command, want in all_cases:
        r = run(json.dumps({"tool_input": {"command": command}, "cwd": "."}))
        if r.returncode != want:
            failures.append(f"want exit {want}, got {r.returncode}: {command!r}")
        elif want == BLOCK:
            # The block message is one line, says why, and says how to fix it.
            lines = r.stderr.strip().splitlines()
            if len(lines) != 1 or not lines[0].startswith(PREFIX) or "Quote the value" not in lines[0]:
                failures.append(f"block message not one line with the fix: {command!r}: {r.stderr!r}")
    # Malformed input fails open, never with a traceback.
    malformed = ['{"tool_input": null}', '{"tool_input": {"command": null}}',
                 '{"tool_input": {"command": 7}}', "[]", "not json", ""]
    for raw in malformed:
        r = run(raw)
        if r.returncode not in (ALLOW, BLOCK) or "Traceback" in r.stderr:
            failures.append(f"malformed input {raw!r}: exit {r.returncode}")
    for failure in failures:
        print("FAIL", failure)
    print(f"guard_zsh_globs: {len(all_cases) + len(malformed)} cases, {len(failures)} failed")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
