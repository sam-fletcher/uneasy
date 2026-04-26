-- sqlc query file for accounts.

-- name: CreateAccount :one
INSERT INTO accounts (username, code_hash, email)
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

-- name: UpdateAccountCode :one
UPDATE accounts SET code_hash = $2, updated_at = now()
WHERE id = $1
RETURNING *;
