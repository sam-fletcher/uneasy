-- sqlc query file for plans and plan tokens.

-- ── Plans ────────────────────────────────────────────────────────────

-- name: CreatePlan :one
-- is_finale_bonus is set at creation for a plan validatePlanPreparation clamped
-- onto row 13 under Explosive Finale — the preparer's one bonus plan. It is
-- part of the INSERT rather than a follow-up UPDATE so the plan row is never
-- briefly visible without the flag that accounts for the slot it just spent.
INSERT INTO plans (
  game_id, plan_type, category, preparer_id,
  target_player_id, target_asset_id,
  row_number, row_order, prepared_at_row, preparation_notes, is_finale_bonus
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetPlanByID :one
SELECT * FROM plans WHERE id = $1;

-- name: ListPlansByGame :many
SELECT * FROM plans WHERE game_id = $1
ORDER BY row_number ASC, row_order ASC;

-- name: ListPlansByRow :many
SELECT * FROM plans
WHERE game_id = $1 AND row_number = $2
ORDER BY row_order ASC;

-- name: ListPendingPlansByRow :many
SELECT * FROM plans
WHERE game_id = $1 AND row_number = $2 AND status = 'pending'
ORDER BY row_order ASC;

-- name: ListUnresolvedPlans :many
SELECT * FROM plans
WHERE game_id = $1 AND status IN ('pending', 'resolving')
ORDER BY row_number ASC, row_order ASC;

-- name: SetPlanStatus :exec
-- NOTE ON 'cancelled': it means the plan NEVER CAME TOGETHER, not that anyone
-- cancelled it — there is no player-initiated cancellation anywhere in the game
-- (no unprepare route, no UI). The only writer of this status is a Make War /
-- Clandestinely Liaise delay reveal that lands past row 13 with no Explosive
-- Finale slot to collapse onto. Player-facing copy says "fell through".
UPDATE plans SET status = $2 WHERE id = $1;

-- name: SetPlanResult :exec
UPDATE plans SET result = $2, resolved_at = now(), status = 'resolved' WHERE id = $1;

-- name: CountPlansOnRow :one
SELECT count(*) FROM plans WHERE game_id = $1 AND row_number = $2;

-- name: GetResolvingPlanForGame :one
-- Returns the single plan currently in 'resolving' state for a game.
SELECT * FROM plans WHERE game_id = $1 AND status = 'resolving' LIMIT 1;

-- name: SetPlanResolutionData :exec
UPDATE plans SET resolution_data = $2 WHERE id = $1;

-- name: SetPlanPreparationNotes :exec
-- Overwrites a plan's preparation_notes. Used by Spread Rumors to clear the
-- rumor text off the public plan row once it's been stored as a hidden Secret.
UPDATE plans SET preparation_notes = $2 WHERE id = $1;

-- name: ShiftRowOrderAtOrAfter :exec
-- Increments row_order by 1 for all plans on (game_id, row_number) whose
-- row_order >= $3. Used to slot a Make Demands plan in *before* its target:
-- the demand takes the target's row_order; the target and any later plans
-- on that row shift up by one so the demand resolves first.
UPDATE plans SET row_order = row_order + 1
WHERE game_id = $1 AND row_number = $2 AND row_order >= $3;

-- name: SetPlanRowAndOrder :exec
-- Sets a plan's row_number and row_order in a single update. Used by
-- variable-delay plans (CL, MW) after their simultaneous reveal resolves:
-- row_order is recomputed from CountPlansOnRow at that moment so the
-- newly-placed plan slots correctly behind any plans already on the row.
UPDATE plans SET row_number = $2, row_order = $3 WHERE id = $1;

-- name: GetPlansTargeting :many
-- Returns Make Demands plans whose targeted_plan_id points at the given
-- plan. Callers use it two ways: to reject a second demand on a target that
-- already has an unresolved one (ValidatePreparation / synthesizeCounterDemand),
-- and to find the resolved+made demand whose option winners govern this plan's
-- resolution (DemandWinnersForTargetPlan → asset recipient, leverage control,
-- retarget, perform-steps).
SELECT * FROM plans
WHERE targeted_plan_id = $1
ORDER BY id;

-- name: SetDemandOptionWinners :exec
-- Persists the four draft-pick winners on the demand plan row once the
-- draft completes. Read by the target plan's resolution path.
UPDATE plans SET demand_option_winners = $2 WHERE id = $1;

-- name: SetPlanTargets :exec
-- Updates target_player_id and target_asset_id on a plan. Used by the
-- Make Demands keep_or_change_target winner to retarget a plan via the
-- demand-retarget endpoint.
UPDATE plans
SET target_player_id = $2, target_asset_id = $3
WHERE id = $1;

-- name: SetPlanTargetedPlan :exec
-- Sets targeted_plan_id on a Make Demands plan row. Called from OnPrepare
-- after the plan row has been created.
UPDATE plans SET targeted_plan_id = $2 WHERE id = $1;

-- name: SetPlanFinaleBonus :exec
-- Spends the preparer's Explosive Finale slot on an EXISTING plan: the
-- dice-delay collapse, where a Make War / Clandestinely Liaise reveal lands past
-- row 13 and the plan drops onto row 13 instead of falling through
-- (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §3). Preparation-time bonus plans set the
-- flag in CreatePlan instead. One-directional: nothing ever clears it, because
-- the slot is spent the moment a plan claims row 13.
UPDATE plans SET is_finale_bonus = true WHERE id = $1;

-- name: CountFinaleBonusPlans :one
-- The Explosive Finale slot accounting: a player's slot is SPENT iff this
-- returns > 0. Derived from plans rather than a flag on players so it stays
-- auditable — you can always point at the plan that spent it.
--
-- The status filter is vacuous today (a bonus plan can never fall through: it
-- already holds row 13, and under Explosive Finale an overflowing delay reveal
-- collapses rather than cancelling). It is kept because it is the behaviour we
-- would want if a cancellation path is ever added, and it costs nothing.
SELECT count(*) FROM plans
WHERE game_id = $1 AND preparer_id = $2
  AND is_finale_bonus = true AND status != 'cancelled';

-- name: CountFallenThroughPlansOfTypeOnRow :one
-- Plans of one type this player prepared on this row that fell through
-- ('cancelled'). Non-zero blocks a re-pick of the same type on the same row.
--
-- The block used to be an accident of the plan token never being deleted; the
-- token is now removed when a plan falls through (the shield records real
-- preparations only), so the block is derived instead — from prepared_at_row,
-- which is NOT NULL and survives cancellation. It is wanted on its own merits:
-- the delay faces are CHOSEN, not rolled, so a free retry would let a preparer
-- re-declare until the average lands where they want.
SELECT count(*) FROM plans
WHERE game_id = $1 AND preparer_id = $2 AND plan_type = $3
  AND status = 'cancelled' AND prepared_at_row = $4;

-- name: ListRecentPlansByPreparer :many
-- Returns the most recently prepared plans for a player in a game, ordered
-- newest-first. Used for esteem lockout checks (SP mar option b).
SELECT * FROM plans
WHERE game_id = $1 AND preparer_id = $2
ORDER BY prepared_at_row DESC, id DESC
LIMIT 20;

-- ── Plan Tokens ──────────────────────────────────────────────────────

-- name: CreatePlanToken :one
INSERT INTO plan_tokens (game_id, plan_type, player_id, plan_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListPlanTokensByGame :many
SELECT * FROM plan_tokens WHERE game_id = $1;

-- name: ListPlanTokensByType :many
SELECT * FROM plan_tokens WHERE game_id = $1 AND plan_type = $2;

-- name: GetPlanTokenByTypeAndPlayer :one
SELECT * FROM plan_tokens
WHERE game_id = $1 AND plan_type = $2 AND player_id = $3;

-- name: DeletePlanTokenByPlan :exec
-- Removes the token a plan placed on its shield. Called when a plan falls
-- through (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §6): "there should be no shield
-- for a plan that wasn't actually prepared". plan_tokens.plan_id is a FK to
-- plans(id), so the plan row itself stays for the audit trail while the shield
-- clears — the token drops out of the engrailed ranking tally and its pip
-- disappears from the prep grid, both of which follow from the plan not having
-- happened. The preparer is still blocked from re-picking that type on that row
-- (see CountFallenThroughPlansOfTypeOnRow); lower-ranked players are not.
DELETE FROM plan_tokens WHERE plan_id = $1;

-- name: DeletePlanTokensByCategory :exec
-- Used during ranking update when all plans on a sheet are filled.
DELETE FROM plan_tokens pt
USING plans p
WHERE pt.plan_id = p.id AND pt.game_id = $1 AND p.category = $2;
