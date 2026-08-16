import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import TableRoster from './TableRoster.svelte';
import type { Player } from '$lib/api';

/*
 * TableRoster's contract is what a seat SAYS — on screen and to a screen
 * reader (adr/LOBBY_AND_CHECKLIST_PLAN.md D3). Colour carries three of its
 * four signals (identity dot, online ring, waiting fill), so the words that
 * stand in for them are the part worth pinning down. Rendered through
 * svelte/server like ChecklistRow.test.ts: no jsdom, and every assertion here
 * is about emitted markup rather than about clicks.
 */

function player(id: number, name: string, extra: Partial<Player> = {}): Player {
	return {
		id,
		game_id: 1,
		account_id: id,
		display_name: name,
		joined_at: '2026-08-16T00:00:00Z',
		is_facilitator: false,
		token_color: null,
		seat_order: id,
		...extra,
	};
}

const alice = player(1, 'alice', { is_facilitator: true });
const bob = player(2, 'bob');

describe('TableRoster — seats', () => {
	it('reads "You" for the current player and the name for everyone else', () => {
		const { body } = render(TableRoster, {
			props: { players: [alice, bob], currentPlayerID: 1 },
		});
		expect(body).toContain('You');
		expect(body).toContain('bob');
		// A roster you can't find yourself in is the finding this component
		// exists to fix — so your own name is never the label.
		expect(body).not.toContain('>alice<');
	});

	it('gives a plain row its state words visually-hidden, in reading order', () => {
		const { body } = render(TableRoster, {
			props: {
				players: [alice, bob],
				currentPlayerID: 1,
				waitingPlayerIDs: new Set([2]),
				members: [
					{ id: 1, display_name: 'alice', online: true },
					{ id: 2, display_name: 'bob', online: false },
				],
			},
		});
		expect(body).toContain('sr-state');
		expect(body).toContain(', online');
		expect(body).toContain(', offline, the game is waiting on them');
	});

	it('omits presence words entirely when no members are supplied', () => {
		const { body } = render(TableRoster, {
			props: { players: [alice, bob], currentPlayerID: 1 },
		});
		// Without `members` every row would otherwise announce "offline".
		expect(body).not.toContain('offline');
		expect(body).not.toContain(', online');
	});
});

describe('TableRoster — tappable rows', () => {
	it('onSelect makes each seat a button labelled with the player', () => {
		const { body } = render(TableRoster, {
			props: { players: [alice, bob], currentPlayerID: 1, onSelect: () => {} },
		});
		expect(body).toContain('<button');
		expect(body).toContain('aria-label="alice (you), facilitator"');
	});

	it('rowLabel replaces that label and is handed the state words', () => {
		// The closing stage's retinue tallies: the counts ride in `trailing`,
		// which is inside the button and therefore hidden by any label — so the
		// label has to restate them, without dropping the presence wording.
		const { body } = render(TableRoster, {
			props: {
				players: [bob],
				currentPlayerID: 1,
				waitingPlayerIDs: new Set([2]),
				onSelect: () => {},
				rowLabel: (p: Player, stateWords: string) =>
					`View ${p.display_name}'s retinue — Peers 2${stateWords}`,
			},
		});
		expect(body).toContain(
			`aria-label="View bob's retinue — Peers 2, the game is waiting on them"`
		);
	});
});
