import { describe, it, expect } from 'vitest';
import type { Asset, Marginalium } from './api';
import { computeCrowns } from './succession';

let nextMarginaliaID = 1;

function marginalia(
	title: string | null,
	opts: { torn?: boolean } = {},
): Marginalium {
	return {
		id: nextMarginaliaID++,
		asset_id: 1,
		position: 1,
		text: title ?? 'note',
		is_torn: opts.torn ?? false,
		torn_at: null,
		torn_by_id: null,
		title,
	};
}

let nextAssetID = 1;

function asset(
	marginaliaList: Marginalium[],
	opts: { destroyed?: boolean } = {},
): Asset {
	const id = nextAssetID++;
	return {
		id,
		game_id: 1,
		owner_id: 1,
		creator_id: 1,
		asset_type: 'peer',
		name: 'A',
		is_main_character: false,
		is_leveraged: false,
		is_destroyed: opts.destroyed ?? false,
		created_at: '2026-01-01T00:00:00Z',
		destroyed_at: null,
		linked_card_suit: null,
		linked_card_value: null,
		marginalia: marginaliaList,
		secret_count: 0,
	};
}

describe('computeCrowns', () => {
	it('returns no crowns while the throne is not established', () => {
		const m = marginalia('monarch');
		const marks = computeCrowns([asset([m])], false);
		expect(marks.size).toBe(0);
	});

	it('crowns the monarch and numbers the successors', () => {
		const mon = marginalia('monarch');
		const heir = marginalia('true_heir');
		const claimant = marginalia('claimant');
		const marks = computeCrowns(
			[asset([mon]), asset([claimant]), asset([heir])],
			true,
		);
		expect(marks.get(mon.id)).toEqual({ role: 'monarch' });
		// Ordinals follow SUCCESSION_ORDER, not asset/array order.
		expect(marks.get(heir.id)).toEqual({ role: 'successor', ordinal: 1 });
		expect(marks.get(claimant.id)).toEqual({ role: 'successor', ordinal: 2 });
	});

	it('ascends the line when the monarch claim is torn', () => {
		const mon = marginalia('monarch', { torn: true });
		const heir = marginalia('true_heir');
		const marks = computeCrowns([asset([mon]), asset([heir])], true);
		expect(marks.has(mon.id)).toBe(false);
		expect(marks.get(heir.id)).toEqual({ role: 'monarch' });
	});

	it('excludes a claim whose asset is destroyed even if untorn', () => {
		const ghost = marginalia('monarch');
		const heir = marginalia('true_heir');
		const marks = computeCrowns(
			[asset([ghost], { destroyed: true }), asset([heir])],
			true,
		);
		expect(marks.has(ghost.id)).toBe(false);
		expect(marks.get(heir.id)).toEqual({ role: 'monarch' });
	});

	it('returns no crowns during an interregnum (all claims lapsed)', () => {
		const mon = marginalia('monarch', { torn: true });
		const heir = marginalia('true_heir', { torn: true });
		const marks = computeCrowns([asset([mon]), asset([heir])], true);
		expect(marks.size).toBe(0);
	});

	it('ignores titles outside the line of succession', () => {
		const mon = marginalia('monarch');
		const paramour = marginalia('paramour');
		const marks = computeCrowns([asset([mon]), asset([paramour])], true);
		expect(marks.get(mon.id)).toEqual({ role: 'monarch' });
		expect(marks.has(paramour.id)).toBe(false);
	});

	it('never marks the general as a waiting successor', () => {
		const mon = marginalia('monarch');
		const general = marginalia('general');
		const marks = computeCrowns([asset([mon]), asset([general])], true);
		expect(marks.get(mon.id)).toEqual({ role: 'monarch' });
		expect(marks.has(general.id)).toBe(false);
	});

	it('reveals the general as monarch once the rest of the line collapses', () => {
		const mon = marginalia('monarch', { torn: true });
		const general = marginalia('general');
		const marks = computeCrowns([asset([mon]), asset([general])], true);
		expect(marks.get(general.id)).toEqual({ role: 'monarch' });
	});

	it('handles one character hoarding several titles', () => {
		const mon = marginalia('monarch');
		const heir = marginalia('favored_heir');
		const marks = computeCrowns([asset([mon, heir])], true);
		expect(marks.get(mon.id)).toEqual({ role: 'monarch' });
		expect(marks.get(heir.id)).toEqual({ role: 'successor', ordinal: 1 });
	});
});
