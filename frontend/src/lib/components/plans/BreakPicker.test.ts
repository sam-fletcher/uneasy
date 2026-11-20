import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import BreakPicker from './BreakPicker.svelte';

// Tearing an asset's last marginalia destroys it, and the owner can avoid that by
// adding another first, so a break picker says so once the note is picked. The
// Exchange Courtiers messy break is one: the warning used to be pinned only by an
// e2e spec, so a panel that stopped passing the pick on would not fail a unit test.

const players = [{ id: 1, display_name: 'ravi' }];
const note = (id: number, assetID: number, torn = false) =>
	({ id, asset_id: assetID, position: id, text: `note ${id}`, is_torn: torn });
const asset = (id: number, notes: ReturnType<typeof note>[]) => ({
	id, name: `Courtier ${id}`, owner_id: 1, asset_type: 'peer', is_destroyed: false,
	is_main_character: false, is_leveraged: false, secret_count: 0, marginalia: notes,
});

const lastOfOne = asset(5, [note(50, 5)]); // one note, three empty slots
const two = asset(6, [note(60, 6), note(61, 6)]);
const text = (html: string) => html.replace(/<!--.*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ');

const draw = (selected: number | null) => text(render(BreakPicker, {
	props: {
		label: 'Marginalium to break', items: [lastOfOne, two] as never, players: players as never,
		emptyMessage: 'Nothing to break.', selectedMarginaliaID: selected, onSelect: () => {},
	},
}).body);

describe('the break picker\'s destruction warning', () => {
	it('warns when the picked note is its asset\'s last, with room to add another', () => {
		const t = draw(50);
		expect(t).toContain("Heads up: this is the asset's last marginalia");
		expect(t).toContain('Tearing it will destroy the asset');
	});

	it('is silent while nothing is picked', () => {
		expect(draw(null)).not.toContain('Heads up');
	});

	it('is silent when the asset keeps another note', () => {
		expect(draw(60)).not.toContain('Heads up');
	});
});
