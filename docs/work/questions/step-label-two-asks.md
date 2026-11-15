---
title: "When one wait holds two different asks, the heading names what they share"
---

The heading is one phrase for everyone the game waits on. Usually that is one ask. In four places the server's
single wait covers two different asks at once, one for each of two people it names, and a phrase for either ask would
tell the other person to do the wrong thing. Each of these is named for what the two asks share, never for one of them:

| plan | the two asks | when only one is open | when both are |
|---|---|---|---|
| Propose Decree, before the debate | the preparer finalizes the text and opens the debate; each lower-ranked player decides whether to join | (the debate, once open) "join or decline the council", then "end the debate" | **"form the council"** |
| Seek Answers, a question in flight | the target answers or vetoes; the actor still has picks of their own | "answer the question" | **"the question and the picks"** |
| Make Introductions, a mar | the actor settles each newcomer's fate; a recipient or author writes a newcomer's marginalia | "settle each newcomer" / "describe a newcomer" | **"the newcomers"** |
| Spread Propaganda's counter-propaganda, Host Festivity | (these already read this way) | | "counter-propaganda", "the festivity" |

So a player reads, for example, "Your move — the question and the picks" whether they were asked the question or are
the one who asked it, and the panel under it shows each their own controls. The cost is that the heading is vaguer
for those states than for any one-ask step. The plan's own function decides which state it is from its data and the
server's acting set (`lib/plans/stepLabels/{propose_decree,seek_answers,make_introductions}.ts`), and a test holds
each state.

- **Keep the shared names** (what is there now). One phrase, true for everyone it names, and the same rule in every
  plan.
- **Keep the one-ask phrase and restore a line for the viewer it is wrong for.** The heading would say "answer the
  question" to the actor, with a line under it ("Waiting for ben to answer your question…") to correct it. Each
  heading then reads sharply, but it is wrong at the top of the screen for one of the two people it names, and the
  correction depends on a line that has to stay in each panel.
- **Split by who is looking:** the target sees "answer the question", the actor "carry out the picks". Each reader gets
  their own task, but the page's heading and a watcher's would then disagree about the step, which is what the
  registered label function avoids (the same call feeds both).

If you would rather another wording for any of the three shared names, it is one line in its file.
