package game

// festivity.go — Pure rules & state helpers for Host Festivity (Phase 3d).

import "slices"

// Phase constants for Host Festivity, stored in ResolutionData.FestivityPhase.
const (
	FestivityPhaseSocializing  = "socializing"
	FestivityPhaseHostChoosing = "host_choosing"
	FestivityPhaseDone         = "done"
)

// Guest outcome values.
const (
	FestivityOutcomeMake   = "make"
	FestivityOutcomeMar    = "mar"
	FestivityOutcomeOptOut = "opt_out"
)

// Make option keys.
const (
	FestivityMakeSpreadRumor    = "spread_rumor"
	FestivityMakeIntroducePeer  = "introduce_peer"
	FestivityMakeTakeCenterPeer = "take_center_peer"
	FestivityMakeChallengeDuel  = "challenge_duel"
)

// Mar option keys.
const (
	FestivityMarRumorAboutYou = "rumor_about_you"
	FestivityMarDisagreement  = "disagreement"
	FestivityMarAcceptDuels   = "accept_duels"
	FestivityMarBreakSelf     = "break_self"
)

// HostFestivityDifficulty returns the host's esteem status (6 - rank), min 1.
func HostFestivityDifficulty(hostEsteemRank int16) int16 {
	return max(int16(DiceSides)-hostEsteemRank, 1)
}

// IsValidFestivityMakeOption returns true if key is a recognized make option.
func IsValidFestivityMakeOption(key string) bool {
	switch key {
	case FestivityMakeSpreadRumor, FestivityMakeIntroducePeer,
		FestivityMakeTakeCenterPeer, FestivityMakeChallengeDuel:
		return true
	}
	return false
}

// IsValidFestivityMarOption returns true if key is a recognized mar option.
func IsValidFestivityMarOption(key string) bool {
	switch key {
	case FestivityMarRumorAboutYou, FestivityMarDisagreement,
		FestivityMarAcceptDuels, FestivityMarBreakSelf:
		return true
	}
	return false
}

// PendingChallenge records a duel challenge awaiting the target's response.
// While set, all further festivity game actions are blocked until the target
// accepts (spawning a duel plan) or declines.
type PendingChallenge struct {
	ChallengerID int64  `json:"challenger_id"`
	TargetID     int64  `json:"target_id"`
	Notes        string `json:"notes,omitempty"`
}

// FestivityState is a focused view over ResolutionData for Host Festivity.
type FestivityState struct {
	Phase             string
	Guests            []int64
	Outcomes          map[string]string // player_id (str) → "make"|"mar"|"opt_out"
	GuestMakes        map[string]string // guest → chosen make option
	GuestMars         map[string]string // guest → chosen mar option
	HostChoices       map[string]string // mar/opt-out guest → host's make option
	GuestRollIDs      map[string]int64  // guest → their roll id
	GuestIOUs         []int64           // player IDs with unused "make guest" IOU
	HostMarInsists    []string          // mar options forced onto the host
	AcceptDuels       []int64           // player IDs with accept_duels flag
	PendingDuelPlanID *int64            // blocking nested duel
	PendingChallenge  *PendingChallenge // challenge awaiting target response
	CenteredAssetIDs  []int64           // peers placed in center via disagreement
}

// FestivityState returns the Host Festivity view of r.
func (r *ResolutionData) FestivityState() FestivityState {
	return FestivityState{
		Phase:             r.FestivityPhase,
		Guests:            r.GuestPlayerIDs,
		Outcomes:          r.GuestOutcomes,
		GuestMakes:        r.GuestMakeChoices,
		GuestMars:         r.GuestMarChoices,
		HostChoices:       r.HostGuestChoices,
		GuestRollIDs:      r.GuestRollIDs,
		GuestIOUs:         r.GuestIOUs,
		HostMarInsists:    r.HostMarInsists,
		AcceptDuels:       r.AcceptDuelsPlayerIDs,
		PendingDuelPlanID: r.PendingDuelPlanID,
		PendingChallenge:  r.PendingChallenge,
		CenteredAssetIDs:  r.CenteredAssetIDs,
	}
}

// SetFestivityState writes s back into r.
func (r *ResolutionData) SetFestivityState(s FestivityState) {
	r.FestivityPhase = s.Phase
	r.GuestPlayerIDs = s.Guests
	r.GuestOutcomes = s.Outcomes
	r.GuestMakeChoices = s.GuestMakes
	r.GuestMarChoices = s.GuestMars
	r.HostGuestChoices = s.HostChoices
	r.GuestRollIDs = s.GuestRollIDs
	r.GuestIOUs = s.GuestIOUs
	r.HostMarInsists = s.HostMarInsists
	r.AcceptDuelsPlayerIDs = s.AcceptDuels
	r.PendingDuelPlanID = s.PendingDuelPlanID
	r.PendingChallenge = s.PendingChallenge
	r.CenteredAssetIDs = s.CenteredAssetIDs
}

// AllGuestsResolved returns true when every guest has an outcome recorded.
func (s *FestivityState) AllGuestsResolved() bool {
	if len(s.Guests) == 0 {
		return false
	}
	for _, id := range s.Guests {
		if _, ok := s.Outcomes[int64ToKey(id)]; !ok {
			return false
		}
	}
	return true
}

// PendingHostChoices returns guests who still need a host make choice (those
// who rolled mar or opted out, minus those already assigned).
func (s *FestivityState) PendingHostChoices() []int64 {
	out := make([]int64, 0)
	for _, id := range s.Guests {
		k := int64ToKey(id)
		oc, ok := s.Outcomes[k]
		if !ok {
			continue
		}
		if oc != FestivityOutcomeMar && oc != FestivityOutcomeOptOut {
			continue
		}
		if _, assigned := s.HostChoices[k]; assigned {
			continue
		}
		out = append(out, id)
	}
	return out
}

// HasAcceptDuels returns true when playerID has the accept_duels flag.
func (s *FestivityState) HasAcceptDuels(playerID int64) bool {
	return slices.Contains(s.AcceptDuels, playerID)
}

// ConsumeIOU removes one IOU for playerID and returns true if one was found.
func (s *FestivityState) ConsumeIOU(playerID int64) bool {
	for i, id := range s.GuestIOUs {
		if id == playerID {
			s.GuestIOUs = append(s.GuestIOUs[:i], s.GuestIOUs[i+1:]...)
			return true
		}
	}
	return false
}

// IsGuest returns true if playerID is on the guest list.
func (s *FestivityState) IsGuest(playerID int64) bool {
	return slices.Contains(s.Guests, playerID)
}

// int64ToKey stringifies an int64 for use as a map key in JSON-ser ResData
// (JSON map keys must be strings).
func int64ToKey(id int64) string {
	// Avoids importing strconv in hot paths; mirrors fmt.Sprintf("%d", id).
	if id == 0 {
		return "0"
	}
	neg := id < 0
	if neg {
		id = -id
	}
	var buf [20]byte
	pos := len(buf)
	for id > 0 {
		pos--
		buf[pos] = byte('0' + id%10)
		id /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
