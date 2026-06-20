import { describe, it, expect } from 'vitest';
import { handleWSMessage, type WSContext } from './ws-handlers';
import { EventTypes } from '$lib/ws';
import type { Plan, RecordRow } from '$lib/api';

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
