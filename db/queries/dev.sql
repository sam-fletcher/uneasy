-- Dev-only queries (mounted behind UNEASY_DEV=1).

-- name: DevWipe :exec
TRUNCATE accounts, games RESTART IDENTITY CASCADE;
