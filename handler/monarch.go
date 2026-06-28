package handler

// monarch.go — the computed monarch role and line of succession (ADR-007).
//
// "The monarch" is durable, computed game state, not a snapshot. It is NOT the
// power-rank-1 player (power drifts during play); it is the controller of the
// asset bearing the highest-priority *title* in the line of succession. This is
// the foundation Propose Decree's auto-seat + signatory selection build on.

import (
	"context"

	dbgen "uneasy/db/gen"
	"uneasy/game"
)

// currentMonarch computes the reigning monarch for a game (ADR-007 §3).
//
// It returns the asset bearing the highest claim in game.SuccessionOrder and
// that asset's current owner (the mechanical signatory). ok is false — meaning
// "this realm has no monarch right now" — when:
//
//   - the throne was never established (no monarch title was ever claimed in the
//     Prologue or via Shake Up); a lone heir is a powerless pretender, or
//   - every throne-line claim has lapsed (interregnum): all such marginalia are
//     torn or their assets destroyed.
//
// Both the is_torn (marginalia) and is_destroyed (asset) filters live in
// ListLiveTitlesByGame; the asset filter is what keeps a direct DestroyAsset
// from leaving an un-torn title crowning a ghost. When ok is false callers fall
// back to the power track (the rules' "or the highest on the power track").
//
// The signature carries an error because a DB failure must NOT masquerade as
// "no monarch" — that would silently mis-seat the council signatory.
//
// monarchAssetID is returned alongside the owner because it is the asset the
// crown UI (Phase D) and take-vs-tear semantics (ADR-007 §6) key off; Propose
// Decree only needs the owner, hence the unparam suppression.
//
//nolint:unparam // monarchAssetID is part of the role contract (Phase D / §6)
func currentMonarch(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
) (monarchAssetID, monarchOwnerID int64, ok bool, err error) {
	g, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		return 0, 0, false, err
	}
	// The load-bearing gate: no throne ever established → no monarch role at all
	// for the whole game, regardless of any stray title marginalia.
	if !g.ThroneEstablished {
		return 0, 0, false, nil
	}

	candidates, err := q.ListLiveTitlesByGame(ctx, gameID)
	if err != nil {
		return 0, 0, false, err
	}

	// Pick the live claim earliest in SUCCESSION_ORDER. Titles outside the line
	// (SuccessionRank ok=false) never qualify. Rows arrive ordered by asset_id,
	// so for a given title the first match is deterministic if (abnormally) two
	// assets share it.
	bestRank := -1
	for _, c := range candidates {
		if c.Title == nil {
			continue // defensive: the query already excludes NULL titles
		}
		rank, inLine := game.SuccessionRank(*c.Title)
		if !inLine {
			continue
		}
		if bestRank == -1 || rank < bestRank {
			bestRank = rank
			monarchAssetID = c.AssetID
			monarchOwnerID = c.OwnerID
		}
	}
	if bestRank == -1 {
		return 0, 0, false, nil // interregnum: throne vacant, fall back to power
	}
	return monarchAssetID, monarchOwnerID, true, nil
}
