import { describe, expect, it } from 'vitest';
import { spreadRumorsPrepDraft } from './prepDraft';

// The draft is broadcast to every client at the table as the preparer types, so it is
// public. A Spread Rumors rumor may be kept secret, and the box that says so can be
// ticked after the text is typed, so the draft never carries the rumor's text at all.

const TEXT = 'The duchess is not who she says she is.';

type Form = Parameters<typeof spreadRumorsPrepDraft>[0];

// A caller that still holds the text, as the panel's own state does. The builder is
// handed it and must not send it.
const form = (over: Partial<Form> = {}): Form => ({
	filterOwnerID: 2,
	targetAssetID: 20,
	keepSecret: false,
	secretAssetID: null,
	notes: TEXT,
	...over,
} as Form);

describe('spreadRumorsPrepDraft', () => {
	it('an open rumor goes out without its text', () => {
		expect(spreadRumorsPrepDraft(form())).toEqual({
			filter_owner_id: 2,
			target_asset_id: 20,
			keep_secret: false,
			secret_asset_id: null,
		});
	});

	it('a kept-secret rumor goes out without its text, and says it is secret and what holds it', () => {
		expect(spreadRumorsPrepDraft(form({ keepSecret: true, secretAssetID: 11 }))).toEqual({
			filter_owner_id: 2,
			target_asset_id: 20,
			keep_secret: true,
			secret_asset_id: 11,
		});
	});

	it.each([true, false])('never carries the text, key or value, with the box ticked = %s', (keepSecret) => {
		const draft = spreadRumorsPrepDraft(form({ keepSecret, secretAssetID: keepSecret ? 11 : null }));
		expect(draft).not.toHaveProperty('notes');
		expect(JSON.stringify(draft)).not.toContain(TEXT);
	});
});
