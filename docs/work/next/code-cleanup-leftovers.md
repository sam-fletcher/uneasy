---
title: "Code and test cleanups nobody has picked up"
order: 75
---

- Plan panels over about 400 lines, to split the way `plans/duel/`, `festivity/`, `war/`,
  `rumors/`, `seek/` and `liaise/` are (a frame, and one component per step): Propose Decree
  (751), Make Introductions (709), Exchange Courtiers (691), Spread Propaganda (687),
  Chronicle Histories (676), Make Demands (506). Two step components are about as long:
  `demand/TargetPlanDemandOverlay.svelte` (505) and `duel/BoutsPhase.svelte` (401).
