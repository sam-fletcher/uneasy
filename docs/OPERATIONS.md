# Operations runbook

This project has no admin UI and no automated moderation — at the scale
it's built for (a few concurrent games, ~100 accounts/year), the operator
does everything by hand with `psql` against the production database. This
document is that manual procedure.

All commands assume you have a `psql` shell open against prod, e.g.:

```bash
fly postgres connect -a <your-postgres-app-name>
```

or, if connecting via `DATABASE_URL` directly:

```bash
psql "$DATABASE_URL"
```

## List accounts

```sql
SELECT id, username, email, created_at FROM accounts ORDER BY created_at DESC;
```

## List games

```sql
SELECT id, join_code, phase, current_row, created_at FROM games ORDER BY created_at DESC;
```

## Delete a game

Migration 041 made every foreign key in a game's ownership tree cascade off
`games`, so deleting a game is a single statement — no need to manually
clear child tables first:

```sql
DELETE FROM games WHERE id = <game_id>;
```

This removes the game and everything under it (players, assets, marginalia,
secrets, plans, dice rolls, chat posts, etc.) but leaves accounts intact —
a deleted player's account, and any other games they're in, are untouched.

## Delete an account

`players.account_id` does **not** cascade from `accounts` (deleting a game
must never be able to delete an account, so the FK runs the other
direction). If the account is still seated in any games, delete those games
first or the account delete will fail on the foreign key:

```sql
-- Find what they're in first:
SELECT g.id, g.join_code, g.phase
FROM games g JOIN players p ON p.game_id = g.id
WHERE p.account_id = <account_id>;

-- Delete each game found above (see "Delete a game"), then:
DELETE FROM accounts WHERE id = <account_id>;
```

Deleting the account cascades to its sessions automatically (`sessions`
does cascade off `accounts`), so there's nothing else to clean up.

## Manually reset a password

There's no self-service password reset at this scale (see the "Forgot
password?" line on the login page — it just points here). To reset one by
hand:

```bash
go run ./cmd/hashpw 'the-new-password'
```

This prints a bcrypt hash. Then, in `psql`:

```sql
UPDATE accounts SET password_hash = '<hash from above>', updated_at = now()
WHERE username = '<their username>';
```

Tell the player their new password out-of-band (email, since that's how
they contacted you). They can change it themselves afterward from their
profile page.

## Moderation

If someone posts something awful: delete the game (see above) if it's
scoped to one table, or delete the account if the abuse is coming from a
specific player across games. There is no report button or automated
filter — the feedback email is the only inbound signal, so keep an eye on
it.
