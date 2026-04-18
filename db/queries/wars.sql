-- sqlc queries for Make War (Phase 3d).

-- ── Wars ──────────────────────────────────────────────────────────────

-- name: CreateWar :one
INSERT INTO wars (game_id, origin_plan_id, started_at_row)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetWar :one
SELECT * FROM wars WHERE id = $1;

-- name: GetWarByOriginPlan :one
SELECT * FROM wars WHERE origin_plan_id = $1;

-- name: ListActiveWarsByGame :many
SELECT * FROM wars WHERE game_id = $1 AND status = 'active'
ORDER BY id;

-- name: EndWar :exec
UPDATE wars
SET status = 'ended', ended_at_row = $2, end_reason = $3
WHERE id = $1;

-- ── War participants ──────────────────────────────────────────────────

-- name: AddWarParticipant :exec
INSERT INTO war_participants (war_id, player_id, side, joined_at_row)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: AddWarParticipantPending :exec
-- Late joiners to an already-active war start with entry_payment_complete=FALSE;
-- they must pay one break/leverage per existing opposing opponent before being
-- counted as a full participant.
INSERT INTO war_participants (
  war_id, player_id, side, joined_at_row, entry_payment_complete
)
VALUES ($1, $2, $3, $4, FALSE)
ON CONFLICT DO NOTHING;

-- name: SetWarParticipantEntryComplete :exec
UPDATE war_participants
SET entry_payment_complete = TRUE
WHERE war_id = $1 AND player_id = $2;

-- name: GetWarParticipant :one
SELECT * FROM war_participants
WHERE war_id = $1 AND player_id = $2;

-- name: ListWarParticipants :many
SELECT * FROM war_participants
WHERE war_id = $1
ORDER BY side, player_id;

-- name: ListActiveWarParticipants :many
-- Active participants: joined, entry-paid, and not surrendered.
SELECT * FROM war_participants
WHERE war_id = $1
  AND surrendered_at_row IS NULL
  AND entry_payment_complete = TRUE
ORDER BY side, player_id;

-- name: SetWarParticipantSurrendered :exec
UPDATE war_participants
SET surrendered_at_row = $3
WHERE war_id = $1 AND player_id = $2;

-- name: ListActiveWarsForPlayer :many
SELECT w.*
FROM wars w
JOIN war_participants wp ON wp.war_id = w.id
WHERE w.game_id = $1
  AND w.status = 'active'
  AND wp.player_id = $2
  AND wp.surrendered_at_row IS NULL
ORDER BY w.id;

-- ── Battle costs ──────────────────────────────────────────────────────

-- name: CreateBattleCost :one
INSERT INTO war_battle_costs (
  war_id, row_number, payer_id, opponent_id, choice,
  asset_id_1, asset_id_2, surrendered, is_entry
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListBattleCostsForRow :many
SELECT * FROM war_battle_costs
WHERE war_id = $1 AND row_number = $2
ORDER BY created_at;

-- name: ListBattleCostsByPayerForRow :many
SELECT * FROM war_battle_costs
WHERE war_id = $1 AND row_number = $2 AND payer_id = $3
ORDER BY created_at;

-- ── Peace proposals ───────────────────────────────────────────────────

-- name: CreatePeaceProposal :one
INSERT INTO war_peace_proposals (war_id, proposer_id, terms)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOpenPeaceProposal :one
SELECT * FROM war_peace_proposals
WHERE war_id = $1 AND status = 'open'
ORDER BY id DESC LIMIT 1;

-- name: GetPeaceProposal :one
SELECT * FROM war_peace_proposals WHERE id = $1;

-- name: SetPeaceProposalStatus :exec
UPDATE war_peace_proposals
SET status = $2, resolved_at = now()
WHERE id = $1;

-- name: UpsertPeaceVote :exec
INSERT INTO war_peace_votes (proposal_id, player_id, accepted)
VALUES ($1, $2, $3)
ON CONFLICT (proposal_id, player_id) DO UPDATE
SET accepted = EXCLUDED.accepted, created_at = now();

-- name: ListPeaceVotes :many
SELECT * FROM war_peace_votes
WHERE proposal_id = $1
ORDER BY player_id;

-- ── Surrender claims ──────────────────────────────────────────────────

-- name: CreateSurrenderClaim :exec
-- Idempotent: if this (surrendered_id, claimant_id) pair already has an open
-- claim, do nothing (e.g. a player tried to surrender twice).
INSERT INTO war_surrender_claims (war_id, surrendered_id, claimant_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: GetSurrenderClaim :one
SELECT * FROM war_surrender_claims
WHERE war_id = $1 AND surrendered_id = $2 AND claimant_id = $3;

-- name: FulfillSurrenderClaim :exec
UPDATE war_surrender_claims
SET asset_id = $2, fulfilled_at = now()
WHERE id = $1;

-- name: ListOpenSurrenderClaimsByWar :many
SELECT * FROM war_surrender_claims
WHERE war_id = $1 AND fulfilled_at IS NULL
ORDER BY id;

-- name: ListOpenSurrenderClaimsByGame :many
SELECT c.* FROM war_surrender_claims c
JOIN wars w ON w.id = c.war_id
WHERE w.game_id = $1 AND c.fulfilled_at IS NULL
ORDER BY c.id;
