import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { declarations, styleOf } from './cssTestKit';

/*
 * The touch floor lives in one place, --size-tap (app.css), so a change to it
 * is one edit and a recipe can't quietly drift to 43px or 40px. This holds the
 * SHARED recipes to the token: any width, height or min-height declared in
 * them that is a literal 44px (or smaller than the floor) fails. Screens that
 * haven't migrated still write 44px by hand; each piece moves onto the token
 * when it migrates its file, so this list grows rather than the rule loosening.
 */

const SRC = join(__dirname, '..', '..', '..');
const read = (rel: string) => readFileSync(join(SRC, rel), 'utf8');

const RECIPES = [
	'lib/components/shared/actionButton.css',
	'lib/components/shared/chip.css',
	'lib/components/shared/tapRow.css',
	'lib/components/shared/modalShell.css',
	'lib/components/shared/TableRoster.svelte',
	'lib/components/shared/ChecklistRow.svelte',
	'lib/components/RetinueSheet.svelte',
	'routes/dev/styleguide/+page.svelte',
];

describe('the touch floor', () => {
	it('is 44px, defined once in app.css', () => {
		const defs = read('app.css').match(/--size-tap:\s*([^;]+);/g) ?? [];
		expect(defs).toHaveLength(1);
		expect(defs[0]).toMatch(/--size-tap:\s*44px;/);
	});

	it('is used, not retyped, by the shared recipes', () => {
		const offenders: string[] = [];
		for (const rel of RECIPES) {
			const source = read(rel);
			const css = rel.endsWith('.css') ? source : styleOf(source);
			for (const d of declarations(css)) {
				if (!/^(min-)?(width|height)$/.test(d.prop)) continue;
				if (/^44px$/.test(d.value)) offenders.push(`${rel}: ${d.selector} { ${d.prop}: ${d.value} }`);
			}
		}
		expect(offenders, 'write var(--size-tap), not 44px').toEqual([]);
	});

	it('every recipe that sets a minimum height or a square close target uses the token', () => {
		const uses = (rel: string, prop: string) =>
			declarations(rel.endsWith('.css') ? read(rel) : styleOf(read(rel))).filter(
				(d) => d.prop === prop && d.value === 'var(--size-tap)'
			).length;
		expect(uses('lib/components/shared/actionButton.css', 'min-height')).toBeGreaterThan(0);
		expect(uses('lib/components/shared/chip.css', 'min-height')).toBeGreaterThan(0);
		expect(uses('lib/components/shared/tapRow.css', 'min-height')).toBeGreaterThan(0);
		expect(uses('lib/components/shared/modalShell.css', 'width')).toBeGreaterThan(0);
		expect(uses('lib/components/shared/modalShell.css', 'height')).toBeGreaterThan(0);
	});
});
