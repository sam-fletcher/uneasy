# Uneasy Lies the Head

A web adaptation of *[Uneasy Lies the Head](https://adambell.itch.io/uneasy-lies-the-head-2e)*,
a 2–5 player competitive GMless tabletop RPG set in a royal court. Players
each control a noble and their retinue, scheming and allying their way
through a 13-row timeline toward a climactic Shake-Up — no winner, just a
dramatic story everyone told together. This adaptation turns it into an
asynchronous, "play by post" web game: make your move, then come back
whenever the next player has made theirs.

![Early-on in the Main Event, showing the chat log and a player's retinue](docs/images/screenshot.png)

## Attribution

*Uneasy Lies the Head* was designed by [Adam Bell](https://adambell.games/).
Published with permission from the author. If you want the original
tabletop rules, [buy the book on itch.io](https://adambell.itch.io/uneasy-lies-the-head-2e).

## Quick start

You need Docker Desktop.

```bash
docker compose up
```

Open http://localhost:8080, sign up, and start a table. See
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for the full dev setup (Go/Node
toolchains, tests, sqlc, dev-login shortcuts) and
[docs/OPERATIONS.md](docs/OPERATIONS.md) for running/administering a live
instance.

## License

The code in this repository is [MIT licensed](LICENSE). The game itself —
its rules, text, and setting — is the intellectual property of Adam Bell and
is used here with his permission; the license does not extend to the game
content.
