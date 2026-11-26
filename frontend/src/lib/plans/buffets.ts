// The "What can happen" reference for the five plans that lacked one: Propose Decree,
// Propose Duel, Make War, Clandestinely Liaise and Make Demands. Each says, from the
// book's plan page, what the plan can do, in the third person (everyone reads it, and a
// second-person line would read as the viewer's own).
//
// Make War and Clandestinely Liaise have no make or mar: a war rolls nothing and a
// liaison is four choices with no dice, so theirs describe what each can bring instead.
//
// Where the book leaves the wording open, this is the closest reading; the owner's to
// settle is listed in docs/work/questions (one file per plan). Nothing here is hidden
// information: it is the printed rules, and the plan's own public facts.

import type { PlanType } from '$lib/api';
import type { BuffetTab } from '$lib/components/plans/shared/Buffet.svelte';
import {
	DEMAND_OPTIONS, DEMAND_OPTION_LABELS, PLAN_SHORT, demandOptionInertReason,
	type DemandOption,
} from '$lib/components/plans/shared';

// ── Propose Decree (book p.47) ──────────────────────────────────────────────

export const DECREE_BUFFET: BuffetTab[] = [
	{
		key: 'make', label: 'Make',
		intro: 'The law goes into force once the signatory adds an addendum:',
		opts: [
			{ key: 'addendum', label: 'The addendum', desc: '— a simple phrase with “but” or “and”, added by the signatory.' },
			{ key: 'law', label: 'The law', desc: '— its text is written under Laws in the public record.' },
			{ key: 'resource', label: 'A resource', desc: '— the preparer creates a resource asset for what they gain from it.' },
		],
	},
	{
		key: 'mar', label: 'Mar',
		intro: 'The law still goes into force, and the council may change it:',
		opts: [
			{ key: 'amend', label: 'Amendments', desc: '— each other council member may amend the law at will, lowest power first.' },
			{ key: 'addendum', label: 'The addendum', desc: '— the signatory then adds one, as on a make.' },
			{ key: 'resource', label: 'No resource', desc: '— the preparer gains no new resource asset.' },
		],
	},
];

// ── Propose Duel (book p.60) ────────────────────────────────────────────────

export const DUEL_BUFFET: BuffetTab[] = [
	{
		key: 'make', label: 'Make',
		always: 'The preparer takes staked assets from the challenged player: as many as the roll’s result.',
		opts: [{ key: 'leverage', label: 'Either way', desc: '— both players then leverage every asset they staked.' }],
	},
	{
		key: 'mar', label: 'Mar',
		always: 'The challenged player may take staked assets from the preparer: as many as the difficulty, or spare them. It is all or nothing.',
		opts: [{ key: 'leverage', label: 'Either way', desc: '— both players then leverage every asset they staked.' }],
	},
];

// ── Make War (book p.49) ────────────────────────────────────────────────────

export const WAR_BUFFET: BuffetTab[] = [
	{
		key: 'cost', label: 'Each row',
		intro: 'Every time the public record advances, each player at war pays the cost of battle, one at a time in reverse power order. For each opponent on the other side, a player chooses one:',
		opts: [
			{ key: 'break', label: 'Break an asset', desc: '— tear one of their own marginalia.' },
			{ key: 'leverage', label: 'Leverage two', desc: '— leverage two of their own assets.' },
			{ key: 'surrender', label: 'Surrender', desc: '— do one of those, then surrender unconditionally.' },
			{ key: 'peace', label: 'Propose peace', desc: '— get everyone at war to agree to peace terms.' },
		],
	},
	{
		key: 'surrender', label: 'Surrender',
		always: 'A player who surrenders unconditionally loses the war, and each opponent takes one asset of their choice from them.',
	},
	{
		key: 'peace', label: 'Peace',
		always: 'Peace terms can be anything everyone at war agrees to: trading assets, mending broken ones, or whatever else makes sense. A player not at war who disagrees may pay the cost of battle to join the talks.',
	},
];

// ── Clandestinely Liaise (book p.55) ────────────────────────────────────────

export const LIAISE_BUFFET: BuffetTab[] = [
	{
		key: 'share', label: 'Things we share',
		intro: 'Each of the two picks one, and the picks are revealed together:',
		opts: [
			{ key: 'look_at_secret', label: 'Look at secrets', desc: '— at one of the partner’s assets, for the secrets it keeps.' },
			{ key: 'update_peer', label: 'Update a peer', desc: '— rewrite one marginalia on the partner’s meeting peer.' },
			{ key: 'break_peer', label: 'Break a peer', desc: '— tear one marginalia on the partner’s meeting peer.' },
			{ key: 'take_gift', label: 'Take a gift', desc: '— take one of the partner’s non-peer assets.' },
			{ key: 'leverage_partner', label: 'Leverage, and bank a die', desc: '— leverage one of the partner’s assets and bank a die for any one future roll.' },
		],
	},
	{
		key: 'again', label: 'Meeting again',
		always: 'Both players reveal a face from 1 to 6 again, and the average, rounded up, is the delay until they meet next. If either reveals no die (0), the meeting is called off.',
	},
];

// ── Make Demands (book p.46 and its clarifications) ─────────────────────────

/** What a demand's `keep_assets` winner takes in place of the target plan's
 *  preparer, per plan: the preparer's own gains, as the server redirects them
 *  (AssetRecipientForPlan). Seek Answers and Chronicle Histories award none, so
 *  they have no entry (`demandOptionInertReason` says so). */
const KEPT_ASSETS: Partial<Record<PlanType, string>> = {
	exchange_courtiers: 'the peer the exchange brings in',
	make_introductions: 'each newcomer who would join the preparer’s retinue',
	spread_propaganda: 'the artifact the make creates',
	spread_rumors: 'any asset the rumor takes',
	propose_decree: 'the resource the law creates',
	propose_duel: 'the staked assets the preparer takes',
	host_festivity: 'what the host gains at the event',
	clandestinely_liaise: 'the gift the preparer takes from their partner',
};

/** What `keep_or_change_target` re-aims, per plan that has an aim. It changes who,
 *  never what (the demand can't rewrite a plan's declared content). */
const RE_AIMED: Partial<Record<PlanType, string>> = {
	exchange_courtiers: 'Keep or change which peer the exchange asks for.',
	spread_rumors: 'Keep or change which asset the rumor is about.',
	propose_duel: 'Keep or change who is challenged. It is settled before the duel’s first step.',
	clandestinely_liaise: 'Keep or change who the meeting is with. It is settled before the liaison’s first step.',
};

/** Target plans whose mar is chosen by their own target, not their preparer (the
 *  server fences a perform-steps winner out of it): `planMarIsTargetDriven`. */
const MAR_STAYS_WITH_TARGET: ReadonlySet<PlanType> = new Set<PlanType>([
	'exchange_courtiers', 'spread_rumors', 'propose_duel',
]);

/**
 * What one of a demand's four options does against a target plan of this type, in a
 * sentence, or why it does nothing. `target` is the player who prepared the target plan.
 * The "What can happen" before the dice and the draft's cards (after them) both say it
 * with this, so the two can't disagree.
 */
export function demandOptionLine(opt: DemandOption, planType: PlanType, target: string): string {
	const inert = demandOptionInertReason(opt, planType);
	if (inert) return `No effect here: ${inert}`;
	switch (opt) {
		case 'control_leverage':
			return `Choose how many of ${target}’s assets go onto their roll.`;
		case 'keep_or_change_target':
			return RE_AIMED[planType] ?? 'Keep or change who the plan is aimed at.';
		case 'keep_assets':
			return `Take ${KEPT_ASSETS[planType] ?? 'what the plan gives its preparer'}, in place of ${target}.`;
		case 'perform_steps': {
			// A plan with no roll of its preparer's own has no "once they have rolled".
			if (planType === 'clandestinely_liaise') return `Make the liaison’s choices in ${target}’s place, and move it on.`;
			if (planType === 'host_festivity') return `Make the host’s choices in ${target}’s place, including when the event ends.`;
			// The bouts come before the duel's roll, so the preparer plays them; the stand-in
			// starts once it has resolved, choosing which staked assets a make takes. The target's
			// seat is never moved [RULINGS, Make Demands: perform-steps is post-roll only].
			if (planType === 'propose_duel') {
				return `Choose which staked assets a make takes, in ${target}’s place, once the duel’s roll has landed. ${target} still plays the bouts. A mar’s claim stays with the plan’s own target.`;
			}
			// Propose Decree has no options to pick: the stand-in enacts the law, and places the
			// addendum when the preparer is the signatory (a signatory who is someone else keeps it).
			if (planType === 'propose_decree') {
				return `Enact the law in ${target}’s place once the dice have landed, and place the addendum too when ${target} is the signatory.`;
			}
			const base = `Pick the make or mar options for ${target} once they have rolled.`;
			return MAR_STAYS_WITH_TARGET.has(planType) ? `${base} A mar’s choice stays with the plan’s own target.` : base;
		}
	}
}

/**
 * "What can happen" for a Make Demands, against the plan it targets. The four options
 * are the book's, each said for this target: what it does, or that it does nothing, with
 * the same reason the draft's cards give (`demandOptionInertReason`). `demander` is who
 * prepared the demand; `target` is who prepared the plan it is aimed at.
 */
export function demandBuffet(targetPlanType: PlanType, who: { demander: string; target: string }): BuffetTab[] {
	return [
		{
			key: 'make', label: 'Make',
			intro: `${who.demander} and ${who.target} take turns choosing one option at a time, whoever is higher on the power track first, until all four are taken. Against ${PLAN_SHORT[targetPlanType]}:`,
			opts: DEMAND_OPTIONS.map(opt => ({
				key: opt,
				label: DEMAND_OPTION_LABELS[opt].split(' — ')[0],
				desc: `— ${demandOptionLine(opt, targetPlanType, who.target)}`,
			})),
		},
		{
			key: 'mar', label: 'Mar',
			always: `${who.target} rebuffs the demand and places one on ${who.demander} in return: a demand against one of ${who.demander}’s plans straight away, or against the next plan ${who.demander} prepares.`,
		},
	];
}
