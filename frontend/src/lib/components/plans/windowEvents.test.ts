import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

// The two panels that watch the table's live events used to add and remove their
// own window listeners by hand, repeating the add/remove pair (and the 'uneasy:'
// prefix) that useWindowEvents exists to hold in one place. A source guard, since
// a component test needs a DOM this project doesn't have: a panel that goes back to
// window.addEventListener fails here.
describe.each(['HostFestivityPanel.svelte', 'ProposeDuelPanel.svelte'])('%s', (file) => {
	const source = readFileSync(join(__dirname, file), 'utf8');

	it('subscribes through useWindowEvents', () => {
		expect(source).toContain("from '$lib/useWindowEvents'");
		expect(source).toMatch(/useWindowEvents\(\s*\[/);
	});

	it('does not hand-subscribe to window events', () => {
		expect(source).not.toMatch(/window\.(add|remove)EventListener/);
		expect(source).not.toContain("'uneasy:");
	});
});
