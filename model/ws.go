package model

// WebSocket message types shared between the hub and the handler.
//
// Payload structs that carry database records use `any` for the embedded
// fields. This avoids a circular import (model → dbgen → model) while still
// giving us typed envelopes for JSON serialization. The actual concrete
// types (dbgen.Game, dbgen.ScenePost, etc.) are passed in by the handler.

// ── Event types (server → client) ────────────────────────────────────────────

const (
	// Phase 1 events (kept)
	EventPresenceSnapshot = "presence.snapshot"
	EventTypingUpdate     = "typing.update"

	// Phase 2: Game state
	EventPhaseChanged = "phase.changed"
	EventFocusChanged = "focus.changed"
	EventRowAdvanced  = "row.advanced"
	EventSceneEnded   = "scene.ended"

	// Phase 2: Tone-setting
	EventToneUpdated = "tone.updated"

	// Phase 2: Scene posts (replaces Phase 1 post.created)
	EventScenePostCreated  = "scene_post.created"
	EventSceneEntryCreated = "scene_entry.created"

	// Phase 2: Assets
	EventAssetCreated      = "asset.created"
	EventAssetUpdated      = "asset.updated"
	EventAssetTaken        = "asset.taken"
	EventAssetLeveraged    = "asset.leveraged"
	EventAssetRefreshed    = "asset.refreshed"
	EventAssetDestroyed    = "asset.destroyed"
	EventMarginaliaAdded   = "marginalia.added"
	EventMarginaliaUpdated = "marginalia.updated"
	EventMarginaliaTorn    = "marginalia.torn"

	// Phase 2: Rankings
	EventRankingsUpdated = "rankings.updated"

	// Phase 2: Plans
	EventPlanPrepared  = "plan.prepared"
	EventPlanResolving = "plan.resolving"
	EventPlanResolved  = "plan.resolved"

	// Phase 3b: Plan mechanics
	EventPlanDelayedArrival    = "plan.delayed_arrival"    // MI: peer scheduled for future row
	EventRumorCreated          = "rumor.created"           // SR: rumor written to record
	EventSPRecursivePlan       = "plan.sp_recursive"       // SP: recursive propaganda created
	EventSecretVisibilityGrant = "secret.visibility_grant" // SA/CL: visibility granted

	// Phase 3c: Propose Decree
	EventLawEnacted          = "law.enacted"           // PD: law created
	EventDecreeCouncilJoined = "decree.council_joined" // PD: player joined council

	// Phase 3c: Clandestinely Liaise
	EventLiaisePhaseChanged    = "liaise.phase_changed"    // CL: phase advanced
	EventLiaiseChoicesRevealed = "liaise.choices_revealed" // CL: both players submitted Things We Share

	// Phase 3c: Simultaneous Reveals (shared by CL and MW)
	EventRevealSubmitted = "reveal.submitted" // player submitted a reveal entry (face hidden)
	EventRevealComplete  = "reveal.complete"  // all participants submitted; faces revealed

	// Phase 3d: Propose Duel
	EventDuelChampionElected = "duel.champion_elected" // peer elected to fight in stead
	EventDuelStakesRevealed  = "duel.stakes_revealed"  // stake counts revealed (both submitted)
	EventDuelBoutResolved    = "duel.bout_resolved"    // bout comparison complete
	EventDuelBoutsComplete   = "duel.bouts_complete"   // all bouts done; dice tallied

	// Phase 2: Dice rolls
	EventRollCreated       = "roll.created"
	EventRollLeverageAdded = "roll.leverage_added"
	EventRollVoteCalled    = "roll.vote_called"
	EventRollVoteCast      = "roll.vote_cast"
	EventRollVoteResolved  = "roll.vote_resolved"
	EventRollResolved      = "roll.resolved"
)

// ── Command types (client → server) ──────────────────────────────────────────

const (
	CmdTypingStart = "typing.start"
	CmdTypingStop  = "typing.stop"
)

// ── Message envelope ─────────────────────────────────────────────────────────

// WSMessage is the JSON envelope for every WebSocket message in both
// directions: {"type": "...", "payload": {...}}.
type WSMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// ── Payload types ────────────────────────────────────────────────────────────

// PresenceMember is one entry in a presence snapshot.
type PresenceMember struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Online      bool   `json:"online"`
}

// PresenceSnapshotPayload is the payload for EventPresenceSnapshot.
type PresenceSnapshotPayload struct {
	Members []PresenceMember `json:"members"`
}

// TypingUpdatePayload is the payload for EventTypingUpdate.
type TypingUpdatePayload struct {
	PlayerID    int64  `json:"player_id"`
	DisplayName string `json:"display_name"`
	Typing      bool   `json:"typing"`
}

// PhaseChangedPayload is the payload for EventPhaseChanged.
type PhaseChangedPayload struct {
	Phase GamePhase `json:"phase"`
}

// FocusChangedPayload is the payload for EventFocusChanged.
type FocusChangedPayload struct {
	PlayerID    int64  `json:"player_id"`
	DisplayName string `json:"display_name"`
}

// RowAdvancedPayload is the payload for EventRowAdvanced.
type RowAdvancedPayload struct {
	RowNumber        int16 `json:"row_number"`
	CrossedEngrailed bool  `json:"crossed_engrailed"`
}

// SceneEndedPayload is the payload for EventSceneEnded.
type SceneEndedPayload struct {
	RowNumber int16 `json:"row_number"`
	PlayerID  int64 `json:"player_id"`
}

// ToneUpdatedPayload is the payload for EventToneUpdated.
type ToneUpdatedPayload struct {
	TopicID int64           `json:"topic_id"`
	Topic   string          `json:"topic"`
	Status  ToneTopicStatus `json:"status"`
}

// ScenePostCreatedPayload is the payload for EventScenePostCreated.
type ScenePostCreatedPayload struct {
	Post any `json:"post"` // dbgen.ScenePost
}

// SceneEntryCreatedPayload is the payload for EventSceneEntryCreated.
type SceneEntryCreatedPayload struct {
	Entry any `json:"entry"` // dbgen.SceneEntry
}

// AssetPayload wraps an asset for various asset events.
type AssetPayload struct {
	Asset any `json:"asset"` // dbgen.Asset
}

// AssetTakenPayload includes old and new owner for context.
type AssetTakenPayload struct {
	Asset      any   `json:"asset"` // dbgen.Asset
	OldOwnerID int64 `json:"old_owner_id"`
	NewOwnerID int64 `json:"new_owner_id"`
}

// AssetIDPayload is used for simple asset-by-ID events (leveraged, refreshed, destroyed).
type AssetIDPayload struct {
	AssetID  int64 `json:"asset_id"`
	PlayerID int64 `json:"player_id,omitempty"`
}

// MarginaliaPayload wraps a marginalia for add/update events.
type MarginaliaPayload struct {
	AssetID    int64 `json:"asset_id"`
	Marginalia any   `json:"marginalia"` // dbgen.Marginalium
}

// MarginaliaTornPayload is for the torn event.
type MarginaliaTornPayload struct {
	AssetID  int64 `json:"asset_id"`
	Position int16 `json:"position"`
	TornByID int64 `json:"torn_by_id"`
}

// RankingsUpdatedPayload carries the full ranking state.
type RankingsUpdatedPayload struct {
	Rankings any `json:"rankings"` // []dbgen.Ranking
}

// PlanPayload wraps a plan for plan events.
type PlanPayload struct {
	Plan any `json:"plan"` // dbgen.Plan
}

// PlanResolvedPayload includes the plan ID and result.
type PlanResolvedPayload struct {
	PlanID int64  `json:"plan_id"`
	Result string `json:"result"`
}

// RollCreatedPayload wraps a dice roll for the created event.
type RollCreatedPayload struct {
	Roll any `json:"roll"` // dbgen.DiceRoll
}

// RollLeverageAddedPayload is for leverage commitment events.
type RollLeverageAddedPayload struct {
	RollID         int64 `json:"roll_id"`
	PlayerID       int64 `json:"player_id"`
	AssetID        int64 `json:"asset_id"`
	IsInterference bool  `json:"is_interference"`
}

// RollVoteCalledPayload is for the vote initiation event.
type RollVoteCalledPayload struct {
	RollID int64 `json:"roll_id"`
}

// RollVoteCastPayload is for individual vote events.
type RollVoteCastPayload struct {
	RollID   int64  `json:"roll_id"`
	PlayerID int64  `json:"player_id"`
	Vote     string `json:"vote"`
}

// RollVoteResolvedPayload is for when all votes are in.
type RollVoteResolvedPayload struct {
	RollID             int64 `json:"roll_id"`
	AdjustedDifficulty int16 `json:"adjusted_difficulty"`
}

// RollResolvedPayload carries the completed roll with all dice.
type RollResolvedPayload struct {
	Roll          any `json:"roll"`           // dbgen.DiceRoll
	Dice          any `json:"dice"`           // []dbgen.DiceRollDice
	CancelledDice any `json:"cancelled_dice"` // []dbgen.DiceRollDice
}

// ── Phase 3b payload types ────────────────────────────────────────────────────

// PlanDelayedArrivalPayload is for EventPlanDelayedArrival (MI).
type PlanDelayedArrivalPayload struct {
	PlanID      int64 `json:"plan_id"`
	PeerAssetID int64 `json:"peer_asset_id"`
	ArrivalRow  int16 `json:"arrival_row"`
}

// RumorCreatedPayload is for EventRumorCreated (Spread Rumors).
type RumorCreatedPayload struct {
	Rumor any `json:"rumor"` // dbgen.Rumor
}

// SPRecursivePlanPayload is for EventSPRecursivePlan.
type SPRecursivePlanPayload struct {
	ParentPlanID    int64 `json:"parent_plan_id"`
	RecursivePlanID int64 `json:"recursive_plan_id"`
	PreparerID      int64 `json:"preparer_id"`
}

// SecretVisibilityGrantPayload is for EventSecretVisibilityGrant.
type SecretVisibilityGrantPayload struct {
	AssetID  int64 `json:"asset_id"`
	PlayerID int64 `json:"player_id"`
}

// ── Phase 3c payload types ────────────────────────────────────────────────────

// LawEnactedPayload is for EventLawEnacted (Propose Decree).
type LawEnactedPayload struct {
	PlanID int64 `json:"plan_id"`
	Law    any   `json:"law"` // dbgen.Law
}

// DecreeCouncilJoinedPayload is for EventDecreeCouncilJoined.
type DecreeCouncilJoinedPayload struct {
	PlanID      int64 `json:"plan_id"`
	PlayerID    int64 `json:"player_id"`
	SignatoryID int64 `json:"signatory_id"`
}

// LiaisePhaseChangedPayload is for EventLiaisePhaseChanged.
type LiaisePhaseChangedPayload struct {
	PlanID int64  `json:"plan_id"`
	Phase  string `json:"phase"`
}

// LiaiseChoicesRevealedPayload is for EventLiaiseChoicesRevealed.
type LiaiseChoicesRevealedPayload struct {
	PlanID  int64 `json:"plan_id"`
	Choices any   `json:"choices"` // []dbgen.LiaiseChoice
}

// RevealSubmittedPayload is for EventRevealSubmitted.
// The face is NOT included until all participants have submitted.
type RevealSubmittedPayload struct {
	RevealID int64 `json:"reveal_id"`
	PlayerID int64 `json:"player_id"`
}

// RevealCompletePayload is for EventRevealComplete.
// Faces are revealed once the reveal is complete.
type RevealCompletePayload struct {
	RevealID    int64               `json:"reveal_id"`
	Entries     []RevealEntryResult `json:"entries"`
	ResultDelay int16               `json:"result_delay"`
}

// RevealEntryResult holds one participant's revealed face.
type RevealEntryResult struct {
	PlayerID int64 `json:"player_id"`
	Face     int16 `json:"face"`
}

// ── Phase 3d payload types — Propose Duel ────────────────────────────────────

// DuelChampionElectedPayload is for EventDuelChampionElected.
type DuelChampionElectedPayload struct {
	PlanID   int64 `json:"plan_id"`
	PlayerID int64 `json:"player_id"`
	AssetID  int64 `json:"asset_id"`
}

// DuelStakesRevealedPayload is for EventDuelStakesRevealed.
type DuelStakesRevealedPayload struct {
	PlanID             int64 `json:"plan_id"`
	PreparerStakeCount int16 `json:"preparer_stake_count"`
	TargetStakeCount   int16 `json:"target_stake_count"`
}

// DuelBoutResolvedPayload is for EventDuelBoutResolved.
type DuelBoutResolvedPayload struct {
	PlanID       int64 `json:"plan_id"`
	BoutNumber   int16 `json:"bout_number"`
	DeclarerID   int64 `json:"declarer_id"`
	ResponderID  int64 `json:"responder_id"`
	DeclarerDie  int16 `json:"declarer_die"`
	ResponderDie int16 `json:"responder_die"`
	WinnerID     int64 `json:"winner_id,omitempty"` // 0 if match
	IsMatch      bool  `json:"is_match"`
}

// DuelBoutsCompletePayload is for EventDuelBoutsComplete.
type DuelBoutsCompletePayload struct {
	PlanID       int64   `json:"plan_id"`
	PreparerDice []int16 `json:"preparer_dice"`
	OpponentDice []int16 `json:"opponent_dice"`
	RollID       int64   `json:"roll_id"`
}
