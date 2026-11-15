// Seek Answers: say what was learned, roll, pick options equal to the result, then
// carry the picks out. The picks are open all at once (the panel draws every kind
// still owed), so one kind owed is named and several are "the picks".
//
// A question in flight is the target's to answer (the server's
// `await_question_answer`). That one kind can hold two different asks: the target's
// answer, and, while the actor still has picks of their own, the actor's too, since the
// server names both. The acting set says which, so the label names what the two share
// when it holds both, and a heading never tells the actor to answer a question that
// someone else was asked.

import { choicesCommitted, parseResolutionData } from '$lib/components/plans/shared';
import { countOf, oneOf, type StepLabelFn } from '../stepLabel';

export const seekAnswersStepLabel: StepLabelFn = ({ plan, roll, acting }) => {
	const rd = parseResolutionData(plan);
	const sa = rd.seek_answers ?? {};
	const picks = (rd.make_mar_choices ?? []).map(c => c.option);
	const committed = choicesCommitted(rd);

	const question = sa.pending_question;
	if (question != null) {
		return acting.some(id => id !== question.target_id) ? 'the question and the picks' : 'answer the question';
	}

	if (!(sa.pre_roll_done ?? false) && roll == null && !committed) return 'describe what was learned';
	if (!committed) return 'choose the options';

	// What is still owed, in the order the panel draws it.
	const owed: string[] = [];
	if (countOf(picks, 'break_resource') > (sa.break_resource_done ?? 0)) owed.push('break a resource');
	if (countOf(picks, 'reveal_secret') > (sa.reveal_secret_done ?? 0)) owed.push("learn an asset's secrets");
	if (countOf(picks, 'declare_truth') > (sa.declare_truth_done ?? 0)) owed.push('declare a truth');
	if (countOf(picks, 'ask_question') > (sa.ask_question_done ?? 0)) owed.push('ask a question');
	if ((sa.mar_self_flaws_required ?? 0) > (sa.mar_self_flaws_applied ?? 0)) owed.push('pay the mar penalty');
	return oneOf(owed, 'carry out the picks', 'complete the plan');
};
