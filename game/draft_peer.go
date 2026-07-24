package game

// draft_peer.go — DraftPeer, the shared shape for a peer who exists in the
// fiction but owns no `assets` row yet.
//
// Two plans introduce peers who are not (yet) in anybody's retinue: Make
// Introductions names them before the dice are cast, and Host Festivity puts
// them in the middle of the party for any guest to claim. Modelling those as
// real assets is what produced the audit's "in a retinue before they arrive"
// bug class — a peer who narratively hasn't shown up could be broken, taken,
// leveraged or staked, and could then be destroyed retroactively.
//
// So they are drafts instead: plain data inside the owning plan's
// resolution_data, with no row anywhere until the peer enters a retinue.
// **Arrival is creation** (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md, D4) — and
// because materialization routes through createAssetWithFirstMarginalia, every
// peer who arrives necessarily carries at least one marginalia, which is why
// this dissolves the blank-asset problem here rather than adding an invariant
// to police.
//
// The two plans share this shape and the materializer beside it, but not the
// lifecycle (D8): when a draft arrives, who may claim it, and what happens to
// one nobody claims are per-plan policies.

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

// DraftPeer is a named peer awaiting materialization into a real asset.
type DraftPeer struct {
	// ID is stable within the plan that holds the draft. Deliberately a string
	// id rather than a slice index: both plans remove drafts mid-flow (MI
	// resolves peers one at a time, HF removes claimed ones from the centre),
	// and an index would silently re-point at a neighbour.
	ID string `json:"id"`
	// Name is the peer's name, chosen when the draft is recorded.
	Name string `json:"name"`
	// Marginalia is the one note the peer will carry into play. Host Festivity
	// collects it when the peer is introduced; Make Introductions collects it at
	// arrival, since the printed rule names peers pre-roll and describes them
	// only once they turn up.
	Marginalia string `json:"marginalia,omitempty"`
	// CreatorID is the player who introduced the peer — not necessarily the one
	// whose retinue they end up in.
	CreatorID int64 `json:"creator_id"`
}

// NewDraftPeerID mints an id for a fresh draft. Random rather than sequential
// so ids stay unique across removals and across the two plans' differing
// lifecycles.
func NewDraftPeerID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read does not fail on any supported platform; fall back to
		// a time-based id rather than mint an empty one that would collide.
		return "d" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// FindDraftPeer returns the draft carrying id, or nil when there is none.
func FindDraftPeer(drafts []DraftPeer, id string) *DraftPeer {
	for i := range drafts {
		if drafts[i].ID == id {
			return &drafts[i]
		}
	}
	return nil
}

// RemoveDraftPeer returns drafts without the one carrying id, preserving order.
// Returns a new slice; the input is untouched.
func RemoveDraftPeer(drafts []DraftPeer, id string) []DraftPeer {
	out := make([]DraftPeer, 0, len(drafts))
	for _, d := range drafts {
		if d.ID != id {
			out = append(out, d)
		}
	}
	return out
}
