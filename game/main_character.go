package game

import (
	"net/http"

	"uneasy/model"
)

// AssetView is the decoupled domain snapshot of an asset that
// DecideMainCharacterChange needs — just enough to make the decision without
// importing the storage row type. The handler maps dbgen.Asset → AssetView.
type AssetView struct {
	AssetType model.AssetType
}

// MarginaliumView is the decoupled domain snapshot of a marginalium row that
// DecideMainCharacterChange needs. The handler maps dbgen.Marginalium → this.
type MarginaliumView struct {
	Position int16
	IsTorn   bool
}

// MCDecision describes what writes the handler should perform when promoting
// a peer to main character. Always set the new MC flag; conditionally tear
// a marginalium on the old MC and conditionally destroy the old MC.
type MCDecision struct {
	NeedsTear     bool
	TearPosition  int16 // valid only when NeedsTear
	DestroysOldMC bool  // tearing this position empties the last untorn slot
}

// MCDecisionError is a typed error from DecideMainCharacterChange. Code is the
// suggested HTTP status; the handler maps it onto its response writer.
type MCDecisionError struct {
	Code    int
	Message string
}

func (e *MCDecisionError) Error() string { return e.Message }

// DecideMainCharacterChange validates a request to promote `target` to main
// character. The caller must pre-load:
//   - target: the asset being promoted (caller has already checked ownership)
//   - oldMC:  the player's current MC in this game (nil if none, or if it is
//     the same asset as target)
//   - oldMCMarginalia: marginalia rows for oldMC (empty/nil if oldMC is nil)
//   - tearPosition: the body's tear_position field (may be nil)
//
// Pure: no DB I/O, no broadcasts. The handler applies the returned decision.
func DecideMainCharacterChange(
	target *AssetView,
	oldMC *AssetView,
	oldMCMarginalia []MarginaliumView,
	tearPosition *int16,
) (MCDecision, *MCDecisionError) {
	if target == nil {
		return MCDecision{}, &MCDecisionError{Code: http.StatusInternalServerError, Message: "missing target asset"}
	}
	if target.AssetType != model.AssetPeer {
		return MCDecision{}, &MCDecisionError{
			Code:    http.StatusBadRequest,
			Message: "only peer assets can be the main character",
		}
	}

	// No prior MC (or only the target itself was MC) → straight flip, no tear.
	if oldMC == nil {
		return MCDecision{}, nil
	}

	untornCount := 0
	for _, m := range oldMCMarginalia {
		if !m.IsTorn {
			untornCount++
		}
	}
	if untornCount == 0 {
		// Old MC has no untorn marginalia. Spirit of the rule is satisfied;
		// proceed without a tear.
		return MCDecision{}, nil
	}

	if tearPosition == nil {
		return MCDecision{}, &MCDecisionError{
			Code:    http.StatusBadRequest,
			Message: "tear_position required when switching main character",
		}
	}
	pos := *tearPosition
	if pos < 1 || pos > 4 {
		return MCDecision{}, &MCDecisionError{
			Code:    http.StatusBadRequest,
			Message: "invalid tear_position: must be 1–4",
		}
	}
	var targetMarg *MarginaliumView
	for i := range oldMCMarginalia {
		if oldMCMarginalia[i].Position == pos {
			targetMarg = &oldMCMarginalia[i]
			break
		}
	}
	if targetMarg == nil {
		return MCDecision{}, &MCDecisionError{Code: http.StatusBadRequest, Message: "no marginalia at tear_position"}
	}
	if targetMarg.IsTorn {
		return MCDecision{}, &MCDecisionError{
			Code:    http.StatusBadRequest,
			Message: "marginalia at tear_position is already torn",
		}
	}

	return MCDecision{
		NeedsTear:     true,
		TearPosition:  pos,
		DestroysOldMC: untornCount == 1,
	}, nil
}
