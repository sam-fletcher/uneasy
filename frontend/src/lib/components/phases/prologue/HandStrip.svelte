<!-- HandStrip.svelte
  Persistent hearts hand for the active player during the prologue
  ranking declare step. Tap a heart to commit it to the active track,
  tap a committed heart to retract. Hearts that are bright on the
  active track render with a "spent" indicator; greyed (wasted) hearts
  return to the hand pool unaffected.

  Hearts locked into a previously-resolved track are disabled.

  Heading: "Maximum commitment if needed".
-->
<script lang="ts">
	import type { CommittedHeart, PlayerCardRow, PrologueTrack } from '$lib/api';
	import { cardRank } from '$lib/prologue/refund';

	interface Props {
		myCards: PlayerCardRow[];
		committed: CommittedHeart[];
		activeTrack: PrologueTrack;
		brightSet: Set<number>;
		busy?: boolean;
		resolvedTracks: Set<PrologueTrack>;
		onCommit: (cardID: number) => void;
		onRetract: (cardID: number) => void;
	}

	let {
		myCards,
		committed,
		activeTrack,
		brightSet,
		busy = false,
		resolvedTracks,
		onCommit,
		onRetract
	}: Props = $props();

	const myHearts = $derived(
		myCards
			.filter((c) => c.card_suit === 'H')
			.sort((a, b) => cardRank(b.card_value) - cardRank(a.card_value))
	);

	function committedFor(cardID: number): CommittedHeart | undefined {
		return committed.find((h) => h.card_id === cardID);
	}

	function isLocked(cardID: number): boolean {
		const c = committedFor(cardID);
		if (!c) return false;
		return c.track !== activeTrack && resolvedTracks.has(c.track);
	}

	function isOnActive(cardID: number): boolean {
		const c = committedFor(cardID);
		return c?.track === activeTrack;
	}

	function handleClick(cardID: number) {
		if (busy) return;
		if (isLocked(cardID)) return;
		if (isOnActive(cardID)) {
			onRetract(cardID);
		} else {
			onCommit(cardID);
		}
	}

	const availableCount = $derived(
		myHearts.filter((h) => !isLocked(h.id)).length
	);
	const onActiveCount = $derived(myHearts.filter((h) => isOnActive(h.id)).length);
	const brightOnActive = $derived(
		myHearts.filter((h) => isOnActive(h.id) && brightSet.has(h.id)).length
	);
</script>

<div class="hand-strip">
	<div class="heading">
		<span class="heading-label">Maximum commitment if needed</span>
		<span class="heading-meta">
			{onActiveCount} on this track
			{#if onActiveCount > 0}({brightOnActive} doing work){/if}
			· {availableCount} of {myHearts.length} available
		</span>
	</div>
	<div class="hearts">
		{#if myHearts.length === 0}
			<span class="empty">You hold no hearts.</span>
		{/if}
		{#each myHearts as h}
			{@const onActive = isOnActive(h.id)}
			{@const locked = isLocked(h.id)}
			{@const greyHere = onActive && !brightSet.has(h.id)}
			<button
				type="button"
				class="heart-card"
				class:on-active={onActive}
				class:grey={greyHere}
				class:locked
				disabled={locked || busy}
				onclick={() => handleClick(h.id)}
				aria-pressed={onActive}
				title={locked
					? 'Locked into a resolved track'
					: onActive
						? greyHere
							? 'On this track but currently wasted (would refund)'
							: 'On this track, doing work'
						: 'Tap to commit to this track'}
			>
				<span class="card-value">{h.card_value}</span>
				<span class="card-suit">♥</span>
			</button>
		{/each}
	</div>
</div>

<style>
	.hand-strip {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		background: #1a1a1a;
		border: 1px solid #2c2c2c;
		border-radius: 8px;
		padding: 0.5rem 0.6rem;
	}
	.heading {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.4rem;
	}
	.heading-label {
		color: #c8a96e;
		font-weight: 600;
		font-size: 0.85rem;
	}
	.heading-meta {
		color: #888;
		font-size: 0.75rem;
	}
	.hearts {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.empty { color: #777; font-size: 0.85rem; }

	.heart-card {
		display: inline-flex;
		flex-direction: row;
		align-items: center;
		gap: 0.15rem;
		min-height: 44px;
		min-width: 44px;
		padding: 0 0.5rem;
		background: #f4ecd8;
		color: #b03030;
		border: 1px solid #888;
		border-radius: 5px;
		font-weight: 700;
		font-size: 0.95rem;
		cursor: pointer;
		font-variant-numeric: tabular-nums;
		transition: transform 80ms ease, box-shadow 80ms ease, opacity 120ms ease;
	}
	.heart-card:hover:not(:disabled) {
		transform: translateY(-1px);
		box-shadow: 0 1px 3px rgba(200, 169, 110, 0.4);
	}
	.heart-card.on-active {
		outline: 2px solid #c8a96e;
		outline-offset: 1px;
	}
	.heart-card.grey {
		opacity: 0.5;
		background: #d8d2c2;
	}
	.heart-card.locked {
		opacity: 0.3;
		cursor: not-allowed;
	}
	.heart-card:disabled { cursor: not-allowed; }
	.card-suit { font-size: 0.85em; }
</style>
