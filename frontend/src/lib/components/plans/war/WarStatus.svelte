<!-- MakeWar/WarStatus.svelte
  Two-column side layout with per-side Join buttons for non-participants
  and a centered "Stay out of it" button below that lifts the war-box
  takeover for the rest of the session.
-->
<script lang="ts">
	import { joinWar, type Player, type WarStateResponse } from '$lib/api';
	import { playerName } from '../shared';
	import { stayOutOfWar } from '$lib/makeWarDismissed';

	let { war, planID, players, amParticipant, onChanged, setError }: {
		war: WarStateResponse;
		planID: number;
		players: Player[];
		amParticipant: boolean;
		onChanged: () => Promise<void> | void;
		setError: (msg: string) => void;
	} = $props();

	let joinBusy = $state(false);

	const sides = [
		{ side: 1 as const, name: 'Declarer' },
		{ side: 2 as const, name: 'Enemies' },
	];

	async function joinSide(side: 1 | 2) {
		if (joinBusy) return;
		joinBusy = true; setError('');
		try {
			await joinWar(planID, side);
			await onChanged();
		} catch (e) {
			setError(e instanceof Error ? e.message : 'Could not join war.');
		} finally { joinBusy = false; }
	}

	function stayOut() {
		stayOutOfWar(planID);
	}
</script>

<div class="choices-section">
	<p class="choices-header">Sides</p>

	<div class="sides-grid">
		{#each sides as { side, name }}
			{@const sideParts = war.participants.filter(p => p.side === side)}
			<div class="side-column">
				<div class="side-header">Side {side} — {name}</div>
				{#if sideParts.length === 0}
					<p class="muted side-empty">(none yet)</p>
				{:else}
					<ul class="side-list">
						{#each sideParts as p}
							<li>
								{playerName(players, p.player_id)}
								{#if p.surrendered_at_row != null}
									<em class="muted">(surrendered, row {p.surrendered_at_row})</em>
								{:else if !p.entry_payment_complete}
									<em class="muted">(joining — owes entry)</em>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}

				{#if !amParticipant && war.status === 'active'}
					<button
						class="action-btn side-join-btn"
						onclick={() => joinSide(side)}
						disabled={joinBusy}
					>
						{joinBusy ? '…' : `Join Side ${side}`}
					</button>
				{/if}
			</div>
		{/each}
	</div>

	{#if !amParticipant && war.status === 'active'}
		<div class="stay-out-row">
			<button class="action-btn stay-out-btn" onclick={stayOut} disabled={joinBusy}>
				Stay out of it
			</button>
		</div>
	{/if}

	{#if war.status === 'ended'}
		<p class="choices-note">
			<strong>The war is over.</strong>
			{#if war.end_reason}({war.end_reason}){/if}
			{#if war.ended_at_row != null} Ended on row {war.ended_at_row}.{/if}
		</p>
	{/if}
</div>

<style>
	.sides-grid {
		display: grid;
		grid-template-columns: 1fr;
		gap: 0.75rem;
	}
	@media (min-width: 560px) {
		.sides-grid {
			grid-template-columns: 1fr 1fr;
		}
	}

	.side-column {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.6rem 0.7rem;
		border: 1px solid #3a322b;
		border-radius: 8px;
		background: rgba(255, 255, 255, 0.02);
	}

	.side-header {
		font-weight: 600;
		color: #c8a96e;
		font-size: 0.9rem;
	}

	.side-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		font-size: 0.9rem;
	}

	.side-empty {
		margin: 0;
	}

	.side-join-btn {
		margin-top: auto;
		min-height: 44px;
	}

	.stay-out-row {
		display: flex;
		justify-content: center;
		margin-top: 0.25rem;
	}

	.stay-out-btn {
		min-height: 44px;
		min-width: 12rem;
	}
</style>
