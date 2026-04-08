-- sqlc query file for public record rows.

-- name: CreatePublicRecordRows :exec
-- Seed rows 1–13 when the game starts the main event.
INSERT INTO public_record_rows (game_id, row_number)
SELECT $1, generate_series(1, 13);

-- name: ListPublicRecordRows :many
SELECT * FROM public_record_rows WHERE game_id = $1 ORDER BY row_number;
