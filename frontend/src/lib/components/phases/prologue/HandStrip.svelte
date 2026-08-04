<!-- HandStrip.svelte
  Persistent WLD hand for the active player during the prologue ranking
  declare step. Tap a card to commit it to the active track, tap a committed
  card to retract. A card that is doing work on the active track renders
  committed; a greyed (wasted) one would be refunded at resolution and returns
  to the hand pool unaffected.

  Cards locked into a previously-resolved track are disabled.

  Heading: "Maximum commitment if needed".

  The deck is gone from the screen (adr/PROLOGUE_UX_ROUND2_PLAN.md, decision 1
  + the owner's Session 3 ruling): these were ♥ card faces, and the hearts step
  was the last place in the app a suit reached the reader. They are WLD cards
  now — the same object the choosing view names, with the same dashed
  not-yet-a-track frame — so a player who learned "WLD" while picking tiles is
  not handed a second notation for it at the moment they spend one. The API
  still calls them hearts; that is storage, not vocabulary.
-->
<script lang="ts">
	import WeightMeter from '$lib/components/shared/WeightMeter.svelte';
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

	// Heaviest first: weight is the tie-break, so the leftmost card is the one
	// worth spending first when two players would otherwise tie.
	const myWilds = $derived(
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
		myWilds.filter((h) => !isLocked(h.id)).length
	);
	const onActiveCount = $derived(myWilds.filter((h) => isOnActive(h.id)).length);
	const brightOnActive = $derived(
		myWilds.filter((h) => isOnActive(h.id) && brightSet.has(h.id)).length
	);
</script>

<div class="hand-strip">
	<div class="heading">
		<span class="heading-label">Maximum commitment if needed</span>
		<span class="heading-meta">
			{onActiveCount} on this track
			{#if onActiveCount > 0}({brightOnActive} doing work){/if}
			· {availableCount} of {myWilds.length} available
		</span>
	</div>
	<div class="wild-hand">
		{#if myWilds.length === 0}
			<span class="empty">You hold no WLD cards.</span>
		{/if}
		{#each myWilds as h}
			{@const onActive = isOnActive(h.id)}
			{@const locked = isLocked(h.id)}
			{@const greyHere = onActive && !brightSet.has(h.id)}
			<button
				type="button"
				class="wild-card"
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
				<span class="wild-code">WLD</span>
				<WeightMeter value={h.card_value} />
			</button>
		{/each}
	</div>
</div>

<style>
	.hand-strip {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		background: var(--color-bg);
		border: 1px solid var(--color-border);
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
		color: var(--color-accent);
		font-size: 0.85rem;
	}
	.heading-meta {
		color: var(--color-text-muted);
		font-size: 0.75rem;
	}
	.wild-hand {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.empty { color: var(--color-text-faint); font-size: 0.85rem; }

	/* The card itself, at tap size: the code it is, and the weight it carries.
	   The dash is on the button's own border rather than on a .track-code chip
	   nested inside it — the button IS the wild card, and a dashed chip inside
	   a solid frame is two boxes around one word (the ruling Session 2 took for
	   .tile-chip.wild). */
	.wild-card {
		display: inline-flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.2rem;
		min-height: 44px;
		min-width: 44px;
		padding: 0.25rem 0.5rem;
		background: var(--color-surface-2);
		border: 1px dashed var(--color-border-strong);
		border-radius: 5px;
		color: var(--color-text-muted);
		font-family: inherit;
		cursor: pointer;
		transition: transform 80ms ease, box-shadow 80ms ease, opacity 120ms ease;
	}
	.wild-code {
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		line-height: 1;
	}
	.wild-card:hover:not(:disabled) {
		transform: translateY(-1px);
		box-shadow: 0 1px 3px color-mix(in srgb, var(--color-accent) 40%, transparent);
	}
	.wild-card.on-active {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
		color: var(--color-text);
	}
	/* Committed here, but the track ranks the same without it — resolution
	   refunds it. Dimmed against the sunken ground so it reads as set aside
	   rather than merely quiet. */
	.wild-card.grey {
		opacity: 0.5;
		background: var(--color-surface-sunken);
	}
	.wild-card.locked {
		opacity: 0.3;
		cursor: not-allowed;
	}
	.wild-card:disabled { cursor: not-allowed; }
	.wild-card:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
	}
	/* Reduced motion (§3c): the lift on hover is decoration on an affordance
	   the border and the tap target already carry. */
	@media (prefers-reduced-motion: reduce) {
		.wild-card { transition: none; }
		.wild-card:hover:not(:disabled) { transform: none; }
	}
</style>
