<!-- SetAsidePlacer.svelte
  Shown during the place_set_asides_X step. Renders the open ranks on
  the active track with the set-aside players slotted in tentatively;
  the track's top-ranked player can reorder. Everyone else sees a
  "Decided by [name]" overlay.

  When there's only one set-aside, the server auto-places it and this
  component is never shown.
-->
<script lang="ts">
	import type { Player } from '$lib/api';

	interface Props {
		players: Player[];
		setAsideOrdering: number[];
		openRanks: number[]; // ascending
		topTrackPlayerID: number;
		isMyTurn: boolean;
		busy?: boolean;
		onReorder: (next: number[]) => void;
		onConfirm: () => void;
	}

	let {
		players,
		setAsideOrdering,
		openRanks,
		topTrackPlayerID,
		isMyTurn,
		busy = false,
		onReorder,
		onConfirm
	}: Props = $props();

	function playerName(id: number | null): string {
		if (id == null) return '';
		return players.find((p) => p.id === id)?.display_name ?? '?';
	}

	function move(i: number, dir: -1 | 1) {
		const j = i + dir;
		if (j < 0 || j >= setAsideOrdering.length) return;
		const next = [...setAsideOrdering];
		[next[i], next[j]] = [next[j], next[i]];
		onReorder(next);
	}
</script>

<div class="placer">
	<div class="placer-head">
		<span class="placer-title">Set-aside placement</span>
		{#if !isMyTurn}
			<span class="decided-by">Decided by {playerName(topTrackPlayerID)}</span>
		{/if}
	</div>

	<ol class="placer-list">
		{#each setAsideOrdering as pid, i}
			<li class="placer-row">
				<span class="rank-label">Rank {openRanks[i] ?? '?'}</span>
				<span class="placer-name">{playerName(pid)}</span>
				{#if isMyTurn}
					<div class="placer-controls">
						<button
							class="arrow"
							aria-label="Move up"
							disabled={i === 0 || busy}
							onclick={() => move(i, -1)}
						>↑</button>
						<button
							class="arrow"
							aria-label="Move down"
							disabled={i === setAsideOrdering.length - 1 || busy}
							onclick={() => move(i, 1)}
						>↓</button>
					</div>
				{/if}
			</li>
		{/each}
	</ol>

	{#if isMyTurn}
		<button class="primary confirm" onclick={onConfirm} disabled={busy}>
			{busy ? '…' : 'Confirm placement'}
		</button>
	{/if}
</div>

<style>
	.placer {
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		padding: 0.6rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.placer-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}
	.placer-title {
		color: var(--color-accent);
		font-size: 0.9rem;
	}
	.decided-by {
		color: var(--color-text-muted);
		font-size: 0.8rem;
		font-style: italic;
	}
	.placer-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.placer-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0.5rem;
		background: var(--color-surface-2);
		border-radius: 4px;
		min-height: 44px;
	}
	.rank-label {
		color: var(--color-text-muted);
		font-size: 0.75rem;
		min-width: 3.5rem;
	}
	.placer-name {
		flex: 1;
		color: var(--color-text);
		font-size: 0.9rem;
	}
	.placer-controls {
		display: flex;
		gap: 0.25rem;
	}
	.arrow {
		background: var(--color-border);
		color: var(--color-accent);
		border: 1px solid #555;
		border-radius: 4px;
		min-width: 36px;
		min-height: 36px;
		font-size: 0.95rem;
		cursor: pointer;
	}
	.arrow:disabled { opacity: 0.3; cursor: not-allowed; }

	.primary {
		background: var(--color-accent);
		color: var(--color-bg);
		padding: 0.5rem 1rem;
		border-radius: 6px;
		border: none;
		cursor: pointer;
	}
	.primary:disabled { opacity: 0.4; cursor: not-allowed; }
	.confirm { align-self: flex-start; min-height: 44px; }
</style>
