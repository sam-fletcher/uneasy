import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import WarStatus from './WarStatus.svelte';
import type { Plan, Player, WarStateResponse } from '$lib/api';

// An ended war can still hold a surrender claim that gates the row. The
// claimant gets their own form; everyone else should be told whose move it is,
// and the end reason should read as words rather than the raw key.

const players = [
	{ id: 1, display_name: 'Ana' },
	{ id: 2, display_name: 'Ben' },
	{ id: 3, display_name: 'Cy' },
] as Player[];

// Ana declared war on Ben.
const plan = {
	id: 1, plan_type: 'make_war', preparer_id: 1, status: 'resolved', row_number: 4,
	resolution_data: JSON.stringify({ make_war: { enemy_player_ids: [2] } }),
} as Plan;

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

const active = { ...ended, status: 'active', end_reason: null, ended_at_row: null, open_claims: [] } as unknown as WarStateResponse;

const noop = () => {};
const props = (currentPlayerID: number | null, war = ended, over: Record<string, unknown> = {}) => ({
	war, plan, players, currentPlayerID, amParticipant: currentPlayerID !== 3,
	begun: true, onChanged: noop, setError: noop, ...over,
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

	it('offers nothing to join once the war is over', () => {
		const { body } = render(WarStatus, { props: props(3) });
		expect(body).not.toContain('You’re not in this war');
	});
});

describe('the sides, named after their leaders', () => {
	it('labels each side with its leader, never "Side 1" or "Declarer"', () => {
		const { body } = render(WarStatus, { props: props(3, active) });
		expect(body).toContain('Ana’s side');
		expect(body).toContain('Ben’s side');
		expect(body).not.toMatch(/Side \d|Declarer|Enemies/);
	});

	it('draws each member with their shape and says what is still owed in words', () => {
		const joining = {
			...active,
			participants: [
				...active.participants,
				{ player_id: 3, side: 1, entry_payment_complete: false, surrendered_at_row: null },
			],
		} as unknown as WarStateResponse;
		const { body } = render(WarStatus, { props: props(1, joining) });
		expect(body).toContain('joining, owes entry');
		expect(body.match(/class="player-shape/g)?.length).toBe(3);
	});

	it('says who surrendered and when', () => {
		const { body } = render(WarStatus, { props: props(3, ended) });
		expect(body).toContain('surrendered, row 4');
	});
});

describe('the way in, for someone outside the war', () => {
	it('is three chips named for the leaders, and a main button that waits for a pick', () => {
		const { body } = render(WarStatus, { props: props(3, active) });
		expect(body).toContain('You’re not in this war.');
		for (const label of ['Join Ana', 'Join Ben', 'Stay out']) expect(body).toContain(label);
		// Nothing is picked yet: every chip is unpressed, and there is no cost to
		// read, so the one main button is greyed out and says there is a choice.
		expect(body.match(/aria-pressed="false"/g)?.length).toBe(3);
		expect(body).not.toContain('aria-pressed="true"');
		expect(body).toMatch(/<button[^>]*\bdisabled\b[^>]*>Choose an option<\/button>/);
		expect(body).not.toContain('entry payment');
	});

	it('does not offer Join Side 1 any more', () => {
		const { body } = render(WarStatus, { props: props(3, active) });
		expect(body).not.toContain('Join Side');
		expect(body).not.toContain('Stay out of it');
	});

	it('is not shown to someone who is in the war', () => {
		for (const id of [1, 2]) {
			const { body } = render(WarStatus, { props: props(id, active) });
			expect(body).not.toContain('You’re not in this war');
			expect(body).not.toContain('Choose an option');
		}
	});
});
