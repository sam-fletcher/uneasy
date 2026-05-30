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
}
