---
title: "Host Festivity: hold the host's pending-mar card while a guest rolls"
order: 77
---

While a guest's roll-and-choice is open, the server refuses every host action
(`hfBlockOnActiveRoll`), including `resolve-host-mar`, where the host answers a guest's insistence
(`plan_host_festivity_routes.go`, around line 176). `HostPendingMar` stays live meanwhile: the host can
pick a mar and its details and only learn from the refusal that they must wait.

The Make picker had the same gap and now gives way to "Waiting for X to finish their roll…" (`HostChoosing`
takes `blockedByOtherRoll` and `activeRollerID`; `HostFestivityPanel` passes them). `HostPendingMar` wants
the same two props, the same line, and a render test next to the Make picker's in `planPanelsC.test.ts`. As
with the Make picker, keep its draft state mounted so a half-written rumor survives the wait.
