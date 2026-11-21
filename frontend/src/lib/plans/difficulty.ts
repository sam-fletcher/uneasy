// What a plan's roll will be set at, and why, for the meter every plan shows before
// its dice land.
//
// Before a roll exists the client has to work the number out itself, from what every
// player already holds: the rankings, the assets, the plan. That is the same
// arithmetic as the server's, one function per plan in game/difficulty.go (and
// MakeDemandsDifficulty in game/demands.go), so a change to either is a change to
// both. Once a roll has been made for the plan its own value is the one shown, so
// this can only ever be wrong before the roll, never in it: the roll panel and the
// meter agree from the cast on, and a drift shows as a meter that jumps at the cast.
//
// Everything here is public: ranks, who holds an asset and whether it is a main
// character, and how many artifacts have been invoked are on every client. No
// secret reaches this, so a watcher's meter is the actor's.

import type { Asset, DiceRoll, Plan, Player, Ranking, RankingCategory } from '$lib/api';
import { choicesCommitted, parseResolutionData, playerName, PLAN_SHORT } from '$lib/components/plans/shared';
import { parseMakeIntroductionsData } from '$lib/plans/resolutionData/make_introductions';

/** A difficulty is between 1 and 6, whatever the arithmetic says
 *  (game.ClampDifficulty; owner ruling 2026-11-01). */
export const MAX_DIFFICULTY = 6;

const clamp = (n: number): number => Math.min(Math.max(n, 1), MAX_DIFFICULTY);

/** What a meter needs: how hard, how much of that is the plan's own base, and the
 *  one line that says where the number came from. */
export interface DifficultyView {
	/** 1 to 6. */
	value: number;
	/** The part that comes from the plan's own base. What lies between this and
	 *  `value` is what the table chose to add (artifacts invoked, peers introduced,
	 *  a power gap). Equal to `value` for a plan that adds nothing. */
	floor: number;
	/** Viewer-neutral: it names players, never "you", because everyone reads it. */
	reason: string;
}

/** The state this reads, all of it on every client. */
export interface DifficultyEnv {
	plans: Plan[];
	assets: Asset[];
	players: Player[];
	rankings: Ranking[];
	/** The table's open roll, if any. Used only when it belongs to the plan asked about. */
	activeRoll: DiceRoll | null;
}

export interface DifficultyOpts {
	/** Chronicle Histories: the artifacts the preparer has ticked so far, not yet
	 *  committed. Without it the plan's own invoked artifacts are counted. */
	invokedCount?: number;
}

function rankOf(rankings: Ranking[], category: RankingCategory, playerID: number | null): number | null {
	if (playerID == null) return null;
	return rankings.find(r => r.category === category && r.player_id === playerID)?.rank ?? null;
}

/** Status is the opposite way round to rank: rank 1 is the highest standing, and a
 *  status of 5. The track a plan reads decides which one is its difficulty. */
const statusOf = (rank: number): number => MAX_DIFFICULTY - rank;

/** Rank 1 is the best standing, and a plan whose difficulty is a rank is easiest at
 *  the top: a first-time player reads "rank 3" as a high number, so it is said. */
const LOWER_IS_BETTER = 'a lower rank is better';

/** "ada's rank on the knowledge track: 2 (a lower rank is better)." */
function rankReason(name: string, category: RankingCategory, rank: number): string {
	return `The difficulty is ${name}’s rank on the ${category} track: ${rank} (${LOWER_IS_BETTER}).`;
}

/** "bea's status on the power track: 6 minus rank 2 is 4." */
function statusReason(name: string, category: RankingCategory, rank: number): string {
	return `The difficulty is ${name}’s status on the ${category} track: ${MAX_DIFFICULTY} minus rank ${rank} is ${statusOf(rank)}.`;
}

/** What the plan's own formula gives, with no roll consulted. Null when it can't be
 *  worked out (a rank not loaded, an asset not on the board), and for the two plans
 *  that never roll. */
function formula(plan: Plan, env: DifficultyEnv, opts: DifficultyOpts): DifficultyView | null {
	const { players, rankings } = env;
	const preparer = playerName(players, plan.preparer_id);

	switch (plan.plan_type) {
		case 'seek_answers':
		case 'spread_propaganda':
		case 'propose_decree': {
			const category: RankingCategory = plan.plan_type === 'seek_answers' ? 'knowledge'
				: plan.plan_type === 'spread_propaganda' ? 'esteem' : 'power';
			const rank = rankOf(rankings, category, plan.preparer_id);
			if (rank == null) return null;
			const value = clamp(rank);
			return { value, floor: value, reason: rankReason(preparer, category, rank) };
		}

		case 'exchange_courtiers':
		case 'propose_duel': {
			const category: RankingCategory = plan.plan_type === 'exchange_courtiers' ? 'power' : 'esteem';
			const rank = rankOf(rankings, category, plan.target_player_id);
			if (rank == null) return null;
			const value = clamp(statusOf(rank));
			return { value, floor: value, reason: statusReason(playerName(players, plan.target_player_id), category, rank) };
		}

		case 'host_festivity': {
			const rank = rankOf(rankings, 'esteem', plan.preparer_id);
			if (rank == null) return null;
			const value = clamp(statusOf(rank));
			return { value, floor: value, reason: statusReason(preparer, 'esteem', rank) };
		}

		case 'spread_rumors': {
			const target = env.assets.find(a => a.id === plan.target_asset_id);
			if (target == null) return null;
			if (target.is_main_character) {
				const rank = rankOf(rankings, 'esteem', target.owner_id);
				if (rank == null) return null;
				const value = clamp(statusOf(rank));
				const owner = playerName(players, target.owner_id);
				return {
					value, floor: value,
					reason: `The rumor targets ${owner}’s main character, so the difficulty is ${owner}’s status on the esteem track: ${MAX_DIFFICULTY} minus rank ${rank} is ${statusOf(rank)}.`,
				};
			}
			const rank = rankOf(rankings, 'esteem', plan.preparer_id);
			if (rank == null) return null;
			const value = clamp(rank);
			return {
				value, floor: value,
				reason: `The target is not a main character, so the difficulty is the preparer’s own rank on the esteem track: ${preparer}, ${rank} (${LOWER_IS_BETTER}).`,
			};
		}

		case 'chronicle_histories': {
			const rank = rankOf(rankings, 'knowledge', plan.preparer_id);
			if (rank == null) return null;
			const invoked = opts.invokedCount
				?? (parseResolutionData(plan).chronicle_histories?.invoked_artifact_ids ?? []).length;
			return {
				value: clamp(Math.max(rank, invoked)),
				floor: clamp(rank),
				reason: `The difficulty is the higher of ${preparer}’s knowledge rank (${rank}; ${LOWER_IS_BETTER}) and the number of artifacts invoked (${invoked}).`,
			};
		}

		case 'make_introductions': {
			// A plan with no peers recorded counts as one, as the server does.
			const peers = Math.max(parseMakeIntroductionsData(plan).peer_count ?? 0, 1);
			return {
				value: clamp(2 + peers),
				floor: 2,
				reason: `2, plus 1 for each of the ${peers} ${peers === 1 ? 'peer' : 'peers'} introduced.`,
			};
		}

		case 'make_demands':
			return demandFormula(plan, env);

		// No dice, so no difficulty: Make War resolves the moment it begins, and
		// Clandestinely Liaise is four choices with no roll at all.
		case 'make_war':
		case 'clandestinely_liaise':
			return null;
	}
}

/** The target plan's own difficulty, plus the gap in power when its preparer outranks
 *  the demander (game.MakeDemandsDifficulty), kept to 1–6. `floor` is the target's own
 *  part: what the demand adds is the gap. */
function demandFormula(plan: Plan, env: DifficultyEnv): DifficultyView | null {
	const target = env.plans.find(p => p.id === plan.targeted_plan_id);
	// A demand can't target a demand (the server refuses it), so there is nothing to read.
	if (target == null || target.plan_type === 'make_demands') return null;

	// A target with no roll of its own (a liaison) is 0: only the gap is left to count.
	const own = formula(target, env, {});
	const targetDiff = own?.value ?? 0;
	if (own == null && target.plan_type !== 'clandestinely_liaise' && target.plan_type !== 'make_war') return null;

	const demanderRank = rankOf(env.rankings, 'power', plan.preparer_id);
	const targetRank = rankOf(env.rankings, 'power', target.preparer_id);
	if (demanderRank == null || targetRank == null) return null;

	const gap = targetRank < demanderRank ? demanderRank - targetRank : 0;
	const raw = targetDiff + gap;
	const value = clamp(raw);

	const targetName = PLAN_SHORT[target.plan_type] ?? target.plan_type;
	const demander = playerName(env.players, plan.preparer_id);
	const targetPlayer = playerName(env.players, target.preparer_id);

	const parts: string[] = [];
	parts.push(own == null
		? `${targetName} has no roll of its own, so only the power gap counts`
		: `${targetName} is difficulty ${targetDiff}`);
	parts.push(gap > 0
		? `${targetPlayer} outranks ${demander} on power by ${gap}, which adds ${gap}`
		: `${targetPlayer} does not outrank ${demander} on power, so nothing is added`);
	let reason = `${parts.join('; ')}.`;
	if (raw > MAX_DIFFICULTY) reason += ` A difficulty is never above ${MAX_DIFFICULTY}.`;
	if (raw < 1) reason += ' A difficulty is never below 1.';

	return { value, floor: Math.min(value, targetDiff), reason };
}

/**
 * The difficulty a plan's roll is set at, or will be, and why. Null when there is
 * nothing to show: the plan never rolls, or what it is worked out from isn't on this
 * client yet and no roll exists to read it from.
 *
 * Once a roll has been made for this plan, its value is the one returned, so the client
 * can only be wrong before it:
 *  - the VOTE's outcome once it has revealed (`adjusted_difficulty`), with a clause saying
 *    the vote moved it, so the meter and the dice panel's "5, up from 3 by the vote" never
 *    disagree. The ballot is hidden and the adjusted value is only written when the last
 *    vote lands, so a roll carries none before the reveal and the base shows until then;
 *  - the roll's value even when the formula could not be worked out, with no reason;
 *  - no reason at all when the formula disagrees with the roll (the plan not yet refetched
 *    after a cast): a reason that explains a number that is not on screen is worse than none.
 */
export function planDifficulty(plan: Plan, env: DifficultyEnv, opts: DifficultyOpts = {}): DifficultyView | null {
	const view = formula(plan, env, opts);
	const roll = env.activeRoll;
	if (roll == null || roll.plan_id !== plan.id) return view;

	const base = clamp(roll.difficulty);
	let value = base;
	let reason = view != null && view.value === base ? view.reason : '';
	const adjusted = roll.adjusted_difficulty;
	if (adjusted != null && clamp(adjusted) !== base) {
		value = clamp(adjusted);
		const moved = `The vote moved it from ${base} to ${value}.`;
		reason = reason ? `${reason} ${moved}` : moved;
	}
	return { value, floor: Math.min(view?.floor ?? value, value), reason };
}

/**
 * Whether THIS plan's dice are still to land: the window in which a plan shows its meter
 * and its "What can happen?". `rollOutcome` on the page follows whichever plan-linked roll
 * is on the table, so on its own it brings them back when another plan's roll opens (a
 * counter-propaganda rolls while the plan it answers still owes steps). So a roll that
 * belongs to another plan is not read, and a plan whose choices are committed has landed.
 */
export function diceNotYetLanded(
	plan: Plan,
	ctx: { rollOutcome: 'make' | 'mar' | null; activeRoll: DiceRoll | null },
): boolean {
	const roll = ctx.activeRoll;
	const mine = roll != null && roll.plan_id === plan.id;
	if (mine && roll.outcome != null) return false;
	if (ctx.rollOutcome != null && (roll == null || mine)) return false;
	return !choicesCommitted(parseResolutionData(plan));
}
