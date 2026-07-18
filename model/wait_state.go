package model

// WaitStateKind names the specific sub-state ComputeWaitState found within a
// game's current phase. For main_event it mirrors the underlying
// RowStateKind verbatim (RowState is already authoritative there); the other
// phases get their own small vocabulary since they have no row-state concept
// of their own.
type WaitStateKind string

const (
	// WaitKindNobody — no one is currently blocking play: fewer than two
	// players have joined the lobby, or the game has ended.
	WaitKindNobody WaitStateKind = "nobody"

	// WaitKindLobbyFacilitator — the lobby has its minimum two players; the
	// facilitator is the one who must start the game.
	WaitKindLobbyFacilitator WaitStateKind = "lobby_facilitator"

	// WaitKindPrologueChoosing — the choosing sub-phase (before any ranking
	// step is set): the on-turn player must claim a box.
	WaitKindPrologueChoosing WaitStateKind = "prologue_choosing"

	// WaitKindPrologueDeclare — a declare_X ranking step: players who
	// haven't yet marked themselves done spending hearts on that track.
	WaitKindPrologueDeclare WaitStateKind = "prologue_declare"

	// WaitKindProloguePlaceSetAsides — a place_set_asides_X ranking step:
	// the track's top-ranked real player must place the set-aside players.
	WaitKindProloguePlaceSetAsides WaitStateKind = "prologue_place_set_asides"

	// WaitKindPrologueClosing — the closing step (all player counts):
	// players who haven't yet marked themselves ready.
	WaitKindPrologueClosing WaitStateKind = "prologue_closing"

	// WaitKindShakeUpRolling — Shake-Up step 1: the current roller.
	WaitKindShakeUpRolling WaitStateKind = "shake_up_rolling"

	// WaitKindShakeUpSpending — Shake-Up step 2: pending reactors, the
	// spender awaiting commit, or whoever's turn it is to announce.
	WaitKindShakeUpSpending WaitStateKind = "shake_up_spending"
)

// WaitState is ComputeWaitState's result: the single server-side answer to
// "who must act right now", across every phase (lobby, prologue, main_event,
// shake_up, ended). Session 3's notification ticker reads ActingPlayerIDs to
// drive per-player reminder timers; main_event delegates entirely to
// ComputeRowState, which stays the authoritative source for that phase's
// richer per-kind fields (PlanID, SceneID, etc.) — those aren't duplicated
// here.
type WaitState struct {
	Phase           GamePhase     `json:"phase"`
	Kind            WaitStateKind `json:"kind"`
	ActingPlayerIDs []int64       `json:"acting_player_ids,omitempty"`
}
