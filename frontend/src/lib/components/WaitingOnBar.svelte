<!-- WaitingOnBar.svelte
  Page-level strip that answers "whose action is the game waiting on?"
  Rendered once in +page.svelte across every phase. Each phase view computes
  its own WaitingOnState and writes it via the page's bindable state.

  Display rules:
    - "Waiting On:" followed by a comma-separated waitee list.
    - The current player renders as the literal word "You" in their player
      colour, sorted to the front of the list.
    - { kind: 'everyone' } collapses the list to the word "Everyone".
    - { kind: 'label', text } renders free text for non-player waitees
      (e.g. "1 more player to join"). Note it is never used to name a ROLE:
      once the lobby has two players the bar names the facilitator as a
      player waitee, so their seat and header chip light up with it.
    - Empty waitees → the bar hides entirely.
-->
<script lang="ts" module>
	import type { Player } from '$lib/api';
	// Types now live in $lib/waitingOn (the pure derivation module) so the
	// dependency points component → lib, not lib → component. Re-exported here
	// for any legacy importers that still reach for them via this component.
	import type { Waitee, WaitingOnState } from '$lib/waitingOn';
	export type { Waitee, WaitingOnState } from '$lib/waitingOn';
</script>

<script lang="ts">
	import { playerColor } from '$lib/playerColor';

	interface Props {
		state: WaitingOnState;
		currentPlayerID: number | null;
		players: Player[];
	}
	let { state, currentPlayerID, players }: Props = $props();

	const orderedWaitees = $derived.by<Waitee[]>(() => {
		const ws = state.waitees;
		const youIdx = ws.findIndex(
			w => w.kind === 'player' && currentPlayerID != null && w.playerID === currentPlayerID
		);
		if (youIdx <= 0) return ws;
		return [ws[youIdx], ...ws.slice(0, youIdx), ...ws.slice(youIdx + 1)];
	});

	const isEveryone = $derived(
		orderedWaitees.length === 1 && orderedWaitees[0].kind === 'everyone'
	);

	// True when the current player is among the waitees (or it's "everyone").
	// When false, the step heading mutes to the same grey as "Waiting On:" —
	// reinforces that the play surface is read-only for this viewer.
	const isWaitingOnMe = $derived(
		isEveryone ||
		(currentPlayerID != null &&
			orderedWaitees.some(w => w.kind === 'player' && w.playerID === currentPlayerID))
	);

	function nameFor(id: number): string {
		return players.find(p => p.id === id)?.display_name ?? '?';
	}
	function colorFor(id: number): string {
		return playerColor(players.find(p => p.id === id));
	}
</script>

{#if orderedWaitees.length > 0}
	<div class="waiting-on-bar">
		{#if state.stepLabel || state.stepSubtitle}
			<div class="heading">
				{#if state.stepLabel}
					<p class="line step-label" class:muted={!isWaitingOnMe}>{state.stepLabel}</p>
				{/if}
				{#if state.stepSubtitle}
					<p class="line step-subtitle">{state.stepSubtitle}</p>
				{/if}
			</div>
		{/if}
		<p class="line waitees-line">
			<span class="label">Waiting On:</span>
			{#if isEveryone}
				<span class="waitee">Everyone</span>
			{:else}
				{#each orderedWaitees as w, i}
					{#if w.kind === 'player' && currentPlayerID != null && w.playerID === currentPlayerID}
						<span class="waitee you" style:color={colorFor(w.playerID)}>You</span>
					{:else if w.kind === 'player'}
						<span class="waitee">{nameFor(w.playerID)}</span>
					{:else if w.kind === 'label'}
						<span class="waitee">{w.text}</span>
					{/if}{#if i < orderedWaitees.length - 1}<span class="sep">,</span>{/if}
				{/each}
			{/if}
		</p>
	</div>
{/if}

<style>
	/* Heading (step label + optional subtitle) on the left, waitees on the
	   right. A flex-wrap row rather than fixed columns: the heading's basis is
	   its own text width, so when label + waitees can't share a line (a long
	   label like "Exchange Courtiers — target responds" at 375px) the waitees
	   drop to their own right-aligned line instead of the label running under
	   "Waiting On:". The heading can still shrink and wrap if it alone is
	   wider than the bar. */
	.waiting-on-bar {
		display: flex;
		flex-wrap: wrap;
		column-gap: 1rem;
		row-gap: 0.15rem;
		align-items: baseline;
		justify-content: space-between;
		padding: 0.5rem 0.75rem;
		background: var(--color-bg);
		border-bottom: 1px solid var(--color-border);
		flex-shrink: 0;
	}
	.heading {
		flex: 1 1 auto;
		min-width: 0;
		display: flex;
		flex-direction: column;
		row-gap: 0.15rem;
	}
	.line {
		margin: 0;
		font-size: 0.9rem;
		line-height: 1.3;
	}
	.waitees-line {
		flex: 0 1 auto;
		margin-left: auto;
		text-align: right;
		color: var(--color-text);
	}
	.label {
		color: var(--color-text-muted);
		margin-right: 0.35rem;
	}
	.waitee {
		display: inline;
	}
	.sep {
		color: var(--color-text-muted);
		margin-right: 0.3rem;
	}
	.step-label {
		color: var(--color-accent);
		font-size: 0.95rem;
	}
	.step-label.muted {
		color: var(--color-text-muted);
		/* Override the global `.muted` rule in plans/planPanel.css, which
		   leaks in because that file is imported as a plain CSS module. */
		font-style: normal;
		font-size: 0.95rem;
	}
	.step-subtitle {
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
</style>
