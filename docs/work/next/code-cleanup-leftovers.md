---
title: "Code and test cleanups nobody has picked up"
order: 75
---

- Delete the legacy `PendingHostMars` queue once no pre-deploy festivity is running.
- `game/duel.go`'s `ResolveBout` returns `NextDeclarer`, which the handler ignores.
- `middleware.AccountContext` is only called from tests, and `make deadcode` reports
  it. `deadcode@latest` is also unpinned.
- `SeedShakeUp` defaults `current_row` to 13, but live play reaches it at 14.
- No e2e spec seeds a Shake-Up board.
- Eleven test files still use raw `t.Errorf`.
- `types.ts` still lists `tone_setting`.
- Plan panels over about 400 lines: Spread Rumors (1,157), Seek Answers (876), Liaise
  (794).
