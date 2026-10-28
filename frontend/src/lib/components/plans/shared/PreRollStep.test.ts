import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import PreRollStep from './PreRollStep.svelte';

/*
 * The pre-roll step is shared by Spread Propaganda and Spread Rumors, so what it
 * promises is checked once here: the actor gets the book's prompt, one field per
 * entry, and a submit that is dead until every field has text; everyone else gets
 * a single waiting line and no form at all. Rendered through svelte/server (no
 * jsdom in this suite), so the pre-fill — an $effect — and the click are out of
 * reach; the real-browser pass covers them.
 */

const fields = [
	{ key: 'rumor', label: 'The rumor', placeholder: 'What are people about to be told?', max: 1000 },
	{ key: 'how', label: 'How you are spreading it', placeholder: 'In taverns?', max: 1000 },
];

const props = {
	title: 'Spread the rumor',
	prompt: "State the rumor you plan to spread and explain how you're spreading it.",
	fields,
	busy: false,
	actorName: 'alice',
	waitingVerb: 'is about to state the rumor',
	onSubmit: () => {},
};

describe('PreRollStep', () => {
	it('gives the actor the prompt, every field, and a submit that waits for text', () => {
		const { body } = render(PreRollStep, { props: { ...props, canAct: true } });
		expect(body).toContain('State the rumor you plan to spread');
		expect(body).toContain('The rumor');
		expect(body).toContain('How you are spreading it');
		expect(body.match(/<textarea/g)).toHaveLength(2);
		// Why the step exists, said to the person writing it.
		expect(body).toContain('The table reads this before anyone decides');
		expect(body).toMatch(/<button[^>]*disabled[^>]*>\s*Post &amp; open the roll/);
	});

	it('gives everyone else one waiting line and no form', () => {
		const { body } = render(PreRollStep, { props: { ...props, canAct: false } });
		expect(body).toContain('alice is about to state the rumor…');
		expect(body).not.toContain('<textarea');
		expect(body).not.toContain('<button');
	});
});
