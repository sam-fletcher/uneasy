-- sqlc query file for liaise_choices (Clandestinely Liaise "Things We Share" phase).

-- name: CreateLiaiseChoice :one
INSERT INTO liaise_choices (plan_id, player_id, choice, target_asset_id, target_marginalia_id, update_text)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (plan_id, player_id) DO UPDATE
  SET choice = EXCLUDED.choice,
      target_asset_id = EXCLUDED.target_asset_id,
      target_marginalia_id = EXCLUDED.target_marginalia_id,
      update_text = EXCLUDED.update_text
RETURNING id, plan_id, player_id, choice, target_asset_id, target_marginalia_id, update_text, created_at;

-- name: ListLiaiseChoicesByPlan :many
SELECT id, plan_id, player_id, choice, target_asset_id, target_marginalia_id, update_text, created_at
FROM liaise_choices
WHERE plan_id = $1
ORDER BY created_at ASC;

-- name: CountLiaiseChoicesByPlan :one
SELECT count(*) FROM liaise_choices WHERE plan_id = $1;
