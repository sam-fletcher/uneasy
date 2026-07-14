# Operations runbook

> **Deployment status:** live at <https://uneasy.up.railway.app> since
> 2026-07-12 (Railway Free + Neon free tier, `aws-us-west-2`). Backup
> restore drill done 2026-07-13: workflow artifact downloaded, decrypted
> with `BACKUP_PASSPHRASE`, restored into a scratch DB, accounts verified.
> The restore commands below and in the workflow header are tested, not
> aspirational. One known quirk: a Postgres 17 dump's `SET
> transaction_timeout` line errors harmlessly when restoring into the
> Postgres 16 dev container.

This project has no admin UI and no automated moderation — at the scale
it's built for (a few concurrent games, ~100 accounts/year), the operator
does everything by hand with `psql` against the production database. This
document is that manual procedure.

All commands assume you have a `psql` shell open against prod. Grab the
connection string from the Neon dashboard (or `$PROD_DATABASE_URL` if you
already have it exported) and connect directly:

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

## Reset a password

There's no self-service password reset at this scale. The "Forgot
password?" link on the login page sends the player to `/locked-out`, a
logged-out form that pings a private Discord channel
(`DISCORD_WEBHOOK_URL`) with their username and how to reach them
(`adr/FEEDBACK_AND_RESET_PLAN.md`). The loop from there:

1. **Discord ping arrives** with the typed username and the requester's
   stated contact channel (email, Discord handle, or "ask my facilitator to
   vouch for me").
2. **Verify the requester** via that contact channel — this is a small,
   facilitated-group game, so a quick "was that you?" over the same channel
   is enough. Don't skip this step; it's the only thing standing in for
   real identity verification.
3. **Generate a reset link**, run locally against the prod `DATABASE_URL`:

   ```bash
   DATABASE_URL="$PROD_DATABASE_URL" PUBLIC_ORIGIN="https://<railway-app>.up.railway.app" \
     go run ./cmd/resetlink '<their username>'
   ```

   This resolves the account case-insensitively, mints a single-use token
   good for 24 hours, stores only its hash, and prints a link:
   `<PUBLIC_ORIGIN>/reset-password?token=...`. The raw token is never stored
   or reprinted — if you lose the output, just run the command again (the
   unused, unexpired token from the first run is harmless to leave behind).
4. **Send the link** over the same contact channel you verified in step 2.
   The player opens it, sets their own new password, and is sent to the
   login page — no placeholder password ever exists, and redeeming the
   token signs out every existing session on that account.

### Break-glass fallback: `hashpw`

If `resetlink` is broken (migration issue, `cmd/resetlink` won't build,
etc.), fall back to setting a password by hand:

```bash
go run ./cmd/hashpw 'the-new-password'
```

This prints a bcrypt hash. Then, in `psql`:

```sql
UPDATE accounts SET password_hash = '<hash from above>', updated_at = now()
WHERE username = '<their username>';
```

This path involves a real plaintext password crossing whatever channel you
send it over — prefer `resetlink` whenever it's working. Tell the player
their new password out-of-band and have them change it immediately from
their profile page.

## Moderation

If someone posts something awful: delete the game (see above) if it's
scoped to one table, or delete the account if the abuse is coming from a
specific player across games. There is no report button or automated
filter — feedback submissions (pinged to the private Discord channel, see
`handler/notify.go`) are the only inbound signal, so keep an eye on it.
