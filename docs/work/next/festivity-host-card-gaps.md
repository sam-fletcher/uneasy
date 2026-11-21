---
title: "Host Festivity: the host's pending-mar card loses its draft and addresses a stand-in as you"
order: 84
---

Two gaps in the host's pending-mar card (`festivity/HostPendingMar.svelte`). Both predate the roll-lock wait,
and neither is fixed.

**The draft is lost on the duel-challenge path.** A guest's make can be challenge-duel. The server records the
guest's outcome and opens a challenge (`hfOpenChallenge`, `handler/plan_host_festivity_routes.go`).
`HostFestivityPanel.svelte` then gates the card on `!fest.pendingChallenge`, which unmounts it, and the host's
draft (picked option, rumor text, asset choice) goes with it. The Make picker (`HostChoosing`) has the same gap.
Fix both: keep the draft out of the mount, or keep the card mounted through a pending challenge.

**A stand-in is addressed as "you".** The card's header reads "X insists you take a Mar", and its option copy
("You choose which one") also addresses the viewer. When a Make Demands perform-steps winner drives the host's
steps, the winner sees the card, but the mar lands on the host. Word the card so a stand-in sees the host named
in the header and in the choice copy.
