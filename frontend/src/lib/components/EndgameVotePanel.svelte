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

  Plain on purpose: no frame, no tint, and no colour that carries a meaning.
  The panel is one step, set out by its heading and the gaps between its parts.
  Each ending is a choice, so each is the house chip (shared/Chip.svelte) and
  the one you picked has the bright gold border; the endings are told apart by
  their words, because a colour on one would lean on the vote. What each ending
  does sits under its chip as plain text, because a chip is a short label and
  these descriptions are paragraphs.

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
	import Chip from '$lib/components/shared/Chip.svelte';
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
			{@const count = summary.counts[mode]}
			<div class="mode">
				<div class="mode-head">
					<Chip
						selected={mine}
						disabled={busy}
						aria-describedby="mode-text-{mode}"
						onclick={() => vote(mode)}
					>{endingModeLabel(mode)}</Chip>
					<span class="mode-count">{count} {count === 1 ? 'vote' : 'votes'}</span>
					{#if mine}<span class="status-word">Your vote</span>{/if}
				</div>
				<div class="mode-text" id="mode-text-{mode}">
					<p class="mode-blurb">{MODE_COPY[mode].blurb}</p>
					<p class="mode-cost">{MODE_COPY[mode].cost}</p>
				</div>
			</div>
		{/each}
	</div>

	{#if actionError}<ErrorText message={actionError} />{/if}
	{#if loadError}<ErrorText message={loadError} />{/if}

	<!-- Where the vote stands, apart from the two endings above it. -->
	<div class="standing">
		<p class="tally" role="status">
			{summary.votedCount} of {summary.seatedCount} votes in.
			{#if summary.iOwe}
				Yours is one of the ones missing — pick one.
			{:else if summary.myMode}
				You may change your vote until the last one lands.
			{/if}
		</p>

		{#if summary.pendingNames.length > 0}
			<p class="muted-text">
				Still waiting on {summary.pendingNames.join(', ')}.
			</p>
		{/if}

		<p class="muted-text">{tieBreak}</p>
	</div>
</div>

<style>
	/* Set out by its heading and the gaps, with no frame. The side padding is the
	   panel's own because the play column gives its content none. */
	.endgame-vote {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding: var(--space-4) var(--space-3);
	}
	h3 {
		margin: 0;
		font-family: var(--font-serif);
		font-size: var(--text-heading);
		color: var(--color-accent);
	}
	.lede {
		margin: 0;
		font-size: var(--text-body);
		line-height: 1.4;
	}

	/* Always one column. The phase column is phone-width at every viewport
	   (docs/STYLE_GUIDE.md "Layout widths"), so two endings side by side would be
	   cramped everywhere rather than roomy anywhere. Each ending is its chip and
	   its two paragraphs; the gap between endings is wider than the gap inside
	   one, so the spacing shows which words belong to which chip. */
	.modes {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
	}
	.mode {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* The chip, then its running tally and, on the one you picked, the words
	   that say so (the gold border alone isn't a signal for everyone). */
	.mode-head {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--space-3);
	}
	.mode-count {
		font-size: var(--text-small);
		font-variant-numeric: tabular-nums;
		color: var(--color-text-secondary);
	}

	.mode-text {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	.mode-blurb,
	.mode-cost {
		margin: 0;
		line-height: 1.4;
	}
	.mode-blurb { font-size: var(--text-body); }
	.mode-cost {
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}

	/* A wider gap above than inside, so the tally reads as its own block and not
	   as the last ending's third paragraph. */
	.standing {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		margin-top: var(--space-2);
	}
	.tally {
		margin: 0;
		font-size: var(--text-body);
		line-height: 1.4;
	}
</style>
