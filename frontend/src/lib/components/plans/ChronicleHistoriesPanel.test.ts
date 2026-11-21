import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ChronicleHistoriesPanel from './ChronicleHistoriesPanel.svelte';
import type { PlanContext } from './types';

// Chronicle Histories' difficulty is the higher of the preparer's knowledge rank and the
// number of artifacts invoked (book p.54). The meter showed it only until the dice were
// cast (so the table lost it just when it decides whether to help or interfere), and the
// picker's label said every artifact "raises the difficulty", which is not so until there
// are more of them than the rank.

const PREPARER = 1;
const OTHER = 2;
const BYSTANDER = 3;
const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
const artifact = (id: number, owner: number) => ({
	id, name: `Relic ${id}`, owner_id: owner, asset_type: 'artifact', is_destroyed: false, is_main_character: false,
	is_leveraged: false, secret_count: 0, marginalia: [] as unknown[],
});

function renderCH(opts: {
	seat: number;
	cast?: boolean;
	invoked?: number[];
	outcome?: 'make' | 'mar' | null;
	rank?: number;
}): string {
	const plan = {
		id: 9, plan_type: 'chronicle_histories', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'x', resolution_data: JSON.stringify({
			chronicle_histories: {
				invoke_phase_closed: opts.cast ?? false,
				invoked_artifact_ids: opts.invoked ?? [],
				make_budget: 2,
			},
		}),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never,
		assets: [artifact(11, PREPARER), artifact(12, OTHER), artifact(13, BYSTANDER), artifact(14, PREPARER), artifact(15, OTHER)] as never,
		players: [
			{ id: PREPARER, display_name: 'ravi' }, { id: OTHER, display_name: 'sol' }, { id: BYSTANDER, display_name: 'mei' },
		] as never,
		rankings: [{ id: 1, game_id: 1, category: 'knowledge', player_id: PREPARER, rank: opts.rank ?? 2 }] as never,
		currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: false, rollOutcome: opts.outcome ?? null,
		// Once the artifacts are cast the roll exists, at the difficulty the server worked out.
		activeRoll: (opts.cast
			? { id: 4, plan_id: 9, status: 'rolling', result: null, difficulty: Math.max(opts.rank ?? 2, (opts.invoked ?? []).length) }
			: null) as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(ChronicleHistoriesPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

describe('Chronicle Histories: the difficulty meter', () => {
	it('shows everyone the starting difficulty before the cast: the rank, and a lower rank is better', () => {
		for (const seat of [PREPARER, OTHER, BYSTANDER]) {
			const t = textOf(renderCH({ seat }));
			expect(t).toContain('Starting difficulty 2');
			expect(t).toContain('knowledge rank (2; a lower rank is better)');
			expect(t).toContain('artifacts invoked (0)');
		}
	});

	it('stays after the cast, for everyone, at the difficulty the roll was made at', () => {
		for (const seat of [PREPARER, OTHER, BYSTANDER]) {
			const t = textOf(renderCH({ seat, cast: true, invoked: [11, 12, 13, 14] }));
			expect(t).toContain('Difficulty 4');
			expect(t).toContain('artifacts invoked (4)');
			expect(t).toContain('1 easy');
			// It is no longer a start: the invocations are final.
			expect(t).not.toContain('Starting difficulty');
		}
	});

	it('stays at the rank after the cast when the artifacts invoked do not pass it', () => {
		const t = textOf(renderCH({ seat: BYSTANDER, cast: true, invoked: [11, 12], rank: 3 }));
		expect(t).toContain('Difficulty 3');
		expect(t).toContain('knowledge rank (3;');
	});

	it('is gone once the dice have landed', () => {
		for (const o of ['make', 'mar'] as const) {
			expect(textOf(renderCH({ seat: BYSTANDER, cast: true, invoked: [11, 12], outcome: o }))).not.toContain('1 easy');
		}
	});

	it('leaves the picker to the invoke phase: it is gone after the cast', () => {
		expect(textOf(renderCH({ seat: PREPARER }))).toContain('Invoke artifacts to shed light on');
		expect(textOf(renderCH({ seat: PREPARER, cast: true, invoked: [11] }))).not.toContain('Invoke artifacts to shed light on');
	});
});

describe('Chronicle Histories: the invoke label', () => {
	it('says the rule: more artifacts than the knowledge rank raises the difficulty, and no fewer do', () => {
		const t = textOf(renderCH({ seat: PREPARER }));
		expect(t).toContain('Invoke artifacts to shed light on (more than the knowledge rank raises the difficulty)');
	});

	it('no longer says every artifact raises it', () => {
		for (const seat of [PREPARER, OTHER]) {
			expect(textOf(renderCH({ seat }))).not.toContain('each raises the difficulty');
		}
	});
});
