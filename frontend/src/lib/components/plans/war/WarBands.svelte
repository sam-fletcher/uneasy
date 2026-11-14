<!--
  WarBands.svelte — the way into the wars, at the top of Play.

  One band for each live war (planned or under way), and the sheet that the
  pressed one opens. It replaces the War button and the drawer.

  Which sheet is open is this component's own state, since nothing else on the
  page needs it. It is stored as a plan id and looked up in the live list, so a
  war that ends under an open sheet closes it. The id is also cleared then (see
  `keepOpenWar`): the war list is emptied whenever its fetch fails, and the next
  good fetch would otherwise find the id again and reopen the sheet unasked.

  `plans` is the live wars' plans (lib/warGate.ts activeWarPlans); `wars` is the
  game's war states, for the sheet's lede; `takeoverPlanIDs` are the wars the play
  area is settling right now. Nothing is drawn with no live war.
-->
<script lang="ts">
	import type { Plan, Player, WarStateResponse } from '$lib/api';
	import type { PlanContext } from '$lib/components/plans/types';
	import { keepOpenWar } from '$lib/warGate';
	import WarBand from './WarBand.svelte';
	import WarSheet from './WarSheet.svelte';

	let { plans, wars, players, ctx, takeoverPlanIDs = [] }: {
		plans: Plan[];
		wars: WarStateResponse[];
		players: Player[];
		ctx: PlanContext;
		takeoverPlanIDs?: number[];
	} = $props();

	let openID = $state<number | null>(null);
	$effect(() => {
		const keep = keepOpenWar(openID, plans.map(p => p.id));
		if (keep !== openID) openID = keep;
	});
	const openPlan = $derived(plans.find(p => p.id === openID) ?? null);
	const openWar = $derived(wars.find(w => w.origin_plan_id === openID) ?? null);
</script>

{#if plans.length > 0}
	<div class="war-bands">
		{#each plans as p (p.id)}
			<WarBand plan={p} {players} onOpen={() => (openID = p.id)} />
		{/each}
	</div>
{/if}

<WarSheet
	plan={openPlan}
	war={openWar}
	{players}
	{ctx}
	settledInPlay={openID != null && takeoverPlanIDs.includes(openID)}
	onClose={() => (openID = null)}
/>

<style>
	.war-bands {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		flex: none;
	}
</style>
