<!-- TurnCard.svelte
  The first object in the choosing view: whose turn it is, how many of your
  three choices you still hold, and the one thing to do next.

  It exists because a claim used to commit with no feedback anywhere the
  player was looking (adr/PROLOGUE_UX_ROUND2_PLAN.md, Motivation). Every
  signal that the turn had moved — the gold ring, the waiting bar, the turn
  count — lived in the header, ~250px above the tiles, and the scroll position
  after a commit was byte-identical, so the page read as static.

  Session 1 lands the on-turn / off-turn pair. The just-claimed state (a
  success-bordered confirmation naming what you made and took, decaying back
  into off-turn) arrives with the motion beat in Session 3.
-->
<script lang="ts">
	import '$lib/components/shared/choicePip.css';

	interface Props {
		/** Who is choosing right now. Null once every player has spent all
		 *  three choices — a transient state, since the server enters the
		 *  ranking step on the last claim, but it renders rather than leaving
		 *  a hole where the card was. */
		activeName: string | null;
		isMyTurn: boolean;
		/** Choices the viewer still holds, 0–3; null for a viewer with no
		 *  player row (spectator), who holds none and gets no pips at all.
		 *  The spent ones are drawn as ghost rings here and as solid pips on
		 *  whichever category headers they went to. */
		unspent: number | null;
		/** The viewer's needlessly-at-risk assets (lib/assetRisk.ts). Owner
		 *  ruling (decision 8): filling marginalia is the right thing to do
		 *  while waiting, so the off-turn card points at it — which also gives
		 *  the header's red number a destination instead of leaving it a bare
		 *  alarm. Suppressed at 0. */
		atRiskCount: number;
		onOpenRetinue?: () => void;
	}

	let { activeName, isMyTurn, unspent, atRiskCount, onOpenRetinue }: Props = $props();

	const TOTAL_PIPS = 3;
	/** Clamped, not trusted: the server caps a player at three claims, but a
	 *  pip row that renders negative or spills past three would misstate the
	 *  one number this card exists to carry. */
	const held = $derived(unspent == null ? null : Math.min(Math.max(unspent, 0), TOTAL_PIPS));
	const showRetinueLine = $derived(!isMyTurn && activeName != null && atRiskCount > 0);
</script>

<section class="turn-card">
	<div class="turn-head">
		{#if isMyTurn}
			<span class="turn-status">Your turn</span>
		{:else if activeName != null}
			<span class="turn-status waiting">{activeName} is choosing</span>
		{:else}
			<span class="turn-status waiting">Everyone has finished choosing</span>
		{/if}

		{#if held != null}
			<!-- Choices you still hold sit solid; a ghost ring is one that has
			     moved down to a category header. -->
			<span class="pips" aria-label={`${held} of ${TOTAL_PIPS} choices left`}>
				{#each Array(TOTAL_PIPS) as _, i}
					<span class="choice-pip" class:spent={i >= held}></span>
				{/each}
			</span>
		{/if}
	</div>

	{#if isMyTurn}
		<p class="turn-line">Claim one tile — from any category.</p>
	{:else if showRetinueLine}
		<p class="turn-line">
			Meanwhile:
			<button type="button" class="inline-link" onclick={() => onOpenRetinue?.()}>
				{atRiskCount === 1
					? '1 of your assets needs marginalia →'
					: `${atRiskCount} of your assets need marginalia →`}
			</button>
		</p>
	{/if}
</section>

<style>
	.turn-card {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		border: 1px solid var(--color-border-warm);
		border-radius: 8px;
		padding: 0.55rem 0.7rem;
	}
	.turn-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.turn-status {
		color: var(--color-accent);
		font-size: 1rem;
		line-height: 1.25;
		min-width: 0;
	}
	.turn-status.waiting { color: var(--color-text-muted); }
	.turn-line {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.82rem;
		line-height: 1.35;
	}

	/* The discs themselves live in shared/choicePip.css — the category headers
	   draw the identical object when one is spent. */
	.pips {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		flex: none;
	}

	/* Inline in a sentence, so it can't take the 44px tap floor without
	   breaking the line it belongs to; the same target is a full-width row in
	   the header's retinue button, which is where a mis-tap lands anyway. */
	.inline-link {
		padding: 0;
		background: none;
		border: none;
		color: var(--color-highlight);
		font: inherit;
		font-size: 0.82rem;
		text-align: left;
		cursor: pointer;
	}
	.inline-link:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
	}
</style>
