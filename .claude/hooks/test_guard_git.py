#!/usr/bin/env python3
"""Tests for guard_git.py: what it must block, and what it must let through.

Run with `make test-hooks` (part of `make check-fast`). Builds two scratch
repositories — one on `main`, one on a feature branch — so the branch-
dependent cases don't depend on where the suite runs.
"""

import json
import os
import subprocess
import sys
import tempfile

HOOK = os.path.join(os.path.dirname(os.path.abspath(__file__)), "guard_git.py")
BLOCK, ALLOW = 2, 0
# Built from parts so this file's own text never reads as the command.
MERGE = "gh pr " + "merge"


def cases(on_main, on_feature):
    m, f = on_main, on_feature
    return [
        # Pushing and force-pushing your own branch.
        (f, "git push -u origin feature", ALLOW),
        (f, "git push --force-with-lease", ALLOW),
        (f, "git push origin feature --force", ALLOW),
        (f, "git push --force-with-lease origin +feature", ALLOW),
        (f, "git push origin --delete old-branch", ALLOW),
        (f, "git push origin fix-main-bug", ALLOW),
        # Anything that updates, rewrites or deletes main.
        (m, "git push", BLOCK),
        (m, "git push origin HEAD", BLOCK),
        (f, "git push origin @:main", BLOCK),
        (f, "git push -u origin main", BLOCK),
        (f, "git push origin heads/main", BLOCK),
        (f, "git push origin +HEAD:refs/heads/main", BLOCK),
        (f, "git push origin :main", BLOCK),
        (f, "git push origin --delete main", BLOCK),
        (f, "git -c foo=bar push origin main", BLOCK),
        (f, "git --git-dir .git push origin main", BLOCK),
        (f, "GIT_DIR=.git git push origin main", BLOCK),
        (f, "command git push origin main", BLOCK),
        (f, 'bash -c "git push origin main"', BLOCK),
        (f, "(git push origin main)", BLOCK),
        (f, "if true; then git push origin main; fi", BLOCK),
        (f, f"cd {m} && git push", BLOCK),
        (f, f"git -C {m} push", BLOCK),
        (m, f"git -C .. -C {os.path.basename(m)} push", BLOCK),
        # Pushes that send every branch, main included.
        (f, "git push --all origin", BLOCK),
        (f, "git push --branches", BLOCK),
        (f, "git push --mirror origin", BLOCK),
        (f, "git push origin :", BLOCK),
        (f, "git push origin 'refs/heads/*:refs/heads/*'", BLOCK),
        # Merging.
        (f, f"{MERGE} 3", BLOCK),
        (f, f"{MERGE} --auto 3", BLOCK),
        (f, "gh api -X PUT repos/o/r/pulls/1/merge", BLOCK),
        (f, "gh api repos/o/r/pulls/1/merge", ALLOW),  # a read: is it merged?
        # Text that only mentions a blocked command.
        (f, f"gh pr create --body \"$(cat <<'EOF'\n| {MERGE} | blocked |\nEOF\n)\"", ALLOW),
        (f, 'git commit -m "notes\n\ngit push origin main is now blocked"', ALLOW),
        (f, f"git commit -F - <<'EOF'\nTitle\n\ngit push origin main; {MERGE} too\nEOF", ALLOW),
        (f, f'echo "done; {MERGE} 3"', ALLOW),
        (f, 'echo "x && git push origin main && y"', ALLOW),
        # Ordinary commands.
        (f, "gh pr ready 3 && gh pr create --draft --body-file x.md", ALLOW),
        (f, "make check-fast 2>&1 | tail -20", ALLOW),
        (f, "ls > /tmp/x && cat < /tmp/x", ALLOW),
        (f, "cd", ALLOW),
        (f, "cd -", ALLOW),
        (f, "", ALLOW),
        (f, "echo 'unbalanced", ALLOW),
    ]


def run(stdin):
    return subprocess.run([sys.executable, "-I", HOOK], input=stdin,
                          capture_output=True, text=True)


def main():
    failures = []
    with tempfile.TemporaryDirectory() as tmp:
        on_main = os.path.join(tmp, "repo")
        on_feature = os.path.join(tmp, "feature")
        for path, branch in ((on_main, "main"), (on_feature, "feature")):
            subprocess.run(["git", "init", "-q", "-b", branch, path], check=True)
        all_cases = cases(on_main, on_feature)
        for cwd, command, want in all_cases:
            got = run(json.dumps({"tool_input": {"command": command}, "cwd": cwd})).returncode
            if got != want:
                failures.append(f"want exit {want}, got {got}: {command!r}")
    # Malformed input fails open, never with a traceback.
    malformed = ['{"tool_input": null}', '{"tool_input": {"command": null}}', "[]",
                 "not json", '{"tool_input": {"command": "git push origin main"}, "cwd": null}']
    for raw in malformed:
        r = run(raw)
        if r.returncode not in (ALLOW, BLOCK) or "Traceback" in r.stderr:
            failures.append(f"malformed input {raw!r}: exit {r.returncode}")
    for failure in failures:
        print("FAIL", failure)
    print(f"guard_git: {len(all_cases) + len(malformed)} cases, {len(failures)} failed")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
