/**
 * Resolution-data shape for Seek Answers, nested under
 * resolution_data.seek_answers. Mirrors game.SeekAnswersResolutionData
 * (uneasy/game/plan_seek_answers_data.go).
 */
export interface SeekAnswersResolutionData {
	/**
	 * Gates the pre-roll narration step. False (or absent) until the preparer
	 * restates their methods and describes one thing they've learned via
	 * cast-roll, which then creates the dice roll. Mirrors Chronicle Histories'
	 * invoke_phase_closed.
	 */
	pre_roll_done?: boolean;
	/**
	 * Resource asset ids flawed during this resolution. Each resource may be
	 * flawed at most once ("a resource asset that has been overlooked until
	 * now"); the break picker filters these out.
	 */
	flawed_resource_ids?: number[];
	/**
	 * Mar penalty: the number of the preparer's own resources that must be
	 * flawed, capped at how many eligible resources they own. 0 on a make.
	 */
	mar_self_flaws_required?: number;
	/** Mar-penalty self-flaws performed so far. */
	mar_self_flaws_applied?: number;
	/**
	 * Completed make-list sub-flow steps (server-authoritative). The picker shows
	 * (picked − done) remaining, so a refresh doesn't re-prompt a finished step.
	 */
	break_resource_done?: number;
	reveal_secret_done?: number;
	declare_truth_done?: number;
	ask_question_done?: number;
	/** Open ask-question awaiting the target's answer or veto. */
	pending_question?: SeekAnswersQuestion | null;
	/** The in-progress ask-question pick already spent its one veto. */
	current_ask_vetoed?: boolean;

	/**
	 * Specifics of each completed make-list step, recorded so the panel can show
	 * the choice that was made (read-only, to every viewer) instead of collapsing
	 * to a count once a picker commits — the Tier-1 committed-state pattern
	 * (ADR-006). Only public facts: reveal stores the asset id (existence is
	 * public), never secret content. Breaks come from flawed_resource_ids.
	 */
	revealed_asset_ids?: number[];
	/** Each truth the preparer declared this resolution, in order. */
	declared_truths?: string[];
	/** Each resolved ask-question: target, question, and the answer given. */
	answered_questions?: SeekAnswersAnsweredQuestion[];
}

/** A completed ask-question: the question put to a target and their answer. */
export interface SeekAnswersAnsweredQuestion {
	target_id: number;
	question: string;
	answer: string;
}

/** An open "ask a player a question" awaiting the target's response. */
export interface SeekAnswersQuestion {
	target_id: number;
	question: string;
	/** True while the target (who outranks the preparer on knowledge) may veto. */
	vetoable: boolean;
}
