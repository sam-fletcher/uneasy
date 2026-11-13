---
title: "What does the Tones chip say when no tone has been set?"
---

Tones open from a chip in the chat header, "Tones · N set" (`components/table/` hands the count to
`ChatPanel`, which draws it as a stopgap until the Chat piece restyles the header). In a new game
nothing has been set, so the chip reads "Tones · 0 set". That is accurate and consistent, but "0 set"
is a little flat on the one control that invites players to write their tones before the Main Event.

- Keep "Tones · 0 set"?
- Or read just "Tones" until something is set, and "Tones · N set" after?

The Chat piece owns the header, so this is a one-line change there either way.
