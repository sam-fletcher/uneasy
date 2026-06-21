package game

// plan_seek_answers_data.go — typed resolution_data for Seek Answers.

// SeekAnswersResolutionData holds Seek Answers plan state stored inside the
// plans.resolution_data JSON column, nested under the "seek_answers" key.
type SeekAnswersResolutionData struct {
	// PreRollDone gates the pre-roll narration step. OnResolve opens the plan in
	// 'resolving' with PreRollDone=false and no roll; the preparer restates their
	// methods and describes one thing they've learned via the cast-roll route,
	// which posts that narration, flips this true, and creates the dice roll.
	// Mirrors Chronicle Histories' invoke_phase_closed.
	PreRollDone bool `json:"pre_roll_done,omitempty"`

	// FlawedResourceIDs records every resource asset flawed during this
	// resolution. Each resource may be flawed at most once — the option is
	// "describe a flaw in any resource asset that has been overlooked until
	// now; break that asset" — so the break-resource route rejects any asset
	// already in this list. Covers both make-list breaks and mar-penalty
	// self-flaws.
	FlawedResourceIDs []int64 `json:"flawed_resource_ids,omitempty"`

	// MarSelfFlawsRequired is the number of the preparer's own resources that
	// must be flawed as the mar penalty. Set once in ApplyChoice on a mar to
	// min(difficulty − result, # of the preparer's eligible own resources at
	// resolution time). 0 on a make. The cap is stable because resources can
	// only gain marginalia mid-resolution, never spawn anew.
	MarSelfFlawsRequired int16 `json:"mar_self_flaws_required,omitempty"`

	// MarSelfFlawsApplied counts mar-penalty self-flaws performed so far (a
	// break-resource call on a preparer-owned resource after a mar). The plan
	// cannot complete until this reaches MarSelfFlawsRequired.
	MarSelfFlawsApplied int16 `json:"mar_self_flaws_applied,omitempty"`

	// BreakResourceDone / RevealSecretDone / DeclareTruthDone / AskQuestionDone
	// count completed make-list sub-flow steps. They are the server-authoritative
	// completion signal (the panel shows picked − done remaining), and the
	// handlers reject any step beyond the picked count — so a stale client
	// re-prompted after a refresh can't run extra steps. Mar-penalty self-flaws
	// are tracked by MarSelfFlawsApplied instead, not here.
	BreakResourceDone int16 `json:"break_resource_done,omitempty"`
	RevealSecretDone  int16 `json:"reveal_secret_done,omitempty"`
	DeclareTruthDone  int16 `json:"declare_truth_done,omitempty"`
	AskQuestionDone   int16 `json:"ask_question_done,omitempty"`

	// RevealedAssetIDs / DeclaredTruths / AnsweredQuestions record the specifics
	// of each completed make-list step, so the panel can show the choice that was
	// made (read-only, to every viewer) instead of collapsing to a count once a
	// picker commits. This is the Tier-1 committed-state pattern (see ADR-006).
	// Only public facts live here — resolution_data is exposed to all players, so
	// reveal records the asset ID (existence is public), never any secret content.
	// FlawedResourceIDs already covers break-resource and mar-penalty steps.
	RevealedAssetIDs  []int64                       `json:"revealed_asset_ids,omitempty"`
	DeclaredTruths    []string                      `json:"declared_truths,omitempty"`
	AnsweredQuestions []SeekAnswersAnsweredQuestion `json:"answered_questions,omitempty"`

	// PendingQuestion is the open ask-question awaiting the target's answer or
	// veto. While set, no new question may be asked and the plan can't complete.
	PendingQuestion *SeekAnswersQuestion `json:"pending_question,omitempty"`
	// CurrentAskVetoed marks that the in-progress ask-question pick already spent
	// its single veto, so the re-asked question ("ask another in its stead")
	// can't be vetoed again. Cleared once that pick is answered.
	CurrentAskVetoed bool `json:"current_ask_vetoed,omitempty"`
}

// SeekAnswersQuestion is an open "ask a player a question" awaiting the target's
// response. Stored in SeekAnswersResolutionData.PendingQuestion.
type SeekAnswersQuestion struct {
	// TargetID is the player who must answer (or veto).
	TargetID int64 `json:"target_id"`
	// Question is the preparer's question text.
	Question string `json:"question"`
	// Vetoable is true while the target — who outranks the preparer on the
	// knowledge track — may veto this formulation ("can veto the first
	// question"). It is false on a re-ask, so the replacement question stands.
	Vetoable bool `json:"vetoable"`
}

// SeekAnswersAnsweredQuestion is a completed ask-question: the question the
// preparer put to a target and the truthful answer they gave. Recorded for
// Tier-1 read-only display (ADR-006); the chat log holds the same exchange.
type SeekAnswersAnsweredQuestion struct {
	// TargetID is the player who answered.
	TargetID int64 `json:"target_id"`
	// Question is the preparer's question text.
	Question string `json:"question"`
	// Answer is the target's truthful answer.
	Answer string `json:"answer"`
}

// EnsureSeekAnswers returns r.SeekAnswers, allocating a zero struct if it was
// nil.
func (r *ResolutionData) EnsureSeekAnswers() *SeekAnswersResolutionData {
	if r.SeekAnswers == nil {
		r.SeekAnswers = &SeekAnswersResolutionData{}
	}
	return r.SeekAnswers
}
