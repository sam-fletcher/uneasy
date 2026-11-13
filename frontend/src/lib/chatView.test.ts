import { describe, expect, it } from 'vitest';
import {
	planHeaderName,
	planMarkFamily,
	resolutionStatus,
	unreadDividerAria,
	unreadDividerLabel,
} from './chatView';

describe('resolutionStatus', () => {
	it('says Resolving while the span has no terminal post, in plain text', () => {
		expect(resolutionStatus(null, 'seek_answers')).toEqual({ word: 'Resolving', tone: 'neutral' });
		// Even for a plan whose result words are suppressed: it is still resolving.
		expect(resolutionStatus(null, 'make_war')).toEqual({ word: 'Resolving', tone: 'neutral' });
		expect(resolutionStatus(null, null)).toEqual({ word: 'Resolving', tone: 'neutral' });
	});

	it('gives Make its green and Mar its red, with the word beside the colour', () => {
		expect(resolutionStatus('make', 'seek_answers')).toEqual({ word: 'Make', tone: 'success' });
		expect(resolutionStatus('mar', 'seek_answers')).toEqual({ word: 'Mar', tone: 'danger' });
	});

	it('wears no colour for a plan nobody won or lost', () => {
		expect(resolutionStatus('cancelled', 'seek_answers')).toEqual({ word: 'Fell through', tone: 'neutral' });
		expect(resolutionStatus('other', 'spread_propaganda')).toEqual({ word: 'No effect', tone: 'neutral' });
	});

	it('shows no Make or Mar for the two plans the book gives neither', () => {
		// The app stores "make" when a war or a liaison ends, and the log says
		// "Make War succeeded.". The header must not claim a result the game lacks.
		for (const t of ['make_war', 'clandestinely_liaise'] as const) {
			expect(resolutionStatus('make', t)).toBeNull();
			expect(resolutionStatus('mar', t)).toBeNull();
			// A fall-through is still news for them.
			expect(resolutionStatus('cancelled', t)?.word).toBe('Fell through');
		}
	});

	it('shows Make as it comes while the plan list has not loaded', () => {
		expect(resolutionStatus('make', null)).toEqual({ word: 'Make', tone: 'success' });
	});
});

describe('planMarkFamily', () => {
	it("draws war's barbute for Make War and the flag for every other plan", () => {
		expect(planMarkFamily('make_war')).toBe('war');
		expect(planMarkFamily('seek_answers')).toBe('plan');
		expect(planMarkFamily('make_demands')).toBe('plan');
		expect(planMarkFamily(null)).toBe('plan');
	});
});

describe('planHeaderName', () => {
	it("uses the plan list's name when it knows the plan", () => {
		expect(planHeaderName('seek_answers', 'ignored')).toBe('Seek Answers');
	});

	it('falls back to what is left of the "is resolving" post', () => {
		expect(planHeaderName(null, 'Exchange Courtiers is resolving.')).toBe('Exchange Courtiers');
		expect(planHeaderName(null, 'Make War is resolving')).toBe('Make War');
	});

	it('shows the whole body rather than nothing if it has an unexpected shape', () => {
		expect(planHeaderName(null, 'Something happened')).toBe('Something happened');
	});
});

describe('the unread divider', () => {
	it('puts the count in its words, since the tab carries none', () => {
		expect(unreadDividerLabel(4)).toBe('4 new');
		expect(unreadDividerLabel(1)).toBe('1 new');
	});

	it('is read out in full', () => {
		expect(unreadDividerAria(1)).toBe('1 new message');
		expect(unreadDividerAria(4)).toBe('4 new messages');
	});
});
