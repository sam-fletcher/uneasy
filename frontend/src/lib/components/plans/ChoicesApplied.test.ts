import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ChoicesApplied from './ChoicesApplied.svelte';
import Buffet from './shared/Buffet.svelte';
import { MAR_OPTIONS } from './shared';

// The applied-choices list is shown to every viewer, not just the player who
// picked, so an option whose picker label speaks to "you" has to read
// neutrally here (audit Tier 6: "Hide yourself" / "your retinue" leaking to
// non-actors).

const OPTIONS = [
	{ key: 'hide_source', label: 'Hide yourself as the source.', summary: 'The source is hidden.' },
	{ key: 'break_target', label: 'Break the target asset' },
];

describe('ChoicesApplied', () => {
	it('prefers an option’s viewer-neutral summary over its second-person label', () => {
		const { body } = render(ChoicesApplied, {
			props: { choices: ['hide_source', 'hide_source'], options: OPTIONS },
		});
		expect(body).toContain('The source is hidden.');
		expect(body).toContain('× 2');
		expect(body).not.toContain('yourself');
	});

	it('falls back to the label when there is no summary', () => {
		const { body } = render(ChoicesApplied, {
			props: { choices: ['break_target'], options: OPTIONS },
		});
		expect(body).toContain('Break the target asset');
	});

	it('Spread Propaganda’s mar options never address a non-actor as "you"', () => {
		const spOptions = MAR_OPTIONS.spread_propaganda ?? [];
		const { body } = render(ChoicesApplied, {
			props: { choices: spOptions.map(o => o.key), options: spOptions },
		});
		expect(body).not.toMatch(/\byour\b|\byourself\b/i);
		expect(body).toContain("preparer's retinue");
	});
});

describe('Buffet', () => {
	it('keeps the space between an option label and its description', () => {
		const { body } = render(Buffet, {
			props: {
				open: true,
				tabs: [{ key: 'make', label: 'Make', opts: [{ key: 'a', label: 'Introduce a new peer —', desc: 'They will join.' }] }],
			},
		});
		const text = body.replace(/<!--.*?-->/g, '');
		expect(text).toMatch(/Introduce a new peer —\s+<span class="muted">They will join\.<\/span>/);
	});
});
