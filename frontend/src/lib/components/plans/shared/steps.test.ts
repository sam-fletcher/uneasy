import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import type { Plan, Player } from '$lib/api';
import FinishedStep from './FinishedStep.svelte';
import DieTile from './DieTile.svelte';
import ResolvingCard from '../ResolvingCard.svelte';
import StepHeading from '$lib/components/shared/StepHeading.svelte';
import { STEP_FRAME_KEY, type StepFrame } from './stepFrame';
import { declarations } from '$lib/components/shared/cssTestKit';
import { aHeading } from '$lib/stepHeadingTestKit';

/*
 * The Play pane reads as a plan's steps in order: finished steps are one line,
 * the roll is a step, and the open step's heading is the current ask. These
 * pin the pieces that make that so: what a finished line is, what a die tile
 * says, the order ResolvingCard lays a plan out in, and (the one that guards
 * private information) that a plan which hides its preparation note stays
 * hidden in the frame too.
 */

const snippet = (html: string) => createRawSnippet(() => ({ render: () => html }));
const read = (rel: string) => readFileSync(join(__dirname, '..', rel), 'utf8');

const players = [
	{ id: 1, display_name: 'alice' },
	{ id: 2, display_name: 'bob' },
] as Player[];
const plan = (over: Partial<Plan> = {}) =>
	({ id: 9, plan_type: 'seek_answers', preparer_id: 1, preparation_notes: '', ...over }) as Plan;

const frame = (over: Partial<StepFrame> = {}): Map<symbol, () => StepFrame> =>
	new Map([[STEP_FRAME_KEY, () => ({
		roll: null, rollOpen: false, heading: null, claimRoll: () => () => {}, ...over,
	})]]);

const strip = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/\s+/g, ' ');

describe('FinishedStep', () => {
	it('is plain text with no button when it has nothing more to say', () => {
		const { body } = render(FinishedStep, { props: { mark: snippet('<i>m</i>'), children: snippet('alice prepared it') } });
		expect(body).toContain('alice prepared it');
		expect(body).not.toContain('<button');
		expect(body).not.toContain('aria-expanded');
	});

	it('is a tappable row with a caret, shut, when it has a detail', () => {
		const { body } = render(FinishedStep, {
			props: { mark: snippet('<i>m</i>'), children: snippet('The roll'), detail: snippet('<p>the dice</p>') },
		});
		expect(body).toContain('<button');
		expect(body).toContain('aria-expanded="false"');
		expect(body).toMatch(/class="[^"]*\btap-row\b/);
		// Shut: the detail is not in the page until it is opened.
		expect(body).not.toContain('the dice');
	});

	it('can start open, showing the detail beneath the line', () => {
		const { body } = render(FinishedStep, {
			props: { mark: snippet('<i>m</i>'), children: snippet('The roll'), detail: snippet('<p>the dice</p>'), open: true },
		});
		expect(body).toContain('aria-expanded="true"');
		expect(body).toContain('the dice');
	});
});

describe('StepHeading', () => {
	const text = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]*>/g, '').replace(/\s+/g, ' ').replace(/ ([.,])/g, '$1').trim();
	const alice = { id: 1, display_name: 'alice', token_color: 'lavender', seat_order: 1 } as Player;
	const bob = { id: 2, display_name: 'bob', token_color: 'peridot', seat_order: 2 } as Player;

	it('is one h2: the who-line above the step, which stands alone', () => {
		const { body } = render(StepHeading, { props: { heading: aHeading('the roll') } });
		expect(body).toMatch(/<h2[^>]*>.*Your move.*The roll<\/span>.*<\/h2>/);
		expect(body).toContain('data-yours="true"');
		expect(text(body)).toBe('Your move. The roll');
	});

	it('names the others owed in their colour, with their shape, and reads aloud with commas and "and"', () => {
		const heading = aHeading('the delay reveal', { yours: false, who: [{ player: alice }, { player: bob }] });
		const { body } = render(StepHeading, { props: { heading } });
		expect(text(body)).toBe('Waiting on alice and bob. The delay reveal');
		// Each name is a labelled PlayerName: the shape first, the colour as a custom property.
		expect(body.match(/class="[^"]*player-name[^"]*labelled/g)).toHaveLength(2);
	});

	it('says "also" beside the viewer and "and others" past the cap', () => {
		const you = aHeading('aid or interfere', { who: [{ player: alice }, { player: bob }], more: true });
		expect(text(render(StepHeading, { props: { heading: you } }).body)).toBe('Your move · also alice, bob and others. Aid or interfere');
	});

	it('draws nothing when no one is asked anything', () => {
		expect(strip(render(StepHeading, { props: { heading: null } }).body)).toBe('');
	});
});

describe('DieTile', () => {
	const tile = (props: Record<string, unknown>) =>
		render(DieTile, { props: { face: null, color: '#123456', label: "bob's die", ...props } }).body;

	it('shows the die mark before it is rolled, never the emoji', () => {
		const body = tile({});
		expect(body).toContain('class="die-mark');
		expect(body).not.toContain('🎲');
		expect(body).not.toContain('class="face"');
	});

	it('shows its face once rolled', () => {
		const body = tile({ face: 4 });
		expect(body).toMatch(/<span class="face[^"]*">4<\/span>/);
		expect(body).not.toContain('class="die-mark');
	});

	it('wears the owner\'s colour as its edge and is named in words', () => {
		const body = tile({ color: '#c5aefe', label: "alice's die, aiding" });
		expect(body).toContain('border-color: #c5aefe');
		expect(body).toContain('role="img"');
		expect(body).toContain('aria-label="alice\'s die, aiding"');
	});

	it('marks a die that is not counting by its edge and its face, never by fading', () => {
		expect(tile({ face: 3, cancelled: true })).toMatch(/class="[^"]*\bcancelled\b/);
		expect(tile({ face: 3, duplicate: true })).toMatch(/class="[^"]*\bduplicate\b/);
		const source = read('shared/DieTile.svelte');
		expect(source).not.toMatch(/opacity/);
		expect(source).toMatch(/\.die-tile\.cancelled \.face\s*\{[^}]*line-through/);
	});
});

describe('ResolvingCard', () => {
	const plain = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ');
	const card = (props: Record<string, unknown>, context?: Map<symbol, () => StepFrame>) =>
		strip(render(ResolvingCard, {
			props: { plan: plan(), players, children: snippet('<p id="body">THE BODY</p>'), ...props },
			context,
		}).body);

	it('says who prepared the plan, as a finished line, and carries no badge', () => {
		const html = card({});
		expect(plain(html)).toContain('alice prepared Seek Answers');
		expect(html).not.toContain('resolving-badge');
		expect(html).not.toContain('plan-badge');
	});

	it('carries the facts fixed at preparation in the preparation line, which may wrap', () => {
		const html = card({ against: snippet("against alice's Seek Answers (row 5)") });
		expect(plain(html)).toContain("alice prepared Seek Answers against alice's Seek Answers (row 5)");
		expect(html).toMatch(/class="done-text[^"]*\bwrap\b/);
		// And with nothing to say it stays the short line, cut with an ellipsis like the rest.
		expect(card({})).not.toMatch(/class="done-text[^"]*\bwrap\b/);
	});

	it('quotes the preparer\'s note as a second finished line', () => {
		const html = card({ plan: plan({ preparation_notes: 'Who holds the holdfast?' }) });
		expect(plain(html)).toContain('alice: “Who holds the holdfast?”');
	});

	it('gives a long note a tap to open the whole of it', () => {
		const long = 'Who really holds the western holdfast, and for how long, and on whose word?';
		expect(card({ plan: plan({ preparation_notes: long }) })).toContain('aria-expanded="false"');
		expect(card({ plan: plan({ preparation_notes: 'Short.' }) })).not.toContain('aria-expanded');
	});

	// Spread Rumors and Spread Propaganda pass showNotes={false}: before the
	// pre-roll the note is only a draft, and a secret rumor's text is nobody's
	// business yet. The frame must not put it back on the screen.
	it('never shows a note the panel asked to keep back', () => {
		const html = card({
			plan: plan({ plan_type: 'spread_rumors', preparation_notes: 'SECRET-RUMOR-TEXT-THAT-MUST-NOT-LEAK' }),
			showNotes: false,
		});
		expect(plain(html)).toContain('alice prepared Spread Rumors');
		expect(html).not.toContain('SECRET-RUMOR-TEXT-THAT-MUST-NOT-LEAK');
	});

	it('lays the steps out in order: finished lines, the roll, the heading, then the body', () => {
		const html = card(
			{ plan: plan({ preparation_notes: 'A note.' }) },
			frame({
				roll: snippet('<section id="roll">THE ROLL</section>'),
				heading: aHeading('the next step'),
			}),
		);
		const said = plain(html);
		const at = (s: string) => {
			const i = said.indexOf(s);
			expect(i, `${s} is on the page`).toBeGreaterThanOrEqual(0);
			return i;
		};
		expect(at('alice prepared')).toBeLessThan(at('alice: “A note.”'));
		expect(at('alice: “A note.”')).toBeLessThan(at('THE ROLL'));
		expect(at('THE ROLL')).toBeLessThan(at('The next step'));
		expect(at('The next step')).toBeLessThan(at('THE BODY'));
	});

	it('puts the panel\'s body in a column of groups, after the frame\'s own lines', () => {
		const html = card({ plan: plan({ preparation_notes: 'A note.' }) }, frame({
			roll: snippet('<section id="roll">THE ROLL</section>'),
			heading: aHeading('the next step'),
		}));
		// The body, and only the body, is inside .step-groups: the finished lines, the
		// roll and the heading keep the frame's own gap.
		const open = html.indexOf('class="step-groups"');
		expect(open, 'a .step-groups column').toBeGreaterThan(-1);
		expect(html.indexOf('THE BODY')).toBeGreaterThan(open);
		for (const framePart of ['alice prepared', 'THE ROLL', 'The next step']) {
			expect(html.indexOf(framePart), framePart).toBeLessThan(open);
		}
	});

	it('leaves the heading to the roll while the roll is open, so there is only one', () => {
		const html = card({}, frame({
			roll: snippet('<section>THE ROLL</section>'),
			rollOpen: true,
			heading: aHeading('aid or interfere'),
		}));
		expect(html).toContain('THE ROLL');
		expect(html).not.toContain('Aid or interfere');
	});

	it('draws no heading and no roll outside a PlanPanel (the war drawer)', () => {
		const html = card({});
		expect(html).toContain('THE BODY');
		expect(html).not.toContain('<h2');
	});
});

describe('planPanel.css: no boxes, no fades', () => {
	// Without its @import lines: the rule reader would fold them into the first selector.
	const css = read('planPanel.css').replace(/@import[^;]*;/g, '');
	const decls = declarations(css);

	it('the plan itself has no border, fill or shadow: it is steps, and nobody presses a plan as a whole', () => {
		for (const sel of ['.plan-panel', '.plan-panel.resolving', '.done-steps', '.plan-form', '.prep-section', '.choices-section']) {
			const own = decls.filter(d => d.selector === sel).map(d => d.prop);
			for (const prop of ['border', 'background', 'box-shadow', 'padding']) {
				expect(own, `${sel} sets ${prop}`).not.toContain(prop);
			}
		}
	});

	it('nothing fades: a fade takes text under the contrast floor', () => {
		const fades = decls.filter(d => d.prop === 'opacity' && d.value !== '1');
		expect(fades.map(d => `${d.selector}: ${d.value}`)).toEqual([]);
	});

	it('the gold wash and the violet chip are gone', () => {
		expect(css).not.toMatch(/surface-active|chip-violet|surface-sunken|border-warm-strong/);
	});
});
