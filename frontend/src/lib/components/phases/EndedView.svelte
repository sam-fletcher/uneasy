<!-- EndedView.svelte
  Ended phase: the closing message and, once rankings exist, the final
  Power/Knowledge/Esteem standings.

  Nothing is framed: the standings are three short lists, each a small label
  and its rows, and the spacing is enough to tell them apart. Each row is the
  player's shape and name in their colour (the same identity the player row
  draws), with their rank first. They are stacked rather than set in three
  columns: a column a third of a phone wide has room for about eight letters
  of a name, and the names are the point of this screen.
-->
<script lang="ts">
	import type { Player, Ranking, RankingCategory } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';

	let { rankings, players }: { rankings: Ranking[]; players: Player[] } = $props();

	const CATEGORIES = ['power', 'knowledge', 'esteem'] as const;

	/** The seated players of one track, best rank first. Dummy slots (no
	 *  player) hold a rank but aren't anyone, so they are left out. */
	function standings(cat: RankingCategory) {
		return rankings
			.filter((r) => r.category === cat && r.player_id !== null)
			.sort((a, b) => a.rank - b.rank)
			.map((r) => ({ rank: r.rank, player: players.find((p) => p.id === r.player_id) ?? null }));
	}
</script>

<div class="phase-view ended">
	<h2>Game Over</h2>
	<p class="closing">The public record is sealed. Thank you for playing.</p>
	{#if rankings.length > 0}
		<section class="standings">
			<h3>Final Rankings</h3>
			{#each CATEGORIES as cat (cat)}
				<div class="track">
					<h4>{cat}</h4>
					<ol>
						{#each standings(cat) as s (s.rank)}
							<li>
								<span class="rank">{s.rank}.</span>
								{#if s.player}
									<PlayerShape player={s.player} />
									<span class="name" style:color={playerColor(s.player)}>{s.player.display_name}</span>
								{:else}
									<span class="name unknown">?</span>
								{/if}
							</li>
						{/each}
					</ol>
				</div>
			{/each}
		</section>
	{/if}
</div>

<style>
	.phase-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: var(--space-4) var(--space-3);
		gap: var(--space-4);
		overflow-y: auto;
		min-height: 0;
	}

	h2,
	h3 {
		margin: 0;
		color: var(--color-accent);
		font-size: var(--text-heading);
	}
	.closing {
		margin: 0;
		font-size: var(--text-body);
	}

	/* The three tracks are neighbours in one section, so the space between them
	   is the larger one and the space inside a track the smaller. */
	.standings {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.track {
		display: flex;
		flex-direction: column;
	}
	/* A small label and a rule, like the profile page's group headings, so the
	   first row of a track has a line above it as the others do. */
	h4 {
		margin: 0;
		padding-bottom: var(--space-1);
		border-bottom: 1px solid var(--color-border);
		font-size: var(--text-label);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--color-text-secondary);
	}
	ol {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) 0;
		border-bottom: 1px solid var(--color-border);
		font-size: var(--text-small);
	}
	.rank {
		min-width: 1.5em;
		color: var(--color-text-secondary);
		font-variant-numeric: tabular-nums;
	}
	.name {
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
	.name.unknown { color: var(--color-text-secondary); }
</style>
