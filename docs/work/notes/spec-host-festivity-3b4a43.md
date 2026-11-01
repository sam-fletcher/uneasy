# Host Festivity regression spec (Phase 4)

## What changed and why

- `frontend/tests/e2e/host_festivity.spec.ts` — nine specs pinning what the
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
  - **The lock, fully:** a second spec covers insist-host-mar, resolve-host-mar,
    host-choice and challenge-duel under an open roll (each asserted by the
    lock's own 409 message, so "already acted" 409s can't pass for it), and that
    they go through once the roller chooses.
  - **Host duel:** the host spends a make on a challenge (declined by a guest;
    the make is spent regardless) and a guest bound by accept_duels can't decline
    (UI disabled, API 409).
  - **Host-resolved mars:** break_self tears the host's chosen marginalia;
    disagreement sends her peer to the centre (a main character is refused); at
    end-event the unclaimed peer rejoins broken.
  - The make-earns-the-host-nothing pin is server-side too (second host-choice
    409) and the host-make log line is polled. "Nothing visible" checks now follow
    a positive assertion on the same page.
  - Two pins rest on provisional ground and say so in the spec: the "all earned
    makes taken before End" gate (not yet ruled) and the compulsory mar (ruled
    "for now").
- `frontend/tests/e2e/helpers/host_festivity.ts` — seeding and driving helpers.
  Alice is seeded at esteem rank 4 (a dummy holds rank 3 in a four-player game),
  so a guest's roll is difficulty 2 and `force-roll` can reach both outcomes.
- The "earns a make" log lines are read over the API (the default chat filter
  hides Minor lines) and only their text is asserted, never the severity — that
  is the open question `docs/work/questions/minor-severity-logs.md`.
- Product code untouched.

## How it was verified

Against this worktree's own build, on a private DB and port: 9 passed, three
runs in a row. `make check-fast` passes. Each guard was broken locally and the
spec went red (all reverted): one-roll lock off → lock spec; IOUs not gating the
end → end spec; insisted mars not gating → end spec; opt-out not earning a make →
earns-a-make and end specs; host allowed to roll → never-rolls spec; mar
resolvable by a non-host → insist spec; a make earning the host a make → three
specs; decline allowed under accept_duels, host duel refused, rejoin-broken
skipped → one spec each; the lock now fails two specs. Not run: `make test-integration` (no Go
changed).

## Behaviour changes to look at before merging

None; tests only.

## Open questions for the owner

None new.

## Noticed, not fixed

- Not pinned: accepting a challenge (starts a nested Propose Duel, which pauses
  the festivity; that flow belongs to the Propose Duel spec), a guest's own
  `challenge-duel` make, and introduce-peer / take-centre-peer.
- The roller's own exemption from the lock can't be shown on insist-host-mar: a
  guest only holds an IOU after their roll-and-choice concludes.
- "The festivity has wound down." (HostFestivityPanel) is unused copy: the panel
  is only mounted for the resolving plan and the table moves on to the next
  scene on resolve, so no player can see it. Not a bug (the table is told by the
  action log and the scene prompt); the line could be deleted.
