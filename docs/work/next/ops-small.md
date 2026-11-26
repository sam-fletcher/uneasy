---
title: "Three small ops changes the owner approved: the endgame route's comment, /dev/ noindex, push text"
order: 88
---

Approved on 2026-11-26.

- **Keep `POST /endgame`, and make its comment say so.** `SetEndgameMode` (`handler/endgame.go`)
  stays as a broken-state backstop for good. Its comment ends "Delete it once the vote has run in a
  real game and it proves unnecessary"; replace that with the reason it stays.
- **`X-Robots-Tag: noindex` on every `/dev/` path,** from the Go static-file handler, so a crawler
  that runs no JavaScript still sees it. `/dev/styleguide` keeps its meta tag.
- **A reminder names the other players at the table,** as the profile's rows do: "The game is
  waiting on you at the table with otto and wren", and "and 2 others" past two names. Today every
  reminder reads "The game is waiting on you!", so a player at several tables can't tell which. The
  title and tag that stop reminders stacking stay as they are.

**Done when:** a server test for the header; a test of the push body's wording at two, three and
five players.
