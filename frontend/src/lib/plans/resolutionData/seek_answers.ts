/**
 * Resolution-data shape for Seek Answers, nested under
 * resolution_data.seek_answers. Mirrors game.SeekAnswersResolutionData
 * (uneasy/game/plan_seek_answers_data.go).
 */
export interface SeekAnswersResolutionData {
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
}

/** An open "ask a player a question" awaiting the target's response. */
export interface SeekAnswersQuestion {
	target_id: number;
	question: string;
	/** True while the target (who outranks the preparer on knowledge) may veto. */
	vetoable: boolean;
}
