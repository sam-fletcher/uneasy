---
title: "Chronicle Histories: first-timer confusions found in the UI pass"
order: 44
---

None break the plan; each is a place a new player stalls or is surprised.

- The difficulty meter shows "knowledge rank 2" without saying a lower rank is
  better, so a rank-4 player can't tell what the floor means.
- With nothing invoked, "Break an invoked artifact" opens an empty picker
  ("No intact marginalia on invoked artifacts.") with no hint to invoke
  something first; the option isn't disabled.
- Nothing warns an artifact's *owner* that a mar is about to tear their
  marginalia: any chooser can pick it, and the owner learns from the log.
- The server's `invoke_another` accepts a destroyed artifact; only the UI hides
  it (a stale client or direct call can invoke a corpse).
- The scene text goes only to the chat, but the pre-roll panel doesn't say so;
  the "Summary" field on each make step is likewise required in the panel though
  the server treats narration as optional.
