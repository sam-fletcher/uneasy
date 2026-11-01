# Host Festivity regression spec (Phase 4)

## What changed and why

- `frontend/tests/e2e/host_festivity.spec.ts` — six specs pinning what the
  audit and the festivity overhaul settled:
  - **The host never rolls:** guests see Roll (at the host's difficulty) and
    Opt out; the host sees the make picker and a disabled End button, and no roll
    controls. The server refuses the host's roll, opt-out and guest-choice, and a
    guest's host-choice and end-event, each with 403.
  - **A guest's make:** rolled through the UI, the option lands, the guest holds
    an IOU, the scorecard says so for everyone, and the host gains no make.
  - **Mar / opt-out earn the host a make:** the host's count goes 1 → 2 → 3, the
    host takes one through the UI, can spend exactly what she earned, and the
    next is a 409.
  - **IOU and insist:** only an IOU holder can insist, once; the insist carries
    no option; the host is handed all four mars and chooses (audit Q13); a
    guest can't resolve it for her; the scorecard shows the IOU spent.
  - **One open roll:** while a roll is open — and after the dice land but before
    the choice — another guest's roll/opt-out and the host's make are 409; the
    bystander sees who they are waiting on with disabled buttons; it all frees
    once the roller chooses.
  - **End-event gate:** refused (409) until every guest has chosen, every earned
    make is taken, the guest's mar is inflicted (compulsory, audit #12) and the
    host has settled the insisted mar; the generic `complete` route is refused;
    only the host is offered the button; afterwards the table is told the
    festivity drew to a close, the panel clears, and every later action is 409.
- `frontend/tests/e2e/helpers/host_festivity.ts` — seeding and driving helpers.
  Alice is seeded at esteem rank 4 (a dummy holds rank 3 in a four-player game),
  so a guest's roll is difficulty 2 and `force-roll` can reach both outcomes.
- The "earns a make" log lines are read over the API (the default chat filter
  hides Minor lines) and only their text is asserted, never the severity — that
  is the open question `docs/work/questions/minor-severity-logs.md`.
- Product code untouched.

## How it was verified

Against this worktree's own build, on a private DB and port: 6 passed, three
runs in a row. `make check-fast` passes. Each guard was broken locally and the
spec went red (all reverted): one-roll lock off → lock spec; IOUs not gating the
end → end spec; insisted mars not gating → end spec; opt-out not earning a make →
earns-a-make and end specs; host allowed to roll → never-rolls spec; mar
resolvable by a non-host → insist spec. Not run: `make test-integration` (no Go
changed).

## Behaviour changes to look at before merging

None; tests only.

## Open questions for the owner

None new.

## Noticed, not fixed

- The audit's duel-for-host and the nested-duel pause are not covered here
  (the duel is Propose Duel's flow; its own spec owns it).
- Once the festivity resolves the panel unmounts, so "The festivity has wound
  down." (HostFestivityPanel) is effectively unreachable for players. Harmless
  dead copy; the spec asserts the real behaviour (panel gone, scene prompt up).
