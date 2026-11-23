import { describe, expect, it } from 'vitest';
import { LIVE_WRAP, MIRROR, mirrorOf, mirrorsOf } from './mirrorMatchers';

const mirror = '<fieldset class="resolve-mirror-wrap resolve-mirror" disabled><p>a</p></fieldset>';
const mirrorFirst = '<fieldset class="resolve-mirror resolve-mirror-wrap" disabled><p>b</p></fieldset>';
const live = '<fieldset class="resolve-mirror-wrap"><p>c</p></fieldset>';
const liveDisabled = '<fieldset class="resolve-mirror-wrap" disabled><p>d</p></fieldset>';
const ariaOnly = '<fieldset class="resolve-mirror-wrap resolve-mirror" aria-disabled="true"><p>e</p></fieldset>';

describe('the mirror matchers', () => {
	it('recognise a disabled fieldset with the mirror class, in either class order', () => {
		expect(mirror).toMatch(MIRROR);
		expect(mirrorFirst).toMatch(MIRROR);
	});

	it('do not take the live wrapper for a mirror, though its class starts with the same letters', () => {
		// `resolve-mirror-wrap` alone, disabled or not, is not a mirror.
		expect(live).not.toMatch(MIRROR);
		expect(liveDisabled).not.toMatch(MIRROR);
	});

	it('do not take an aria-disabled fieldset for a disabled one', () => {
		expect(ariaOnly).not.toMatch(MIRROR);
	});

	it('tell the live wrapper from a mirror, whichever order the classes are in', () => {
		expect(live).toMatch(LIVE_WRAP);
		expect(liveDisabled).toMatch(LIVE_WRAP);
		expect(mirror).not.toMatch(LIVE_WRAP);
		expect(mirrorFirst).not.toMatch(LIVE_WRAP);
	});

	it('pull out each mirror whole, and not the live wrapper beside it', () => {
		const page = `${live}<div>${mirror}</div>${mirrorFirst}`;
		expect(mirrorsOf(page)).toEqual([mirror, mirrorFirst]);
		expect(mirrorOf(page)).toBe(mirror);
		expect(mirrorOf(live)).toBe('');
		expect(mirrorsOf('<p>none</p>')).toEqual([]);
	});
});
