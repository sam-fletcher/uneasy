package game

// plan_liaise_data.go — typed resolution_data for Clandestinely Liaise.
//
// LiaiseResolutionData is attached to ResolutionData as the optional
// `Liaise` field; it holds all per-plan state that doesn't fit a dedicated
// SQL column (phase cursor, partner pointer, simultaneous-reveal links,
// keep-secret submissions). See RESOLUTION_DATA_TYPING_PLAN.md for the
// design rationale.

// LiaisePhase enumerates the phases of a Clandestinely Liaise plan.
// Values are stable on-wire strings.
type LiaisePhase string

const (
	LiaisePhaseTogetherAtLast       LiaisePhase = "together_at_last"
	LiaisePhaseSecretsWeKeep        LiaisePhase = "secrets_we_keep"
	LiaisePhaseThingsWeShare        LiaisePhase = "things_we_share"
	LiaisePhaseWhenWillISeeYouAgain LiaisePhase = "when_will_i_see_you_again"
	LiaisePhaseDone                 LiaisePhase = "done"
)

// LiaiseResolutionData holds all Clandestinely Liaise plan state stored
// inside the plans.resolution_data JSON column, nested under the "liaise"
// key.
// A Clandestinely Liaise is a meeting between two SPECIFIC peers — one from
// each player's retinue — chosen when the plan is prepared. The meeting "is
// about" those two peers: PreparerPeerID is the preparer's meeting peer and
// PartnerPeerID is the partner's. The Things We Share options that touch a peer
// (update/break) target the OTHER player's meeting peer specifically.
type LiaiseResolutionData struct {
	Phase           LiaisePhase  `json:"phase,omitempty"`
	PartnerID       *int64       `json:"partner_id,omitempty"`
	PreparerPeerID  *int64       `json:"preparer_peer_id,omitempty"`
	PartnerPeerID   *int64       `json:"partner_peer_id,omitempty"`
	DelayRevealID   *int64       `json:"delay_reveal_id,omitempty"`
	RedelayRevealID *int64       `json:"redelay_reveal_id,omitempty"`
	KeptSecrets     []KeptSecret `json:"kept_secrets,omitempty"`
}

// LoadLiaiseData is a read-only convenience that parses a plan's
// resolution_data column and returns the inner LiaiseResolutionData as a
// value (zero struct when the nested key is absent).
//
// For writes, work with the fat ResolutionData via LoadResolutionData /
// SaveResolutionData and use EnsureLiaise to obtain a non-nil pointer.
func LoadLiaiseData(raw *string) LiaiseResolutionData {
	rd := LoadResolutionData(raw)
	if rd.Liaise == nil {
		return LiaiseResolutionData{}
	}
	return *rd.Liaise
}

// EnsureLiaise returns r.Liaise, allocating a zero struct if it was nil.
// Use from write paths so handlers don't need to nil-check before mutating.
func (r *ResolutionData) EnsureLiaise() *LiaiseResolutionData {
	if r.Liaise == nil {
		r.Liaise = &LiaiseResolutionData{}
	}
	return r.Liaise
}
