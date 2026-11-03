import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ExchangeCourtiersPanel from './ExchangeCourtiersPanel.svelte';
import { MAR_OPTIONS } from './shared';
import type { PlanContext } from './types';

// Every seat that isn't acting must be told, in the third person, who the plan
// is waiting on; only the actual addressee sees second-person wording.

const PREPARER = 1;
const TARGET = 2;
const BYSTANDER = 3;
const PLAYERS = [
	{ id: PREPARER, display_name: 'ravi' },
	{ id: TARGET, display_name: 'sol' },
	{ id: BYSTANDER, display_name: 'mel' },
];
const ASSETS = [
	{ id: 5, name: 'Lord Vane', owner_id: TARGET, asset_type: 'peer', is_destroyed: false },
	{ id: 6, name: 'Dame Orr', owner_id: PREPARER, asset_type: 'peer', is_destroyed: false },
];

function renderFor(
	seat: number,
	resolution: Record<string, unknown>,
	rollOutcome: 'make' | 'mar' | null = null,
	assets = ASSETS,
): string {
	const plan = {
		id: 9, plan_type: 'exchange_courtiers', status: 'resolving', preparer_id: PREPARER,
		target_player_id: TARGET, target_asset_id: 5, preparation_notes: 'x',
		resolution_data: JSON.stringify({ exchange_courtiers: resolution }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never,
		assets: assets as never, players: PLAYERS as never, rankings: [], currentPlayerID: seat,
		isFocusPlayer: false, rollActive: false, rollOutcome,
		activeRoll: rollOutcome ? ({ id: 4, plan_id: 9, status: 'resolved', result: 1, difficulty: 2 } as never) : null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(ExchangeCourtiersPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

const text = (html: string) => html.replace(/<!--.*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ');

describe('before the offer', () => {
	it('tells a bystander the target is proposing', () => {
		expect(text(renderFor(BYSTANDER, {}))).toContain('Waiting for sol to propose a fair trade');
	});
	it('names the preparer when they have no peers to trade', () => {
		const t = text(renderFor(BYSTANDER, {}, null, ASSETS.filter(a => a.owner_id !== PREPARER)));
		expect(t).toContain('Waiting for ravi to proceed to the dice roll');
	});
	it('tells the preparer the target is proposing', () => {
		expect(text(renderFor(PREPARER, {}))).toContain('Waiting for sol to propose a fair trade');
	});
});

describe('after the offer', () => {
	const offered = { fair_trade_asset_id: 6 };
	it('addresses the target in the second person', () => {
		expect(text(renderFor(TARGET, offered))).toContain('You asked for Dame Orr');
	});
	it('names the target for a bystander', () => {
		const t = text(renderFor(BYSTANDER, offered));
		expect(t).not.toContain('You asked for');
		expect(t).toContain('sol asked for Dame Orr');
	});
});

describe('after the roll', () => {
	const declined = { fair_trade_asset_id: 6, fair_trade_accepted: false };
	it('tells the target the preparer is choosing on a make', () => {
		expect(text(renderFor(TARGET, declined, 'make'))).toContain('Waiting for ravi to say how the trade went');
	});
	it('tells the preparer the target is choosing on a mar', () => {
		expect(text(renderFor(PREPARER, declined, 'mar'))).toContain('Waiting for sol to choose how to respond');
	});
	it('tells a bystander the target is choosing on a mar', () => {
		expect(text(renderFor(BYSTANDER, declined, 'mar'))).toContain('Waiting for sol to choose how to respond');
	});
	it('tells a bystander the preparer is choosing on a make', () => {
		expect(text(renderFor(BYSTANDER, declined, 'make'))).toContain('Waiting for ravi to say how the trade went');
	});
});

describe('between a decline and the roll arriving', () => {
	it('does not announce a make to a bystander', () => {
		const t = text(renderFor(BYSTANDER, { fair_trade_asset_id: 6, fair_trade_accepted: false }));
		expect(t).not.toContain('say how the trade went');
	});
});

describe('Riposte wording', () => {
	it('is written for the target who chooses it', () => {
		const riposte = MAR_OPTIONS.exchange_courtiers!.find(o => o.key === 'riposte')!;
		expect(riposte.desc).not.toMatch(/you may break/i);
		expect(riposte.desc).toMatch(/they may break/i);
	});
});
