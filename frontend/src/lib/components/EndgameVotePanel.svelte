<!-- EndgameVotePanel.svelte
  Takeover panel for RowStateAwaitEndgameVote: row 7 is over, the public record
  is running out of rows, and the whole table votes on how the game ends before
  row 8 begins (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §7).

  Same shape as MainCharacterChoicePanel — a single blocking takeover of the
  phase column, driven by a row-state kind, visible to everyone. The only
  difference between players is whether their own vote is cast: every seated
  player votes, and there is NO facilitator control of any kind here. No close,
  no force, no skip, no timeout. A non-responder stalls the game, which is
  intended (adr/FACILITATOR_POWERS_AUDIT.md); the pressure comes from the
  Waiting On bar and the notification system, not from a button.

  Violet family throughout — procedural, "the machinery of resolution is in
  motion" (docs/STYLE_GUIDE.md, adr/009-design-tokens.md). Gold stays a label.

  This is the first and only time a player sees this screen, so it explains
  itself in full: what is happening and why now, both modes in plain language
  with their consequences, the running tally, who the table is still waiting on,
  and the tie-break rule stated UP FRONT so a tie is never a surprise.
-->
<script lang="ts">
	import '$lib/components/shared/statusText.css';
	import type { EndgameMode, EndingVoteEntry, Player } from '$lib/api';
	import { castEndingVote, getEndingVote } from '$lib/api';
	import { useWindowEvents } from '$lib/useWindowEvents';
	import { EventTypes } from '$lib/ws';
	import {
		ENDING_MODES,
		applyVoteCast,
		endingModeLabel,
		summariseEndingVote,
	} from '$lib/endgameVote';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import { onMount } from 'svelte';

	interface Props {
		gameID: number;
		/** The seated roster — every one of them owes a vote. */
		players: Player[];
		currentPlayerID: number | null;
		/** The facilitator, named because ties break to their side. Null only in
		 *  the broken state where a table has no facilitator. */
		facilitatorID: number | null;
	}

	let { gameID, players, currentPlayerID, facilitatorID }: Props = $props();

	// What each mode actually does, in play. Wording matches the server's
	// endingModeConsequence (handler/ending_vote.go) so the panel and the
	// resolution log post tell the same story.
	const MODE_COPY: Record<EndgameMode, { blurb: string; cost: string }> = {
		smooth_landing: {
			blurb: 'The game winds down using normal plan preparation.',
			cost: 'No plan may be prepared that would resolve after row 13. Fewer plans are available each row until only the shortest fit.',
		},
		explosive_finale: {
			blurb: 'Everything lands at once.',
			cost: 'Each player may prepare one plan with a reduced delay in order to fit on row 13. Row 13 then resolves all plans without scenes in between.',
		},
	};

	// ── The ballot ────────────────────────────────────────────────────────────
	// Fetched once, then kept live by endgame.vote_cast broadcasts. Merging is
	// keyed by player_id because the vote is an upsert: a changed vote replaces
	// that player's entry rather than adding a second one.
	let votes = $state<EndingVoteEntry[]>([]);
	let loadError = $state('');
	let actionError = $state('');
	let busy = $state(false);

	async function load() {
		try {
			const state = await getEndingVote(gameID);
			votes = state.votes;
			loadError = '';
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load the vote.';
		}
	}
	onMount(load);

	useWindowEvents([EventTypes.EndgameVoteCast], (e) => {
		const cast = (e as CustomEvent<EndingVoteEntry>).detail;
		if (cast?.player_id != null) votes = applyVoteCast(votes, cast);
	});

	const summary = $derived(summariseEndingVote({ votes, players, currentPlayerID }));

	// The tie-break, stated up front rather than revealed at the tie. Only
	// clause 2 (ties go the facilitator's way) is surfaced: with two modes a tie
	// needs an even split down the middle, so the facilitator is necessarily on
	// one side and clause 3 cannot fire. Name it when Long Campaign ships.
	const tieBreak = $derived.by(() => {
		if (facilitatorID != null && facilitatorID === currentPlayerID) {
			return 'If the vote ties, your side wins — ties go to the facilitator.';
		}
		const name = players.find((p) => p.id === facilitatorID)?.display_name;
		return name == null
			? `If the vote ties, the facilitator's side wins.`
			: `If the vote ties, ${name}'s side wins — ties go to the facilitator.`;
	});

	async function vote(mode: EndgameMode) {
		if (busy || mode === summary.myMode) return;
		busy = true;
		actionError = '';
		try {
			const state = await castEndingVote(gameID, mode);
			// The route answers with the whole ballot, so a client that missed a
			// broadcast catches up here. The last vote also resolves the game's
			// ending mode and completes the deferred row advance server-side; the
			// row_state.changed that follows swaps this panel away.
			votes = state.votes;
			loadError = '';
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not record your vote.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="endgame-vote">
	<h3>How does the game end?</h3>

	<p class="lede">
		The public record is running out of rows — it's time to vote on how you want the game to end.
		There is also a third option — play another 13 rows — but it's not available yet. Submit Feedback if you want it to be.
	</p>

	<div class="modes" role="group" aria-label="Ending modes">
		{#each ENDING_MODES as mode}
			{@const mine = summary.myMode === mode}
			<button
				type="button"
				class="mode"
				class:mine
				aria-pressed={mine}
				disabled={busy}
				onclick={() => vote(mode)}
			>
				<span class="mode-head">
					<span class="mode-name">{endingModeLabel(mode)}</span>
					<span class="mode-count" aria-label="{summary.counts[mode]} votes">
						{summary.counts[mode]}
					</span>
				</span>
				<span class="mode-blurb">{MODE_COPY[mode].blurb}</span>
				<span class="mode-cost">{MODE_COPY[mode].cost}</span>
				{#if mine}<span class="mode-mine">Your vote</span>{/if}
			</button>
		{/each}
	</div>

	{#if actionError}<ErrorText message={actionError} />{/if}
	{#if loadError}<ErrorText message={loadError} />{/if}

	<p class="tally" role="status">
		{summary.votedCount} of {summary.seatedCount} votes in.
		{#if summary.iOwe}
			Yours is one of the ones missing — pick one.
		{:else if summary.myMode}
			You may change your vote until the last one lands.
		{/if}
	</p>

	{#if summary.pendingNames.length > 0}
		<p class="muted-text small">
			Still waiting on {summary.pendingNames.join(', ')}.
		</p>
	{/if}

	<p class="muted-text small tiebreak">{tieBreak}</p>
</div>

<style>
	/* Procedural violet — the resolution machinery is in motion. The frame
	   carries it; the fill stays on the surface ladder, like .plan-panel
	   .resolving. */
	.endgame-vote {
		border: 1px solid var(--color-chip-violet-border);
		border-radius: 8px;
		padding: 1rem;
		background: var(--color-surface, var(--color-bg));
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	h3 {
		margin: 0;
		font-family: var(--font-serif);
		color: var(--color-accent);
	}
	.lede {
		margin: 0;
		font-size: 0.9rem;
		line-height: 1.4;
	}

	/* Always one column. The phase column is phone-width at every viewport
	   (300–380; docs/STYLE_GUIDE.md "Layout widths"), so two cards side by side
	   would be cramped everywhere rather than roomy anywhere — there is no
	   width at which the flip pays. Stacked full-width cards it is. */
	.modes {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	/* The whole card is the vote button: the biggest tap target the column
	   affords, and tapping the other card is how you change your mind. */
	.mode {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		width: 100%;
		min-height: 44px;
		padding: 0.6rem 0.7rem;
		text-align: left;
		font-family: var(--font-serif);
		background: var(--color-surface-2);
		border: 1px solid var(--color-border-strong);
		border-radius: 6px;
		cursor: pointer;
	}
	.mode:hover:not(:disabled) {
		border-color: color-mix(in srgb, var(--color-border-strong) 75%, white);
	}
	.mode:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
	.mode:disabled { cursor: not-allowed; opacity: 0.6; }

	/* The viewer's own choice — the chip trio, brightened on the border the way
	   an interactive selected state may (STYLE_GUIDE, "Chip trios"). */
	.mode.mine {
		background: var(--color-chip-violet-bg);
		border-color: var(--color-chip-violet-text);
	}

	.mode-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.mode-name {
		font-size: 0.95rem;
		color: var(--color-text);
	}
	.mode.mine .mode-name { color: var(--color-chip-violet-text); }

	/* A standalone numeric counter — the one place bold is reserved for. */
	.mode-count {
		flex-shrink: 0;
		font-weight: 700;
		font-size: 1rem;
		font-variant-numeric: tabular-nums;
		color: var(--color-text-muted);
	}
	.mode.mine .mode-count { color: var(--color-chip-violet-text); }

	.mode-blurb {
		font-size: 0.85rem;
		line-height: 1.35;
		color: var(--color-text);
	}
	.mode-cost {
		font-size: 0.8rem;
		line-height: 1.35;
		color: var(--color-text-muted);
	}
	.mode-mine {
		margin-top: 0.15rem;
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--color-chip-violet-text);
	}

	.tally {
		margin: 0;
		font-size: 0.85rem;
		line-height: 1.4;
		color: var(--color-text-secondary);
	}
	.tiebreak {
		padding-top: 0.4rem;
		border-top: 1px solid var(--color-border);
	}
</style>
