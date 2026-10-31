---
name: drive-plan
description: Put a dev game into a chosen state of one of the 12 plans (preparation, pre-roll, a forced make or mar, a sub-flow) and look at it as each player at 375px. Use when verifying plan UI or mechanics by hand, reproducing a plan bug, or checking what a non-acting player sees.
---

# Driving a plan to a state, and seeing it as each player

Three dev-only tools (they exist only when the server runs with
`UNEASY_DEV=1`) do the setup and the looking:

| Tool | Job |
|---|---|
| `go run ./cmd/plandriver` | Seed a 3-player main-event game, prepare one plan with a payload that plan actually validates, fast-forward to its resolution row |
| `POST /api/dev/force-roll` | Force an open roll to `make`, `mar` or an exact `result`, through the **real** resolution path (broadcasts, chat post, auto-apply all fire) |
| `node frontend/tests/shot.mjs` | Screenshot + visible text of a table **as one player**, in its own browser context |

**In a worktree, these default ports test `main`, not your code.** Start your
own server and pass `-base` / `--base` — see the `test-worktree` skill.

## 1. Set up

```bash
go run ./cmd/plandriver setup <plan_type> [-players alice,bob,carol] [-force make|mar] [-base URL]
```

`go run ./cmd/plandriver` with no arguments lists the twelve plan types. The
tool prints the game id and table URL, and says which kind of resolution you
got:

- **Make Demands** opens its roll the moment resolution starts, so `-force`
  works in the same command.
- **The other eleven open a pre-roll step first** and have no dice yet. Drive
  that step (through the UI, or the plan's own extra route — `sp-cast-roll`,
  `sr-cast-roll`, `seek-cast-roll`, …), then force:
  `go run ./cmd/plandriver force <game_id> make|mar [-result N]`.
- **Make War and Clandestinely Liaise** have no row at all until their
  simultaneous delay reveal is answered.

Use your own player names (not alice/bob/carol) when other sessions may be
driving games at the same time, so the logs and screenshots stay legible.

## 2. Look, as every player who matters

```bash
cd frontend
node tests/shot.mjs --game <id> --as <player> --label <short-name>          # 375px, mobile first
node tests/shot.mjs --game <id> --as <player> --text-only                   # just the text
node tests/shot.mjs --game <id> --as <player> --label <name> --width 1280   # desktop, second
```

Each run is a fresh browser with its own cookie jar, so runs can go in
parallel and nobody becomes anybody else. **Don't switch players inside one
browser** (the browser pane, or tabs of one Playwright context): the session
cookie is per-origin, so logging in as bob silently turns every tab into bob.

For any plan state, look as **the actor, the other party (target, partner,
host), and a bystander**. The Waiting On bar must name everyone who has to
act, and nobody else; non-actors should see neutral wording, not the actor's
controls.

`--click <text>` is for client-only UI state (opening an overlay). Anything
the server records must be driven over the API, because every shot.mjs run
replays its clicks from scratch.

## 3. Clean up

```bash
go run ./cmd/plandriver cleanup <game_id> [-base URL]
```

## Traps the driver already handles (know them when you go off-script)

- **Preparing a plan passes focus** to the next player, and only the focus
  player may prepare. That's why a Make Demands setup takes two turns. A raw
  `POST /api/dev/advance-row` does **not** rotate focus.
- `advance-row {"plan_id":N}` flips that plan straight to `resolving`; a
  separate resolve call afterwards then 409s.
- The ready endpoint's field is **`is_ready`**. Sending `ready` silently
  decodes as false and the roll never resolves.
- Exchange Courtiers needs **both** `target_player_id` and `target_asset_id`.
- A seeded retinue holds one peer — the main character — so peer-targeting
  plans hit the main-character-replacement gate unless you add a peer.
- `force-roll` overwrites **every** die face. On a Propose Duel final roll it
  discards the dice staked under the assets — force a duel by choosing its
  bouts instead.
- `force-roll` refuses what can't happen: a mar at difficulty 1, or a result
  above the actor's dice count.
