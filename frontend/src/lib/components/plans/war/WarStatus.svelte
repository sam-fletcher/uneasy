<!-- MakeWar/WarStatus.svelte
  Participants grouped by side, join buttons for non-participants, and the
  short "war ended" summary when applicable.
-->
<script lang="ts">
	import { joinWar, type Player, type WarStateResponse } from '$lib/api';
	import { playerName } from '../shared';

	let { war, planID, players, amParticipant, onChanged, setError }: {
		war: WarStateResponse;
		planID: number;
		players: Player[];
		amParticipant: boolean;
		onChanged: () => Promise<void> | void;
		setError: (msg: string) => void;
	} = $props();

	let joinBusy = $state(false);

	function sideName(s: 1 | 2): string {
		return s === 1 ? 'Side 1 (declarer)' : 'Side 2 (enemies)';
	}

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
</script>

<div class="choices-section">
	<p class="choices-header">Sides</p>
	{#each [1, 2] as s}
		{@const sideParts = war.participants.filter(p => p.side === s)}
		<div class="choices-note">
			<strong>{sideName(s as 1 | 2)}:</strong>
			{#if sideParts.length === 0}
				<em>(empty)</em>
			{:else}
				{#each sideParts as p, i}
					{i > 0 ? ', ' : ''}
					<span>
						{playerName(players, p.player_id)}
						{#if p.surrendered_at_row != null}
							<em>(surrendered, row {p.surrendered_at_row})</em>
						{:else if !p.entry_payment_complete}
							<em>(joining — owes entry)</em>
						{/if}
					</span>
				{/each}
			{/if}
		</div>
	{/each}

	{#if !amParticipant && war.status === 'active'}
		<p class="choices-note">
			Join the war (free during the delay reveal; afterwards you'll
			owe a cost-of-battle entry against every existing opposing
			participant before counting as a full member):
		</p>
		<div style="display:flex;gap:0.5rem;flex-wrap:wrap;">
			<button class="action-btn" onclick={() => joinSide(1)} disabled={joinBusy}>
				{joinBusy ? '…' : 'Join Side 1'}
			</button>
			<button class="action-btn" onclick={() => joinSide(2)} disabled={joinBusy}>
				{joinBusy ? '…' : 'Join Side 2'}
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
