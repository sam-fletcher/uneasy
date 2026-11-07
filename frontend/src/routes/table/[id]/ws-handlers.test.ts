import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { handleWSMessage, type WSContext } from './ws-handlers';
import { EventTypes } from '$lib/ws';
import { breakableAssets } from '$lib/components/plans/shared';
import { atRiskCountByPlayer } from '$lib/tableHeader';
import { getPlan, getRoll, type DiceRoll, type DiceRollDie, type Plan, type RecordRow } from '$lib/api';

// Only getPlan and getRoll are stubbed; the rest of the client is never reached
// by these tests.
vi.mock('$lib/api', async importOriginal => ({
	...(await importOriginal<typeof import('$lib/api')>()),
	getPlan: vi.fn(),
	getRoll: vi.fn(),
}));
const getPlanMock = vi.mocked(getPlan);
const getRollMock = vi.mocked(getRoll);
beforeEach(() => {
	getPlanMock.mockReset();
	getPlanMock.mockRejectedValue(new Error('unstubbed'));
	getRollMock.mockReset();
	getRollMock.mockRejectedValue(new Error('unstubbed'));
});

// Minimal Plan factory — only the fields the record-sync logic reads.
function makePlan(over: Partial<Plan> = {}): Plan {
	return {
		id: 1,
		game_id: 1,
		plan_type: 'seek_answers',
		category: 'might',
		preparer_id: 7,
		target_player_id: null,
		target_asset_id: null,
		row_number: 3,
		row_order: 0,
		prepared_at_row: 3,
		status: 'pending',
		result: null,
		resolved_at: null,
		preparation_notes: null,
		resolution_data: null,
		...over,
	} as Plan;
}

function emptyRows(): RecordRow[] {
	return [1, 2, 3, 4].map(n => ({ row_number: n, entries: [], plans: [] }));
}

// Build a ctx with only the fields these handlers touch; the rest are unused.
function makeCtx(over: Partial<WSContext> = {}): WSContext {
	return {
		plans: [],
		recordRows: emptyRows(),
		preparePlanDraft: { foo: 'bar' },
		toneWrites: new Map(),
		...over,
	} as unknown as WSContext;
}

describe('plan.prepared record sync', () => {
	it('adds the prepared plan to its record row without a reload', () => {
		const ctx = makeCtx();
		const plan = makePlan({ id: 42, row_number: 3 });

		handleWSMessage(ctx, { type: EventTypes.PlanPrepared, payload: { plan } });

		expect(ctx.plans.map(p => p.id)).toEqual([42]);
		expect(ctx.recordRows.find(r => r.row_number === 3)?.plans.map(p => p.id)).toEqual([42]);
		// Other rows are untouched.
		expect(ctx.recordRows.find(r => r.row_number === 1)?.plans).toEqual([]);
		// The highlight draft is cleared on commit.
		expect(ctx.preparePlanDraft).toBeNull();
	});

	it('skips the record patch while a variable-delay plan has no row yet', () => {
		const ctx = makeCtx();
		const plan = makePlan({ id: 42, row_number: null });

		handleWSMessage(ctx, { type: EventTypes.PlanPrepared, payload: { plan } });

		expect(ctx.plans.map(p => p.id)).toEqual([42]);
		expect(ctx.recordRows.every(r => r.plans.length === 0)).toBe(true);
	});

	it('re-broadcast with the assigned row moves the chip off any stale row', () => {
		const ctx = makeCtx();
		const onRow2 = makePlan({ id: 42, row_number: 2 });
		handleWSMessage(ctx, { type: EventTypes.PlanPrepared, payload: { plan: onRow2 } });
		expect(ctx.recordRows.find(r => r.row_number === 2)?.plans.map(p => p.id)).toEqual([42]);

		// Reveal closes and assigns the real row (4); the same plan re-broadcasts.
		const onRow4 = makePlan({ id: 42, row_number: 4 });
		handleWSMessage(ctx, { type: EventTypes.PlanPrepared, payload: { plan: onRow4 } });

		expect(ctx.recordRows.find(r => r.row_number === 2)?.plans).toEqual([]);
		expect(ctx.recordRows.find(r => r.row_number === 4)?.plans.map(p => p.id)).toEqual([42]);
		// No duplicate in the flat plan list.
		expect(ctx.plans.filter(p => p.id === 42)).toHaveLength(1);
	});

	it('orders chips by row_order, not arrival order', () => {
		const ctx = makeCtx();
		// A plan with a higher row_order arrives first (e.g. a delay-reveal war
		// landing on a busy row, or a plan re-broadcast), then one that should
		// resolve — and display — before it. Live order must match row_order so
		// the Public Record agrees with the backend resolution order.
		const second = makePlan({ id: 99, row_number: 3, row_order: 1 });
		handleWSMessage(ctx, { type: EventTypes.PlanPrepared, payload: { plan: second } });
		const first = makePlan({ id: 42, row_number: 3, row_order: 0 });
		handleWSMessage(ctx, { type: EventTypes.PlanPrepared, payload: { plan: first } });

		expect(ctx.recordRows.find(r => r.row_number === 3)?.plans.map(p => p.id))
			.toEqual([42, 99]);
	});

	it('keeps the chip status in sync on resolve', () => {
		const plan = makePlan({ id: 42, row_number: 3, status: 'pending' });
		const ctx = makeCtx({
			plans: [plan],
			recordRows: emptyRows().map(r =>
				r.row_number === 3 ? { ...r, plans: [plan] } : r
			),
		});

		handleWSMessage(ctx, {
			type: EventTypes.PlanResolved,
			payload: { plan_id: 42, result: 'make' },
		});

		const chip = ctx.recordRows.find(r => r.row_number === 3)?.plans[0];
		expect(chip?.status).toBe('resolved');
		expect(chip?.result).toBe('make');
		// And the flat plan list stays in lockstep with the backend value.
		expect(ctx.plans.find(p => p.id === 42)?.status).toBe('resolved');
	});

	it('clears the active roll when its own plan resolves', () => {
		const plan = makePlan({ id: 42, row_number: 3, status: 'resolving' });
		const ctx = makeCtx({
			plans: [plan],
			activeRoll: { id: 9, plan_id: 42, outcome: 'make' },
			activeRollDice: [{ id: 1 }],
			activeRollVotes: [{ player_id: 1 }],
			activeRollParticipants: [{ player_id: 1 }],
		} as unknown as Partial<WSContext>);

		handleWSMessage(ctx, {
			type: EventTypes.PlanResolved,
			payload: { plan_id: 42, result: 'make' },
		});

		expect(ctx.activeRoll).toBeNull();
		expect(ctx.activeRollDice).toEqual([]);
		expect(ctx.activeRollVotes).toEqual([]);
		expect(ctx.activeRollParticipants).toEqual([]);
	});

	it('leaves an active roll that belongs to a different plan', () => {
		const plan = makePlan({ id: 42, row_number: 3, status: 'resolving' });
		const otherRoll = { id: 9, plan_id: 99, outcome: null };
		const ctx = makeCtx({
			plans: [plan],
			activeRoll: otherRoll,
		} as unknown as Partial<WSContext>);

		handleWSMessage(ctx, {
			type: EventTypes.PlanResolved,
			payload: { plan_id: 42, result: 'make' },
		});

		expect(ctx.activeRoll).toBe(otherRoll);
	});
});

describe('single-plan refetch ordering', () => {
	const detail = (plan: Plan) => ({ plan }) as Awaited<ReturnType<typeof getPlan>>;
	const flush = () => new Promise(r => setTimeout(r, 0));
	// The duel events also announce themselves on window; vitest runs in node.
	beforeEach(() => { vi.stubGlobal('window', { dispatchEvent: vi.fn() }); });
	afterEach(() => { vi.unstubAllGlobals(); });

	it('a refetch that predates plan.resolved cannot roll the plan back', async () => {
		const resolving = makePlan({ id: 42, status: 'resolving' });
		const ctx = makeCtx({ plans: [resolving] });

		// The first fetch is read while the plan is still resolving and is slow to
		// land; whatever starts after the plan.resolved event sees it resolved.
		let landStale!: (d: Awaited<ReturnType<typeof getPlan>>) => void;
		getPlanMock.mockImplementationOnce(() => new Promise(res => { landStale = res; }));
		getPlanMock.mockResolvedValue(detail(makePlan({ id: 42, status: 'resolved', result: 'make' })));

		handleWSMessage(ctx, { type: EventTypes.DuelBoutsComplete, payload: { plan_id: 42 } });
		handleWSMessage(ctx, { type: EventTypes.PlanResolved, payload: { plan_id: 42, result: 'make' } });
		await flush();
		landStale(detail(resolving));
		await flush();

		expect(ctx.plans.find(p => p.id === 42)?.status).toBe('resolved');
	});

	it('the newest fetch wins when responses arrive out of order', async () => {
		const ctx = makeCtx({ plans: [makePlan({ id: 42, status: 'resolving' })] });
		const older = makePlan({ id: 42, status: 'resolving', preparation_notes: 'older' });
		const newer = makePlan({ id: 42, status: 'resolving', preparation_notes: 'newer' });

		let landOlder!: (d: Awaited<ReturnType<typeof getPlan>>) => void;
		getPlanMock.mockImplementationOnce(() => new Promise(res => { landOlder = res; }));
		getPlanMock.mockResolvedValueOnce(detail(newer));

		handleWSMessage(ctx, { type: EventTypes.DuelBoutDeclared, payload: { plan_id: 42 } });
		handleWSMessage(ctx, { type: EventTypes.DuelBoutResolved, payload: { plan_id: 42 } });
		await flush();
		landOlder(detail(older));
		await flush();

		expect(ctx.plans.find(p => p.id === 42)?.preparation_notes).toBe('newer');
	});

	it('plan.updated refetches that plan, whichever route saved it', async () => {
		const ctx = makeCtx({ plans: [makePlan({ id: 42, status: 'resolving' })] });
		getPlanMock.mockResolvedValue(detail(makePlan({ id: 42, status: 'resolving', preparation_notes: 'fresh' })));

		handleWSMessage(ctx, { type: EventTypes.PlanUpdated, payload: { plan_id: 42 } });
		await flush();

		expect(getPlanMock).toHaveBeenCalledWith(42);
		expect(ctx.plans.find(p => p.id === 42)?.preparation_notes).toBe('fresh');
	});
});

describe('asset.destroyed', () => {
	const asset = (id: number, over: Record<string, unknown> = {}) =>
		({ id, name: `a${id}`, owner_id: 1, marginalia: [], is_destroyed: false, destroyed_at: null, ...over });

	it('keeps the asset, flagged destroyed, so name lookups still resolve (as after a reload)', () => {
		const ctx = makeCtx({ assets: [asset(1), asset(2)] as never });
		handleWSMessage(ctx, { type: EventTypes.AssetDestroyed, payload: { asset_id: 2 } });
		expect(ctx.assets).toHaveLength(2);
		const gone = ctx.assets.find(a => a.id === 2)!;
		expect(gone.name).toBe('a2');
		expect(gone.is_destroyed).toBe(true);
		expect(ctx.assets.find(a => a.id === 1)!.is_destroyed).toBe(false);
	});

	it("stamps destroyed_at from the server's value, not the client clock", () => {
		const ctx = makeCtx({ assets: [asset(1)] as never });
		handleWSMessage(ctx, {
			type: EventTypes.AssetDestroyed,
			payload: { asset_id: 1, destroyed_at: '2026-01-02T03:04:05Z' },
		});
		expect(ctx.assets[0].destroyed_at).toBe('2026-01-02T03:04:05Z');
	});

	it('falls back to a client stamp when the payload has none', () => {
		const ctx = makeCtx({ assets: [asset(1)] as never });
		handleWSMessage(ctx, { type: EventTypes.AssetDestroyed, payload: { asset_id: 1 } });
		expect(ctx.assets[0].destroyed_at).toEqual(expect.any(String));
	});

	it('keeps a destroyed asset out of the break pickers and at-risk counts, and live ones in', () => {
		const marg = [{ id: 1, is_torn: false }, { id: 2, is_torn: false }];
		const ctx = makeCtx({
			assets: [
				asset(1, { asset_type: 'resource', marginalia: marg }),
				asset(2, { asset_type: 'resource', marginalia: marg }),
			] as never,
		});
		handleWSMessage(ctx, { type: EventTypes.AssetDestroyed, payload: { asset_id: 1 } });
		expect(ctx.assets.find(a => a.id === 1)!.is_destroyed).toBe(true);
		expect(breakableAssets(ctx.assets).map(a => a.id)).toEqual([2]);
		expect(atRiskCountByPlayer(ctx.assets).get(1) ?? 0).toBeLessThanOrEqual(1);
	});
});

describe('active roll fetch ordering', () => {
	type RollDetail = Awaited<ReturnType<typeof getRoll>>;
	const flush = () => new Promise(r => setTimeout(r, 0));
	// demand.leverage_set also announces itself on window; vitest runs in node.
	beforeEach(() => { vi.stubGlobal('window', { dispatchEvent: vi.fn() }); });
	afterEach(() => { vi.unstubAllGlobals(); });

	const makeRoll = (over: Partial<DiceRoll> = {}): DiceRoll => ({
		id: 9, game_id: 1, plan_id: null, row_number: 3, is_shake_up: false,
		actor_id: 7, difficulty: 3, adjusted_difficulty: null, result: null,
		outcome: null, stage: 'leverage', created_at: '', resolved_at: null,
		shake_up_category: null, ...over,
	});
	const makeDie = (id: number): DiceRollDie => ({
		id, roll_id: 9, player_id: 7, is_interference: false,
	} as DiceRollDie);
	const detail = (roll: DiceRoll, over: Partial<RollDetail> = {}): RollDetail => ({
		roll, dice: [], votes: [], participants: [], ...over,
	});
	const resolvedRoll = () => makeRoll({ outcome: 'make', result: 5, stage: 'resolved' });

	const created = (roll: DiceRoll) =>
		({ type: EventTypes.RollCreated, payload: { roll } });
	const resolved = (roll: DiceRoll, dice: DiceRollDie[]) =>
		({ type: EventTypes.RollResolved, payload: { roll, dice } });

	// The first getRoll hangs until the test lands it by hand; later calls
	// answer straight away with `then`.
	function holdFirstFetch(then: RollDetail) {
		let land!: (d: RollDetail) => void;
		getRollMock.mockImplementationOnce(() => new Promise(res => { land = res; }));
		getRollMock.mockResolvedValue(then);
		return (d: RollDetail) => land(d);
	}

	it('normal order: the fetch installs the roll, roll.resolved updates it', async () => {
		const ctx = makeCtx();
		getRollMock.mockResolvedValueOnce(detail(makeRoll(), { dice: [makeDie(1)] }));

		handleWSMessage(ctx, created(makeRoll()));
		await flush();
		expect(ctx.activeRoll?.outcome).toBeNull();
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1]);

		handleWSMessage(ctx, resolved(resolvedRoll(), [makeDie(1), makeDie(2)]));
		expect(ctx.activeRoll?.outcome).toBe('make');
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1, 2]);
		await flush();
		// A roll whose fetch had landed costs no further requests.
		expect(getRollMock).toHaveBeenCalledTimes(1);
	});

	it('roll.resolved before the fetch lands leaves the page resolved', async () => {
		const ctx = makeCtx();
		const landStale = holdFirstFetch(detail(resolvedRoll(), {
			dice: [makeDie(1), makeDie(2)],
			participants: [{ roll_id: 9, player_id: 7, intent: null, is_ready: true }],
		}));

		handleWSMessage(ctx, created(makeRoll()));
		handleWSMessage(ctx, resolved(resolvedRoll(), [makeDie(1), makeDie(2)]));

		// Resolved at once, from the event's own payload — before any fetch lands.
		expect(ctx.activeRoll?.outcome).toBe('make');
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1, 2]);

		// The fetch read before the roll resolved lands late: it must not undo that.
		landStale(detail(makeRoll(), { dice: [makeDie(1)] }));
		await flush();

		expect(ctx.activeRoll?.outcome).toBe('make');
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1, 2]);
		// A fetch started after the resolution fills in what the event doesn't carry.
		expect(ctx.activeRollParticipants.map(p => p.player_id)).toEqual([7]);
	});

	it('a stale fetch cannot re-install the unresolved roll, even if the refetch fails', async () => {
		const ctx = makeCtx();
		let landStale!: (d: RollDetail) => void;
		getRollMock.mockImplementationOnce(() => new Promise(res => { landStale = res; }));
		// Everything after the stale fetch is left rejecting (the default stub).

		handleWSMessage(ctx, created(makeRoll()));
		handleWSMessage(ctx, resolved(resolvedRoll(), [makeDie(1)]));
		landStale(detail(makeRoll()));
		await flush();

		expect(ctx.activeRoll?.outcome).toBe('make');
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1]);
	});

	it('roll.resolved replaces an older roll still on screen while the new roll fetches', async () => {
		const older = makeRoll({ id: 4, outcome: 'mar', stage: 'resolved' });
		const ctx = makeCtx({
			activeRoll: older,
			activeRollDice: [makeDie(40)],
			activeRollVotes: [{ roll_id: 4, player_id: 7, voted: true }],
			activeRollParticipants: [{ roll_id: 4, player_id: 7, intent: null, is_ready: true }],
		} as unknown as Partial<WSContext>);
		const landStale = holdFirstFetch(detail(resolvedRoll(), { dice: [makeDie(1)] }));

		handleWSMessage(ctx, created(makeRoll()));
		handleWSMessage(ctx, resolved(resolvedRoll(), [makeDie(1)]));

		// Straight away, before any fetch lands: the new roll is on screen, and
		// the older roll's ballot and participants are not under it. (A later
		// fetch would also overwrite them, so this must be read before it lands.)
		expect(ctx.activeRoll?.id).toBe(9);
		expect(ctx.activeRoll?.outcome).toBe('make');
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1]);
		expect(ctx.activeRollVotes).toEqual([]);
		expect(ctx.activeRollParticipants).toEqual([]);

		landStale(detail(makeRoll()));
		await flush();
		expect(ctx.activeRoll?.outcome).toBe('make');
	});

	it('roll.resolved for a roll nobody is loading is still ignored', () => {
		const shown = makeRoll({ id: 4 });
		const ctx = makeCtx({ activeRoll: shown } as unknown as Partial<WSContext>);

		handleWSMessage(ctx, resolved(resolvedRoll(), [makeDie(1)]));

		expect(ctx.activeRoll).toBe(shown);
		expect(getRollMock).not.toHaveBeenCalled();
	});

	it('the newest fetch wins when responses arrive out of order', async () => {
		const ctx = makeCtx();
		let landOlder!: (d: RollDetail) => void;
		getRollMock.mockImplementationOnce(() => new Promise(res => { landOlder = res; }));
		getRollMock.mockResolvedValueOnce(detail(makeRoll(), { dice: [makeDie(1), makeDie(2)] }));

		handleWSMessage(ctx, created(makeRoll()));
		// A leverage die lands while the first fetch is out: the event can't be
		// patched in (no roll on screen yet), so it takes a fresh fetch instead.
		handleWSMessage(ctx, {
			type: EventTypes.RollLeverageAdded,
			payload: { roll_id: 9, die: makeDie(2) },
		});
		await flush();
		landOlder(detail(makeRoll(), { dice: [makeDie(1)] }));
		await flush();

		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1, 2]);
	});

	it('a failed fetch falls back to the event payload, unless something newer has landed', async () => {
		const ctx = makeCtx();
		getRollMock.mockRejectedValueOnce(new Error('offline'));
		handleWSMessage(ctx, created(makeRoll()));
		await flush();
		expect(ctx.activeRoll?.id).toBe(9);
		expect(ctx.activeRollDice).toEqual([]);

		// Now the same failure racing a resolution: the fallback is the
		// unresolved roll, so it must not overwrite the resolved one.
		const raced = makeCtx();
		let failStale!: (e: Error) => void;
		getRollMock.mockImplementationOnce(() => new Promise((_, rej) => { failStale = rej; }));
		handleWSMessage(raced, created(makeRoll()));
		handleWSMessage(raced, resolved(resolvedRoll(), [makeDie(1)]));
		failStale(new Error('offline'));
		await flush();
		expect(raced.activeRoll?.outcome).toBe('make');
	});

	it("a plan-linked roll's fetch cannot bring the roll back after its plan resolved", async () => {
		const plan = makePlan({ id: 42, status: 'resolving' });
		const ctx = makeCtx({ plans: [plan] });
		const landStale = holdFirstFetch(detail(resolvedRoll()));
		getPlanMock.mockResolvedValue({ plan: makePlan({ id: 42, status: 'resolved', result: 'make' }) } as Awaited<ReturnType<typeof getPlan>>);
		const linked = makeRoll({ plan_id: 42 });

		handleWSMessage(ctx, created(linked));
		handleWSMessage(ctx, resolved({ ...resolvedRoll(), plan_id: 42 }, [makeDie(1)]));
		handleWSMessage(ctx, { type: EventTypes.PlanResolved, payload: { plan_id: 42, result: 'make' } });
		landStale(detail(linked));
		await flush();

		expect(ctx.activeRoll).toBeNull();
		expect(ctx.activeRollDice).toEqual([]);
	});

	it("a plan-linked roll's failed fetch cannot bring the roll back after its plan resolved", async () => {
		const ctx = makeCtx({ plans: [makePlan({ id: 42, status: 'resolving' })] });
		getPlanMock.mockResolvedValue({ plan: makePlan({ id: 42, status: 'resolved', result: 'make' }) } as Awaited<ReturnType<typeof getPlan>>);
		let landFirst!: (d: RollDetail) => void;
		getRollMock.mockImplementationOnce(() => new Promise(res => { landFirst = res; }));
		// The fetch the resolution starts is left rejecting (the default stub): its
		// fallback is the announced roll, which must not outlive the plan either.
		const linked = makeRoll({ plan_id: 42 });

		handleWSMessage(ctx, created(linked));
		handleWSMessage(ctx, resolved({ ...resolvedRoll(), plan_id: 42 }, [makeDie(1)]));
		handleWSMessage(ctx, { type: EventTypes.PlanResolved, payload: { plan_id: 42, result: 'make' } });
		landFirst(detail(linked));
		await flush();

		expect(ctx.activeRoll).toBeNull();
		expect(ctx.activeRollDice).toEqual([]);
	});

	it('demand.leverage_set on the roll on screen cannot let a late answer undo roll.resolved', async () => {
		const ctx = makeCtx({
			activeRoll: makeRoll(),
			activeRollDice: [makeDie(1)],
		} as unknown as Partial<WSContext>);
		getPlanMock.mockResolvedValue({ plan: makePlan({ id: 42 }) } as Awaited<ReturnType<typeof getPlan>>);
		const landStale = holdFirstFetch(detail(resolvedRoll(), { dice: [makeDie(1), makeDie(2)] }));

		handleWSMessage(ctx, { type: EventTypes.DemandLeverageSet, payload: { plan_id: 42, roll_id: 9 } });
		handleWSMessage(ctx, resolved(resolvedRoll(), [makeDie(1), makeDie(2)]));
		// The leverage fetch was read before the roll resolved and lands last.
		landStale(detail(makeRoll(), { dice: [makeDie(1)] }));
		await flush();

		expect(ctx.activeRoll?.outcome).toBe('make');
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1, 2]);
	});

	it("an event about another roll leaves the loading roll's fetch alone", async () => {
		const ctx = makeCtx();
		const landFirst = holdFirstFetch(detail(makeRoll(), { dice: [makeDie(99)] }));

		handleWSMessage(ctx, created(makeRoll()));
		handleWSMessage(ctx, {
			type: EventTypes.RollLeverageAdded,
			payload: { roll_id: 4, die: makeDie(2) },
		});
		landFirst(detail(makeRoll(), { dice: [makeDie(1)] }));
		await flush();

		expect(getRollMock).toHaveBeenCalledTimes(1);
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([1]);
	});

	// Every event about a roll that has nothing on screen to patch while the
	// roll's own fetch is out must supersede that fetch: its answer predates the
	// event. A new roll event that skips this goes unnoticed without this table.
	const rollEvents: [string, string, Record<string, unknown>][] = [
		['roll.leverage_added', EventTypes.RollLeverageAdded, { roll_id: 9, die: makeDie(2) }],
		['roll.stage_changed', EventTypes.RollStageChanged, { roll_id: 9, stage: 'resolved' }],
		['roll.intent_set', EventTypes.RollIntentSet, { roll_id: 9, player_id: 7, intent: 'aid' }],
		['roll.ready_changed', EventTypes.RollReadyChanged, { roll_id: 9, player_id: 7, is_ready: true }],
		['roll.vote_cast', EventTypes.RollVoteCast, { roll_id: 9, player_id: 7 }],
		['roll.vote_resolved', EventTypes.RollVoteResolved, { roll_id: 9, adjusted_difficulty: 4, ballot: [] }],
		['demand.leverage_set', EventTypes.DemandLeverageSet, { plan_id: 42, roll_id: 9 }],
	];
	it.each(rollEvents)('%s while the roll is still loading supersedes the stale fetch', async (_name, type, payload) => {
		const ctx = makeCtx();
		getPlanMock.mockResolvedValue({ plan: makePlan({ id: 42 }) } as Awaited<ReturnType<typeof getPlan>>);
		// The second fetch is the one that sees the event's effect (a die).
		const landStale = holdFirstFetch(detail(makeRoll(), { dice: [makeDie(99)] }));

		handleWSMessage(ctx, created(makeRoll()));
		handleWSMessage(ctx, { type, payload } as Parameters<typeof handleWSMessage>[1]);
		expect(getRollMock).toHaveBeenCalledTimes(2);
		landStale(detail(makeRoll(), { dice: [] }));
		await flush();

		expect(ctx.activeRoll?.id).toBe(9);
		expect(ctx.activeRollDice.map(d => d.id)).toEqual([99]);
	});
});
