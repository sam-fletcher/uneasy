<!-- MakeWar/PeaceFlow.svelte
  Open peace-proposal display + accept/reject voting. Proposing peace is
  not done here — it's an option inside the cost-of-battle picker, since
  proposing replaces paying cost that row.
-->
<script lang="ts">
	import EndsNote from '../shared/EndsNote.svelte';
	import '$lib/components/shared/actionButton.css';
	import { votePeace, type Player, type WarStateResponse } from '$lib/api';
	import { playerName } from '../shared';

	type Proposal = NonNullable<WarStateResponse['open_proposal']>;

	let { proposal, planID, players, currentPlayerID, amFullParticipant, onChanged, setError }: {
		proposal: Proposal;
		planID: number;
		players: Player[];
		currentPlayerID: number | null;
		amFullParticipant: boolean;
		onChanged: () => Promise<void> | void;
		setError: (msg: string) => void;
	} = $props();

	const myProposalVote = $derived(
		proposal.votes.find(v => v.player_id === currentPlayerID) ?? null,
	);
	const canVoteProposal = $derived(
		amFullParticipant
		&& proposal.proposer_id !== currentPlayerID
		&& !myProposalVote,
	);

	let voteBusy = $state(false);
	async function castVote(accepted: boolean) {
		if (voteBusy) return;
		voteBusy = true; setError('');
		try {
			await votePeace(planID, proposal.id, accepted);
			await onChanged();
		} catch (e) {
			setError(e instanceof Error ? e.message : 'Could not vote.');
		} finally { voteBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">
		Peace proposal from {playerName(players, proposal.proposer_id)}
	</p>
	<p class="plan-notes">"{proposal.terms}"</p>
	<p class="choices-note">
		Accepted by:
		{#if proposal.votes.filter(v => v.accepted).length === 0}
			nobody yet
		{:else}
			{proposal.votes.filter(v => v.accepted)
				.map(v => playerName(players, v.player_id)).join(', ')}
		{/if}
	</p>
	{#if proposal.awaiting.length > 0}
		<p class="choices-note muted">
			Waiting on: {proposal.awaiting.map(id => playerName(players, id)).join(', ')}
		</p>
	{/if}
	{#if canVoteProposal}
		<div class="form-row">
			<button class="action-btn primary"
				onclick={() => castVote(true)} disabled={voteBusy}>
				{voteBusy ? '…' : 'Accept'}
			</button>
			<button class="action-btn secondary"
				onclick={() => castVote(false)} disabled={voteBusy}>
				{voteBusy ? '…' : 'Reject'}
			</button>
		</div>
		<EndsNote show={proposal.awaiting.length === 1 && proposal.votes.every(v => v.accepted)}
			text="Everyone else has accepted: if you accept, the war ends in peace. A rejection keeps it going." />
	{:else if myProposalVote}
		<p class="choices-note muted">
			You voted: {myProposalVote.accepted ? 'accept' : 'reject'}
		</p>
	{:else if !amFullParticipant}
		<p class="choices-note muted">Only active war participants vote on peace.</p>
	{/if}
</div>
