<!-- Festivity/ChallengeBanner.svelte
  Display + accept/decline UI for an outstanding duel challenge. While
  this is present, all other festivity actions pause. Targets who took
  `accept_duels` cannot decline.
-->
<script lang="ts">
	import { respondChallenge, type Player } from '$lib/api';
	import { playerName } from '../shared';

	type Challenge = { challenger_id: number; target_id: number; notes?: string };

	let { planID, challenge, players, currentPlayerID, mustAccept, onPlansChanged }: {
		planID: number;
		challenge: Challenge;
		players: Player[];
		currentPlayerID: number | null;
		mustAccept: boolean;
		onPlansChanged: () => void;
	} = $props();

	const challengeIsForMe = $derived(
		currentPlayerID != null && challenge.target_id === currentPlayerID,
	);

	let respondBusy = $state(false);
	let respondError = $state('');

	async function onRespond(accept: boolean) {
		if (respondBusy) return;
		respondBusy = true; respondError = '';
		try { await respondChallenge(planID, accept); onPlansChanged(); }
		catch (e) { respondError = e instanceof Error ? e.message : 'Could not respond.'; }
		finally { respondBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">
		Duel challenge:
		<strong>{playerName(players, challenge.challenger_id)}</strong>
		→ <strong>{playerName(players, challenge.target_id)}</strong>
	</p>
	{#if challenge.notes}
		<p class="plan-notes">"{challenge.notes}"</p>
	{/if}
	{#if challengeIsForMe}
		{#if respondError}<p class="res-error">{respondError}</p>{/if}
		<div class="form-row">
			<button class="action-btn primary"
				onclick={() => onRespond(true)}
				disabled={respondBusy}>
				{respondBusy ? '…' : 'Accept challenge'}
			</button>
			<button class="action-btn"
				onclick={() => onRespond(false)}
				disabled={respondBusy || mustAccept}
				title={mustAccept ? 'You took accept_duels and cannot decline' : ''}>
				Decline
			</button>
		</div>
	{:else}
		<p class="choices-note muted">
			Awaiting the target's response. All festivity actions are paused.
		</p>
	{/if}
</div>
