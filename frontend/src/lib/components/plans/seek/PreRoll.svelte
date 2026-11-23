<!-- Seek/PreRoll.svelte
  Pre-roll: restate methods + one thing learned, then cast.

  OnResolve opens the plan with no roll; the preparer writes their pre-roll narration
  and casts via cast-roll, which posts it and creates the dice. The step is the shared
  PreRollStep, so its button says it opens the roll. The narration is the preparer's
  own; only they get the form, and everyone else reads the open step's heading for
  whose move it is.
-->
<script lang="ts">
	import { castSeekAnswersRoll, type Plan } from '$lib/api';
	import PreRollStep from '../shared/PreRollStep.svelte';
	import type { PlanContext } from '../types';
	import type { SeekScene } from './shared';
	import { TEXT_LIMITS } from '$lib/textLimits';

	let { ctx, plan, scene, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: SeekScene;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onRollCreated = $derived(ctx.onRollCreated);

	let preRollBusy = $state(false);
	async function submitPreRoll(p: Plan, values: Record<string, string>) {
		const narration = (values.narration ?? '').trim();
		if (preRollBusy || !narration) return;
		preRollBusy = true; error = '';
		try {
			const res = await castSeekAnswersRoll(p.id, narration);
			if (res.roll) onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not cast your dice.';
		} finally { preRollBusy = false; }
	}
</script>

<PreRollStep
	title="What have you learned?"
	prompt="Restate your methods of research, then describe one thing that you've learned while researching."
	fields={[{
		key: 'narration',
		label: 'Your methods, and one thing you have learned',
		placeholder: 'Where you have been looking, and what turned up.',
		rows: 4,
		max: TEXT_LIMITS.LONG_TEXT,
	}]}
	canAct={scene.isPreparer}
	busy={preRollBusy}
	onSubmit={(v) => submitPreRoll(plan, v)}
/>
