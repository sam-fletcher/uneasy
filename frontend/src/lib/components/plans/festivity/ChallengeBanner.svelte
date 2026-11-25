<!-- Festivity/ChallengeBanner.svelte
  Display + accept/decline UI for an outstanding duel challenge. While
  this is present, all other festivity actions pause. Targets who took
  `accept_duels` cannot decline.
-->
<script lang="ts">
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import '$lib/components/shared/actionButton.css';
	import { respondChallenge, type Player } from '$lib/api';
	import { playerName } from '../shared';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

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
	<p class="choices-header">Duel challenge</p>
	<p class="ft-prompt">
		<PlayerName {players} id={challenge.challenger_id} /> challenges
		<PlayerName {players} id={challenge.target_id} />.
	</p>
	{#if challenge.notes}
		<p class="plan-notes">"{challenge.notes}"</p>
	{/if}
	{#if challengeIsForMe}
		{#if respondError}<ErrorText message={respondError} variant="panel" />{/if}
		<div class="step-actions">
			<button class="action-btn primary"
				onclick={() => onRespond(true)}
				disabled={respondBusy}>
				{respondBusy ? '…' : 'Accept challenge'}
			</button>
			<button class="action-btn secondary"
				onclick={() => onRespond(false)}
				disabled={respondBusy || mustAccept}>
				Decline
			</button>
		</div>
		<!-- Said on the page: a greyed button explained only by a tooltip says
		     nothing on a touch screen, which has no hover. -->
		{#if mustAccept}
			<p class="choices-note muted">
				You must accept any duel challenge during this event, so you can't decline.
			</p>
		{/if}
	{:else}
		<p class="choices-note muted">
			Awaiting the target's response. All festivity actions are paused.
		</p>
	{/if}
</div>
