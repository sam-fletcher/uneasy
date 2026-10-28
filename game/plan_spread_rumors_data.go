package game

// plan_spread_rumors_data.go — typed resolution_data for Spread Rumors.

// SpreadRumorsResolutionData holds Spread Rumors plan state stored inside the
// plans.resolution_data JSON column, nested under the "spread_rumors" key.
type SpreadRumorsResolutionData struct {
	// PreRollDone gates the pre-roll narration step. OnResolve opens the plan in
	// 'resolving' with no roll; the preparer states the rumor and how it is
	// spreading via sr-cast-roll, which posts that narration publicly, records it
	// below, flips this true and creates the dice roll — so everyone deciding
	// whether to help or interfere has read it first (principle 1).
	//
	// Plans that rolled before the step existed carry no flag; the panel and the
	// make-choice guard treat "a roll exists" as equivalent.
	PreRollDone bool `json:"pre_roll_done,omitempty"`
	// RumorText is the rumor as stated in the pre-roll — the committed version,
	// which may differ from the draft in preparation_notes (or in a kept Secret).
	// It is what a make writes on the Rumors sheet. Public: stating it is what
	// makes a secret rumor public (the book's "keep it secret *for now*").
	RumorText string `json:"rumor_text,omitempty"`
	// HowSpread is the preparer's description of how the rumor is getting out.
	HowSpread string `json:"how_spread,omitempty"`

	// CounterRumorText is what the mar actor wrote at the start of the mar branch:
	// "The target player describes the rumor that spreads about you instead." It
	// becomes the Rumors-sheet row on a mar; the preparer's own failed rumor never
	// does. Its presence is the server-side gate on the mar's option step.
	CounterRumorText string `json:"counter_rumor_text,omitempty"`
	// CounterRumorAuthorID is who wrote it, and so the counter-rumor's source
	// unless they pick "hide yourself as the source".
	CounterRumorAuthorID int64 `json:"counter_rumor_author_id,omitempty"`
	// CounterTargetAssetID is the preparer's main character as it stood when the
	// mar's options were applied (not at preparation): the counter-rumor's subject,
	// and the asset its break and leverage picks land on. Pinned so the rumor row
	// and those picks keep agreeing even if the flag moves mid-resolution.
	CounterTargetAssetID *int64 `json:"counter_target_asset_id,omitempty"`

	// SourceHidden is set once a "hide yourself as the source" pick is applied; it
	// causes the rumor's authorship to be hidden in the public record. "Reveal
	// yourself" never touches it — that option is gloating, not attribution.
	SourceHidden bool `json:"source_hidden,omitempty"`
	// RumorID is the rumor row created at resolve time.
	RumorID *int64 `json:"rumor_id,omitempty"`
	// IsSecret is set when the preparer chose to keep the rumor secret at prep
	// time ("write it on the underside of one of your assets"). The rumor text
	// then lives only in the Secret below — never in preparation_notes — until a
	// Make publishes it. SecretAssetID/SecretID are non-sensitive metadata (the
	// prepared-log post already names the holding asset).
	IsSecret      bool   `json:"is_secret,omitempty"`
	SecretAssetID *int64 `json:"secret_asset_id,omitempty"`
	SecretID      *int64 `json:"secret_id,omitempty"`

	// PendingTakeConsent is non-nil while a "take asset" choice is waiting for
	// the victim (the player who would lose the asset) to agree or disagree.
	// While set, the make/mar choices are NOT yet committed — the aggressor's
	// picks live inside this struct and are only applied once the victim
	// agrees. ComputeRowState surfaces this as await_take_consent.
	PendingTakeConsent *TakeConsentRequest `json:"pending_take_consent,omitempty"`
	// TakeAssetDenied is set when the victim disagreed. It disables the
	// "take asset" option when the aggressor returns to the option picker, so
	// the resolution can't loop on an asset the owner won't surrender.
	TakeAssetDenied bool `json:"take_asset_denied,omitempty"`
	// TakeResolved is set once the agreed-to transfer(s) have been applied. It
	// tells the panel the take-asset step is complete (the transfer happens at
	// consent time, not in a later sub-flow).
	TakeResolved bool `json:"take_resolved,omitempty"`

	// BreakTargetDone / HideSourceDone count how many of the picked break_target
	// / hide_source sub-flow steps have actually been carried out (a marginalia
	// torn, a source-secret written). They are the server-authoritative
	// completion signal so the panel doesn't re-prompt after a refresh/remount —
	// the per-step picker shows (picked − done) remaining. The handlers also
	// reject any step beyond the picked count, so a stale client can't write a
	// duplicate.
	BreakTargetDone int `json:"break_target_done,omitempty"`
	HideSourceDone  int `json:"hide_source_done,omitempty"`

	// Specifics of each completed step, recorded so the panel can render what was
	// actually done read-only to every viewer rather than collapsing to a counter
	// (Tier-1 committed state, ADR-006).
	//
	// All three are public facts. hide_source is fiction-level anonymity — the
	// preparer is already named by the plan.prepared post, and the hide-source log
	// entry states the concealment outright (the dramatic-irony convention) — so
	// naming the sheltering asset here leaks nothing the table doesn't have. The
	// Secret's *text* is never stored or displayed here.
	BrokenAssetIDs     []int64 `json:"broken_asset_ids,omitempty"`
	HideSourceAssetIDs []int64 `json:"hide_source_asset_ids,omitempty"`
	TakenAssetIDs      []int64 `json:"taken_asset_ids,omitempty"`
}

// TakeConsentRequest captures an open "take asset" consent gate: the
// aggressor's full set of make/mar picks (applied verbatim on agreement), the
// specific assets they want to take, and who must consent. It is stored inside
// SpreadRumorsResolutionData.PendingTakeConsent.
type TakeConsentRequest struct {
	// Choices is the full flat list of picked option keys, applied exactly as
	// MakeChoice would once the victim agrees.
	Choices []string `json:"choices"`
	// Result is the roll outcome these choices belong to ("make" | "mar").
	Result string `json:"result"`
	// AssetIDs are the victim's assets the aggressor wants to take (one per
	// "take_asset" pick). On make these may be any of the victim's assets, not
	// only the rumor's target asset.
	AssetIDs []int64 `json:"asset_ids"`
	// VictimID owns AssetIDs and must agree before anything is committed.
	VictimID int64 `json:"victim_id"`
	// RequestedBy is the aggressor (the preparer on make, the target-asset
	// owner on mar).
	RequestedBy int64 `json:"requested_by"`
}

// EnsureSpreadRumors returns r.SpreadRumors, allocating a zero struct if it
// was nil. Use from write paths.
func (r *ResolutionData) EnsureSpreadRumors() *SpreadRumorsResolutionData {
	if r.SpreadRumors == nil {
		r.SpreadRumors = &SpreadRumorsResolutionData{}
	}
	return r.SpreadRumors
}
