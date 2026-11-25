import { describe, expect, it } from 'vitest';
import { diceNotYetLanded, planDifficulty, type DifficultyEnv } from './difficulty';

// One table can't say whether the client's reading of a difficulty matches the
// server's (game/difficulty.go, game/demands.go), so each formula is pinned here
// from the book's wording: the three "your rank" plans, the "status" plans (6 minus
// rank), Chronicle's higher-of-two, Make Introductions' 2 + peers, Spread Rumors'
// two branches, and Make Demands' target-plus-gap. Once a roll exists for the plan,
// the roll's own value is the one shown, so the client can only be wrong before it.

const ADA = 1;
const BEA = 2;
const CYR = 3;

const players = [
	{ id: ADA, display_name: 'ada' },
	{ id: BEA, display_name: 'bea' },
	{ id: CYR, display_name: 'cyr' },
] as never;

type Cat = 'power' | 'esteem' | 'knowledge';
function ranks(entries: [Cat, number, number][]) {
	return entries.map(([category, player_id, rank], i) => ({ id: i + 1, game_id: 1, category, player_id, rank })) as never;
}

function plan(over: Record<string, unknown> = {}) {
	return {
		id: 10, plan_type: 'seek_answers', status: 'resolving', preparer_id: ADA,
		target_player_id: null, target_asset_id: null, targeted_plan_id: null,
		resolution_data: null, ...over,
	} as never;
}

function env(over: Partial<DifficultyEnv> = {}): DifficultyEnv {
	return { plans: [], assets: [], players, rankings: [], activeRoll: null, ...over };
}

describe('planDifficulty: the plans whose difficulty is the preparer\'s rank', () => {
	it('Seek Answers is the preparer\'s knowledge rank', () => {
		const v = planDifficulty(plan(), env({ rankings: ranks([['knowledge', ADA, 2]]) }));
		expect(v).toMatchObject({ value: 2, floor: 2 });
		expect(v?.reason).toContain('knowledge');
		expect(v?.reason).toContain('ada');
	});

	it('Spread Propaganda is the preparer\'s esteem rank', () => {
		const v = planDifficulty(plan({ plan_type: 'spread_propaganda' }), env({ rankings: ranks([['esteem', ADA, 3]]) }));
		expect(v).toMatchObject({ value: 3, floor: 3 });
		expect(v?.reason).toContain('esteem');
	});

	it('Propose Decree is the preparer\'s power rank', () => {
		const v = planDifficulty(plan({ plan_type: 'propose_decree' }), env({ rankings: ranks([['power', ADA, 4]]) }));
		expect(v).toMatchObject({ value: 4, floor: 4 });
		expect(v?.reason).toContain('power');
	});

	it('says a lower rank is better, for every plan whose difficulty is a rank', () => {
		for (const [type, cat] of [['seek_answers', 'knowledge'], ['spread_propaganda', 'esteem'], ['propose_decree', 'power']] as const) {
			const v = planDifficulty(plan({ plan_type: type }), env({ rankings: ranks([[cat, ADA, 3]]) }));
			expect(v?.reason, type).toContain('a lower rank is better');
		}
	});

	it('is nothing at all when the rank is not known', () => {
		expect(planDifficulty(plan(), env())).toBeNull();
	});
});

describe('planDifficulty: the plans whose difficulty is a status (6 minus rank)', () => {
	it('Exchange Courtiers is the target player\'s power status', () => {
		const p = plan({ plan_type: 'exchange_courtiers', target_player_id: BEA });
		expect(planDifficulty(p, env({ rankings: ranks([['power', BEA, 2]]) }))).toMatchObject({ value: 4, floor: 4 });
		expect(planDifficulty(p, env({ rankings: ranks([['power', BEA, 5]]) }))?.value).toBe(1);
		const v = planDifficulty(p, env({ rankings: ranks([['power', BEA, 2]]) }));
		expect(v?.reason).toContain('bea');
		expect(v?.reason).toContain('status');
	});

	it('Propose Duel is the challenged player\'s esteem status', () => {
		const p = plan({ plan_type: 'propose_duel', target_player_id: CYR });
		expect(planDifficulty(p, env({ rankings: ranks([['esteem', CYR, 1]]) }))).toMatchObject({ value: 5, floor: 5 });
	});

	it('Host Festivity is the host\'s esteem status', () => {
		const p = plan({ plan_type: 'host_festivity' });
		expect(planDifficulty(p, env({ rankings: ranks([['esteem', ADA, 4]]) }))?.value).toBe(2);
	});
});

describe('planDifficulty: Spread Rumors, in its two branches', () => {
	const mc = { id: 50, owner_id: BEA, is_main_character: true };
	const other = { id: 51, owner_id: BEA, is_main_character: false };
	const rk = ranks([['esteem', ADA, 3], ['esteem', BEA, 2]]);

	it('a main character\'s difficulty is its owner\'s esteem STATUS, not the preparer\'s rank', () => {
		const v = planDifficulty(plan({ plan_type: 'spread_rumors', target_asset_id: 50 }), env({ assets: [mc] as never, rankings: rk }));
		expect(v).toMatchObject({ value: 4, floor: 4 });
		expect(v?.reason).toContain('main character');
		expect(v?.reason).toContain('bea');
	});

	it('any other asset\'s difficulty is the preparer\'s own esteem rank', () => {
		const v = planDifficulty(plan({ plan_type: 'spread_rumors', target_asset_id: 51 }), env({ assets: [other] as never, rankings: rk }));
		expect(v).toMatchObject({ value: 3, floor: 3 });
		expect(v?.reason).toContain('not a main character');
		expect(v?.reason).toContain('ada');
	});

	it('is nothing until the target asset is known', () => {
		expect(planDifficulty(plan({ plan_type: 'spread_rumors', target_asset_id: 99 }), env({ rankings: rk }))).toBeNull();
	});
});

describe('planDifficulty: Chronicle Histories, the higher of rank and artifacts invoked', () => {
	const p = plan({ plan_type: 'chronicle_histories' });
	const rk = ranks([['knowledge', ADA, 3]]);

	it('stays at the rank while the artifacts invoked do not pass it', () => {
		expect(planDifficulty(p, env({ rankings: rk }), { invokedCount: 0 })).toMatchObject({ value: 3, floor: 3 });
		expect(planDifficulty(p, env({ rankings: rk }), { invokedCount: 3 })).toMatchObject({ value: 3, floor: 3 });
	});

	it('rises past the rank only once more artifacts than that are invoked', () => {
		expect(planDifficulty(p, env({ rankings: rk }), { invokedCount: 5 })).toMatchObject({ value: 5, floor: 3 });
	});

	it('never goes above 6', () => {
		expect(planDifficulty(p, env({ rankings: rk }), { invokedCount: 9 })?.value).toBe(6);
	});

	it('reads the artifacts already invoked from the plan when nothing is being chosen', () => {
		const done = plan({
			plan_type: 'chronicle_histories',
			resolution_data: JSON.stringify({ chronicle_histories: { invoked_artifact_ids: [1, 2, 3, 4] } }),
		});
		expect(planDifficulty(done, env({ rankings: rk }))).toMatchObject({ value: 4, floor: 3 });
	});

	it('names both parts in its reason', () => {
		const v = planDifficulty(p, env({ rankings: rk }), { invokedCount: 5 });
		expect(v?.reason).toContain('knowledge rank (3;');
		expect(v?.reason).toContain('artifacts invoked (5)');
	});

	it('says a lower rank is better, as the Chronicle meter has since the first-timer pass', () => {
		expect(planDifficulty(p, env({ rankings: rk }), { invokedCount: 0 })?.reason).toContain('a lower rank is better');
	});
});

describe('planDifficulty: Make Introductions, 2 plus one per peer', () => {
	const mi = (peers: number | undefined) => plan({
		plan_type: 'make_introductions',
		resolution_data: peers === undefined ? null : JSON.stringify({ make_introductions: { peer_count: peers } }),
	});

	it('is 2 plus each peer, with the 2 as the floor', () => {
		expect(planDifficulty(mi(2), env())).toMatchObject({ value: 4, floor: 2 });
		expect(planDifficulty(mi(4), env())).toMatchObject({ value: 6, floor: 2 });
	});

	it('counts a plan with no peers recorded as one, as the server does', () => {
		expect(planDifficulty(mi(0), env())?.value).toBe(3);
		expect(planDifficulty(mi(undefined), env())?.value).toBe(3);
	});

	it('keeps its reason in the singular for one peer', () => {
		expect(planDifficulty(mi(1), env())?.reason).toContain('1 peer introduced');
	});
});

describe('planDifficulty: Make Demands, the target plan\'s own difficulty plus the power gap', () => {
	const target = (over: Record<string, unknown> = {}) => plan({ id: 20, preparer_id: BEA, plan_type: 'seek_answers', ...over });
	const demand = plan({ id: 21, preparer_id: ADA, plan_type: 'make_demands', targeted_plan_id: 20 });

	it('adds the gap in power when the target player ranks above the demander', () => {
		// seek answers by bea (knowledge 2) -> 2; bea power 1, ada power 4 -> gap 3.
		const v = planDifficulty(demand, env({
			plans: [target(), demand],
			rankings: ranks([['knowledge', BEA, 2], ['power', BEA, 1], ['power', ADA, 4]]),
		}));
		expect(v).toMatchObject({ value: 5, floor: 2 });
		expect(v?.reason).toContain('Seek Answers');
		expect(v?.reason).toContain('adds 3');
	});

	it('adds nothing when the target player does not outrank the demander', () => {
		const v = planDifficulty(demand, env({
			plans: [target(), demand],
			rankings: ranks([['knowledge', BEA, 2], ['power', BEA, 4], ['power', ADA, 1]]),
		}));
		expect(v).toMatchObject({ value: 2, floor: 2 });
		expect(v?.reason).toContain('nothing');
	});

	it('is capped at 6', () => {
		// A duel on a rank-1 esteem target is 5, and a rank-5 demander against a
		// rank-1 preparer adds 4: 9 on paper, 6 in play.
		const duel = target({ plan_type: 'propose_duel', target_player_id: CYR });
		const v = planDifficulty(demand, env({
			plans: [duel, demand],
			rankings: ranks([['esteem', CYR, 1], ['power', BEA, 1], ['power', ADA, 5]]),
		}));
		expect(v?.value).toBe(6);
		expect(v?.floor).toBe(5);
		expect(v?.reason).toContain('never above 6');
	});

	it('against a Clandestinely Liaise only the gap counts, and never below 1', () => {
		const liaise = target({ plan_type: 'clandestinely_liaise' });
		const none = planDifficulty(demand, env({
			plans: [liaise, demand],
			rankings: ranks([['power', BEA, 4], ['power', ADA, 1]]),
		}));
		expect(none).toMatchObject({ value: 1, floor: 0 });
		expect(none?.reason).toContain('no roll of its own');
		const gap = planDifficulty(demand, env({
			plans: [liaise, demand],
			rankings: ranks([['power', BEA, 1], ['power', ADA, 3]]),
		}));
		expect(gap).toMatchObject({ value: 2, floor: 0 });
	});

	it('shows the roll\'s own value once the demand has rolled, keeping the target\'s part as the floor', () => {
		const roll = { id: 3, plan_id: 21, difficulty: 6, adjusted_difficulty: null } as never;
		const v = planDifficulty(demand, env({
			plans: [target(), demand],
			rankings: ranks([['knowledge', BEA, 2], ['power', BEA, 1], ['power', ADA, 4]]),
			activeRoll: roll,
		}));
		expect(v).toMatchObject({ value: 6, floor: 2 });
	});

	it('is nothing when the target plan is not on the record', () => {
		expect(planDifficulty(demand, env({ plans: [demand] }))).toBeNull();
	});
});

describe('planDifficulty: the roll wins once there is one', () => {
	const rk = ranks([['knowledge', ADA, 2]]);

	it('uses the roll made for this plan, not the formula, and keeps the formula\'s reason while they agree', () => {
		const roll = { id: 3, plan_id: 10, difficulty: 2, adjusted_difficulty: null } as never;
		const v = planDifficulty(plan(), env({ rankings: rk, activeRoll: roll }));
		expect(v?.value).toBe(2);
		expect(v?.reason).toContain('knowledge');
	});

	it('ignores a roll that belongs to another plan', () => {
		const roll = { id: 3, plan_id: 77, difficulty: 6, adjusted_difficulty: null } as never;
		expect(planDifficulty(plan(), env({ rankings: rk, activeRoll: roll }))?.value).toBe(2);
	});

	it('says nothing of the formula when the roll disagrees with it: the reason would explain a number that is not on screen', () => {
		// A Chronicle plan not yet refetched after the cast still counts no artifacts invoked.
		const roll = { id: 3, plan_id: 10, difficulty: 4, adjusted_difficulty: null } as never;
		const v = planDifficulty(plan(), env({ rankings: rk, activeRoll: roll }));
		expect(v?.value).toBe(4);
		expect(v?.reason).toBe('');
	});

	it('falls back to the roll\'s own value, with no reason, when the formula cannot be worked out', () => {
		// A Spread Rumors target that is not among the assets this client holds.
		const rumor = plan({ plan_type: 'spread_rumors', target_asset_id: 99 });
		const roll = { id: 3, plan_id: 10, difficulty: 4, adjusted_difficulty: null } as never;
		expect(planDifficulty(rumor, env({ rankings: rk }))).toBeNull();
		expect(planDifficulty(rumor, env({ rankings: rk, activeRoll: roll }))).toMatchObject({ value: 4, floor: 4, reason: '' });
	});
});

// The difficulty vote is a hidden ballot: the adjusted difficulty is written only when the last
// vote lands (the same step that reveals it), so a roll carries it as null until then. Once it
// is there the dice panel says "5, up from 3 by the vote", and a meter beside it saying 3
// would be two numbers labelled "difficulty" that disagree: so the meter follows the vote.
describe('planDifficulty: the difficulty vote', () => {
	const rk = ranks([['knowledge', ADA, 3]]);
	const rolled = (difficulty: number, adjusted: number | null) =>
		({ id: 3, plan_id: 10, difficulty, adjusted_difficulty: adjusted, stage: adjusted == null ? 'voting' : 'leverage' }) as never;

	it('shows the vote\'s outcome once it has revealed, and says the vote moved it', () => {
		const v = planDifficulty(plan(), env({ rankings: rk, activeRoll: rolled(3, 5) }));
		expect(v?.value).toBe(5);
		expect(v?.reason).toContain('The vote moved it from 3 to 5.');
		// The part the plan sets is still the floor; what the vote added reads as the rest.
		expect(v?.floor).toBe(3);
	});

	it('follows a vote that lowered it, and keeps the floor under the value', () => {
		const v = planDifficulty(plan(), env({ rankings: rk, activeRoll: rolled(3, 1) }));
		expect(v?.value).toBe(1);
		expect(v?.floor).toBe(1);
		expect(v?.reason).toContain('The vote moved it from 3 to 1.');
	});

	it('says nothing of a vote that changed nothing', () => {
		const v = planDifficulty(plan(), env({ rankings: rk, activeRoll: rolled(3, 3) }));
		expect(v?.value).toBe(3);
		expect(v?.reason).not.toContain('vote');
	});

	it('never shows an adjusted value before the reveal: a roll still voting carries none, so the base shows', () => {
		const v = planDifficulty(plan(), env({ rankings: rk, activeRoll: rolled(3, null) }));
		expect(v?.value).toBe(3);
		expect(v?.reason).not.toContain('vote');
	});

	it('still says it when the formula could not be worked out', () => {
		const rumor = plan({ plan_type: 'spread_rumors', target_asset_id: 99 });
		const v = planDifficulty(rumor, env({ rankings: rk, activeRoll: rolled(3, 5) }));
		expect(v).toMatchObject({ value: 5, reason: 'The vote moved it from 3 to 5.' });
	});
});

// Every panel shows the meter and its "What can happen" until THIS plan's dice land. The
// page's `rollOutcome` follows whichever plan-linked roll is on the table, so a gate on it
// alone brings them back when another plan's roll opens (a counter-propaganda rolls while
// the plan it answers still owes steps).
describe('diceNotYetLanded', () => {
	const open = (over: Record<string, unknown> = {}) => ({ id: 3, plan_id: 10, outcome: null, ...over }) as never;
	const ctx = (rollOutcome: 'make' | 'mar' | null, activeRoll: unknown) => ({ rollOutcome, activeRoll }) as never;

	it('is true before any roll, and while this plan\'s roll is out', () => {
		expect(diceNotYetLanded(plan(), ctx(null, null))).toBe(true);
		expect(diceNotYetLanded(plan(), ctx(null, open()))).toBe(true);
	});

	it('is false once this plan\'s roll has an outcome, from either source', () => {
		expect(diceNotYetLanded(plan(), ctx('mar', open()))).toBe(false);
		expect(diceNotYetLanded(plan(), ctx(null, open({ outcome: 'make' })))).toBe(false);
		expect(diceNotYetLanded(plan(), ctx('make', null))).toBe(false);
	});

	it('is false once the plan\'s choices are committed, even though another plan\'s roll is open', () => {
		const done = plan({ resolution_data: JSON.stringify({ choice_submitted: true }) });
		const other = open({ plan_id: 77 });
		expect(diceNotYetLanded(done, ctx(null, other))).toBe(false);
	});

	it('is true for a plan that has not rolled when another plan\'s roll has landed', () => {
		expect(diceNotYetLanded(plan(), ctx('make', open({ plan_id: 77, outcome: 'make' })))).toBe(true);
	});
});

describe('planDifficulty: plans with no roll of their own', () => {
	it('Make War and Clandestinely Liaise have no difficulty to show', () => {
		expect(planDifficulty(plan({ plan_type: 'make_war' }), env())).toBeNull();
		expect(planDifficulty(plan({ plan_type: 'clandestinely_liaise' }), env())).toBeNull();
	});
});
