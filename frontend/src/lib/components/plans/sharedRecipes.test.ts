import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { declarations, stripComments } from '$lib/components/shared/cssTestKit';

/*
 * The plan panels share four recipes that used to be written out again in each
 * file: the gap between the groups of a step, the step's button row, the count
 * after a lead line, and the option card (with the chips). These hold each to
 * ONE definition. A panel that wants the look uses the shared piece; a panel
 * that writes its own copy again fails here, and a second copy is how the gap
 * ended up 12px in some panels and 24px in others.
 */

const SRC = join(__dirname, '..', '..', '..'); // frontend/src
const COMPONENTS = join(SRC, 'lib', 'components');

function walk(dir: string, out: string[] = []): string[] {
	for (const name of readdirSync(dir)) {
		const full = join(dir, name);
		if (statSync(full).isDirectory()) walk(full, out);
		else if (/\.(svelte|css)$/.test(name)) out.push(full);
	}
	return out;
}
const FILES = walk(SRC);
const rel = (f: string) => f.slice(SRC.length + 1);

/** The CSS a file declares: its <style> blocks (.svelte) or the whole sheet (.css). */
function cssOf(file: string): string {
	const src = readFileSync(file, 'utf8');
	if (file.endsWith('.css')) return src;
	return [...stripComments(src).matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
}
/** The markup of a .svelte file: no script, no style, no comments. */
function markupOf(file: string): string {
	return stripComments(readFileSync(file, 'utf8'))
		.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
		.replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '');
}
const read = (relPath: string) => readFileSync(join(COMPONENTS, relPath), 'utf8');
const has = (selector: string, name: string) =>
	new RegExp(`(^|[^\\w-])\\.${name}(?![\\w-])`).test(selector);

/** Class names that used to be written out in a panel and now come from a shared piece. */
const SHARED_ONLY: Record<string, string[]> = {
	// the class → the only files that may declare a rule for it
	'step-actions': ['lib/components/shared/actionButton.css'],
	'step-groups': ['lib/components/plans/planPanel.css'],
};
/** Names that must not come back at all: each was replaced by the piece named beside it. */
const GONE: Record<string, string> = {
	'demand-actions': '.step-actions.stacking',
	'fest-body': '.step-groups',
	'setup-groups': '.step-groups',
	'demand-banner': '.step-groups',
	'choice-card': '<OptionCard>',
	'player-chip': '<Chip> (PlayerChips)',
	'chip-btn': '<Chip>',
	'res-warning': '<Notice>',
};

describe('one definition each', () => {
	it.each(Object.entries(SHARED_ONLY))('.%s is declared only in its shared sheet', (name, owners) => {
		const declaredIn = FILES.filter((f) =>
			declarations(cssOf(f)).some((d) => has(d.selector, name))).map(rel);
		expect(declaredIn).toEqual(owners);
	});

	it.each(Object.entries(GONE))('.%s is gone: use %s', (name) => {
		const declaredIn = FILES.filter((f) =>
			declarations(cssOf(f)).some((d) => has(d.selector, name))).map(rel);
		expect(declaredIn, `a rule for .${name}`).toEqual([]);
		const usedIn = FILES.filter((f) => f.endsWith('.svelte') &&
			new RegExp(`class(?::|=)[^>]*(^|[^\\w-])${name}(?![\\w-])`).test(markupOf(f))).map(rel);
		expect(usedIn, `markup using .${name}`).toEqual([]);
	});
});

describe('the group gap', () => {
	const decls = declarations(read('plans/planPanel.css'));
	const gap = (selector: string) => decls.find((d) =>
		d.prop === 'gap' && d.selector.split(',').map((x) => x.trim()).includes(selector))?.value;

	it('between groups (24px) is bigger than between the lines of a group (12px)', () => {
		expect(gap('.step-groups')).toBe('var(--space-5)');
		expect(gap('.choices-section')).toBe('var(--space-3)');
	});

	it('is the frame\'s: the card puts the panel\'s body in it, and the panels that nest groups use it', () => {
		expect(markupOf(join(COMPONENTS, 'plans/ResolvingCard.svelte'))).toMatch(
			/<div class="step-groups">\s*\{@render children\(\)\}\s*<\/div>/);
		for (const f of ['plans/HostFestivityPanel.svelte', 'plans/duel/SetupPhase.svelte', 'plans/demand/TargetPlanDemandOverlay.svelte']) {
			expect(markupOf(join(COMPONENTS, f)), f).toContain('class="step-groups"');
		}
	});
});

describe('the step\'s button row', () => {
	const rows: [string, number][] = [
		['plans/duel/SetupPhase.svelte', 1],
		['plans/duel/RollPhase.svelte', 2],
		['plans/festivity/SocializingTurn.svelte', 1],
		['plans/festivity/ChallengeBanner.svelte', 1],
		['DiceRollPanel.svelte', 3],
		['plans/demand/TargetPlanDemandOverlay.svelte', 2],
	];
	it.each(rows)('%s draws it from the shared class', (file, count) => {
		const uses = markupOf(join(COMPONENTS, file)).match(/class="step-actions( stacking)?"/g) ?? [];
		expect(uses).toHaveLength(count);
	});

	it('only the demand overlay stacks: its labels are sentences', () => {
		const stacking = rows.filter(([f]) => /class="step-actions stacking"/.test(markupOf(join(COMPONENTS, f)))).map(([f]) => f);
		expect(stacking).toEqual(['plans/demand/TargetPlanDemandOverlay.svelte']);
	});
});

describe('the option card and the chips', () => {
	it.each([
		'plans/MakeDemandsPanel.svelte',
		'plans/MakeMarPicker.svelte',
		'plans/ReAimPanel.svelte',
		'plans/duel/BoutsPhase.svelte',
		'plans/festivity/HostPendingMar.svelte',
	])('%s draws its options with <OptionCard>', (file) => {
		const markup = markupOf(join(COMPONENTS, file));
		expect(markup).toContain('<OptionCard');
		// No hand-built card: a button carrying the card's pieces.
		expect(markup).not.toMatch(/option-(badge|title|desc|picked)/);
	});

	it('the face chips of a die-face reveal are <Chip>', () => {
		const markup = markupOf(join(COMPONENTS, 'plans/SimultaneousRevealInput.svelte'));
		expect(markup).toContain('<Chip');
		expect(markup).not.toMatch(/<button[^>]*chip/);
	});
});
