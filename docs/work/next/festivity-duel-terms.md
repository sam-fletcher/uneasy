---
title: "Host Festivity: a challenger names the duel's type (arms or wits) and place"
order: 85
---

Ruled by the owner on 2026-11-26. A festivity's make "Challenge somebody to a duel" resolves Propose
Duel "immediately, skipping the delay" (p.61). That skips the delay, not the preparation, and Propose
Duel's preparation names the type and the place (p.60). Today a festivity duel carries only the
challenger's optional note (or "Duel triggered by festivity"), so its heading and log line say only
"a duel" and nothing names a place.

**Fix:** the challenge option takes the type (arms or wits) and a place, as an ordinary Propose
Duel's preparation does, and the spawned duel carries them, so its heading and log line read "a duel
of arms" or "of wits". Files: the challenge route (`hfOpenChallenge`,
`handler/plan_host_festivity_routes.go`), the duel it spawns, and the challenge form in
`plans/festivity/`.

**Done when:** an integration test reads the type and place on the spawned duel and its log line; a
375px screenshot of the challenge form and of the duel's heading.
