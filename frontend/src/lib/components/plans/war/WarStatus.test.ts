import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import WarStatus from './WarStatus.svelte';
import type { Player, WarStateResponse } from '$lib/api';

// An ended war can still hold a surrender claim that gates the row. The
// claimant gets their own form; everyone else should be told whose move it is,
// and the end reason should read as words rather than the raw key.

const players = [
	{ id: 1, display_name: 'Ana' },
	{ id: 2, display_name: 'Ben' },
	{ id: 3, display_name: 'Cy' },
] as Player[];

const ended = {
	war_id: 1,
	origin_plan_id: 1,
	status: 'ended',
	end_reason: 'surrender',
	ended_at_row: 4,
	participants: [
		{ player_id: 1, side: 1, entry_payment_complete: true, surrendered_at_row: null },
		{ player_id: 2, side: 2, entry_payment_complete: true, surrendered_at_row: 4 },
	],
	outstanding_costs: [],
	open_claims: [{ id: 1, surrendered_id: 2, claimant_id: 1 }],
} as unknown as WarStateResponse;

const noop = () => {};
const props = (currentPlayerID: number | null, war = ended) => ({
	war, planID: 1, players, currentPlayerID, amParticipant: currentPlayerID !== 3,
	onChanged: noop, setError: noop,
});

describe('WarStatus after a surrender', () => {
	it('tells a bystander who is choosing what to take', () => {
		const { body } = render(WarStatus, { props: props(3) });
		expect(body).toContain('Ana is choosing one of');
		expect(body).toContain("Ben's assets to take");
	});

	it('does not repeat the note to the claimant, who has the form', () => {
		const { body } = render(WarStatus, { props: props(1) });
		expect(body).not.toContain('is choosing one of');
	});

	it('says nothing once the claim is taken', () => {
		const { body } = render(WarStatus, { props: props(3, { ...ended, open_claims: [] } as WarStateResponse) });
		expect(body).not.toContain('is choosing one of');
	});

	it('writes the end reason in words', () => {
		const { body } = render(WarStatus, { props: props(3) });
		expect(body).toContain('a side surrendered');
		expect(body).not.toContain('(surrender)');
	});
});
