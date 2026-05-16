<!-- SimultaneousRevealInput.svelte
  Shared widget for simultaneous die-face reveals (liaise delay/redelay,
  make war delay, duel stake count). Each participant picks a face; nothing
  is shown until every expected participant has submitted, at which point
  all faces are revealed.
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount } from 'svelte';
	import { getReveal, submitReveal, type SimultaneousReveal } from '$lib/api';
	import { useWindowEvents } from '$lib/useWindowEvents';
	import { REVEAL_EVENTS } from '$lib/ws';
	import D6Face from './D6Face.svelte';

	interface Participant {
		player_id: number;
		display_name: string;
	}

	interface Props {
		revealID: number;
		currentPlayerID: number;
		participants: Participant[];
		/** If true, 0 is a valid face (used by liaise cancel). Default: false. */
		allowZero?: boolean;
		/** Inclusive upper bound on the face value. Default: 6. */
		maxFace?: number;
		/** Label shown above the picker. */
		prompt?: string;
	}

	let {
		revealID,
		currentPlayerID,
		participants,
		allowZero = false,
		maxFace = 6,
		prompt = 'Pick a face',
	}: Props = $props();

	let reveal = $state<SimultaneousReveal | null>(null);
	let picked = $state<number | null>(null);
	let busy = $state(false);
	let error = $state('');

	const minFace = $derived(allowZero ? 0 : 1);
	const faces = $derived(
		Array.from({ length: maxFace - minFace + 1 }, (_, i) => minFace + i)
	);

	const mySubmission = $derived(
		reveal?.entries.find(e => e.player_id === currentPlayerID) ?? null
	);
	const submittedIDs = $derived(new Set(reveal?.entries.map(e => e.player_id) ?? []));
	const waitingOn = $derived(
		participants.filter(p => !submittedIDs.has(p.player_id))
	);

	function nameFor(playerID: number): string {
		return participants.find(p => p.player_id === playerID)?.display_name ?? `Player ${playerID}`;
	}

	async function refresh() {
		try {
			reveal = await getReveal(revealID);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load reveal';
		}
	}

	async function submit() {
		if (picked == null || busy) return;
		busy = true;
		error = '';
		try {
			await submitReveal(revealID, picked);
			await refresh();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to submit';
		} finally {
			busy = false;
		}
	}

	function onRevealEvent(e: Event) {
		const detail = (e as CustomEvent<{ reveal_id: number }>).detail;
		if (detail.reveal_id === revealID) refresh();
	}

	useWindowEvents(REVEAL_EVENTS, onRevealEvent);
	onMount(refresh);
</script>

<div class="choices-section">
	{#if !reveal}
		<p class="choices-note">Loading reveal…</p>
	{:else if reveal.is_complete}
		<p class="choices-header"><strong>Reveal complete</strong></p>
		<ul class="choice-item" style="list-style:none;padding:0;">
			{#each reveal.entries as entry}
				<li>{nameFor(entry.player_id)}: <strong>{entry.face}</strong></li>
			{/each}
		</ul>
		{#if reveal.result_delay != null}
			<p class="choices-note">Result delay: <strong>{reveal.result_delay}</strong></p>
		{/if}
	{:else if mySubmission}
		<p class="choices-note">
			You've submitted. Waiting on {waitingOn.length}
			{waitingOn.length === 1 ? 'other' : 'others'}:
			{waitingOn.map(p => p.display_name).join(', ')}
		</p>
	{:else}
		<p class="choices-header">{prompt}</p>
		<div class="chip-row" style="margin:0.5rem 0;">
			{#each faces as face}
				<button
					type="button"
					class="chip-btn face-chip"
					class:active={picked === face}
					aria-label="Pick {face}"
					onclick={() => (picked = face)}
				>
					<D6Face value={face} size={28} />
				</button>
			{/each}
		</div>
		<button class="action-btn primary" onclick={submit} disabled={busy || picked == null}>
			{busy ? '…' : 'Submit'}
		</button>
		{#if waitingOn.length < participants.length}
			<p class="choices-note">
				{participants.length - waitingOn.length} of {participants.length} submitted.
			</p>
		{/if}
	{/if}

	{#if error}<p class="error-text">{error}</p>{/if}
</div>
