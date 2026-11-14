import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import {
	activeWarPlans, dedupeWars, keepOpenWar, nameWarInTakeover, PEACE_VOTE_LABEL, peaceVoteLabel, SETTLE_WARS_LABEL,
	warGateActive, warGatePlans, warPanelHidden,
} from './warGate';
import type { Plan, RowState, WarStateResponse } from '$lib/api';

// Minimal builders; only the fields the helpers read matter.
function plan(id: number, over: Partial<Plan> = {}): Plan {
	return { id, plan_type: 'make_war', status: 'resolved', row_number: 2, ...over } as Plan;
}
function war(originPlanID: number, over: Partial<WarStateResponse> = {}): WarStateResponse {
	return {
		war_id: originPlanID * 10,
		origin_plan_id: originPlanID,
		status: 'active',
		outstanding_costs: [],
		open_claims: [],
		...over,
	} as WarStateResponse;
}
const owes = [{ payer_id: 1, opponent_id: 2 }];
const rs = (kind: string): RowState => ({ kind }) as RowState;

describe('warGateActive', () => {
	it('is true for the two war gates the war panel can discharge', () => {
		expect(warGateActive(rs('await_battle_cost'))).toBe(true);
		expect(warGateActive(rs('await_surrender_claim'))).toBe(true);
	});

	it('is false for every other kind, and while the row state is loading', () => {
		for (const kind of ['scene_setting', 'plan_resolving', 'await_delay_reveal', 'await_dice_roll']) {
			expect(warGateActive(rs(kind))).toBe(false);
		}
		expect(warGateActive(null)).toBe(false);
		expect(warGateActive(undefined)).toBe(false);
	});
});

describe('warGatePlans', () => {
	// The bug this guards: the cost of battle falls on rows AFTER the war's own,
	// when the Make War plan is long since 'resolved'. A routing that depended on
	// the plan still resolving (as the plan registry does) shows nothing here.
	it('routes a RESOLVED plan whose war owes a cost — the original blank play area', () => {
		const out = warGatePlans(rs('await_battle_cost'), [war(7, { outstanding_costs: owes })], [plan(7)]);
		expect(out.map(p => p.id)).toEqual([7]);
	});

	it('routes a surrender claim, which carries no war_id on the row state', () => {
		const claims = [{ id: 1, surrendered_id: 1, claimant_id: 2 }];
		const out = warGatePlans(rs('await_surrender_claim'), [war(7, { open_claims: claims })], [plan(7)]);
		expect(out.map(p => p.id)).toEqual([7]);
	});

	// The nightly e2e failure: GET /wars once listed a war in both its "active" and
	// "ended, claims open" groups, the client held it twice, and the play area's
	// keyed {#each} over these plans threw each_key_duplicate.
	it('never returns the same plan twice, even if the same war is listed twice', () => {
		const claims = [{ id: 1, surrendered_id: 1, claimant_id: 2 }];
		const wars = [war(7, { open_claims: claims }), war(7, { status: 'ended', open_claims: claims })];
		const out = warGatePlans(rs('await_surrender_claim'), wars, [plan(7)]);
		expect(out.map(p => p.id)).toEqual([7]);
	});

	it('surfaces every war that owes something, not just the one the row state names', () => {
		const wars = [war(7, { outstanding_costs: owes }), war(8, { outstanding_costs: owes })];
		const out = warGatePlans(rs('await_battle_cost'), wars, [plan(7), plan(8)]);
		expect(out.map(p => p.id)).toEqual([7, 8]);
	});

	it('skips a war with nothing outstanding, and an ended war that owes a cost', () => {
		const wars = [
			war(7),
			war(8, { status: 'ended', outstanding_costs: owes }),
			war(9, { outstanding_costs: owes }),
		];
		const out = warGatePlans(rs('await_battle_cost'), wars, [plan(7), plan(8), plan(9)]);
		expect(out.map(p => p.id)).toEqual([9]);
	});

	// A surrender that empties a side ends the war in the same call that opens
	// its claims (every 1-v-1 war), and the claims keep the row on
	// await_surrender_claim. Skipping ended wars left the claimant a gated row
	// and no form to clear it.
	it('routes an ENDED war that still has an open surrender claim', () => {
		const claims = [{ id: 1, surrendered_id: 1, claimant_id: 2 }];
		const wars = [
			war(7, { status: 'ended', open_claims: claims }),
			war(8, { status: 'ended' }),
		];
		const out = warGatePlans(rs('await_surrender_claim'), wars, [plan(7), plan(8)]);
		expect(out.map(p => p.id)).toEqual([7]);
	});

	it('returns nothing when no war gate is open', () => {
		const wars = [war(7, { outstanding_costs: owes })];
		expect(warGatePlans(rs('scene_setting'), wars, [plan(7)])).toEqual([]);
		expect(warGatePlans(null, wars, [plan(7)])).toEqual([]);
	});

	it('returns nothing while the war list is still loading, rather than a placeholder', () => {
		expect(warGatePlans(rs('await_battle_cost'), [], [plan(7)])).toEqual([]);
	});

	it('drops a war whose origin plan is not in the plan list', () => {
		expect(warGatePlans(rs('await_battle_cost'), [war(7, { outstanding_costs: owes })], [])).toEqual([]);
	});
});

describe('activeWarPlans', () => {
	it('keeps a war that is under way, resolved or still pending', () => {
		const plans = [plan(7), plan(8, { status: 'pending', row_number: 5 })];
		expect(activeWarPlans(plans, [war(7), war(8)]).map(p => p.id)).toEqual([7, 8]);
	});

	// "Active Wars (1)" over an empty sheet: listWars only returns ACTIVE wars,
	// so a finished war has no entry — and the old filter, which looked for an
	// entry marked 'ended', could never match it.
	it('drops a finished war, which is simply absent from the active-war list', () => {
		expect(activeWarPlans([plan(7)], [])).toEqual([]);
	});

	// The server also lists an ended war while a surrender claim is open on it,
	// so being in the list no longer means the war is under way.
	it('drops an ended war that is listed only for its open claim', () => {
		const claims = [{ id: 1, surrendered_id: 1, claimant_id: 2 }];
		expect(activeWarPlans([plan(7)], [war(7, { status: 'ended', open_claims: claims })])).toEqual([]);
	});

	it('drops a cancelled declaration and a plan still in its delay reveal', () => {
		const plans = [plan(7, { status: 'cancelled' }), plan(8, { row_number: null })];
		expect(activeWarPlans(plans, [war(7), war(8)])).toEqual([]);
	});

	it('ignores other plan types', () => {
		expect(activeWarPlans([plan(7, { plan_type: 'seek_answers' })], [war(7)])).toEqual([]);
	});
});

describe('peaceVoteLabel', () => {
	const proposal = (awaiting: number[]) =>
		({ id: 1, proposer_id: 1, terms: 'Peace.', status: 'open', votes: [], awaiting }) as unknown as WarStateResponse['open_proposal'];
	// A war owing a cost: payer 1 pays first. With `awaiting`, a peace proposal is open.
	const owing = (id: number, payer: number, awaiting?: number[]) => war(id, {
		outstanding_costs: [{ payer_id: payer, opponent_id: 99 }],
		open_proposal: awaiting ? proposal(awaiting) : null,
	});
	const onCost = (warID?: number): RowState => ({ kind: 'await_battle_cost', war_id: warID }) as RowState;

	// A peace vote is the same row state as a payment, so without this the heading
	// told the voter "Your move — pay the cost of battle".
	it('says "vote on the peace proposal" while the one war that owes has a proposal open', () => {
		expect(peaceVoteLabel(onCost(70), [owing(7, 1, [2])], 2)).toBe(PEACE_VOTE_LABEL);
		expect(PEACE_VOTE_LABEL).toBe('vote on the peace proposal');
	});

	it('leaves the payment label alone when no proposal is open', () => {
		expect(peaceVoteLabel(onCost(70), [owing(7, 1)], 1)).toBeNull();
	});

	it('does nothing for any other row state, or before the wars have loaded', () => {
		const w = owing(7, 1, [2]);
		expect(peaceVoteLabel(rs('scene_setting'), [w], 2)).toBeNull();
		expect(peaceVoteLabel(null, [w], 2)).toBeNull();
		expect(peaceVoteLabel(onCost(70), [], 2)).toBeNull();
	});

	// Two wars can owe on one row. The row state names one of them, and which one
	// is arbitrary (the server picks from a map), while acting_player_ids covers
	// both. So the label must not depend on the war the row names, or every heading
	// would flip between two answers each time the row state is recomputed.
	describe('with two wars owing on the same row', () => {
		// War A is at a vote (player 2 and 3 must vote); war B is at a payment (player 4 pays).
		const a = owing(7, 1, [2, 3]);
		const b = owing(8, 4);

		it('is the same whichever war the row state names, and in whatever order the wars come', () => {
			for (const viewer of [2, 3, 4, 5]) {
				const answers = new Set([
					peaceVoteLabel(onCost(a.war_id), [a, b], viewer),
					peaceVoteLabel(onCost(b.war_id), [a, b], viewer),
					peaceVoteLabel(onCost(undefined), [b, a], viewer),
					peaceVoteLabel({ kind: 'await_battle_cost' } as RowState, [b, a], viewer),
				]);
				expect(answers.size, `viewer ${viewer}`).toBe(1);
			}
		});

		it('tells each player their own business: a voter votes, the payer pays', () => {
			expect(peaceVoteLabel(onCost(), [a, b], 2)).toBe(PEACE_VOTE_LABEL);
			expect(peaceVoteLabel(onCost(), [a, b], 3)).toBe(PEACE_VOTE_LABEL);
			expect(peaceVoteLabel(onCost(), [a, b], 4)).toBeNull();
		});

		it('says something true of both to a player who is waiting on both, or on neither', () => {
			expect(peaceVoteLabel(onCost(), [a, b], 5)).toBe(SETTLE_WARS_LABEL);
			// One player who must vote on A and pay for B.
			const both = owing(9, 2);
			expect(peaceVoteLabel(onCost(), [a, both], 2)).toBe(SETTLE_WARS_LABEL);
		});

		it('is the vote label for everyone once both wars are at a vote', () => {
			const c = owing(8, 4, [5]);
			for (const viewer of [1, 2, 4, 5, 6]) expect(peaceVoteLabel(onCost(), [a, c], viewer)).toBe(PEACE_VOTE_LABEL);
		});
	});

	it('ignores a war that is not blocking the row (nothing owed, or ended)', () => {
		const idle = war(9, { open_proposal: proposal([2]) } as Partial<WarStateResponse>); // no outstanding costs
		expect(peaceVoteLabel(onCost(), [owing(7, 1), idle], 2)).toBeNull();
		const ended = war(9, { status: 'ended', outstanding_costs: owes, open_proposal: proposal([2]) } as Partial<WarStateResponse>);
		expect(peaceVoteLabel(onCost(), [owing(7, 1), ended], 2)).toBeNull();
	});
});

// The sheet that is open is stored as a plan id. When its war leaves the list the
// id must be cleared, not just hidden: a failed war fetch empties the list (and
// closes the sheet), and the next good fetch brings the same war back.
describe('keepOpenWar', () => {
	it('keeps the open sheet while its war is live, and shows none when none is open', () => {
		expect(keepOpenWar(7, [7, 8])).toBe(7);
		expect(keepOpenWar(null, [7, 8])).toBeNull();
	});

	it('closes the sheet when its war drops out of the list', () => {
		expect(keepOpenWar(7, [8])).toBeNull();
		expect(keepOpenWar(7, [])).toBeNull();
	});

	it('does not reopen it when the war comes back: the stored id was cleared', () => {
		let open: number | null = 7;
		open = keepOpenWar(open, []);      // the war list failed to load and came back empty
		open = keepOpenWar(open, [7]);     // the next fetch lists the war again
		expect(open).toBeNull();
	});
});

describe('nameWarInTakeover', () => {
	const band = new Set([7, 8]);

	it('is not needed for one war under its own band', () => {
		expect(nameWarInTakeover(7, [7], band)).toBe(false);
	});

	it('is needed for each panel when two wars owe at once', () => {
		expect(nameWarInTakeover(7, [7, 8], band)).toBe(true);
		expect(nameWarInTakeover(8, [7, 8], band)).toBe(true);
	});

	it('is needed for a war with no band: one that has just ended and left a claim', () => {
		expect(nameWarInTakeover(9, [9], band)).toBe(true);
	});
});

describe('warPanelHidden', () => {
	const claims = [{ id: 1, surrendered_id: 1, claimant_id: 2 }];

	it('hides a resolved plan whose war has ended', () => {
		expect(warPanelHidden(plan(7), war(7, { status: 'ended' }))).toBe(true);
	});

	// A surrender that empties a side ends the war in the same call that opens
	// its claims; reverting this exception strands the claimant.
	it('stays up while an ended war still has an open surrender claim', () => {
		expect(warPanelHidden(plan(7), war(7, { status: 'ended', open_claims: claims }))).toBe(false);
	});

	it('stays up for a war under way, an unresolved plan, or a war still loading', () => {
		expect(warPanelHidden(plan(7), war(7))).toBe(false);
		expect(warPanelHidden(plan(7, { status: 'resolving' }), war(7, { status: 'ended' }))).toBe(false);
		expect(warPanelHidden(plan(7), null)).toBe(false);
		expect(warPanelHidden(null, war(7, { status: 'ended' }))).toBe(false);
	});
});

// The pure functions above only protect the play area if the view still calls
// them. MainEventView is too heavy to render without a browser and its war list
// arrives through an async fetch, so this guards the wiring at the source: the
// takeover branch must exist, render the Make War panel, and sit AHEAD of the
// scene branches — the server's precedence puts the war gate above them.
describe('MainEventView war-gate wiring', () => {
	const src = readFileSync(join(__dirname, 'components/phases/MainEventView.svelte'), 'utf8');

	it('renders the Make War panel from a war-gate branch', () => {
		const branch = src.indexOf('{:else if warGateTakeover}');
		expect(branch, 'the {:else if warGateTakeover} branch is gone').toBeGreaterThan(-1);
		const body = src.slice(branch, src.indexOf('{:else if', branch + 10));
		expect(body).toContain('planType="make_war"');
		expect(body).toContain('warGateWarPlans');
	});

	it('puts the war gate ahead of the scene branches', () => {
		const gate = src.indexOf('{:else if warGateTakeover}');
		expect(gate).toBeGreaterThan(-1);
		expect(gate).toBeLessThan(src.indexOf("{:else if activeScene && activeScene.kind === 'turn'}"));
		expect(gate).toBeLessThan(src.indexOf("{:else if rowState?.kind === 'scene_setting'"));
	});

	it('puts a band for each live war above the open step’s heading, and no drawer is left', () => {
		const band = src.indexOf('<WarBands');
		expect(band, 'the war bands are not mounted').toBeGreaterThan(-1);
		expect(band).toBeLessThan(src.indexOf('<StepHeading {heading} />'));
		expect(src).not.toMatch(/warDrawer|RetinueSheet|Active Wars/);
	});

	it('derives the takeover from the shared helpers', () => {
		expect(src).toMatch(/warGateActive\(rowState\)/);
		expect(src).toMatch(/warGatePlans\(rowState, wars, plans\)/);
		expect(src).toMatch(/activeWarPlans\(plans, wars\)/);
	});
});

// The panel's war arrives through an onMount fetch, so the server renderer
// can't put it in a given state; guard that it still asks warPanelHidden.
describe('MakeWarPanel hide wiring', () => {
	const src = readFileSync(join(__dirname, 'components/plans/MakeWarPanel.svelte'), 'utf8');

	it('decides shouldHide through warPanelHidden', () => {
		expect(src).toMatch(/shouldHide = \$derived\([^;]*warPanelHidden\(plan, war\)/);
	});
});

describe('dedupeWars', () => {
	it('leaves a list with no repeats alone, in order', () => {
		const wars = [war(8), war(7)];
		expect(dedupeWars(wars).map(w => w.origin_plan_id)).toEqual([8, 7]);
	});

	it('keeps one entry per war, and the later (fresher) one', () => {
		const out = dedupeWars([war(7, { status: 'active' }), war(8), war(7, { status: 'ended' })]);
		expect(out.map(w => w.origin_plan_id)).toEqual([7, 8]);
		expect(out[0].status).toBe('ended');
	});
});
