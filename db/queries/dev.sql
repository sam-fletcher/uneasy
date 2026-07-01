-- Dev-only queries (mounted behind UNEASY_DEV=1).

-- name: DeleteGame :execrows
-- Hard-deletes a single game and, via ON DELETE CASCADE (migrations 039 and
-- 041), all of its rows across every game-scoped table. Returns the number
-- of games deleted (0 if the id didn't exist).
DELETE FROM games WHERE id = $1;
