<!-- PrologueHelp.svelte
  The body of the prologue's local help disclosure, in its two variants.

  'choosing' — while players are claiming tiles. What the prologue is for,
  what a choice buys you, and what the cards will eventually do.
  'ranking'  — the ANY-spending step. The same model, one stage later: the
  cards are no longer a promise about rankings, they ARE the rankings.

  ONE FILE, TWO VARIANTS, on purpose. The two bodies share most of their
  copy (the weight tie-break verbatim, the asset-editing reminder, the
  "cards set the initial rankings" rule reworded for tense), and the failure
  mode of two files would be the ranking step keeping a sentence the
  choosing step has since corrected. Where they differ they differ because
  the step differs: the make/take legend is dead weight once nothing is being
  made, and the set-aside rule is unreadable until you can see the board it
  describes.

  Copy conventions this file is bound by (docs/STYLE_GUIDE.md "Motion & the
  deck"): the screen says ANY, never "heart"; in running prose ANY is marked
  as a label ("a card marked ANY") or the qualifier is dropped, because bare
  "your ANY cards" parses as the quantifier first.
-->
<script lang="ts">
	import '$lib/components/shared/trackCode.css';
	import AssetTypeIcon from '$lib/components/AssetTypeIcon.svelte';
	import WeightMeter from '$lib/components/shared/WeightMeter.svelte';
	import { SUIT_MEANINGS, assetTypeFor } from '$lib/prologue/choosing';

	interface Props {
		/** Which prologue step is asking. See the header comment. */
		variant: 'choosing' | 'ranking';
	}

	const { variant }: Props = $props();
</script>

<!-- True in every prologue step and easy to miss in all of them, but it is an
     aside wherever it lands — so each variant places it where it interrupts
     least. Choosing: after the paragraph about what a claim gets you, since
     that is what there is to go and edit. Ranking: last, so the four rules
     about spending read as one unbroken run. -->
{#snippet editAssetsNote()}
	<p class="prologue-subtext">
		You can edit your assets (including your main character) at any time in your player menu (top of the screen).
	</p>
{/snippet}

{#if variant === 'choosing'}
	<p class="prologue-lede">
		We start the game with the prologue, where we take turns fleshing out
		our characters and the world they inhabit.
	</p>
	<p class="prologue-lede">
		One turn you might decide your character is the monarch, and then the
		next you might say that they hail from a castle on the coast.
	</p>
	<p class="prologue-subtext">
		You get three choices — spend them however you like. If you want three
		titles, take three titles. Each tile you claim creates an asset and
		grants 2 cards.
	</p>
	<p class="prologue-subtext">
		Cards let you create <span class="steal-color">or steal</span> another asset,
		and improve your rank in either Power, Knowledge, or Esteem.
	</p>
	{@render editAssetsNote()}
{:else}
	<p class="prologue-lede">
		The cards everyone has gathered set the initial
		rankings in Power, Knowledge, and Esteem.
	</p>
	<p class="prologue-subtext">
		Each track counts the cards you hold for it, and we settle them one at
		a time: Power, then Knowledge, then Esteem.
	</p>
	<p class="prologue-subtext">
		A card marked ANY counts for whichever track you spend it on.
		Keep any eye on how many cards marked ANY the other players have
		— you might want to hold some back if a later track matters more to you.
	</p>
{/if}

{#if variant === 'choosing'}
	<!-- Type → code → track, in the same two icons and three letters the
	     tiles use, so the legend teaches the tile rather than a third
	     notation (Round 2, §2e). It replaces the suit legend, which taught
	     only "♣ makes a holding" while the track board below taught only
	     "♣ is Power" — and the two readings collide (♠ makes an artifact
	     but raises Esteem), so half a lesson was worse than none.

	     Choosing only: it is a table about what MAKING a card asset does, and
	     by the ranking step nothing is being made. -->
	<table class="legend">
		<thead>
			<tr>
				<th scope="col">When you make a…</th>
				<th scope="col">It raises your…</th>
			</tr>
		</thead>
		<tbody>
			{#each SUIT_MEANINGS as m (m.suit)}
				<tr>
					<th scope="row">
						<AssetTypeIcon type={assetTypeFor(m.suit)} size={14} />
						{m.assetType}
					</th>
					<td>
						<span class="track-code" class:wild={m.track == null}>{m.code}</span>
						{#if m.track}
							{m.track}
						{:else}
							<!-- The chip says "any"; this says "later". It used to read
							     "any track — you choose at the end", which was the row
							     explaining WLD by saying the word the code should have
							     been — and after the rename it stuttered (Round 3,
							     decision 4). -->
							<span class="wild-note">You choose the track at the end</span>
						{/if}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
{/if}

<!-- Both variants: the tie-break is a promise while you are gathering cards
     and a live consideration while you are spending them. -->
<p class="legend-note">
	<WeightMeter value="K" />
	Rank tie-breaker weight. If two players hold the same number
	of a track's cards, the heavier ones break the tie — and a
	track's own card beats an ANY card of the same weight.
</p>

{#if variant === 'choosing'}
	<h4>Starting rankings</h4>
	<!-- "A card marked ANY", not "Your ANY cards": the code is a real
	     English word now, so in running prose it parses as the
	     quantifier first and the label second. "marked" is what tells
	     the reader this is a label (Round 3, decision 3). -->
	<p class="prologue-subtext">
		The cards you gather set the initial rankings. A card marked ANY ranks
		nothing on its own — you choose which track to spend each one on at
		the end.
	</p>
{/if}

<style>
	/* Sized for a help body, not for a page opening: this used to be the
	   prologue's first paragraph at 1.05rem, and at that size inside a
	   disclosure it reads as a second heading. It keeps the brighter ink to
	   stay the lede. */
	.prologue-lede {
		margin: 0;
		color: var(--color-text);
		font-size: 0.95rem;
		line-height: 1.45;
	}
	.prologue-subtext {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.9rem;
		line-height: 1.4;
	}
	h4 {
		margin: 0.25rem 0 0;
		color: var(--color-accent);
		font-size: 0.9rem;
	}
	/* Matches the take chips on the tiles below — blue/attention, because a
	   take is an opportunity for the reader, not a warning to them. See
	   PrologueView's .tile-chip.take for the full reasoning. */
	.steal-color { color: var(--color-highlight); }

	/* Genuinely tabular (one row per asset type, two independent readings per
	   row), so a real table — the column headers are what make the second
	   reading legible, and they carry to screen readers for free. */
	.legend {
		border-collapse: collapse;
		/* Content-sized, not stretched: a full-width table pushes the columns
		   apart until the row stops reading as one statement. Shrunk to its
		   contents, the block centres visibly and each row scans as
		   "[icon] Holding → POW Power". */
		width: max-content;
		max-width: 100%;
		margin-inline: auto;
		font-size: 0.9rem;
		color: var(--color-text-secondary);
		text-align: left;
	}
	.legend th,
	.legend td {
		padding: 0.15rem 0.75rem 0.15rem 0;
		font-weight: inherit;
		vertical-align: middle;
	}
	.legend th:last-child,
	.legend td:last-child { padding-right: 0; }
	.legend thead th {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
		padding-bottom: 0.3rem;
		/* The peer row's "You choose the track at the end" is the widest cell
		   in the table by some way; without this the squeeze lands on the
		   headings, which then wrap while their neighbour doesn't. Let the long
		   note wrap instead — it's a sentence, they're labels. */
		white-space: nowrap;
	}
	/* The icon rides in the row header beside its own word, which is what the
	   tile chips show too — one glance teaches both. */
	.legend tbody th {
		display: flex;
		align-items: center;
		gap: 0.35rem;
	}
	.legend tbody td { color: var(--color-text); }
	/* A wild ranks nothing until you assign it, so the cell reads as a note
	   rather than a track name. */
	.wild-note { color: var(--color-text-secondary); font-style: italic; }
	/* The weight meter's one appearance outside a tile expansion: a static
	   sample, so the meter has been seen before it turns up on a row. */
	.legend-note {
		display: flex;
		/* Top-aligned, not centred: the note runs to three lines at 375 and a
		   centred meter lands beside the middle one, reading as a stray mark
		   rather than the thing the sentence is about. */
		align-items: flex-start;
		gap: 0.4rem;
		margin: 0;
		color: var(--color-text-faint);
		font-size: 0.75rem;
		line-height: 1.35;
	}
	/* :global, because the meter is shared/WeightMeter.svelte's root element —
	   it carries that component's scope class, not this one's. */
	.legend-note :global(.weight) { margin-top: 0.15rem; }
</style>
