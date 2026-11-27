// ChallengeBanner.test.ts — what the target reads of a festivity duel challenge.
//
// The challenger names the duel's type and place (book p.60; RULINGS, Host
// Festivity, 2026-11-26), so the target sees both before accepting. A challenge
// made before the terms existed carries no type, and the banner then names none.

import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ChallengeBanner from './ChallengeBanner.svelte';

const ALICE = 1; // the challenger
const BOB = 2; // the target
const players = [
	{ id: ALICE, display_name: 'alice', seat_order: 1 },
	{ id: BOB, display_name: 'bob', seat_order: 2 },
] as never;

const plain = (html: string) =>
	html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ');

const banner = (challenge: { duel_type?: 'arms' | 'wits'; notes?: string }) => plain(render(ChallengeBanner, {
	props: {
		planID: 5,
		challenge: { challenger_id: ALICE, target_id: BOB, ...challenge },
		players, currentPlayerID: ALICE, mustAccept: false, onPlansChanged: () => {},
	},
}).body);

describe('a festivity duel challenge names its terms', () => {
	it('says the duel is one of wits, and gives the place the challenger named', () => {
		const t = banner({ duel_type: 'wits', notes: 'The sunken garden.' });
		expect(t).toContain('alice challenges bob to a duel of wits.');
		expect(t).toContain('"The sunken garden."');
	});

	it('says the duel is one of arms', () => {
		expect(banner({ duel_type: 'arms', notes: 'The orchard.' })).toContain('to a duel of arms.');
	});

	it('names no type for a challenge made before the terms existed', () => {
		const t = banner({ notes: 'Duel triggered by festivity' });
		expect(t).toContain('alice challenges bob.');
		expect(t).not.toContain('to a duel of');
	});
});
