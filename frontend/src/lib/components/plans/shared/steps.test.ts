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
import { declarations, winning } from '$lib/components/shared/cssTestKit';

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
	it('is an h2 saying the ask, and records whose move it is', () => {
		const { body } = render(StepHeading, { props: { heading: { text: 'Your move — the roll', yours: true } } });
		expect(body).toMatch(/<h2[^>]*>Your move — the roll<\/h2>/);
		expect(body).toContain('data-yours="true"');
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
	const card = (props: Record<string, unknown>, context?: Map<symbol, () => StepFrame>) =>
		strip(render(ResolvingCard, {
			props: { plan: plan(), players, children: snippet('<p id="body">THE BODY</p>'), ...props },
			context,
		}).body);

	it('says who prepared the plan, as a finished line, and carries no badge', () => {
		const html = card({});
		expect(html).toContain('alice prepared Seek Answers');
		expect(html).not.toContain('resolving-badge');
		expect(html).not.toContain('plan-badge');
	});

	it('quotes the preparer\'s note as a second finished line', () => {
		const html = card({ plan: plan({ preparation_notes: 'Who holds the holdfast?' }) });
		expect(html).toContain('alice: “Who holds the holdfast?”');
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
		expect(html).toContain('alice prepared Spread Rumors');
		expect(html).not.toContain('SECRET-RUMOR-TEXT-THAT-MUST-NOT-LEAK');
	});

	it('lays the steps out in order: finished lines, the roll, the heading, then the body', () => {
		const html = card(
			{ plan: plan({ preparation_notes: 'A note.' }) },
			frame({
				roll: snippet('<section id="roll">THE ROLL</section>'),
				heading: { text: 'Your move — the next step', yours: true },
			}),
		);
		const at = (s: string) => {
			const i = html.indexOf(s);
			expect(i, `${s} is on the page`).toBeGreaterThanOrEqual(0);
			return i;
		};
		expect(at('alice prepared')).toBeLessThan(at('alice: “A note.”'));
		expect(at('alice: “A note.”')).toBeLessThan(at('THE ROLL'));
		expect(at('THE ROLL')).toBeLessThan(at('Your move — the next step'));
		expect(at('Your move — the next step')).toBeLessThan(at('THE BODY'));
	});

	it('leaves the heading to the roll while the roll is open, so there is only one', () => {
		const html = card({}, frame({
			roll: snippet('<section>THE ROLL</section>'),
			rollOpen: true,
			heading: { text: 'Your move — aid or interfere', yours: true },
		}));
		expect(html).toContain('THE ROLL');
		expect(html).not.toContain('Your move — aid or interfere');
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

	// The panels still draw their chips with the older class names. Each must
	// still be the house chip, state for state: if chip.css changes a selected
	// chip and these don't follow, the panels quietly stop matching it.
	describe('.player-chip and .chip-btn are the house chip', () => {
		const chipCss = readFileSync(join(__dirname, '../../shared/chip.css'), 'utf8');
		const rules = css;
		const states: Array<[string, { chip: object; legacy: object }]> = [
			['at rest', { chip: { classes: ['chip'] }, legacy: { classes: ['chip-btn'] } }],
			['selected', { chip: { classes: ['chip', 'selected'] }, legacy: { classes: ['chip-btn', 'active'] } }],
			['hovered', { chip: { classes: ['chip'], pseudos: ['hover'] }, legacy: { classes: ['chip-btn'], pseudos: ['hover'] } }],
		];
		for (const [name, { chip, legacy }] of states) {
			for (const prop of ['border-color', 'box-shadow', 'color', 'border-radius', 'min-height', 'background']) {
				it(`${prop} ${name}`, () => {
					expect(winning(rules, prop, legacy), `.chip-btn ${prop}`).toBe(winning(chipCss, prop, chip));
				});
			}
		}
	});
});
