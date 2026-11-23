<!-- Seek/ResolvedSoFar.svelte
  Tier-1 committed state (ADR-006): the specifics of every step done so far, shown at
  full opacity to EVERY viewer so the made choices persist after a picker commits. Sits
  outside the greyed picker fieldset, like Propose Decree's live law text.
-->
<script lang="ts">
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import type { SeekScene } from './shared';

	let { ctx, scene }: {
		ctx: PlanContext;
		scene: SeekScene;
	} = $props();

	const players = $derived(ctx.players);
	const assetName = (id: number) => scene.assetName(id);
</script>

{#if scene.anyResolved}
	<div class="resolved-so-far">
		<p class="resolved-so-far-label">Resolved so far:</p>
		<ul class="resolved-so-far-list">
			{#each scene.makeBrokenIDs as id}
				<li>Broke <em>{assetName(id)}</em></li>
			{/each}
			{#each scene.revealedAssetIDs as id}
				<li>Learned the secrets of <em>{assetName(id)}</em></li>
			{/each}
			{#each scene.declaredTruths as t}
				<li>Declared true: “{t}”</li>
			{/each}
			{#each scene.answeredQuestions as q}
				<li>
					Asked {playerName(players, q.target_id)}: “{q.question}” —
					answered “{q.answer}”
				</li>
			{/each}
			{#each scene.penaltyBrokenIDs as id}
				<li>Broke own resource, <em>{assetName(id)}</em></li>
			{/each}
		</ul>
	</div>
{/if}
