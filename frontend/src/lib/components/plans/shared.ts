// Shared constants, parsers, and helpers for plan components.
import type { Plan, Player, Asset, PlanType, ResolutionData, RankingCategory } from '$lib/api';

/** One-line flavour description per plan type. Sourced from "Plan Titles.md"
 * (the in-rules card copy). Kept short so plan cards stay scannable. */
export const PLAN_DESCRIPTION: Record<PlanType, string> = {
	make_demands:         'Demand control of the resolution of another player’s plan.',
	propose_decree:       'Bring a new law to the royal council to enact sweeping legal change.',
	exchange_courtiers:   'Take another player’s peer into your retinue.',
	make_war:             'Declare war. Agree to peace terms, or break assets every round.',
	spread_propaganda:    'Set society ablaze; create an artifact that embodies the shift.',
	spread_rumors:        'Spread rumors to damage the reputation of an asset.',
	propose_duel:         'Go 1-on-1 in a battle of arms or wits to win assets.',
	host_festivity:       'Throw a ball, host a dinner, go on a hunt. Anything can happen.',
	make_introductions:   'Introduce new peers to court. Add them to your retinue.',
	seek_answers:         'Learn secrets, break assets, declare truths, get answers.',
	chronicle_histories:  'Explore a situation from history. Break artifacts.',
	clandestinely_liaise: 'Meet in secret. Learn secrets, change assets.',
};

/** Static row delay per plan, mirroring each handler's `PlanMetadata.Delay`
 * (handler/plan_*.go). A plan prepared on row N resolves on row N + delay.
 * `-1` marks the three variable-delay plans — Make Demands, Make War, and
 * Clandestinely Liaise — whose delay is only known after prep (target plan /
 * dice reveal), so the grid shows an icon for them instead of a number. */
export const PLAN_DELAY: Record<PlanType, number> = {
	make_demands:         -1,
	propose_decree:       4,
	exchange_courtiers:   5,
	make_war:             -1,
	spread_propaganda:    3,
	spread_rumors:        4,
	propose_duel:         5,
	host_festivity:       6,
	make_introductions:   3,
	seek_answers:         4,
	chronicle_histories:  5,
	clandestinely_liaise: -1,
};

/** Display order within each track column (top → bottom). */
export const TRACK_ORDER: Record<RankingCategory, PlanType[]> = {
	power:     ['make_demands', 'propose_decree', 'exchange_courtiers', 'make_war'],
	knowledge: ['make_introductions', 'seek_answers', 'chronicle_histories', 'clandestinely_liaise'],
	esteem:    ['spread_propaganda', 'spread_rumors', 'propose_duel', 'host_festivity'],
};

export const PLAN_SHORT: Record<PlanType, string> = {
	exchange_courtiers:   'Exchange Courtiers',
	make_introductions:   'Make Introductions',
	spread_propaganda:    'Spread Propaganda',
	seek_answers:         'Seek Answers',
	spread_rumors:        'Spread Rumors',
	chronicle_histories:  'Chronicle Histories',
	propose_decree:       'Propose Decree',
	clandestinely_liaise: 'Clandestinely Liaise',
	propose_duel:         'Propose Duel',
	host_festivity:       'Host Festivity',
	make_war:             'Make War',
	make_demands:         'Make Demands',
};

export interface PlanChoiceOption { key: string; label: string; }

export const MAKE_OPTIONS: Partial<Record<PlanType, PlanChoiceOption[]>> = {
	exchange_courtiers: [
		{ key: 'messy',      label: '(0) Messy — target may break one of your assets' },
		{ key: 'legal',      label: '(1) Legal — everything went to plan' },
		{ key: 'conspiracy', label: '(2) Conspiracy — the peer was in on it' },
	],
	make_introductions: [
		{ key: 'peers_arrive', label: 'Peers arrive — add marginalia to each new peer' },
	],
	spread_propaganda: [
		{ key: 'create_artifact', label: 'Create an artifact representing the societal shift' },
	],
};

export const MAR_OPTIONS: Partial<Record<PlanType, PlanChoiceOption[]>> = {
	exchange_courtiers: [
		{ key: 'fair_trade', label: '(1) A Fair Trade — the trade goes through anyway' },
		{ key: 'riposte',    label: '(2) Riposte — you take one of their peers (they may break it first)' },
		{ key: 'forfeit',    label: '(3) Forfeit — you take one of their peers' },
	],
	// Make Introductions mar is per-peer (other_retinue / broken_arrival /
	// delayed / broken_journey) and rendered directly in MakeIntroductionsPanel,
	// not via the flat MakeMarPicker — so it has no entry here.
	spread_propaganda: [
		{ key: 'give_peer',    label: "(a) A peer leaves your retinue (give to another player)" },
		{ key: 'lay_low',      label: '(b) Keep your head down — next plan cannot involve esteem' },
		{ key: 'break_self',   label: '(c) Word of your laughable ideas gets around — break yourself' },
		{ key: 'counter_prop', label: "(d) Top interferer spreads their own propaganda now (resolves immediately)" },
	],
};

// ── Resolution data parser ────────────────────────────────────────────────

/** Parse plan.resolution_data into the typed ResolutionData shape. Returns
 * an empty object if the field is null or the JSON is malformed. */
export function parseResolutionData(plan: Plan | null | undefined): ResolutionData {
	if (!plan?.resolution_data) return {};
	try { return JSON.parse(plan.resolution_data) as ResolutionData; }
	catch { return {}; }
}

// ── Make Demands helpers ──────────────────────────────────────────────────

/** The four draft options drafted by demander + target preparer after a made
 * demand. Each maps to a piece of cross-cutting authority over the target
 * plan's resolution. Match game.DemandOption* in uneasy/game/demands.go. */
export type DemandOption =
	| 'control_leverage'
	| 'keep_or_change_target'
	| 'keep_assets'
	| 'perform_steps';

export const DEMAND_OPTION_LABELS: Record<DemandOption, string> = {
	control_leverage:      'Control leverage — leverage the target preparer’s assets onto their roll',
	keep_or_change_target: 'Keep or change target — re-aim the target plan',
	keep_assets:           'Keep assets — receive any assets the target plan would have given the preparer',
	perform_steps:         'Perform steps — submit the target plan’s make/mar choice in their place',
};

export const DEMAND_OPTIONS: DemandOption[] = [
	'control_leverage', 'keep_or_change_target', 'keep_assets', 'perform_steps',
];

export type DemandWinners = Partial<Record<DemandOption, number>>;

/** Decode a demand plan's draft picks into a winners map (option → playerID).
 * Returns an empty map if the draft is incomplete. */
export function demandWinnersFromPlan(demand: Plan): DemandWinners {
	const choices = parseResolutionData(demand).make_demands?.draft_choices ?? [];
	const winners: DemandWinners = {};
	for (const c of choices) {
		if (DEMAND_OPTIONS.includes(c.option as DemandOption)) {
			winners[c.option as DemandOption] = c.player_id;
		}
	}
	return winners;
}

/** Find the resolved+made Make Demands plan (if any) targeting the given
 * plan. There is at most one such demand per target per backend invariant. */
export function activeDemandAgainst(plan: Plan, allPlans: Plan[]): Plan | null {
	for (const p of allPlans) {
		if (p.plan_type !== 'make_demands') continue;
		if (p.targeted_plan_id !== plan.id) continue;
		if (p.status !== 'resolved' || p.result !== 'make') continue;
		return p;
	}
	return null;
}

// ── Generic helpers ───────────────────────────────────────────────────────

export function playerName(players: Player[], id: number | null): string {
	if (id == null) return '?';
	return players.find(p => p.id === id)?.display_name ?? '?';
}

export function assetName(assets: Asset[], id: number | null): string {
	if (id == null) return '?';
	return assets.find(a => a.id === id)?.name ?? '?';
}

/** Intact marginalia across all of a player's non-destroyed assets. */
export function intactMarginalia(assets: Asset[], ownerID: number | null) {
	if (ownerID == null) return [];
	return assets
		.filter(a => a.owner_id === ownerID && !a.is_destroyed)
		.flatMap(a => (a.marginalia ?? [])
			.filter(m => !m.is_torn)
			.map(m => ({ ...m, assetName: a.name, assetID: a.id }))
		);
}

/**
 * Non-destroyed assets that still have at least one untorn marginalia.
 * Used everywhere AssetCardSelectable's marginalia-pick mode renders a
 * card per parent asset. Optional `ownerID` narrows to a single owner.
 */
export function assetsWithIntactMarginalia(
	assets: Asset[],
	ownerID?: number | null,
): Asset[] {
	return assets.filter(a =>
		!a.is_destroyed
		&& (ownerID == null || a.owner_id === ownerID)
		&& (a.marginalia ?? []).some(m => !m.is_torn),
	);
}

/**
 * Players other than the given one. Standard prep-target list shape;
 * returns the full list if `exclude` is null (e.g. spectator view).
 */
export function playersExcept(players: Player[], exclude: number | null): Player[] {
	return players.filter(p => p.id !== exclude);
}

/**
 * Intact, un-leveraged assets owned by `ownerID`. Used by Clandestinely
 * Liaise's keep-secret picker, Make War's leverage_two list, etc.
 * `null` ownerID returns [].
 */
export function ownerUnleveragedAssets(assets: Asset[], ownerID: number | null): Asset[] {
	if (ownerID == null) return [];
	return assets.filter(a =>
		a.owner_id === ownerID && !a.is_destroyed && !a.is_leveraged,
	);
}

/**
 * Intact assets owned by `ownerID`. `null` ownerID returns [].
 */
export function ownerIntactAssets(assets: Asset[], ownerID: number | null): Asset[] {
	if (ownerID == null) return [];
	return assets.filter(a => a.owner_id === ownerID && !a.is_destroyed);
}

