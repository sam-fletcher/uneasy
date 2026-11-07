---
title: "Small plan UI gaps left from the October audit"
order: 73
---

Each was checked against the code on 2026-11-07, but re-check before building.

**Exchange Courtiers**
- The riposte picker offers the peer's last intact marginalia, which the server
  refuses.
- The messy-break picker has no destruction warning.
- The riposte choice and an accepted fair trade post no line of their own.

**Host Festivity**
- The host's make picker stays live during a guest's roll or a pending challenge, and
  the server refuses it.
- Decline on a must-accept challenge explains itself only in a `title`, which touch
  screens can't show.
- Log lines for immediate insisted mars don't name who insisted, unlike the others.
- Insisting is one irreversible tap; consider two steps if playtesters trip on it.

**Make War**
- What surrender does is explained only in a `title`, though it's irreversible.
- A war that hasn't reached its row still reads "War — ongoing".

**Propose Duel**
- "Send someone else" has no padding, so it's under 44px.
- The "you fight in person" branch can't fire, since the main character counts as a
  peer.
- The setup line names a main character without the asset mark.

**Seek Answers**
- "Must also describe a flaw" gives no number, and shows on a make too.
- "No resource can be broken" prints twice.

**Spread Propaganda**
- Mar labels keep the book's (a)–(d) lettering.
- The counter-propaganda option doesn't name the top interferer or show the tally.

**Make Demands**
- The target picker is still a native `<select>`.

**Other**
- About 180 "you" lines in plan panels were never reviewed for wrong-person copy.
- `RetinueView`'s main-character picker has a red focus ring, not the app-wide gold.
- The scene panel still prints "Focus: <name>", which may be a leftover.
