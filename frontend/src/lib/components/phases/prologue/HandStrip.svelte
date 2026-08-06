<!-- HandStrip.svelte
  Persistent hand of wilds for the active player during the prologue ranking
  declare step. Tap a card to commit it to the active track, tap a committed
  card to retract. A card that is doing work on the active track renders
  committed; a greyed (wasted) one would be refunded at resolution and returns
  to the hand pool unaffected.

  Cards locked into a previously-resolved track are disabled — and wear that
  track's code in place of ANY, so the card says where it went.

  THE PANEL CARRIES THE INSTRUCTION. It used to be headed "Maximum commitment
  if needed", which is PROLOGUE_RANKING_UI_PLAN.md's framing of the *model*
  shipped verbatim as UI copy: it names neither the action nor the track, and
  the only sentence that did ("Rankings: Spend ANY on Power") lived ~250px up
  in the status strip, sharing a row with "Waiting On:". A playtester never
  realised the cards were tappable at all. The heading is now the verb and the
  track, with the rule that makes the step safe ("any you don't need come
  back") directly under it — the fact that removes the fear of tapping.

  THE CARD IS A CONTROL. Its resting frame was 1px dashed --color-border-strong
  on --color-surface-2: 1.79:1 against the page, under WCAG 1.4.11's 3:1 floor
  for an interactive boundary, and byte-identical to this phase's *inert*
  idiom (.track-code.wild, .tile-chip.wild, .mark.wild, StandingStrip's
  .mslot.dummy — a rank slot nobody can ever occupy). The dash stays, because
  that is what says "no track yet"; the colour moves to --color-accent-dim,
  already the app's idle-affordance gold (the MC-toggle's resting state). Gold
  as an OUTLINE, never a fill — the fill is what would claim this is selected.

  The deck is gone from the screen (adr/PROLOGUE_UX_ROUND2_PLAN.md, decision 1
  + the owner's Session 3 ruling): these were ♥ card faces, and the hearts step
  was the last place in the app a suit reached the reader. They wear the ANY
  code now — the same object the choosing view names, with the same dashed
  not-yet-a-track frame — so a player who learned "ANY" while picking tiles is
  not handed a second notation for it at the moment they spend one. The code
  comes from trackCode('H') rather than a literal, so it cannot drift from the
  chips on the tiles. The API still calls them hearts; that is storage, not
  vocabulary. (`wild` in this file's identifiers is the concept, ANY is the
  label — Round 3, decision 2.)
-->
<script lang="ts">
	import WeightMeter from '$lib/components/shared/WeightMeter.svelte';
	import type { CommittedHeart, PlayerCardRow, PrologueTrack } from '$lib/api';
	import { cardRank } from '$lib/prologue/refund';
	import { codeForTrack, labelForTrack, trackCode } from '$lib/prologue/choosing';

	interface Props {
		myCards: PlayerCardRow[];
		committed: CommittedHeart[];
		activeTrack: PrologueTrack;
		brightSet: Set<number>;
		resolvedTracks: Set<PrologueTrack>;
		onCommit: (cardID: number) => void;
		onRetract: (cardID: number) => void;
	}

	let {
		myCards,
		committed,
		activeTrack,
		brightSet,
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

	/** The resolved track a locked card was spent on. Only meaningful when
	 *  isLocked is true — a refunded card leaves `committed` entirely at
	 *  resolution, so anything still in there on a resolved track is spent. */
	function lockedTrack(cardID: number): PrologueTrack | null {
		const c = committedFor(cardID);
		return c && isLocked(cardID) ? c.track : null;
	}

	function isOnActive(cardID: number): boolean {
		const c = committedFor(cardID);
		return c?.track === activeTrack;
	}

	// No in-flight lock. The parent applies a tap locally and syncs behind it,
	// coalescing repeats, so there is no window where a tap has to be refused —
	// and a `busy` gate here would refuse taps for a whole round trip while the
	// card visibly looked ready for the next one.
	function handleClick(cardID: number) {
		if (isLocked(cardID)) return;
		if (isOnActive(cardID)) {
			onRetract(cardID);
		} else {
			onCommit(cardID);
		}
	}

	const trackName = $derived(labelForTrack(activeTrack));

	const availableCount = $derived(myWilds.filter((h) => !isLocked(h.id)).length);
	const onActiveCount = $derived(myWilds.filter((h) => isOnActive(h.id)).length);
	const brightOnActive = $derived(
		myWilds.filter((h) => isOnActive(h.id) && brightSet.has(h.id)).length
	);
	/** Is anything currently struck? The caption under the hand explains the
	 *  strike, so it only earns its two lines while a struck card is on screen. */
	const anyWasted = $derived(onActiveCount > brightOnActive);

	/**
	 * Every ANY card is gone into an earlier track. Distinct from holding none
	 * at all, and previously indistinguishable from it: the empty-state copy
	 * keys off `myWilds.length`, which is still > 0 here, so this player got the
	 * full commit UI — a heading urging them to spend, two ghost chips and
	 * "0 of 2 available" — with nothing saying the step was over for them. It is
	 * a guaranteed state by Esteem for anyone who spent early.
	 */
	const allSpent = $derived(myWilds.length > 0 && availableCount === 0);

	/** Either no-op case: nothing this player can place on this track, whether
	 *  because they never held an ANY card or because all of theirs are locked.
	 *  The two need different *words* (one is a fact about the deal, the other
	 *  about their own earlier choices) but the same quiet frame. */
	const nothingToDo = $derived(myWilds.length === 0 || allSpent);

	// (No counter line. It read "0 on this track · 2 of 2 available" and, after
	// one tap, "1 on this track (1 doing work) · 2 of 2 available" — three
	// numbers about the same two cards, one of which ("available" = not locked
	// into a *resolved* track) every reader parsed as "unspent" and so as a
	// contradiction. The cards now carry their own state in the code they wear,
	// which is the fact the prose was standing in for.)
</script>

<div class="hand-strip" class:inert={nothingToDo}>
	<div class="heading">
		<!-- The verb and the track, in that order. Nothing else on this panel
		     named either. -->
		<h3 class="heading-label">
			{#if myWilds.length === 0}
				No cards to spend
			{:else if allSpent}
				Nothing left to spend
			{:else}
				Spend on {trackName}
			{/if}
		</h3>
		<p class="heading-hint">
			{#if myWilds.length === 0}
				<!-- Not "no ANY cards": in this step the hand is *entirely* these
				     cards, so the qualifier was always redundant — and "no ANY cards"
				     reads as a quantifier now that the code is a real word (Round 3,
				     decision 3). -->
				Nothing to place on this track — mark yourself done below.
			{:else if allSpent}
				Every card you held is locked into an earlier track. Mark yourself
				done below.
			{:else}
				<!-- The safety rule, not a restatement of the heading: the reason a
				     new player hesitates over the first tap is the fear of wasting a
				     card, and this is the sentence that answers it. -->
				Tap a card to put it on {trackName}. Place as many as you're willing to spend — 
				any you don't end up needing come back to your hand.
			{/if}
		</p>
	</div>
	{#if myWilds.length > 0}
		<div class="wild-hand">
			{#each myWilds as h}
				{@const onActive = isOnActive(h.id)}
				{@const locked = isLocked(h.id)}
				{@const lockedOn = lockedTrack(h.id)}
				{@const greyHere = onActive && !brightSet.has(h.id)}
				<button
					type="button"
					class="wild-card"
					class:on-active={onActive}
					class:grey={greyHere}
					class:locked
					disabled={locked}
					onclick={() => handleClick(h.id)}
					aria-pressed={locked ? undefined : onActive}
					title={locked
						? lockedOn
							? `Spent on ${labelForTrack(lockedOn)}`
							: 'Locked into a resolved track'
						: onActive
							? greyHere
								? `On ${trackName} but currently wasted (would come back)`
								: `On ${trackName}, doing work`
							: `Tap to put on ${trackName}`}
				>
					<!--
					     THE CODE IS THE CARD'S STATE. A card reads ANY while it is
					     still a choice, becomes this track's code the instant it is
					     placed, and keeps the code of whatever track finally took it.
					     Retract it — or let resolution refund it — and it goes back to
					     ANY, because it is one again.

					     This is what retires the counter line that used to sit above
					     the hand ("1 on Esteem, 0 doing work · 1 spent earlier"): every
					     clause in it was compensating for cards that all looked
					     identical, so the panel had to narrate in prose what the
					     objects could simply be. Three cards reading EST / ANY / POW
					     need no key.
					-->
					<span class="wild-code">
						{lockedOn
							? codeForTrack(lockedOn)
							: onActive
								? codeForTrack(activeTrack)
								: trackCode('H')}
					</span>
					<WeightMeter value={h.card_value} />
				</button>
			{/each}
		</div>
		<!-- One caption for the whole hand rather than one per 44px chip. Only
		     rendered when a card is actually in that state, so it reads as a
		     legend for what the reader is looking at rather than a standing
		     rubric. -->
		{#if anyWasted}
			<p class="hand-note">
				A struck-out code means that card isn't changing your rank — it comes
				back to your hand at resolution, costing you nothing.
			</p>
		{/if}
	{/if}
</div>

<style>
	/*
	 * A real container, not an outline around the page colour. This was
	 * `background: var(--color-bg)` — identical to the page — inside a
	 * --color-border hairline at 1.38:1, so the "panel" grouped nothing and
	 * the hand read as loose chips floating under the board.
	 */
	.hand-strip {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		background: var(--color-surface);
		border: 1px solid var(--color-border-warm);
		border-radius: 8px;
		padding: 0.6rem 0.7rem;
	}
	/* Nothing to do here. The frame drops back to the ordinary cool border and
	   the ghost cards below carry no gold at all, so the panel stops
	   advertising an action this player cannot take. Covers both no-op cases —
	   held none, and spent them all. */
	.hand-strip.inert {
		border-color: var(--color-border);
	}
	.hand-strip.inert .heading-label {
		color: var(--color-text-muted);
	}

	.heading {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.heading-label {
		margin: 0;
		color: var(--color-accent);
		font-size: 1rem;
		font-weight: 500;
	}
	.heading-hint {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.82rem;
		line-height: 1.4;
	}
	.wild-hand {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
	}
	.hand-note {
		margin: 0;
		color: var(--color-text-faint);
		font-size: 0.7rem;
		line-height: 1.4;
	}

	/* The card itself, at tap size: the code it is, and the weight it carries.
	   The dash is on the button's own border rather than on a .track-code chip
	   nested inside it — the button IS the wild card, and a dashed chip inside
	   a solid frame is two boxes around one word (the ruling Session 2 took for
	   .tile-chip.wild).

	   --color-accent-dim is the documented idle-affordance gold (app.css:
	   "outline/idle gold — MC-toggle idle state"), which is exactly this
	   card's job: tappable, not yet chosen. It clears 3:1 against the page
	   where --color-border-strong sat at 1.79, and it un-collides the card
	   from the four *inert* dashed objects in this phase, all of which stay
	   --color-border / --color-border-strong. */
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
		border: 1px dashed var(--color-accent-dim);
		border-radius: 5px;
		color: var(--color-text);
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
		border-style: solid;
		background: var(--color-surface-active);
	}
	/* Committed here, but the track ranks the same without it — resolution
	   refunds it. Struck as well as dimmed, matching TrackBoard's .mark.inert
	   for the same card in the same state: dim alone reads as "quiet", and
	   this needs to read as "cancelled". The two views showed the same fact two
	   different ways, and the weaker of the two was the one on the control the
	   player is actually deciding with. */
	.wild-card.grey {
		opacity: 0.55;
		background: var(--color-surface-sunken);
		outline-color: var(--color-accent-dim);
	}
	.wild-card.grey .wild-code {
		text-decoration: line-through;
		text-decoration-thickness: 1px;
	}
	/* Spent on an earlier track. No gold anywhere — it is not an affordance any
	   more — and the code it wears (POW / KNO) is what says so, rather than a
	   0.3-opacity copy of an available card that only the counter could tell
	   you about. Solid border: it has a track now, so the dashed
	   not-yet-a-track idiom no longer applies to it. */
	.wild-card.locked {
		opacity: 0.75;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		color: var(--color-text-faint);
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
