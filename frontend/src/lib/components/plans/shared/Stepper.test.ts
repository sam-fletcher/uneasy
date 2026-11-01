import { describe, expect, it } from 'vitest';
import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import Stepper from './Stepper.svelte';

const label = createRawSnippet(() => ({ render: () => '<i>Hide the source</i>' }));

describe('Stepper', () => {
	it('shows the count and the label', () => {
		const { body } = render(Stepper, { props: { count: 2, onstep: () => {}, children: label } });
		expect(body).toContain('>2<');
		expect(body).toContain('Hide the source');
	});

	it('disables − at zero by default and + only when told to', () => {
		const { body } = render(Stepper, { props: { count: 0, onstep: () => {}, children: label } });
		const [dec, inc] = body.match(/<button[^>]*>/g)!;
		expect(dec).toContain('disabled');
		expect(inc).not.toContain('disabled');
	});

	it('honours explicit disabled flags and the dimmed row', () => {
		const { body } = render(Stepper, {
			props: { count: 3, onstep: () => {}, decDisabled: true, incDisabled: true, dimmed: true, children: label },
		});
		const [dec, inc] = body.match(/<button[^>]*>/g)!;
		expect(dec).toContain('disabled');
		expect(inc).toContain('disabled');
		expect(body).toContain('dimmed');
	});

	it('names the buttons after the option when given a label', () => {
		const { body } = render(Stepper, { props: { count: 1, label: 'Take Asset', onstep: () => {}, children: label } });
		expect(body).toContain('aria-label="Fewer Take Asset"');
		expect(body).toContain('aria-label="More Take Asset"');
	});

	it('gives the buttons accessible names', () => {
		const { body } = render(Stepper, { props: { count: 1, onstep: () => {}, children: label } });
		expect(body).toContain('aria-label="Fewer"');
		expect(body).toContain('aria-label="More"');
	});
});
