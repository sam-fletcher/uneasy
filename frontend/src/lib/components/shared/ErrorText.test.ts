import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ErrorText from './ErrorText.svelte';

/*
 * ErrorText is the one way an error reaches the screen (docs/STYLE_GUIDE.md
 * "Errors"). Its contract is its markup: role="alert", so a screen reader
 * announces an error that was inserted after an action, and the danger mark
 * beside the words, so the message reads as a notice and not only as red text.
 * Rendered through svelte/server, as the other component tests are: every
 * assertion is about what the component emits, so no DOM is needed.
 */

const alertText = (props: Record<string, unknown>) =>
	render(ErrorText, { props: { message: 'Could not save.', ...props } }).body;

describe('ErrorText', () => {
	it('announces itself: the message is a role="alert" paragraph', () => {
		const html = alertText({});
		expect(html).toMatch(/<p[^>]*role="alert"/);
		expect(html).toContain('Could not save.');
	});

	it('draws the danger mark beside the words, as decoration', () => {
		const html = alertText({});
		expect(html).toContain('data-tone="danger"');
		// The words say what went wrong, so the mark is hidden from assistive tech.
		expect(html).toMatch(/class="[^"]*error-mark[^"]*"[^>]*aria-hidden="true"/);
	});

	it('keeps the class namespace each variant had', () => {
		expect(alertText({})).toMatch(/class="[^"]*\berror-text\b/);
		expect(alertText({ variant: 'panel' })).toMatch(/class="[^"]*\bres-error\b/);
	});

	it('passes the caller\'s extra class through, so the local size still applies', () => {
		expect(alertText({ extra: 'inline' })).toMatch(/class="[^"]*\binline\b/);
	});
});
