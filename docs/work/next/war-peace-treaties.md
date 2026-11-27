---
title: "Make War: agreed peace terms are kept on the Laws tab as treaties, and the closing line quotes them"
order: 84
after: next/war-cost-and-spoils-words.md
---

Ruled by the owner on 2026-11-26. A peace proposal's terms are stored with it
(`war_peace_proposals.terms`, required), posted to the log once when proposed, and shown in the war's
panel while the vote is open. Once everyone accepts, nothing keeps them: the closing line says only
"the war is over", and a returning player has to find the proposal in chat.

- **The Laws tab gains a "Treaties" group below the laws** (`LawsRumors.svelte`). It's a group of its
  own so a treaty never reads as a law a decree could amend. Each treaty is a plain entry, as a law
  is: the terms in the proposer's words, then a byline with the war's name and the players who agreed
  (each name with its shape, as bylines carry). It has no Edit button. Everyone agreed to the terms,
  so no one player may change them. The group shows only when the table has a treaty.
- **Derive treaties from accepted proposals.** Read them from `war_peace_proposals` with
  `status = 'accepted'`, and the players who agreed from `war_peace_votes`, rather than copying them
  into a new table. Send them with the laws payload, so the Laws tab and its "new" dot need no second
  fetch.
- **The "new" dot.** A treaty is new to you, like a law, until you've looked at Laws. Accepting a
  proposal must bump the laws revision that `player_laws_rumors_seen.laws_seen_rev` is compared
  against. Find where a law's creation bumps it, and do the same at acceptance.
- **The closing line quotes the terms:** "All active participants agreed to peace terms: '…' — the
  war is over." (`handler/plan_make_war_peace.go`). Quote it as other log lines quote player text.

Terms stay words: the app carries none of them out. Whether terms may move assets, repair a broken one
or resolve an unprepared plan is in `questions/make-war-peace-proposal-loose-ends.md`.

**Done when:** an integration test accepts a proposal, then reads the treaty in the laws payload (its
terms, war name and who agreed) and finds the laws revision moved; a component test renders the
Treaties group with no Edit button; a 375px screenshot of Laws with a law and a treaty.
