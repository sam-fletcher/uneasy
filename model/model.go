// Package model contains the core data types shared across the application.
// These mirror the database schema across all applied migrations.
package model

import "time"

// ── Identity ─────────────────────────────────────────────────────────────────

// UserToken represents a browser identity before (and independent of) any
// game membership. One row per cookie — the "who are you" layer.
type UserToken struct {
	Token       string    `json:"token"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// ── Game ─────────────────────────────────────────────────────────────────────

// GamePhase enumerates the phases a game can be in.
type GamePhase string

const (
	PhaseLobby      GamePhase = "lobby"
	PhaseToneSetting GamePhase = "tone_setting"
	PhasePrologue   GamePhase = "prologue"
	PhaseMainEvent  GamePhase = "main_event"
	PhaseShakeUp    GamePhase = "shake_up"
	PhaseEnded      GamePhase = "ended"
)

// Game represents a single play session ("table").
type Game struct {
	ID             int64     `json:"id"`
	JoinCode       string    `json:"join_code"`
	CreatedAt      time.Time `json:"created_at"`
	FacilitatorID  *int64    `json:"facilitator_id"`
	Phase          GamePhase `json:"phase"`
	CurrentRow     int16     `json:"current_row"`
	FocusPlayerID  *int64    `json:"focus_player_id"`
	EndingMode     *string   `json:"ending_mode"`
	DummyTokenMode string    `json:"dummy_token_mode"`
}

// ── Player ───────────────────────────────────────────────────────────────────

// Player represents a person's seat at a specific game table.
type Player struct {
	ID            int64     `json:"id"`
	GameID        int64     `json:"game_id"`
	DisplayName   string    `json:"display_name"`
	CookieToken   string    `json:"-"` // never send the raw token to the client
	JoinedAt      time.Time `json:"joined_at"`
	IsFacilitator bool      `json:"is_facilitator"`
	TokenColor    *string   `json:"token_color"`
	SeatOrder     *int16    `json:"seat_order"`
}

// ── Tone Setting ─────────────────────────────────────────────────────────────

// ToneTopicStatus enumerates the status options for a tone topic.
type ToneTopicStatus string

const (
	ToneDefault     ToneTopicStatus = "default"
	ToneInclude     ToneTopicStatus = "include"
	ToneAvoidDetail ToneTopicStatus = "avoid_detail"
	ToneNever       ToneTopicStatus = "never"
)

// ToneTopic is one entry in the tone-setting exercise.
type ToneTopic struct {
	ID     int64           `json:"id"`
	GameID int64           `json:"game_id"`
	Topic  string          `json:"topic"`
	Status ToneTopicStatus `json:"status"`
}

// ── Rankings ─────────────────────────────────────────────────────────────────

// RankingCategory enumerates the three ranking tracks.
type RankingCategory string

const (
	CategoryPower     RankingCategory = "power"
	CategoryKnowledge RankingCategory = "knowledge"
	CategoryEsteem    RankingCategory = "esteem"
)

// Ranking is one player's position on one ranking track.
type Ranking struct {
	ID       int64           `json:"id"`
	GameID   int64           `json:"game_id"`
	PlayerID *int64          `json:"player_id"` // nil = dummy token
	Category RankingCategory `json:"category"`
	Rank     int16           `json:"rank"`
}

// ── Assets ───────────────────────────────────────────────────────────────────

// AssetType enumerates the four asset types.
type AssetType string

const (
	AssetPeer     AssetType = "peer"
	AssetHolding  AssetType = "holding"
	AssetArtifact AssetType = "artifact"
	AssetResource AssetType = "resource"
)

// Asset is a notecard in a player's retinue.
type Asset struct {
	ID              int64     `json:"id"`
	GameID          int64     `json:"game_id"`
	OwnerID         int64     `json:"owner_id"`
	CreatorID       int64     `json:"creator_id"`
	AssetType       AssetType `json:"asset_type"`
	Name            string    `json:"name"`
	IsMainCharacter bool      `json:"is_main_character"`
	IsLeveraged     bool      `json:"is_leveraged"`
	IsDestroyed     bool      `json:"is_destroyed"`
	CreatedAt       time.Time `json:"created_at"`
	DestroyedAt     *time.Time `json:"destroyed_at"`
}

// Marginalia is a descriptive phrase on one of the four margins of an asset.
type Marginalia struct {
	ID       int64      `json:"id"`
	AssetID  int64      `json:"asset_id"`
	Position int16      `json:"position"`
	Text     string     `json:"text"`
	IsTorn   bool       `json:"is_torn"`
	TornAt   *time.Time `json:"torn_at"`
	TornByID *int64     `json:"torn_by_id"`
}

// Secret is text written on the underside of an asset.
type Secret struct {
	ID         int64      `json:"id"`
	AssetID    int64      `json:"asset_id"`
	AuthorID   int64      `json:"author_id"`
	Text       string     `json:"text"`
	IsRevealed bool       `json:"is_revealed"`
	RevealedAt *time.Time `json:"revealed_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ── Public Record ────────────────────────────────────────────────────────────

// ScenePost is a message in a scene thread on the public record.
type ScenePost struct {
	ID        int64     `json:"id"`
	GameID    int64     `json:"game_id"`
	RowNumber *int16    `json:"row_number"`
	PlanID    *int64    `json:"plan_id"`
	AuthorID  int64     `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// SceneEntry is a one-line summary on the public record for a scene.
type SceneEntry struct {
	ID        int64     `json:"id"`
	GameID    int64     `json:"game_id"`
	RowNumber int16     `json:"row_number"`
	AuthorID  int64     `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ── Plans ────────────────────────────────────────────────────────────────────

// PlanType enumerates the 12 plan types.
type PlanType string

const (
	PlanMakeDemands         PlanType = "make_demands"
	PlanProposeDecree       PlanType = "propose_decree"
	PlanExchangeCourtiers   PlanType = "exchange_courtiers"
	PlanMakeWar             PlanType = "make_war"
	PlanMakeIntroductions   PlanType = "make_introductions"
	PlanSeekAnswers         PlanType = "seek_answers"
	PlanChronicleHistories  PlanType = "chronicle_histories"
	PlanClandestinelyLiaise PlanType = "clandestinely_liaise"
	PlanSpreadPropaganda    PlanType = "spread_propaganda"
	PlanSpreadRumors        PlanType = "spread_rumors"
	PlanProposeDuel         PlanType = "propose_duel"
	PlanHostFestivity       PlanType = "host_festivity"
)

// PlanStatus enumerates the states of a plan.
type PlanStatus string

const (
	PlanPending   PlanStatus = "pending"
	PlanResolving PlanStatus = "resolving"
	PlanResolved  PlanStatus = "resolved"
	PlanCancelled PlanStatus = "cancelled"
)

// Plan represents a prepared plan on the public record.
type Plan struct {
	ID               int64      `json:"id"`
	GameID           int64      `json:"game_id"`
	PlanType         PlanType   `json:"plan_type"`
	Category         string     `json:"category"`
	PreparerID       int64      `json:"preparer_id"`
	TargetPlayerID   *int64     `json:"target_player_id"`
	TargetAssetID    *int64     `json:"target_asset_id"`
	RowNumber        int16      `json:"row_number"`
	RowOrder         int16      `json:"row_order"`
	PreparedAtRow    int16      `json:"prepared_at_row"`
	Status           PlanStatus `json:"status"`
	Result           *string    `json:"result"`
	ResolvedAt       *time.Time `json:"resolved_at"`
	PreparationNotes *string    `json:"preparation_notes"`
}

// PlanToken represents a player's token on a plan shield.
type PlanToken struct {
	ID       int64     `json:"id"`
	GameID   int64     `json:"game_id"`
	PlanType string    `json:"plan_type"`
	PlayerID int64     `json:"player_id"`
	PlanID   int64     `json:"plan_id"`
	PlacedAt time.Time `json:"placed_at"`
}

// ── Dice Rolls ───────────────────────────────────────────────────────────────

// DiceRoll represents a single dice roll event.
type DiceRoll struct {
	ID                 int64      `json:"id"`
	GameID             int64      `json:"game_id"`
	PlanID             *int64     `json:"plan_id"`
	RowNumber          *int16     `json:"row_number"`
	IsShakeUp          bool       `json:"is_shake_up"`
	ActorID            int64      `json:"actor_id"`
	Difficulty         int16      `json:"difficulty"`
	AdjustedDifficulty *int16     `json:"adjusted_difficulty"`
	Result             *int16     `json:"result"`
	Outcome            *string    `json:"outcome"`
	CreatedAt          time.Time  `json:"created_at"`
	ResolvedAt         *time.Time `json:"resolved_at"`
}

// DiceRollDie is one die in a roll's pool.
type DiceRollDie struct {
	ID               int64  `json:"id"`
	RollID           int64  `json:"roll_id"`
	PlayerID         int64  `json:"player_id"`
	IsInterference   bool   `json:"is_interference"`
	LeveragedAssetID *int64 `json:"leveraged_asset_id"`
	Face             *int16 `json:"face"`
	IsCancelled      bool   `json:"is_cancelled"`
}

// DifficultyVote is a thumbs-up/down on a roll's difficulty.
type DifficultyVote struct {
	RollID   int64     `json:"roll_id"`
	PlayerID int64     `json:"player_id"`
	Vote     string    `json:"vote"`
	VotedAt  time.Time `json:"voted_at"`
}

// ── Laws & Rumors ────────────────────────────────────────────────────────────

// Law is a law placed under the public record.
type Law struct {
	ID            int64     `json:"id"`
	GameID        int64     `json:"game_id"`
	Text          string    `json:"text"`
	Addendum      *string   `json:"addendum"`
	OriginPlanID  *int64    `json:"origin_plan_id"`
	SignatoryID   *int64    `json:"signatory_id"`
	CreatedAt     time.Time `json:"created_at"`
	IsActive      bool      `json:"is_active"`
	DisplayOrder  int16     `json:"display_order"`
}

// Rumor is a rumor placed under the public record.
type Rumor struct {
	ID             int64     `json:"id"`
	GameID         int64     `json:"game_id"`
	Text           string    `json:"text"`
	TargetAssetID  *int64    `json:"target_asset_id"`
	OriginPlanID   *int64    `json:"origin_plan_id"`
	SourcePlayerID *int64    `json:"source_player_id"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	DisplayOrder   int16     `json:"display_order"`
}

// ── Deprecated (Phase 1 — will be removed once scene_posts fully replace it) ──

// Post is a flat message in a table's feed (Phase 1 only).
// Kept temporarily for compilation; the DB table is dropped in migration 006.
type Post struct {
	ID        int64     `json:"id"`
	GameID    int64     `json:"game_id"`
	AuthorID  int64     `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
