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

	// Submission status uses revealed_at, not face: faces stay hidden until the
	// whole reveal completes, but revealed_at is set the moment a player submits.
	const mySubmission = $derived(
		reveal?.entries.find(e => e.player_id === currentPlayerID && e.revealed_at != null) ?? null
	);
	const submittedIDs = $derived(
		new Set(reveal?.entries.filter(e => e.revealed_at != null).map(e => e.player_id) ?? [])
	);
	const waitingOn = $derived(
		participants.filter(p => !submittedIDs.has(p.player_id))
	);

	function nameFor(playerID: number): string {
		return participants.find(p => p.player_id === playerID)?.display_name ?? `Player ${playerID}`;
	}

	async function refresh() {
		try {
			reveal = await getReveal(revealID);
			// Restore the local highlight after a reload: the server returns the
			// viewer's own face even before completion, so re-seed `picked` from it.
			if (picked == null && mySubmission?.face != null) {
				picked = mySubmission.face;
			}
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
		<ul class="reveal-list">
			{#each reveal.entries as entry}
				<li>{nameFor(entry.player_id)}: <strong>{entry.face}</strong></li>
			{/each}
		</ul>
		{#if reveal.result_delay != null}
			<p class="choices-note">Result delay: <strong>{reveal.result_delay}</strong></p>
		{/if}
	{:else}
		{@const submitted = mySubmission != null}
		<p class="choices-header">{prompt}</p>
		<!-- After submitting, the picker stays visible with the chosen face
		     highlighted but locked: the faces are disabled and Submit flips to a
		     non-interactive "Submitted" state. -->
		<div class="chip-row face-row">
			{#each faces as face}
				<button
					type="button"
					class="chip-btn face-chip"
					class:active={picked === face}
					aria-label="Pick {face}"
					disabled={submitted || busy}
					onclick={() => (picked = face)}
				>
					<D6Face value={face} size={28} />
				</button>
			{/each}
		</div>
		<div class="submit-row">
			{#if submitted}
				<button class="action-btn submitted" disabled aria-disabled="true">
					<span class="tick">✓</span> Submitted
				</button>
			{:else}
				<button class="action-btn primary" onclick={submit} disabled={busy || picked == null}>
					{busy ? '…' : 'Submit'}
				</button>
			{/if}
		</div>
		{#if submitted}
			<p class="choices-note">
				{#if waitingOn.length === 0}
					All submissions in — revealing…
				{:else}
					Waiting on {waitingOn.length}
					{waitingOn.length === 1 ? 'other' : 'others'}:
					{waitingOn.map(p => p.display_name).join(', ')}
				{/if}
			</p>
		{:else if waitingOn.length < participants.length}
			<p class="choices-note">
				{participants.length - waitingOn.length} of {participants.length} submitted.
			</p>
		{/if}
	{/if}

	{#if error}<p class="error-text">{error}</p>{/if}
</div>

<style>
	.face-row {
		justify-content: center;
	}
	/* Read-only list of revealed faces — not a choice input. */
	.reveal-list {
		list-style: none;
		padding: 0;
		margin: 0;
		font-size: 0.82rem;
		color: var(--color-text-muted);
		line-height: 1.4;
	}
	.submit-row {
		display: flex;
		justify-content: center;
	}
	/* Locked submit button: clearly inactive (grey) once the face is in, with a
	   confirming check. Distinct from a primary :disabled which only fades. */
	.action-btn.submitted {
		background: var(--color-surface-2);
		color: var(--color-text-muted);
		border: 1px solid var(--color-border-strong);
		cursor: default;
		opacity: 1; /* override the global .action-btn:disabled fade */
	}
	.action-btn.submitted .tick {
		color: var(--color-success); /* green tick */
		font-weight: 700;
	}
	/* Locked faces: the chosen one keeps its highlight; the rest fade so the
	   selection stays legible without inviting another click. */
	.face-chip:disabled {
		cursor: default;
	}
	.face-chip:disabled:not(.active) {
		opacity: 0.35;
	}
</style>
