package game

// festivity.go — Pure rules & state helpers for Host Festivity.
//
// Storage layout: all Host Festivity state lives in the optional `Festivity`
// pointer on the fat ResolutionData (see plan.go). Handlers go through
// r.EnsureFestivity() for writes and r.Festivity (or LoadFestivityData) for
// reads.

import "slices"

// A Host Festivity has no discrete phases or turns. The whole event is one open
// stretch of socializing: guests roll/opt-out and pick make/mar options, the
// host takes their extra makes, and successful guests inflict extra mars on the
// host — all freely interleaved, in any order. The only ordering constraint is
// that a single roll-and-choice must conclude before the next action starts
// (see ActiveRoller). The event is "open" while the plan is resolving and ends
// when the host winds it down (CanComplete gates that — see EventEndable).

// Guest outcome values.
const (
	FestivityOutcomeMake   = "make"
	FestivityOutcomeMar    = "mar"
	FestivityOutcomeOptOut = "opt_out"
	// FestivityOutcomeHost is the host's pre-recorded outcome. The host does not
	// roll or opt out: as the one throwing the event, they've earned an extra
	// make for hosting. It is assigned in OnResolve so the host is never prompted
	// to roll (which is strictly worse than the guaranteed make).
	FestivityOutcomeHost = "host"
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

// FestivityResolutionData holds all Host Festivity plan state stored inside
// the plans.resolution_data JSON column, nested under the "festivity" key.
//
// There is no stored guest list: every player at the table attends as a guest
// (the roster is fixed once a game starts), so callers derive the guest set
// from the game's players and pass it to the helpers below.
type FestivityResolutionData struct {
	Outcomes          map[string]string `json:"outcomes,omitempty"`         // player_id (str) → "make"|"mar"|"opt_out"|"host"
	GuestMakes        map[string]string `json:"guest_makes,omitempty"`      // guest → chosen make option
	GuestMars         map[string]string `json:"guest_mars,omitempty"`       // guest → chosen mar option
	HostMakesTaken    []string          `json:"host_makes_taken,omitempty"` // make options the host has spent (their spoils)
	GuestRollIDs      map[string]int64  `json:"guest_roll_ids,omitempty"`   // guest → their roll id
	GuestIOUs         []int64           `json:"guest_ious,omitempty"`       // players who succeeded and may still inflict a mar on the host
	HostMarInsists    []string          `json:"host_mar_insists,omitempty"` // mar options inflicted on the host
	AcceptDuels       []int64           `json:"accept_duels,omitempty"`     // player IDs with accept_duels flag
	PendingDuelPlanID *int64            `json:"pending_duel_plan_id,omitempty"`
	PendingChallenge  *PendingChallenge `json:"pending_challenge,omitempty"`
	CenteredAssetIDs  []int64           `json:"centered_asset_ids,omitempty"` // peers placed in center via disagreement
}

// LoadFestivityData is a read-only convenience that parses a plan's
// resolution_data column and returns the inner FestivityResolutionData as a
// value (zero struct when the nested key is absent).
func LoadFestivityData(raw *string) FestivityResolutionData {
	rd := LoadResolutionData(raw)
	if rd.Festivity == nil {
		return FestivityResolutionData{}
	}
	return *rd.Festivity
}

// EnsureFestivity returns r.Festivity, allocating a zero struct if it was nil.
func (r *ResolutionData) EnsureFestivity() *FestivityResolutionData {
	if r.Festivity == nil {
		r.Festivity = &FestivityResolutionData{}
	}
	return r.Festivity
}

// AllGuestsResolved returns true when every guest has an outcome recorded.
// guests is the table roster (every player attends).
func (s *FestivityResolutionData) AllGuestsResolved(guests []int64) bool {
	if len(guests) == 0 {
		return false
	}
	for _, id := range guests {
		if _, ok := s.Outcomes[int64ToKey(id)]; !ok {
			return false
		}
	}
	return true
}

// EarnedHostMakes returns how many extra makes the host has earned: one for
// hosting (the host's own "host" outcome) plus one for every guest who rolled a
// mar or opted out. These are the host's spoils, not debts owed to those guests
// — the trigger guest is irrelevant once counted. guests is the table roster.
func (s *FestivityResolutionData) EarnedHostMakes(guests []int64) int {
	n := 0
	for _, id := range guests {
		switch s.Outcomes[int64ToKey(id)] {
		case FestivityOutcomeMar, FestivityOutcomeOptOut, FestivityOutcomeHost:
			n++
		}
	}
	return n
}

// RemainingHostMakes returns the number of extra makes the host has earned but
// not yet taken.
func (s *FestivityResolutionData) RemainingHostMakes(guests []int64) int {
	return s.EarnedHostMakes(guests) - len(s.HostMakesTaken)
}

// EventEndable reports whether the host may wind the event down: every guest has
// chosen an option, the host has taken all their earned makes, and every
// outstanding mar (a successful guest's IOU) has been inflicted. guests is the
// table roster.
func (s *FestivityResolutionData) EventEndable(guests []int64) bool {
	return s.AllGuestsResolved(guests) &&
		s.RemainingHostMakes(guests) <= 0 &&
		len(s.GuestIOUs) == 0
}

// HasAcceptDuels returns true when playerID has the accept_duels flag.
func (s *FestivityResolutionData) HasAcceptDuels(playerID int64) bool {
	return slices.Contains(s.AcceptDuels, playerID)
}

// ConsumeIOU removes one IOU for playerID and returns true if one was found.
func (s *FestivityResolutionData) ConsumeIOU(playerID int64) bool {
	for i, id := range s.GuestIOUs {
		if id == playerID {
			s.GuestIOUs = append(s.GuestIOUs[:i], s.GuestIOUs[i+1:]...)
			return true
		}
	}
	return false
}

// PendingGuests returns the roster members who have not yet recorded an
// outcome (rolled-and-chosen, or opted out). guests is the table roster (every
// player attends), preserving its order in the result.
func (s *FestivityResolutionData) PendingGuests(guests []int64) []int64 {
	out := make([]int64, 0, len(guests))
	for _, id := range guests {
		if _, ok := s.Outcomes[int64ToKey(id)]; !ok {
			out = append(out, id)
		}
	}
	return out
}

// ActiveRoller returns the guest whose roll-and-choice is in progress — they
// have created a roll but not yet recorded an outcome (still resolving the dice
// or picking make/mar). A roll-and-choice must conclude before the next action
// starts, so at most one such guest normally exists; the roster is scanned in
// order for determinism. Returns 0 if no roll is in progress. guests is the
// table roster.
func (s *FestivityResolutionData) ActiveRoller(guests []int64) int64 {
	for _, id := range guests {
		k := int64ToKey(id)
		if _, rolling := s.GuestRollIDs[k]; !rolling {
			continue
		}
		if _, resolved := s.Outcomes[k]; resolved {
			continue
		}
		return id
	}
	return 0
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
