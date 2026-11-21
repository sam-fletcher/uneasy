import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import MakeMarPicker from './MakeMarPicker.svelte';

// The make/mar picker is drawn for the player who chooses, and, greyed out and inert,
// for everyone else (the watcher's mirror). The mirror is the same cards in the shared
// disabled fieldset, described without "you" (a card's `desc` may speak to the player
// choosing), and with nothing to press.

const options = [
	{ key: 'messy', level: 0, label: 'Messy', desc: 'The target will break one of your assets.',
		neutralDesc: 'The target will break one of the preparer\'s assets.' },
	{ key: 'legal', level: 1, label: 'Legal', desc: 'Everything went according to plan.' },
];
const base = { outcome: 'make' as const, options, selected: [] as string[], busy: false, onToggle: () => {}, onSubmit: () => {}, single: true };
const text = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

describe('MakeMarPicker', () => {
	it('is live by default: the card\'s own wording, and a button that applies the pick', () => {
		const body = render(MakeMarPicker, { props: base }).body;
		expect(body).not.toContain('resolve-mirror');
		expect(text(body)).toContain('one of your assets');
		expect(body).toMatch(/<button[^>]*>\s*Apply choices/);
	});

	it('read-only: the same cards inside the greyed mirror, in neutral words, with no button', () => {
		const body = render(MakeMarPicker, { props: { ...base, readOnly: true } }).body;
		expect(body).toMatch(/<fieldset[^>]*resolve-mirror[^>]*disabled/);
		const t = text(body);
		expect(t).toContain('Messy');
		expect(t).toContain('Legal');
		expect(t).toContain('one of the preparer\'s assets');
		expect(t).not.toContain('your assets');
		expect(body).not.toContain('Apply choices');
	});

	it('read-only: its prompt says what is chosen, not what the viewer should do', () => {
		const single = text(render(MakeMarPicker, { props: { ...base, readOnly: true } }).body);
		expect(single).toContain('One of these options is chosen:');
		expect(single).not.toContain('Choose one option');
		const multi = text(render(MakeMarPicker, { props: { ...base, single: false, readOnly: true } }).body);
		expect(multi).toContain('The options to apply:');
		expect(multi).not.toContain('Select options');
		// ...and stays an instruction for the player choosing.
		expect(text(render(MakeMarPicker, { props: base }).body)).toContain('Choose one option:');
		expect(text(render(MakeMarPicker, { props: { ...base, single: false } }).body)).toContain('Select options to apply:');
	});

	it('read-only: a card with no neutral wording keeps its own', () => {
		expect(text(render(MakeMarPicker, { props: { ...base, readOnly: true } }).body)).toContain('Everything went according to plan.');
	});

	it('read-only: keeps the lock reasons, so a watcher sees which tiers the margin puts out of reach', () => {
		const t = text(render(MakeMarPicker, {
			props: { ...base, readOnly: true, disabledKeys: ['legal'], lockReason: (k: string) => (k === 'legal' ? 'needs +1' : undefined) },
		}).body);
		expect(t).toContain('needs +1');
	});

	it('read-only: shows no ends-the-plan note, which is about pressing the button', () => {
		const t = text(render(MakeMarPicker, { props: { ...base, readOnly: true, endsText: 'This is the last step.' } }).body);
		expect(t).not.toContain('This is the last step.');
	});
});
