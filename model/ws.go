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

	// Lobby
	EventPlayerJoined = "player.joined"

	// Phase 2: Game state
	EventPhaseChanged     = "phase.changed"
	EventFocusChanged     = "focus.changed"
	EventRowAdvanced      = "row.advanced"
	EventSceneEnded       = "scene.ended"
	EventSceneStarted     = "scene.started"
	EventScenePeerClaimed = "scene.peer_claimed"
	// EventRowStateChanged is broadcast whenever the row's authoritative
	// state (RowState — the rulebook's step or a pre-step gate) transitions.
	// Carries the new RowState so clients render off the server's verdict
	// instead of inferring it from other events. See model/row_state.go.
	EventRowStateChanged = "row_state.changed"

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
	EventRumorUpdated          = "rumor.updated"           // rumor text edited
	EventSPRecursivePlan       = "plan.sp_recursive"       // SP: recursive propaganda created
	EventSecretVisibilityGrant = "secret.visibility_grant" // SA/CL: visibility granted
	EventSecretCreated         = "secret.created"          // any player wrote a secret

	// Phase 3c: Propose Decree
	EventLawEnacted          = "law.enacted"           // PD: law created
	EventLawUpdated          = "law.updated"           // law text/addendum edited
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

	// Phase 3d: Make War
	EventWarDeclared        = "war.declared"
	EventWarPlayerJoined    = "war.player_joined"
	EventWarBattleCostDue   = "war.battle_cost_due"
	EventWarBattleCostPaid  = "war.battle_cost_paid"
	EventWarPlayerSurrender = "war.player_surrendered"
	EventWarAssetSeized     = "war.asset_seized"
	EventWarEntryCompleted  = "war.entry_completed"
	EventWarPeaceProposed   = "war.peace_proposed"
	EventWarPeaceVote       = "war.peace_vote"
	EventWarEnded           = "war.ended"

	// Phase 3d: Host Festivity
	EventFestivityGuestJoined       = "festivity.guest_joined"
	EventFestivityGuestRolled       = "festivity.guest_rolled"
	EventFestivityGuestChose        = "festivity.guest_chose"
	EventFestivityHostChose         = "festivity.host_chose"
	EventFestivityInsistHostMar     = "festivity.insist_host_mar"
	EventFestivityDuelTriggered     = "festivity.duel_triggered"
	EventFestivityPhaseChanged      = "festivity.phase_changed"
	EventFestivityChallengeIssued   = "festivity.challenge_issued"
	EventFestivityChallengeDeclined = "festivity.challenge_declined"

	// Phase 4c: Shake-Up
	EventShakeUpStepChanged    = "shake_up.step_changed"
	EventShakeUpRolled         = "shake_up.rolled"
	EventShakeUpSpendOpened    = "shake_up.spend_opened"
	EventShakeUpAdjusted       = "shake_up.adjusted"
	EventShakeUpSpendCommitted = "shake_up.spend_committed"
	EventShakeUpEnded          = "shake_up.ended"

	// Phase 4d: Endgame mode selection
	EventEndgameModeSet = "endgame.mode_set" // facilitator picked smooth_landing / explosive_finale

	// Ephemeral scene-setup draft: focus player's in-flight selections,
	// fanned out so non-focus players see what's being filled in. Not
	// persisted; late joiners simply wait for the next keystroke.
	EventSceneSetupDraft = "scene_setup.draft"

	// Phase 4b: Structured prologue
	EventPrologueChoiceClaimed      = "prologue.choice_claimed"           // a box was claimed
	EventPrologueTurnAdvanced       = "prologue.turn_advanced"            // active player changed
	EventPrologueRankingStepChanged = "prologue.ranking_step_changed"     // entered/advanced ranking sub-flow
	EventPrologueHeartsDeclared     = "prologue.hearts_declared"          // legacy: count-based heart declaration
	EventPrologueTrackRanked        = "prologue.track_ranked"             // a track's ranks finalized (set-asides surfaced)
	EventPrologueSetAsidesPlaced    = "prologue.set_asides_placed"        // rank-1 player placed set-aside players
	EventPrologueCommittedHeartsChg = "prologue.committed_hearts_changed" // a player adjusted their committed hearts on a track
	EventPrologueDoneChanged        = "prologue.done_changed"             // a player toggled their per-track Done flag
	EventPrologueExtraPeerCreated   = "prologue.extra_peer_created"       // a player claimed their extra peer (≤3-player games)

	// Phase 2: Dice rolls
	EventRollCreated       = "roll.created"
	EventRollLeverageAdded = "roll.leverage_added"
	EventRollVoteCast      = "roll.vote_cast"
	EventRollVoteResolved  = "roll.vote_resolved"
	EventRollResolved      = "roll.resolved"
	EventRollStageChanged  = "roll.stage_changed"
	EventRollIntentSet     = "roll.intent_set"
	EventRollReadyChanged  = "roll.ready_changed"
)

// ── Command types (client → server) ──────────────────────────────────────────

const (
	CmdTypingStart     = "typing.start"
	CmdTypingStop      = "typing.stop"
	CmdSceneSetupDraft = "scene_setup.draft"
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

// PlayerJoinedPayload is the payload for EventPlayerJoined.
type PlayerJoinedPayload struct {
	Player any `json:"player"` // dbgen.Player
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
	SceneID   int64 `json:"scene_id,omitempty"`
}

// SceneStartedPayload is the payload for EventSceneStarted.
type SceneStartedPayload struct {
	Scene any `json:"scene"` // dbgen.Scene
	Peers any `json:"peers"` // []scenePeerView from handler
}

// ScenePeerClaimedPayload is the payload for EventScenePeerClaimed.
type ScenePeerClaimedPayload struct {
	SceneID      int64 `json:"scene_id"`
	PeerAssetID  int64 `json:"peer_asset_id"`
	ControllerID int64 `json:"controller_id"`
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

// RollVoteCastPayload is for individual vote events. The vote value is
// intentionally omitted to preserve the hidden ballot — clients only learn
// that the player has voted, not which way.
type RollVoteCastPayload struct {
	RollID   int64 `json:"roll_id"`
	PlayerID int64 `json:"player_id"`
}

// RollVoteResolvedPayload is for when all votes are in. Ballot is the full
// per-player ±1 reveal that lands simultaneously.
type RollVoteResolvedPayload struct {
	RollID             int64 `json:"roll_id"`
	AdjustedDifficulty int16 `json:"adjusted_difficulty"`
	Ballot             any   `json:"ballot"` // []voteView from handler
}

// RollStageChangedPayload announces a server-driven stage transition.
type RollStageChangedPayload struct {
	RollID int64  `json:"roll_id"`
	Stage  string `json:"stage"`
}

// RollIntentSetPayload announces a participant's intent pick.
type RollIntentSetPayload struct {
	RollID   int64  `json:"roll_id"`
	PlayerID int64  `json:"player_id"`
	Intent   string `json:"intent"`
}

// RollReadyChangedPayload announces a participant's ready-flag flip. Forced
// is true when the server flipped the flag (auto-ready sweep, auto-unready
// sweep) rather than the player choosing.
type RollReadyChangedPayload struct {
	RollID   int64  `json:"roll_id"`
	PlayerID int64  `json:"player_id"`
	IsReady  bool   `json:"is_ready"`
	Forced   bool   `json:"forced,omitempty"`
	Reason   string `json:"reason,omitempty"`
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

// SecretCreatedPayload is for EventSecretCreated. Intentionally omits the
// secret text so listeners without visibility don't leak. Clients with
// visibility can re-fetch.
type SecretCreatedPayload struct {
	AssetID  int64 `json:"asset_id"`
	AuthorID int64 `json:"author_id"`
}

// ── Phase 3c payload types ────────────────────────────────────────────────────

// LawEnactedPayload is for EventLawEnacted (Propose Decree).
type LawEnactedPayload struct {
	PlanID int64 `json:"plan_id"`
	Law    any   `json:"law"` // dbgen.Law
}

// LawUpdatedPayload is for EventLawUpdated (author edits).
type LawUpdatedPayload struct {
	Law any `json:"law"` // dbgen.Law
}

// RumorUpdatedPayload is for EventRumorUpdated (author edits).
type RumorUpdatedPayload struct {
	Rumor any `json:"rumor"` // dbgen.Rumor
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

// ── Phase 3d payload types — Host Festivity ──────────────────────────────────

// FestivityGuestJoinedPayload is for EventFestivityGuestJoined.
type FestivityGuestJoinedPayload struct {
	PlanID   int64 `json:"plan_id"`
	PlayerID int64 `json:"player_id"`
}

// FestivityGuestRolledPayload is for EventFestivityGuestRolled.
// Action is "roll" or "opt_out". RollID is 0 when opting out.
type FestivityGuestRolledPayload struct {
	PlanID   int64  `json:"plan_id"`
	PlayerID int64  `json:"player_id"`
	Action   string `json:"action"`
	RollID   int64  `json:"roll_id,omitempty"`
}

// FestivityGuestChosePayload is for EventFestivityGuestChose.
type FestivityGuestChosePayload struct {
	PlanID   int64  `json:"plan_id"`
	PlayerID int64  `json:"player_id"`
	Outcome  string `json:"outcome"`
	Choice   string `json:"choice"`
}

// FestivityHostChosePayload is for EventFestivityHostChose.
type FestivityHostChosePayload struct {
	PlanID        int64  `json:"plan_id"`
	GuestPlayerID int64  `json:"guest_player_id"`
	Choice        string `json:"choice"`
}

// FestivityInsistHostMarPayload is for EventFestivityInsistHostMar.
type FestivityInsistHostMarPayload struct {
	PlanID     int64  `json:"plan_id"`
	InsisterID int64  `json:"insister_id"`
	MarOption  string `json:"mar_option"`
}

// FestivityDuelTriggeredPayload is for EventFestivityDuelTriggered.
type FestivityDuelTriggeredPayload struct {
	PlanID     int64 `json:"plan_id"`
	DuelPlanID int64 `json:"duel_plan_id"`
}

// FestivityPhaseChangedPayload is for EventFestivityPhaseChanged.
type FestivityPhaseChangedPayload struct {
	PlanID int64  `json:"plan_id"`
	Phase  string `json:"phase"`
}

// FestivityChallengeIssuedPayload is for EventFestivityChallengeIssued.
type FestivityChallengeIssuedPayload struct {
	PlanID       int64 `json:"plan_id"`
	ChallengerID int64 `json:"challenger_id"`
	TargetID     int64 `json:"target_id"`
	// MustAccept is true if the target has the accept_duels mar, meaning the
	// decline option should be disabled in the UI.
	MustAccept bool `json:"must_accept"`
}

// FestivityChallengeDeclinedPayload is for EventFestivityChallengeDeclined.
type FestivityChallengeDeclinedPayload struct {
	PlanID       int64 `json:"plan_id"`
	ChallengerID int64 `json:"challenger_id"`
	TargetID     int64 `json:"target_id"`
}

// ── Phase 3d payload types — Make War ────────────────────────────────────────

// WarParticipantInfo is one (player, side) pair for war events.
type WarParticipantInfo struct {
	PlayerID int64 `json:"player_id"`
	Side     int16 `json:"side"`
}

// WarDeclaredPayload is broadcast when the delay reveal completes and the
// war's plan row is placed on the public record.
type WarDeclaredPayload struct {
	PlanID       int64                `json:"plan_id"`
	WarID        int64                `json:"war_id"`
	Participants []WarParticipantInfo `json:"participants"`
	TargetRow    int16                `json:"target_row"`
}

// WarPlayerJoinedPayload is for EventWarPlayerJoined.
type WarPlayerJoinedPayload struct {
	WarID    int64 `json:"war_id"`
	PlayerID int64 `json:"player_id"`
	Side     int16 `json:"side"`
}

// WarBattleCostDuePayload is for EventWarBattleCostDue (broadcast when a new
// row begins while a war is active, listing who still owes cost payments).
type WarBattleCostDuePayload struct {
	WarID     int64              `json:"war_id"`
	RowNumber int16              `json:"row_number"`
	Payers    []CostOwedByPlayer `json:"payers"`
}

// CostOwedByPlayer lists one player's outstanding costs for the current row.
type CostOwedByPlayer struct {
	PlayerID    int64   `json:"player_id"`
	OpponentIDs []int64 `json:"opponent_ids"`
}

// WarBattleCostPaidPayload is for EventWarBattleCostPaid.
type WarBattleCostPaidPayload struct {
	WarID       int64  `json:"war_id"`
	RowNumber   int16  `json:"row_number"`
	PayerID     int64  `json:"payer_id"`
	OpponentID  int64  `json:"opponent_id"`
	Choice      string `json:"choice"`
	Surrendered bool   `json:"surrendered"`
}

// WarPlayerSurrenderPayload is for EventWarPlayerSurrender.
type WarPlayerSurrenderPayload struct {
	WarID     int64 `json:"war_id"`
	PlayerID  int64 `json:"player_id"`
	RowNumber int16 `json:"row_number"`
}

// WarAssetSeizedPayload is for EventWarAssetSeized.
type WarAssetSeizedPayload struct {
	WarID         int64 `json:"war_id"`
	SurrenderedID int64 `json:"surrendered_id"`
	ClaimantID    int64 `json:"claimant_id"`
	AssetID       int64 `json:"asset_id"`
}

// WarEntryCompletedPayload is for EventWarEntryCompleted.
type WarEntryCompletedPayload struct {
	WarID    int64 `json:"war_id"`
	PlayerID int64 `json:"player_id"`
	Side     int16 `json:"side"`
}

// WarPeaceProposedPayload is for EventWarPeaceProposed.
type WarPeaceProposedPayload struct {
	WarID      int64  `json:"war_id"`
	ProposalID int64  `json:"proposal_id"`
	ProposerID int64  `json:"proposer_id"`
	Terms      string `json:"terms"`
}

// WarPeaceVotePayload is for EventWarPeaceVote.
type WarPeaceVotePayload struct {
	WarID      int64 `json:"war_id"`
	ProposalID int64 `json:"proposal_id"`
	PlayerID   int64 `json:"player_id"`
	Accepted   bool  `json:"accepted"`
}

// WarEndedPayload is for EventWarEnded.
type WarEndedPayload struct {
	WarID     int64  `json:"war_id"`
	Reason    string `json:"reason"`
	RowNumber int16  `json:"row_number"`
}

// ── Phase 4b payload types — Structured prologue ─────────────────────────────

// PrologueChoiceClaimedPayload is for EventPrologueChoiceClaimed.
type PrologueChoiceClaimedPayload struct {
	PlayerID   int64  `json:"player_id"`
	SheetType  string `json:"sheet_type"`
	ChoiceName string `json:"choice_name"`
	TurnNumber int16  `json:"turn_number"`
}

// PrologueTurnAdvancedPayload is for EventPrologueTurnAdvanced. CurrentPlayerID
// is null when no players remain with turns to take (the choosing sub-phase
// is complete and the facilitator can begin ranking).
type PrologueTurnAdvancedPayload struct {
	CurrentPlayerID *int64 `json:"current_player_id"`
	TurnNumber      int    `json:"turn_number"`
}

// PrologueRankingStepChangedPayload is for EventPrologueRankingStepChanged.
// Step is one of game.PrologueStep* constants, or "" when leaving the
// ranking sub-flow (i.e. transitioning to main_event).
type PrologueRankingStepChangedPayload struct {
	Step string `json:"step"`
}

// PrologueHeartsDeclaredPayload is for EventPrologueHeartsDeclared.
type PrologueHeartsDeclaredPayload struct {
	PlayerID int64  `json:"player_id"`
	Track    string `json:"track"`
	Count    int16  `json:"count"`
}

// PrologueTrackRankedPayload is for EventPrologueTrackRanked. Ranked is the
// rank-1-first ordered player IDs; SetAside is the players the rank-1
// player still needs to slot in.
type PrologueTrackRankedPayload struct {
	Track    string  `json:"track"`
	Ranked   []int64 `json:"ranked"`
	SetAside []int64 `json:"set_aside"`
}

// PrologueSetAsidesPlacedPayload is for EventPrologueSetAsidesPlaced. The
// final ranking with set-asides slotted in.
type PrologueSetAsidesPlacedPayload struct {
	Track  string  `json:"track"`
	Ranked []int64 `json:"ranked"`
}

// PrologueCommittedHeartsChangedPayload is for EventPrologueCommittedHeartsChg.
// CardIDs is the player's full committed-card list for the given track after
// the change (replaces, not deltas).
type PrologueCommittedHeartsChangedPayload struct {
	PlayerID int64   `json:"player_id"`
	Track    string  `json:"track"`
	CardIDs  []int64 `json:"card_ids"`
}

// PrologueDoneChangedPayload is for EventPrologueDoneChanged.
type PrologueDoneChangedPayload struct {
	PlayerID int64  `json:"player_id"`
	Track    string `json:"track"`
	Done     bool   `json:"done"`
}

// PrologueExtraPeerCreatedPayload is for EventPrologueExtraPeerCreated.
type PrologueExtraPeerCreatedPayload struct {
	PlayerID  int64  `json:"player_id"`
	TitleName string `json:"title_name"`
	AssetID   int64  `json:"asset_id"`
}

// EndgameModeSetPayload is for EventEndgameModeSet.
type EndgameModeSetPayload struct {
	Mode string `json:"mode"`
}

// ── Phase 4c payload types — Shake-Up ────────────────────────────────────────

// ShakeUpStepChangedPayload is for EventShakeUpStepChanged.
type ShakeUpStepChangedPayload struct {
	Category string `json:"category"`
	Step     int16  `json:"step"`
}

// ShakeUpRolledPayload is for EventShakeUpRolled.
type ShakeUpRolledPayload struct {
	PlayerID int64 `json:"player_id"`
	Result   int16 `json:"result"`
	Total    int16 `json:"total"`
}

// ShakeUpSpendOpenedPayload is for EventShakeUpSpendOpened.
type ShakeUpSpendOpenedPayload struct {
	Spend any `json:"spend"` // dbgen.ShakeUpSpend
}

// ShakeUpAdjustedPayload is for EventShakeUpAdjusted.
type ShakeUpAdjustedPayload struct {
	SpendID      int64 `json:"spend_id"`
	AdjustmentID int64 `json:"adjustment_id"`
	PlayerID     int64 `json:"player_id"`
	Adjustment   int16 `json:"adjustment"`
}

// ShakeUpSpendCommittedPayload is for EventShakeUpSpendCommitted.
type ShakeUpSpendCommittedPayload struct {
	SpendID   int64 `json:"spend_id"`
	FinalCost int16 `json:"final_cost"`
}

// ShakeUpEndedPayload is for EventShakeUpEnded.
type ShakeUpEndedPayload struct{}

// SceneSetupDraftPayload is for EventSceneSetupDraft (also reused as the
// CmdSceneSetupDraft body). The server stamps PlayerID from the sending
// client; any value provided by the sender is overwritten. Pointer fields
// distinguish "cleared" (null) from "absent" — though in practice the
// focus client sends the full snapshot every change.
type SceneSetupDraftPayload struct {
	PlayerID       int64   `json:"player_id"`
	HoldingID      *int64  `json:"holding_id"`
	CustomLocation string  `json:"custom_location"`
	TimeElapsed    string  `json:"time_elapsed"`
	TimeNote       string  `json:"time_note"`
	PresentPeerIDs []int64 `json:"present_peer_ids"`
}
