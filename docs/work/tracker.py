#!/usr/bin/env python3
"""Print the work tracker, or check that its files are well formed.

The tracker is one file per item (docs/work/next/) and one per open owner
question (docs/work/questions/), so parallel PRs never edit the same file.
This script assembles the overview a single TRACKER.md used to give.

    python3 docs/work/tracker.py          # print it (make tracker)
    python3 docs/work/tracker.py --check  # fail on a malformed file (make check-fast)
"""

import pathlib
import sys

WORK = pathlib.Path(__file__).resolve().parent


def front_matter(path):
    """The key: value pairs between the opening --- lines, or None."""
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0] != "---":
        return None
    fields = {}
    for line in lines[1:]:
        if line == "---":
            return fields
        key, sep, value = line.partition(":")
        if sep:
            fields[key.strip()] = value.strip().strip('"')
    return None


def load(folder, required):
    items, problems = [], []
    for path in sorted((WORK / folder).glob("*.md")):
        fields = front_matter(path)
        rel = f"{folder}/{path.name}"
        if fields is None:
            problems.append(f"{rel}: no front matter (--- title: ... ---)")
            continue
        for key in required:
            if not fields.get(key):
                problems.append(f"{rel}: missing '{key}'")
        if "order" in required and fields.get("order") and not fields["order"].isdigit():
            problems.append(f"{rel}: 'order' must be a whole number")
        for key in ("after", "blocked"):
            target = fields.get(key)
            if target and not (WORK / target).exists():
                problems.append(f"{rel}: '{key}' points at {target}, which doesn't exist")
        items.append((rel, fields))
    return items, problems


def main():
    next_items, p1 = load("next", ("title", "order"))
    questions, p2 = load("questions", ("title",))
    problems = p1 + p2
    if (WORK / "TRACKER.md").exists():
        problems.append(
            "TRACKER.md is back: the tracker is one file per item now "
            "(see docs/work/README.md). Move its entries into next/ or questions/."
        )

    if "--check" in sys.argv:
        for p in problems:
            print(f"tracker: {p}", file=sys.stderr)
        return 1 if problems else 0

    print("Next (by order):")
    for rel, f in sorted(next_items, key=lambda i: (int(i[1].get("order") or 0), i[0])):
        extra = ""
        if f.get("blocked"):
            extra = f"  [blocked on {f['blocked']}]"
        elif f.get("after"):
            extra = f"  [after {f['after']}]"
        print(f"  {f.get('order', '?'):>4}  {f.get('title', rel)}{extra}")
        print(f"        {rel}")
    print("\nWaiting on the owner:")
    for rel, f in questions:
        print(f"  - {f.get('title', rel)}")
        print(f"    {rel}")
    print("\nIn flight: gh pr list")
    for p in problems:
        print(f"\ntracker: {p}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
