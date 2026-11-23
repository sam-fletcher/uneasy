#!/usr/bin/env python3
"""Print the work tracker, or check that its files are well formed.

The tracker is one file per item (docs/work/next/) and one per open owner
question (docs/work/questions/), so parallel PRs never edit the same file.
This script assembles the overview a single TRACKER.md used to give.

    python3 docs/work/tracker.py          # print it (make tracker)
    python3 docs/work/tracker.py --check  # fail on a malformed file (make check-fast)

Every question file must say what its writer checked (`checked:`) before
filing it, except the files frozen in LEGACY_QUESTIONS.
"""

import pathlib
import sys

WORK = pathlib.Path(__file__).resolve().parent

# Question files already on origin/main when the `checked:` rule landed. They
# predate the rule, so they are exempt from `checked:` (they still need a
# title). This set never grows: every new question must say what it checked,
# because about a third of the questions filed before the rule were already
# answered by a ruling. It is a literal list, not a git lookup, because CI
# checks out shallow. A name here whose file has since been deleted is
# harmless.
LEGACY_QUESTIONS = frozenset({
    "break-importance-owner-only.md",
    "chat-scenes-and-speech.md",
    "chat-speaking-as-outside-scenes.md",
    "chronicle-mar-announcement.md",
    "ci-e2e-cadence.md",
    "closing-stage-length.md",
    "connection-notice-look.md",
    "declined-counter-propaganda-pick.md",
    "decree-blank-addendum.md",
    "decree-buffet-wording.md",
    "decree-join-mirror-length.md",
    "delete-set-endgame-mode.md",
    "duel-call-and-tray.md",
    "duel-stand-in-bouts.md",
    "ended-standings-layout.md",
    "festivity-abandoned-peer-break-severity.md",
    "festivity-duel-type-and-place.md",
    "festivity-insist-two-step.md",
    "festivity-peer-edge-cases.md",
    "giving-away-a-main-character.md",
    "kept-secret-line.md",
    "landing-kicker-size.md",
    "liaise-buffet-wording.md",
    "make-demands-held-winner-ready.md",
    "make-demands-pre-roll.md",
    "make-war-peace-proposal-loose-ends.md",
    "marginalia-checkbox-size.md",
    "mi-mar-post-lists-fates.md",
    "notice-marks.md",
    "option-card-lock-reason.md",
    "own-writes-not-new.md",
    "plan-grid-redesign.md",
    "plan-lead-lines-group-gap.md",
    "plan-meter-after-the-dice.md",
    "plan-panel-stays-after-resolve.md",
    "plan-panels-b-main-button.md",
    "plan-substep-lines.md",
    "player-row-details.md",
    "profile-desktop-width.md",
    "profile-finished-unread.md",
    "profile-other-buttons.md",
    "push-blocked-rule.md",
    "push-body-names-the-table.md",
    "real-device-checks.md",
    "record-shakeup-pips.md",
    "record-war-liaise-resolving.md",
    "rename-an-asset-affordance.md",
    "render-crash-report.md",
    "retinue-owner-shape-placement.md",
    "riposte-last-marginalia.md",
    "scene-labels-and-fields.md",
    "scene-panel-focus-line.md",
    "scene-place-mark.md",
    "scene-watcher-picks.md",
    "seen-across-devices-choices.md",
    "shake-up-act-track.md",
    "shake-up-lost-main-character.md",
    "shake-up-notice-tones.md",
    "shake-up-repeated-words.md",
    "spread-propaganda-interferer-tie.md",
    "spread-propaganda-short-mar.md",
    "spread-rumors-blank-prep.md",
    "star-mark.md",
    "step-ask-heading-lobby-shakeup-vote.md",
    "step-heading-wording.md",
    "step-label-liaise-phases.md",
    "step-label-several-picks.md",
    "step-label-two-asks.md",
    "styleguide-crawlers.md",
    "table-title-wording.md",
    "tombstone-cross.md",
    "tone-setting-stray-rows.md",
    "tones-chip-wording.md",
    "update-banner-tint.md",
    "war-buffet-wording.md",
    "war-stay-out-reversible.md",
    "war-takeover-layout.md",
})

CHECKED_HINT = (
    " (say what you read: the rulings, principles, decisions, ADR or style-guide"
    " sections, and why none of them settles the question)"
)


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


def load(folder, required, legacy=frozenset(), legacy_exempt=()):
    """Parses each file in a folder. Files named in `legacy` may omit the keys
    in `legacy_exempt`; every other file must have all of `required`."""
    items, problems = [], []
    for path in sorted((WORK / folder).glob("*.md")):
        fields = front_matter(path)
        rel = f"{folder}/{path.name}"
        if fields is None:
            problems.append(f"{rel}: no front matter (--- title: ... ---)")
            continue
        exempt = path.name in legacy
        for key in required:
            if exempt and key in legacy_exempt:
                continue
            if not fields.get(key):
                hint = CHECKED_HINT if key == "checked" else ""
                problems.append(f"{rel}: missing '{key}'{hint}")
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
    questions, p2 = load("questions", ("title", "checked"),
                         legacy=LEGACY_QUESTIONS, legacy_exempt=("checked",))
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
