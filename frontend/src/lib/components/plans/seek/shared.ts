// shared.ts
// What the Seek Answers steps have in common: the option list, the "what can happen?"
// reference, and the facts about one plan that more than one step reads — what has been
// picked, which sub-steps still owe something, which resources and assets each picker
// offers. Worked out once from the plan context and the plan and handed to each step as
// `scene`, rather than each step repeating the same lookups.
//
// Plain data, no runes: the panel calls it inside a template `{@const}`, so it is
// recomputed whenever the context or the plan changes, as the `$derived`s it replaces
// were.

import { forfeitSeekStep, type Plan } from '$lib/api';
import type { BuffetTab } from '$lib/components/plans/shared/Buffet.svelte';
import { hiddenCount } from '$lib/secretCounts';
import {
	choicesCommitted, parseResolutionData, breakableAssets, resolutionActorID,
	seekAnswersMarPenalty,
} from '../shared';
import type { PlanContext } from '../types';
import type { SeekDrafts } from './drafts.svelte';

export const OPTIONS = [
	{ key: 'break_resource', label: 'Break a resource (describe a flaw)' },
	{ key: 'declare_truth',  label: 'Declare something true about the world' },
	{ key: 'ask_question',   label: 'Ask a player a question (they must be truthful)' },
	{ key: 'reveal_secret',  label: "Learn the secrets of an asset" },
] as const;

// Read-only "what can happen?" reference (shared Buffet), shown until the
// picks are made so the others can weigh them before aiding or interfering.
export const BUFFET_TABS: BuffetTab[] = [
	{
		key: 'make', label: 'Make', intro: 'Pick options equal to the dice result (repeatable):',
		opts: OPTIONS.map(o => ({ key: o.key, label: o.label })),
	},
	{
		key: 'mar', label: 'Mar',
		always: 'The same picks — and the preparer also breaks one of their own resource assets for each point the roll fell short of the difficulty.',
		opts: OPTIONS.map(o => ({ key: o.key, label: o.label })),
	},
];

/** Said under the button whose press leaves nothing owed. */
export const LAST_STEP_TEXT = 'This is the last step: Seek Answers resolves when you submit.';

/** How many of the picked choices are `key`. */
export function countIn(choices: string[], key: string): number {
	return choices.filter(c => c === key).length;
}

/**
 * Discharge a depletable step that has remaining picks but no live target — e.g. every
 * breakable resource is gone. The server re-checks that no target exists, so this only ever
 * resolves a genuine dead-end (it can't skip a step the preparer could still perform).
 * Without it the plan wedges. One busy flag, on the drafts, covers all three skip buttons.
 */
export async function forfeitStep(
	drafts: SeekDrafts,
	planID: number,
	step: 'break_resource' | 'reveal_secret' | 'mar_penalty',
	onDone: () => void,
	setError: (message: string) => void,
) {
	if (drafts.forfeitBusy) return;
	drafts.forfeitBusy = true; setError('');
	try {
		await forfeitSeekStep(planID, step);
		onDone();
	} catch (e) {
		setError(e instanceof Error ? e.message : 'Could not skip step.');
	} finally { drafts.forfeitBusy = false; }
}

/**
 * `known(assetID)`: how many of an asset's secrets the viewer can already read (the shared
 * secret-counts lookup, which a component has to fetch from context in its own script).
 */
export function seekScene(ctx: PlanContext, plan: Plan, known: (assetID: number) => number) {
	const { assets, players, currentPlayerID, plans, rollOutcome, activeRoll } = ctx;

	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = currentPlayerID === plan.preparer_id;
	// Post-roll steps belong to the perform_steps winner when one holds the plan,
	// else the preparer (mirrors the server's actsForPreparer). Pre-roll steps
	// stay on isPreparer: perform_steps is post-roll only.
	const amChoiceActor = currentPlayerID != null
		&& currentPlayerID === resolutionActorID(plan, plans);

	// Seek Answers resolution state: which resources have been flawed (each may
	// be flawed at most once) and the mar self-flaw penalty progress.
	const saData = parseResolutionData(plan).seek_answers ?? {};
	// Pre-roll is done once this plan's dice exist. pre_roll_done is the
	// server-authoritative flag for plans created under the pre-roll flow; the
	// roll-existence and committed-choices fallbacks free legacy in-flight plans
	// — resolved before the pre-roll step shipped, so they have no flag — from
	// being trapped on the pre-roll screen.
	const rollForThisPlan = activeRoll != null && activeRoll.plan_id === plan.id;
	const hasCommittedChoices = choicesCommitted(parseResolutionData(plan));
	const preRollDone = (saData.pre_roll_done ?? false) || rollForThisPlan || hasCommittedChoices;

	const existingChoices = (parseResolutionData(plan).make_mar_choices ?? []).map(c => c.option);
	const choicesDone = existingChoices.length > 0 || hasCommittedChoices;

	const flawedIDs = new Set(saData.flawed_resource_ids ?? []);
	const marRequired = saData.mar_self_flaws_required ?? 0;
	const marApplied = saData.mar_self_flaws_applied ?? 0;
	const marRemaining = Math.max(0, marRequired - marApplied);
	// Server-recorded make-list sub-flow progress: completion is server-authoritative
	// (resolution_data counters), so a refresh/remount can't re-prompt — and re-run — a
	// step that already happened; the handlers also reject any step beyond the pick.
	const breakDone = saData.break_resource_done ?? 0;
	const revealDone = saData.reveal_secret_done ?? 0;
	const declareTruthDone = saData.declare_truth_done ?? 0;
	const askQuestionDone = saData.ask_question_done ?? 0;
	const pendingQuestion = saData.pending_question ?? null;
	// Who vetoed the in-progress pick. The replacement question stays with them.
	const askVetoedTargetID = saData.current_ask_vetoed_target_id ?? null;

	const preparerID = plan.preparer_id;
	const isMar = rollOutcome === 'mar';

	// ── Tier-1 committed state: the specifics of each completed step, recorded in
	// resolution_data and shown read-only to every viewer (see ADR-006) so the
	// choice that was made persists in the panel instead of vanishing when its
	// picker commits. flawed_resource_ids splits by owner: on a mar the preparer's
	// own flaws are the penalty, everything else is a make-list break.
	const assetName = (id: number): string =>
		assets.find(a => a.id === id)?.name ?? 'an asset';
	const revealedAssetIDs = saData.revealed_asset_ids ?? [];
	const declaredTruths = saData.declared_truths ?? [];
	const answeredQuestions = saData.answered_questions ?? [];
	const flawedList = saData.flawed_resource_ids ?? [];
	const penaltyBrokenIDs = flawedList.filter(
		id => isMar && assets.find(a => a.id === id)?.owner_id === preparerID,
	);
	const makeBrokenIDs = flawedList.filter(
		id => !(isMar && assets.find(a => a.id === id)?.owner_id === preparerID),
	);
	const anyResolved = makeBrokenIDs.length > 0 || revealedAssetIDs.length > 0
		|| declaredTruths.length > 0 || answeredQuestions.length > 0 || penaltyBrokenIDs.length > 0;

	// Make-list "break a resource" picker: any breakable resource not yet flawed
	// this plan — one with intact marginalia, or a blank one, whose break
	// destroys it outright. On a mar, exclude the preparer's own resources —
	// those go through the penalty flow (the server attributes a break by owner).
	const resourcesWithMarginalia = breakableAssets(assets.filter(a => a.asset_type === 'resource'));
	const brResourcesWithMarginalia = resourcesWithMarginalia
		.filter(a => !flawedIDs.has(a.id))
		.filter(a => !isMar || a.owner_id !== preparerID);
	// Mar penalty picker: the preparer's own resources, not yet flawed.
	const penaltyResources = resourcesWithMarginalia
		.filter(a => a.owner_id === preparerID && !flawedIDs.has(a.id));
	// What a mar will cost the preparer, known as soon as the dice land: the
	// shortfall, capped at the resources they can still flaw (the server fixes the
	// same number when the choices are applied). Null on a make, or before the dice.
	let marPenalty: { shortfall: number; owed: number } | null = null;
	if (rollOutcome === 'mar' && activeRoll != null && activeRoll.result != null) {
		const difficulty = activeRoll.adjusted_difficulty ?? activeRoll.difficulty;
		marPenalty = {
			shortfall: Math.max(difficulty - activeRoll.result, 0),
			owed: seekAnswersMarPenalty(difficulty, activeRoll.result, penaltyResources.length),
		};
	}
	// Reveal-secret targets another player's asset ("ask a player to show you the
	// underside of a specific one of their assets") — never the preparer's own.
	// The preparer (the only player who ever picks reveal targets) sees assets
	// that still hold a secret THEY can't read, since revealing one they already
	// know fully is a no-op. That filter is viewer-scoped via the shared lookup,
	// so a read-only observer can't reproduce it — which secrets the preparer
	// knows is confidential. For the greyed mirror we fall back to public
	// existence (any secret at all), identical for every viewer; the lists differ
	// only for an asset the preparer already knows fully, which is rare and
	// harmless to show greyed.
	const revealableAssets = assets.filter(a =>
		!a.is_destroyed && a.owner_id !== preparerID
		&& (isPreparer ? hiddenCount(a, known(a.id)) > 0 : a.secret_count > 0),
	);

	// Other players the preparer can question.
	const questionTargets = players.filter(p => p.id !== plan.preparer_id);

	// Per-option availability for the counts picker: an option with no valid
	// target can't be picked, so we disable it. declare_truth is always possible;
	// the others need a live target (a breakable resource, a questionable player,
	// or an asset whose secrets you don't already hold).
	const optionAvailable: Record<string, boolean> = {
		break_resource: brResourcesWithMarginalia.length > 0,
		declare_truth: true,
		ask_question: questionTargets.length > 0,
		reveal_secret: revealableAssets.length > 0,
	};
	// Per-option cap (null = uncapped): the depletable options (break a resource,
	// reveal a secret) can't be picked more times than they have live targets —
	// otherwise the surplus picks commit and then wedge with no target to spend
	// them on. The non-depletable options (declare a truth; ask a question — you
	// can re-ask the same player) are uncapped, so the dice quota can always be
	// reached through them.
	const optionCap: Record<string, number | null> = {
		break_resource: brResourcesWithMarginalia.length,
		declare_truth: null,
		ask_question: null,
		reveal_secret: revealableAssets.length,
	};

	// ── What each sub-step still owes ────────────────────────────────────────
	const brRemaining = Math.max(0, countIn(existingChoices, 'break_resource') - breakDone);
	const rsRemaining = Math.max(0, countIn(existingChoices, 'reveal_secret') - revealDone);
	const dtRemaining = Math.max(0, countIn(existingChoices, 'declare_truth') - declareTruthDone);
	const aqRemaining = Math.max(0, countIn(existingChoices, 'ask_question') - askQuestionDone);
	const subflowsDone = brRemaining === 0 && rsRemaining === 0 && marRemaining === 0
		&& dtRemaining === 0 && aqRemaining === 0 && pendingQuestion == null;
	// Submits still owed. The server resolves the plan the moment the last lands, so
	// the submit that takes this to one is the one that ends Seek Answers. A pending
	// question counts as one (its answer), and asking a question never ends it.
	const totalOwed = brRemaining + rsRemaining + dtRemaining + aqRemaining + marRemaining;

	return {
		isPreparer, amChoiceActor, saData, preRollDone, existingChoices, choicesDone,
		marRequired, marRemaining, pendingQuestion, askVetoedTargetID, isMar,
		assetName, revealedAssetIDs, declaredTruths, answeredQuestions,
		penaltyBrokenIDs, makeBrokenIDs, anyResolved,
		brResourcesWithMarginalia, penaltyResources, marPenalty, revealableAssets,
		questionTargets, optionAvailable, optionCap,
		brRemaining, rsRemaining, dtRemaining, aqRemaining, subflowsDone, totalOwed,
	};
}

export type SeekScene = ReturnType<typeof seekScene>;
