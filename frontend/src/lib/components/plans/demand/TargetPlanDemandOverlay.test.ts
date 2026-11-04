import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import TargetPlanDemandOverlay from './TargetPlanDemandOverlay.svelte';
import { ACTIVE_ROLL_KEY } from './activeRollContext';

// The overlay's forms must show only where the server's routes would act on
// them: not to the plan's own preparer (no hand-off), and not once the target's
// roll has landed (/demand-retarget and /demand-leverage answer 409).

const ALICE = 1; // the target plan's preparer
const BOB = 2; // the demander
const players = [{ id: ALICE, display_name: 'alice' }, { id: BOB, display_name: 'bob' }] as never;

const KEEP = 'Keep the current target';
const LEVERAGE = 'Leverage none';

function overlay(o: {
	viewer: number; targetType?: string; status?: string; rolled?: boolean;
	keepOrChange?: number; leverage?: number; retargetFinalized?: boolean;
	noRoll?: boolean; otherPlanRoll?: boolean; resolution?: string;
}): string {
	const target = {
		id: 10, plan_type: o.targetType ?? 'spread_rumors', status: o.status ?? 'resolving',
		preparer_id: ALICE, target_player_id: BOB, target_asset_id: null,
		resolution_data: JSON.stringify({
			...(o.resolution ? JSON.parse(o.resolution) : {}),
			demand_retarget_finalized: o.retargetFinalized ?? false,
		}),
	};
	const demand = {
		id: 11, plan_type: 'make_demands', status: 'resolved', result: 'make',
		preparer_id: BOB, targeted_plan_id: 10,
		resolution_data: JSON.stringify({ make_demands: { draft_choices: [
			{ option: 'control_leverage', player_id: o.leverage ?? BOB },
			{ option: 'keep_or_change_target', player_id: o.keepOrChange ?? BOB },
			{ option: 'keep_assets', player_id: BOB },
			{ option: 'perform_steps', player_id: BOB },
		] } }),
	};
	// By default the target has an open roll; noRoll / otherPlanRoll vary that.
	const roll = o.noRoll ? null
		: o.otherPlanRoll ? { id: 6, plan_id: 99, outcome: null }
		: { id: 5, plan_id: 10, outcome: o.rolled ? 'make' : null, result: o.rolled ? 4 : null };
	return render(TargetPlanDemandOverlay, {
		props: {
			plan: target as never, plans: [target, demand] as never, players, assets: [],
			currentPlayerID: o.viewer,
		},
		context: new Map([[ACTIVE_ROLL_KEY, () => roll]]),
	}).body;
}

describe('TargetPlanDemandOverlay forms', () => {
	it('shows the demander both forms while the roll is open', () => {
		const body = overlay({ viewer: BOB });
		expect(body).toContain(KEEP);
		expect(body).toContain(LEVERAGE);
	});

	it('shows a bystander neither form', () => {
		const body = overlay({ viewer: 3 });
		expect(body).not.toContain(KEEP);
		expect(body).not.toContain(LEVERAGE);
	});

	it('gives the preparer who won retarget the form, without a roll held for them', () => {
		const body = overlay({ viewer: ALICE, keepOrChange: ALICE, leverage: ALICE });
		expect(body).toContain('Re-aim this plan');
		expect(body).not.toContain('the roll waits on you');
		expect(body).not.toContain(LEVERAGE);
	});

	it('shows no leverage form before the plan has a roll', () => {
		const body = overlay({ viewer: BOB, noRoll: true });
		expect(body).toContain(KEEP);
		expect(body).not.toContain(LEVERAGE);
	});

	it('ignores an active roll that belongs to another plan', () => {
		const body = overlay({ viewer: BOB, otherPlanRoll: true });
		expect(body).not.toContain(LEVERAGE);
		expect(body).toContain(KEEP);
	});

	it('stops saying the roll waits once the winner has settled', () => {
		const body = overlay({ viewer: 3, retargetFinalized: true });
		expect(body).not.toContain('the roll waits until they say');
		expect(overlay({ viewer: 3 })).toContain('the roll waits until they say');
	});

	it('drops both forms once the dice have landed', () => {
		const body = overlay({ viewer: BOB, rolled: true, retargetFinalized: true });
		expect(body).not.toContain(KEEP);
		expect(body).not.toContain('Re-aim this plan');
		expect(body).not.toContain(LEVERAGE);
		expect(body).not.toContain('adjust it until');
	});

	it('keeps the retarget form for a plan that has not resolved, before any roll exists', () => {
		const body = overlay({ viewer: BOB, status: 'pending' });
		expect(body).toContain(KEEP);
	});

	it('says the retarget stays open until the dice land on a rolling plan', () => {
		const body = overlay({ viewer: BOB, retargetFinalized: true });
		expect(body).toContain('until the dice land');
		expect(body).not.toContain('until the plan resolves');
	});

	it('closes the form on a duel or liaison once the winner has settled the aim', () => {
		for (const t of ['clandestinely_liaise', 'propose_duel']) {
			const body = overlay({ viewer: BOB, targetType: t, retargetFinalized: true });
			expect(body).not.toContain(KEEP);
			expect(body).not.toContain('Re-aim this plan');
		}
	});

	it('holds a duel or liaison before its first step, not a roll', () => {
		for (const t of ['clandestinely_liaise', 'propose_duel']) {
			const winner = overlay({ viewer: BOB, targetType: t, noRoll: true });
			expect(winner).toContain(KEEP);
			expect(winner).toContain('the plan waits on you before its first step');
			expect(winner).not.toContain('the roll waits');
			expect(overlay({ viewer: 3, targetType: t, noRoll: true })).toContain('the plan waits until they say');
		}
	});

	it('closes a duel to a preparer winner once the first step has begun', () => {
		const started = JSON.stringify({ duel: { phase: 'staking' } });
		const body = overlay({ viewer: ALICE, keepOrChange: ALICE, targetType: 'propose_duel', noRoll: true, resolution: started });
		expect(body).not.toContain('Re-aim this plan');
		const early = overlay({ viewer: ALICE, keepOrChange: ALICE, targetType: 'propose_duel', noRoll: true });
		expect(early).toContain('Re-aim this plan');
	});

	it('keeps the form open on a duel or liaison that has not started resolving (no phase yet)', () => {
		for (const t of ['clandestinely_liaise', 'propose_duel']) {
			const body = overlay({ viewer: BOB, targetType: t, status: 'pending', noRoll: true,
				resolution: JSON.stringify({ duel: { phase: '' }, liaise: { phase: '' } }) });
			expect(body).toContain(KEEP);
		}
	});

	it('closes it for no one waiting once the duel is under way', () => {
		const body = overlay({ viewer: 3, targetType: 'propose_duel', noRoll: true,
			resolution: JSON.stringify({ duel: { phase: 'bouts' } }) });
		expect(body).not.toContain('the plan waits until they say');
	});

	it('is a dud on every other plan: no form, no roll-hold copy', () => {
		for (const t of ['spread_propaganda', 'seek_answers', 'make_introductions', 'chronicle_histories']) {
			const body = overlay({ viewer: BOB, targetType: t });
			expect(body).not.toContain(KEEP);
			expect(body).not.toContain('the roll waits');
			expect(body).toContain('came to nothing here');
		}
	});
});

describe('TargetPlanDemandOverlay keep_assets banner', () => {
	it('promises spoils from a plan that awards assets', () => {
		expect(overlay({ viewer: 3 })).toContain('go to bob instead');
	});

	it.each(['seek_answers', 'chronicle_histories'])('does not on %s, which awards none', (t) => {
		const body = overlay({ viewer: 3, targetType: t });
		expect(body).toContain('Demand in effect from');
		expect(body).not.toContain('go to bob instead');
	});
});
