<!-- EndedView.svelte
  Ended phase: the closing message and, once rankings exist, the final
  Power/Knowledge/Esteem standings.
-->
<script lang="ts">
	import type { Player, Ranking, RankingCategory } from '$lib/api';

	let { rankings, players }: { rankings: Ranking[]; players: Player[] } = $props();

	function rankingLabel(playerID: number | null): string {
		if (playerID === null) return 'Dummy';
		return players.find(p => p.id === playerID)?.display_name ?? '?';
	}
</script>

<div class="phase-view ended">
	<h2>Game Over</h2>
	<p class="muted-text">The public record is sealed. Thank you for playing.</p>
	{#if rankings.length > 0}
		<h3>Final Rankings</h3>
		<div class="rankings-preview">
			{#each ['power', 'knowledge', 'esteem'] as cat}
				<div class="rank-col">
					<h4>{cat}</h4>
					{#each rankings.filter(r => r.category === (cat as RankingCategory) && r.player_id !== null).sort((a, b) => a.rank - b.rank) as r}
						<div class="rank-slot-display">{r.rank}. {rankingLabel(r.player_id)}</div>
					{/each}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.phase-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: 1rem 0.75rem;
		gap: 1rem;
		overflow-y: auto;
		min-height: 0;
	}

	.phase-view h2 {
		color: var(--color-accent);
		font-size: 1.3rem;
		margin: 0;
	}

	.phase-view h3 {
		color: var(--color-accent);
		font-size: 1rem;
		margin: 0;
	}

	.rankings-preview {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}

	.rank-col { display: flex; flex-direction: column; gap: 0.2rem; }

	.rank-col h4 {
		font-size: 0.8rem;
		color: var(--color-accent);
		text-transform: capitalize;
		margin: 0 0 0.4rem;
	}

	.rank-slot-display {
		font-size: 0.85rem;
		color: var(--color-text-muted);
		padding: 0.15rem 0;
	}
</style>
