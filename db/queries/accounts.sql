-- sqlc query file for accounts.

-- name: CreateAccount :one
INSERT INTO accounts (username, password_hash, email)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAccountByID :one
SELECT * FROM accounts WHERE id = $1;

-- name: GetAccountByUsername :one
SELECT * FROM accounts WHERE LOWER(username) = LOWER($1);

-- name: UpdateAccountUsername :one
UPDATE accounts SET username = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateAccountEmail :one
UPDATE accounts SET email = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateAccountPassword :one
UPDATE accounts SET password_hash = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateAccountNotifyCadence :one
-- $2 may be NULL (notifications off) — callers must be presence-aware so a
-- caller-supplied null is distinguishable from "field omitted".
UPDATE accounts SET notify_cadence_hours = $2, updated_at = now()
WHERE id = $1
RETURNING *;
