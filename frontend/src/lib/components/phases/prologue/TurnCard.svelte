<!-- TurnCard.svelte
  The first object in the choosing view: whose turn it is, how many of your
  three choices you still hold, and the one thing to do next.

  It exists because a claim used to commit with no feedback anywhere the
  player was looking (adr/PROLOGUE_UX_ROUND2_PLAN.md, Motivation). Every
  signal that the turn had moved — the gold ring, the waiting bar, the turn
  count — lived in the header, ~250px above the tiles, and the scroll position
  after a commit was byte-identical, so the page read as static.

  Three states. Session 1 landed the on-turn / off-turn pair; Session 3 adds
  the third, just-claimed: a success-bordered confirmation of what the claim
  actually did, which decays back into off-turn on the parent's timer. It is
  the near half of the motion beat — the far half is the claimed tile pulsing
  where it sits, which the parent owns because only it knows the tile's node.
-->
<script module lang="ts">
	/** What a just-committed claim produced, for the confirmation line. `made`
	 *  counts the assets the claim creates (the sheet's own asset, plus each
	 *  card nobody held); `takes` names what it pulled off other players.
	 *
	 *  The parent computes this *before* it reloads, since the whole point is
	 *  what changed — after the reload the taken asset is simply yours and the
	 *  fresh cards are simply held, with nothing left to say it just happened. */
	export interface JustClaimed {
		tileName: string;
		made: number;
		takes: { assetName: string | null; ownerName: string }[];
	}
</script>

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
		/** Set for a few seconds after the viewer's own claim commits, then
		 *  cleared by the parent — which is also what ends the pip's spend
		 *  animation, so the two beats start and stop together. */
		justClaimed?: JustClaimed | null;
		onOpenRetinue?: () => void;
	}

	let {
		activeName,
		isMyTurn,
		unspent,
		atRiskCount,
		justClaimed = null,
		onOpenRetinue
	}: Props = $props();

	const TOTAL_PIPS = 3;
	/** Clamped, not trusted: the server caps a player at three claims, but a
	 *  pip row that renders negative or spills past three would misstate the
	 *  one number this card exists to carry. */
	const held = $derived(unspent == null ? null : Math.min(Math.max(unspent, 0), TOTAL_PIPS));
	const showRetinueLine = $derived(
		justClaimed == null && !isMyTurn && activeName != null && atRiskCount > 0
	);
	/** The pip the claim just spent: the first ghost in the row, i.e. the one
	 *  that has this instant stopped being solid. Only meaningful while the
	 *  confirmation is up. */
	const spendingIndex = $derived(justClaimed != null && held != null ? held : -1);
</script>

<section class="turn-card" class:claimed={justClaimed != null}>
	<div class="turn-head">
		{#if justClaimed}
			<span class="turn-status done">✓ Claimed {justClaimed.tileName}</span>
		{:else if isMyTurn}
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
					<span
						class="choice-pip"
						class:spent={i >= held}
						class:spending={i === spendingIndex}
					></span>
				{/each}
			</span>
		{/if}
	</div>

	{#if justClaimed}
		<!-- What the claim actually did, in the two verbs the rules use. The
		     asset names are the player's own words, so they carry the italic
		     the rest of the app gives an asset name. One line, clipped: see
		     .turn-line.confirm — and the full story is in the chat log either
		     way, which is where a player goes to re-read it. -->
		<p class="turn-line confirm">
			{justClaimed.made === 1 ? 'Made 1 asset' : `Made ${justClaimed.made} assets`}<!--
			--><!-- Each take is its own span with a CSS margin, not a newline in
			     the markup: Svelte collapses the whitespace around an each block's
			     boundaries, which ran the previous clause's name straight into the
			     next separator ("from bob· took …").

			     The nameless fallback is "an asset", not "a card" (Round 3,
			     decision 9): what changes hands is the asset — the card is the
			     pointer to it — and the clause's other branch names that same
			     asset. This branch is already the vague one (the name wouldn't
			     resolve), and vague about the right noun beats precise about the
			     wrong one. -->{#each justClaimed.takes as t, i (i)}<span
				class="take-clause">· took {#if t.assetName}<em>{t.assetName}</em>{:else}an asset{/if} from {t.ownerName}</span>{/each}
		</p>
	{:else if isMyTurn}
		<p class="turn-line">Claim one tile — from any category.</p>
	{:else if showRetinueLine}
		<p class="turn-line">
			While you wait:
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
	/* The confirmation only borrows the frame for a few seconds, so it says so
	   with the border rather than a new box — the card must not change size on
	   the way in or out, or the tiles below it jump under the reader's thumb at
	   the exact moment they're being asked to look at one. */
	.turn-card.claimed { border-color: var(--color-success); }
	.turn-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	/* Exactly one line, in every state — see the .claimed note above. A
	   display name is username-derived and capped, so the clip is a backstop,
	   not the normal case. */
	.turn-status {
		color: var(--color-accent);
		font-size: 1rem;
		line-height: 1.25;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.turn-status.waiting { color: var(--color-text-muted); }
	/* Same size as the other two states, deliberately: the tick and the green
	   already say which state this is, and a smaller face here would make the
	   head a different height on the way in and out. */
	.turn-status.done { color: var(--color-success); }
	.turn-line {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.82rem;
		line-height: 1.35;
	}
	/* One line, clipped — the card is then exactly as tall in all three states
	   (measured 61.3px at 344, 375 and the 440 cap), so neither the arrival nor
	   the six-second decay moves the tiles underneath. That jolt would land at
	   exactly the moment the reader is being asked to look at one of them, and
	   this card exists to fix a feedback problem, not to create a second one.
	   A claim with two takes overruns even at 440; the full sentence is in the
	   chat log, which is where a player goes to re-read it anyway. */
	.turn-line.confirm {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.take-clause { margin-left: 0.35em; }
	.turn-line em { font-style: italic; color: var(--color-text); }

	/* The discs themselves live in shared/choicePip.css — the category headers
	   draw the identical object when one is spent, and the spend animation is
	   defined there too, so both homes stay one object. */
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
