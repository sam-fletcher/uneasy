---
title: "Two rules the app reads differently from the book: the decree's addendum, and an open rumor at preparation"
order: 78
---

Ruled by the owner on 2026-11-26, from the book's own pages.

- **Propose Decree: the signatory's addendum is required.** The law "goes into place after the
  signatory adds an addendum" (p.47), on a make and on a mar. Today `set-addendum`
  (`handler/plan_propose_decree.go`) accepts a blank. Refuse a blank with a 400, and make the
  panel's button wait for text, the way every other required text field does.
- **Spread Rumors: an open rumor may be left unwritten at preparation.** The book asks for the text
  at preparation only "if you'd like to keep it secret" (p.59); otherwise the rumor is stated at the
  pre-roll. Today preparation refuses a blank unless the rumor is kept secret
  (`handler/plan_spread_rumors.go`, "requires preparation_notes"). Allow a blank for an open rumor;
  a secret one still needs its text, since a secret always has words. Check the pre-roll step with a
  blank draft: it should start empty and still require the rumor before the roll.

**Done when:** integration tests hold both (a blank addendum refused, an open blank rumor prepared
and then stated at the pre-roll, a secret blank rumor still refused); 375px screenshots of the
addendum step and the rumor's preparation and pre-roll.
