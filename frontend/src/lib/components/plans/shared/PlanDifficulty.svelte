<!-- shared/PlanDifficulty.svelte
  A plan's difficulty meter, drawn the same way in every plan that rolls. Each panel
  puts this in the open step before the dice land, so the table can see how hard the
  roll is, and why, while it decides whether to help or interfere (the meter was only
  ever in Make Introductions and Chronicle Histories; everywhere else the number was
  a bare figure in the roll's own line).

  What the number is, and why, is worked out in lib/plans/difficulty.ts from state
  every player holds, so a watcher's meter is the actor's. It draws nothing for a plan
  that never rolls, or when what it reads isn't on this client yet.
-->
<script lang="ts">
	import '../planPanel.css';
	import type { Plan } from '$lib/api';
	import { planDifficulty } from '$lib/plans/difficulty';
	import type { PlanContext } from '../types';
	import DifficultyMeter from './DifficultyMeter.svelte';

	let { plan, ctx, invokedCount, headline = 'Difficulty' }: {
		plan: Plan;
		ctx: PlanContext;
		/** Chronicle Histories: the artifacts the preparer has ticked so far. */
		invokedCount?: number;
		headline?: string;
	} = $props();

	const view = $derived(planDifficulty(
		plan,
		{
			plans: ctx.plans, assets: ctx.assets, players: ctx.players,
			rankings: ctx.rankings, activeRoll: ctx.activeRoll,
		},
		{ invokedCount },
	));
</script>

{#if view}
	<div class="choices-section">
		<DifficultyMeter {headline} value={view.value} floor={view.floor} reason={view.reason} />
	</div>
{/if}
