import { describe, expect, it } from 'vitest';
import type { Plan } from '$lib/api';
import { planStatusWord, sceneSummary } from './recordLines';

// What a plan line in the Public Record's Events list says about its plan, and
// how a scene's summary reads. The status word reads down the right edge of the
// list, so these rules decide what a table sees at a glance.

type Row = Pick<Plan, 'plan_type' | 'status' | 'result'>;
const plan = (plan_type: Plan['plan_type'], status: Plan['status'], result: Plan['result']): Row => ({
	plan_type,
	status,
	result,
});

describe('planStatusWord', () => {
	it('says "Resolving", in plain text, on the plan being played now', () => {
		expect(planStatusWord(plan('seek_answers', 'resolving', null))).toEqual({
			text: 'Resolving',
			tone: 'plain',
		});
	});

	it('says Make in green and Mar in red on a resolved plan', () => {
		expect(planStatusWord(plan('propose_decree', 'resolved', 'make'))).toEqual({
			text: 'Make',
			tone: 'success',
		});
		expect(planStatusWord(plan('propose_decree', 'resolved', 'mar'))).toEqual({
			text: 'Mar',
			tone: 'danger',
		});
	});

	it('says "No effect", with no colour, on a plan that closed without a roll', () => {
		// The server leaves the result NULL for a plan whose target was gone, or a
		// declined counter-propaganda: nothing was won or lost.
		expect(planStatusWord(plan('spread_propaganda', 'resolved', null))).toEqual({
			text: 'No effect',
			tone: 'plain',
		});
	});

	it('says nothing on a future plan', () => {
		expect(planStatusWord(plan('exchange_courtiers', 'pending', null))).toBeNull();
		// Whatever the column holds: a pending plan has no outcome to report.
		expect(planStatusWord(plan('exchange_courtiers', 'pending', 'make'))).toBeNull();
	});

	it('says nothing on a cancelled plan, which never has a row anyway', () => {
		expect(planStatusWord(plan('make_introductions', 'cancelled', null))).toBeNull();
	});

	// The server stores result = "make" for a war and a liaison when they end (the
	// log even reads "Make War succeeded."), but the book gives neither a Make or
	// a Mar. Reading the column straight off the plan would put a green Make on a
	// war, which is the one thing this rule is here to stop.
	describe('Make War and Clandestinely Liaise', () => {
		for (const type of ['make_war', 'clandestinely_liaise'] as const) {
			it(`${type} never shows Make, though the server stored "make"`, () => {
				expect(planStatusWord(plan(type, 'resolved', 'make'))).toBeNull();
			});

			it(`${type} never shows Mar or No effect either`, () => {
				expect(planStatusWord(plan(type, 'resolved', 'mar'))).toBeNull();
				expect(planStatusWord(plan(type, 'resolved', null))).toBeNull();
			});

			it(`${type} says nothing before its turn`, () => {
				expect(planStatusWord(plan(type, 'pending', null))).toBeNull();
			});

			it(`${type} still says "Resolving" while it is the plan being played`, () => {
				// "Resolving" is where the table is, not an outcome. See
				// docs/work/questions/record-war-liaise-resolving.md.
				expect(planStatusWord(plan(type, 'resolving', null))?.text).toBe('Resolving');
			});
		}

		it('is the only pair exempt: every other plan type shows Make', () => {
			const others = [
				'exchange_courtiers', 'make_introductions', 'spread_propaganda', 'seek_answers',
				'spread_rumors', 'chronicle_histories', 'propose_decree', 'propose_duel',
				'host_festivity', 'make_demands',
			] as const;
			for (const type of others) {
				expect(planStatusWord(plan(type, 'resolved', 'make'))?.text, type).toBe('Make');
			}
		});
	});
});

describe('sceneSummary', () => {
	it('takes the "Scene:" prefix off, since the line draws the scene mark instead', () => {
		expect(sceneSummary('Scene: The harbour fire')).toEqual({
			isScene: true,
			text: 'The harbour fire',
		});
	});

	it('shows a line without the prefix whole, and does not call it a scene', () => {
		expect(sceneSummary('A word in the stables')).toEqual({
			isScene: false,
			text: 'A word in the stables',
		});
	});

	it('only reads the prefix at the start', () => {
		expect(sceneSummary('After the Scene: it rained').isScene).toBe(false);
	});
});
