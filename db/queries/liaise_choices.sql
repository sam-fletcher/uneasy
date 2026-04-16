-- sqlc query file for liaise_choices (Clandestinely Liaise "Things We Share" phase).

-- name: CreateLiaiseChoice :one
INSERT INTO liaise_choices (plan_id, player_id, choice, target_asset_id, banked_die_face)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (plan_id, player_id) DO UPDATE
  SET choice = EXCLUDED.choice,
      target_asset_id = EXCLUDED.target_asset_id,
      banked_die_face = EXCLUDED.banked_die_face
RETURNING id, plan_id, player_id, choice, target_asset_id, banked_die_face, created_at;

-- name: ListLiaiseChoicesByPlan :many
SELECT id, plan_id, player_id, choice, target_asset_id, banked_die_face, created_at
FROM liaise_choices
WHERE plan_id = $1
ORDER BY created_at ASC;

-- name: CountLiaiseChoicesByPlan :one
SELECT count(*) FROM liaise_choices WHERE plan_id = $1;
