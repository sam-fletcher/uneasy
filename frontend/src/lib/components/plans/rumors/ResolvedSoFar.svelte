<!-- Rumors/ResolvedSoFar.svelte
  Tier-1 committed state (ADR-006): the specifics of each completed step, shown
  read-only to every viewer so a committed choice persists in the panel once its picker
  commits.

  Nothing here is hidden information. hide_source is fiction-level anonymity — the
  plan.prepared post already names the preparer, and the hide-source log entry states
  the concealment outright as dramatic irony — so listing it, and naming the sheltering
  asset, gives the table nothing it doesn't have. The Secret's *text* is never in
  resolution_data and never shown here.
-->
<script lang="ts">
	import type { Plan } from '$lib/api';
	import { parseResolutionData, assetName } from '../shared';
	import type { PlanContext } from '../types';
	import type { RumorScene } from './shared';

	let { ctx, plan, scene }: {
		ctx: PlanContext;
		plan: Plan;
		scene: RumorScene;
	} = $props();

	const assets = $derived(ctx.assets);

	const srData = $derived(scene.srData);
	const brokenAssetIDs = $derived(srData.broken_asset_ids ?? []);
	const hideSourceAssetIDs = $derived(srData.hide_source_asset_ids ?? []);
	const takenAssetIDs = $derived(srData.taken_asset_ids ?? []);
	const revealedSource = $derived(
		(parseResolutionData(plan).make_mar_choices ?? []).some(c => c.option === 'reveal_source'),
	);
	const leveragedCount = $derived(
		(parseResolutionData(plan).make_mar_choices ?? []).filter(c => c.option === 'leverage_target').length,
	);
	const anyResolved = $derived(
		brokenAssetIDs.length > 0 || hideSourceAssetIDs.length > 0 || takenAssetIDs.length > 0
			|| revealedSource || leveragedCount > 0,
	);
</script>

{#if anyResolved}
	<div class="resolved-so-far">
		<p class="resolved-so-far-label">Resolved so far:</p>
		<ul class="resolved-so-far-list">
			{#each brokenAssetIDs as id}
				<li>Broke <em>{assetName(assets, id)}</em></li>
			{/each}
			{#if leveragedCount > 0}
				<li>
					Leveraged <em>{assetName(assets, (scene.mar ? scene.mcAsset?.id : plan.target_asset_id) ?? 0)}</em>
					{#if leveragedCount > 1}(×{leveragedCount}){/if}
				</li>
			{/if}
			{#each takenAssetIDs as id}
				<li>Took <em>{assetName(assets, id)}</em></li>
			{/each}
			{#each hideSourceAssetIDs as id}
				<li>
					Hid as the source — the secret is discoverable from
					<em>{assetName(assets, id)}</em>
				</li>
			{/each}
			{#if revealedSource}
				<li>{scene.actorName} relished the rumor's glory</li>
			{/if}
		</ul>
	</div>
{/if}
