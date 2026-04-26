// Shared constants, parsers, and helpers for plan components.
import type { Plan, Player, Asset, PlanType, ResolutionData } from '$lib/api';

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

export const PLAN_LABELS: Record<PlanType, string> = {
	exchange_courtiers:   'Exchange Courtiers (Power, delay 5)',
	make_introductions:   'Make Introductions (Knowledge, delay 3)',
	spread_propaganda:    'Spread Propaganda (Esteem, delay 3)',
	seek_answers:         'Seek Answers (Knowledge, delay 1)',
	spread_rumors:        'Spread Rumors (Esteem, delay 1)',
	chronicle_histories:  'Chronicle Histories (Knowledge, delay 1)',
	propose_decree:       'Propose Decree (Power, delay 3)',
	clandestinely_liaise: 'Clandestinely Liaise (Esteem, delay 3)',
	propose_duel:         'Propose Duel (Power/Esteem, delay 5)',
	host_festivity:       'Host Festivity (Esteem, delay 5)',
	make_war:             'Make War (Power, delay 5)',
	make_demands:         'Make Demands (Power, delay 5)',
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
		{ key: 'riposte',    label: '(2) Riposte — target takes your peer (you may break it first)' },
		{ key: 'forfeit',    label: '(3) Forfeit — target takes your peer' },
	],
	make_introductions: [
		{ key: 'other_retinue',     label: "(a) Peer enters another player's retinue" },
		{ key: 'broken_arrival',    label: '(b) Arrives broken — another writes marginalia, then one is torn' },
		{ key: 'other_retinue_2',   label: '(c) Delayed → enters another retinue instead (Phase 2 simplification)' },
		{ key: 'broken_journey',    label: '(d) Arrives broken with an arduous journey' },
	],
	spread_propaganda: [
		{ key: 'give_peer',    label: "(a) A peer leaves your retinue (give to another player)" },
		{ key: 'lay_low',      label: '(b) Keep your head down — next plan cannot involve esteem' },
		{ key: 'break_self',   label: '(c) Word of your laughable ideas gets around — break yourself' },
		{ key: 'counter_prop', label: "(d) Interfering player describes counter-propaganda in the follow-scene" },
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
	const choices = parseResolutionData(demand).draft_choices ?? [];
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
