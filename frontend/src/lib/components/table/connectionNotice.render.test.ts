import { describe, expect, it, vi } from 'vitest';
import { render } from 'svelte/server';
import ConnectionNotice from './ConnectionNotice.svelte';

/*
 * The strip the table shows while its live connection is down. When it shows is
 * ws.ts's business (see ws.test.ts); this pins what it says and how it behaves.
 * Rendered through svelte/server, as the other component tests are, so every
 * assertion is about what the component emits.
 */

const strip = (trying: boolean) =>
	render(ConnectionNotice, { props: { trying, onTry: vi.fn() } }).body;

describe('ConnectionNotice', () => {
	it('is announced when it appears, as a warning notice and not an error', () => {
		const html = strip(false);
		// role="status" (Notice's `live`): polite, so it doesn't interrupt typing.
		expect(html).toMatch(/<p[^>]*role="status"/);
		expect(html).toContain('data-tone="warning"');
		expect(html).not.toContain('role="alert"');
	});

	it('says what is wrong in words, so colour is never the only signal', () => {
		const html = strip(false);
		expect(html).toContain('Connection lost');
		expect(html).toContain("This table isn't updating");
		expect(html).toContain('Reconnecting');
	});

	it('offers "Try now" as the house secondary button', () => {
		const html = strip(false);
		expect(html).toMatch(/<button[^>]*class="[^"]*\baction-btn\b[^"]*\bsecondary\b[^"]*"[^>]*>Try now<\/button>/);
		expect(html).not.toContain('aria-disabled');
	});

	it('while an attempt is in flight reads "Trying…" and is aria-disabled, not disabled', () => {
		const html = strip(true);
		expect(html).toContain('>Trying…</button>');
		expect(html).toContain('aria-disabled="true"');
		// `disabled` would drop it from the tab order, taking a keyboard
		// user's focus with it.
		expect(html).not.toMatch(/<button[^>]*\sdisabled/);
	});
});
