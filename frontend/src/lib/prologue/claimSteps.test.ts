import { describe, it, expect } from 'vitest';
import { deriveClaimSteps, type ClaimStepsInput } from './claimSteps';

function baseInput(overrides: Partial<ClaimStepsInput> = {}): ClaimStepsInput {
	return {
		assetTitle: 'Your new artifact asset',
		assetText: '',
		assetMarginalia: '',
		marginalia: null,
		cards: [],
		...overrides,
	};
}

describe('deriveClaimSteps', () => {
	it('the asset step needs both name and marginalia to be complete', () => {
		expect(deriveClaimSteps(baseInput()).find((s) => s.key === 'asset')!.complete).toBe(false);
		expect(
			deriveClaimSteps(baseInput({ assetText: 'Crown', assetMarginalia: '' })).find(
				(s) => s.key === 'asset'
			)!.complete
		).toBe(false);
		expect(
			deriveClaimSteps(baseInput({ assetText: 'Crown', assetMarginalia: 'Gilded' })).find(
				(s) => s.key === 'asset'
			)!.complete
		).toBe(true);
	});

	it('the asset step summary is the trimmed asset name', () => {
		const steps = deriveClaimSteps(baseInput({ assetText: '  Crown  ', assetMarginalia: 'Gilded' }));
		expect(steps.find((s) => s.key === 'asset')!.summary).toBe('Crown');
	});

	it('omits the marginalia step for hailing_from (no second text field)', () => {
		const steps = deriveClaimSteps(baseInput({ marginalia: null }));
		expect(steps.some((s) => s.key === 'marginalia')).toBe(false);
	});

	it('includes a marginalia step (titles or laws_rumors) with its own completion', () => {
		const steps = deriveClaimSteps(
			baseInput({ marginalia: { title: 'A title held by your main character', text: '' } })
		);
		const step = steps.find((s) => s.key === 'marginalia')!;
		expect(step.title).toBe('A title held by your main character');
		expect(step.complete).toBe(false);

		const filled = deriveClaimSteps(
			baseInput({ marginalia: { title: 'A new Law', text: 'No taxation without song.' } })
		);
		const filledStep = filled.find((s) => s.key === 'marginalia')!;
		expect(filledStep.complete).toBe(true);
		expect(filledStep.summary).toBe('No taxation without song.');
	});

	it('places asset, marginalia, and card steps in that order', () => {
		const steps = deriveClaimSteps(
			baseInput({
				marginalia: { title: 'A title', text: '' },
				cards: [
					{ key: 'H::K', title: 'K♥', isTake: false, text: '' },
					{ key: 'D::A', title: 'A♦', isTake: false, text: '' },
				],
			})
		);
		expect(steps.map((s) => s.key)).toEqual(['asset', 'marginalia', 'card:H::K', 'card:D::A']);
	});

	it('a fresh (make) card step is complete only once text is entered', () => {
		const steps = deriveClaimSteps(
			baseInput({ cards: [{ key: 'H::K', title: 'K♥', isTake: false, text: '' }] })
		);
		expect(steps.find((s) => s.key === 'card:H::K')!.complete).toBe(false);

		const filled = deriveClaimSteps(
			baseInput({ cards: [{ key: 'H::K', title: 'K♥', isTake: false, text: 'My Peer' }] })
		);
		expect(filled.find((s) => s.key === 'card:H::K')!.complete).toBe(true);
		expect(filled.find((s) => s.key === 'card:H::K')!.summary).toBe('My Peer');
	});

	it('a take card step is always pre-completed regardless of its (unused) text', () => {
		const steps = deriveClaimSteps(
			baseInput({
				cards: [{ key: 'H::K', title: 'K♥', isTake: true, text: '' }],
			})
		);
		const step = steps.find((s) => s.key === 'card:H::K')!;
		expect(step.complete).toBe(true);
		expect(step.isTake).toBe(true);
	});
});
