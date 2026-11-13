<!--
  TonesSheet.svelte — the table's Tones, as a sheet.

  It opens from the "Tones · N set" chip in the chat header and rises over the
  Chat column. It has two shapes, because it has two jobs:

    - BEFORE the Main Event, tones are set here: the whole grid of topics, one
      tile each, tap to cycle through No opinion, Include, Avoid detail and
      Never; a key that sticks to the top while you scroll; and a box to add a
      topic of your own.
    - ONCE TONES LOCK, the sheet is reference. A grid of every topic is hard to
      scan for the few that matter, and its tiles can't be pressed any more yet
      are drawn as controls. So it lists only the topics someone set, grouped by
      how firm the position is (Never first, since it matters most), with one
      line counting the rest. The grouping is `$lib/toneGroups`.

  The page owns the data and the writes (they are shared with the live-update
  handler, which drops this client's own echoes); this owns the sheet and the
  add-a-topic box.

  The claim that no one is told who set a tone is load-bearing and true:
  tone_topics is keyed on the game with no player column, and handler/tone.go
  broadcasts a WS event without emitting an action-log post. The tile does change
  colour for everyone, which is the point; it just carries no name. Don't add
  attribution or a log entry here without rewriting the lede.

  Words come with the colours: the key and the group heads name each position.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import { lockedTones, restLine } from '$lib/toneGroups';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import type { ToneTopic } from '$lib/api';

	interface Props {
		open: boolean;
		onClose: () => void;
		topics: ToneTopic[];
		/** True once the main event has begun: the tones are reference then. */
		locked: boolean;
		/** A failed write, shown here: the page's banner would be painted
		 *  underneath this sheet. */
		error: string;
		onCycle: (topic: ToneTopic) => void;
		/** Adds a topic. Resolves true if it was added (the box then clears). */
		onAdd: (text: string) => Promise<boolean>;
	}

	const { open, onClose, topics, locked, error, onCycle, onAdd }: Props = $props();

	let newTopicText = $state('');
	let addingTopic = $state(false);

	async function submitNewTopic() {
		const text = newTopicText.trim();
		if (!text || addingTopic) return;
		addingTopic = true;
		try {
			if (await onAdd(text)) newTopicText = '';
		} finally {
			addingTopic = false;
		}
	}

	const STATUS_WORDS: Record<string, string> = {
		default: 'No opinion',
		avoid_detail: 'Avoid detail',
		include: 'Include',
		never: 'Never',
	};

	const lockedList = $derived(lockedTones(topics));
	const setCount = $derived(lockedList.groups.reduce((n, g) => n + g.topics.length, 0));
	const rest = $derived(restLine(setCount, lockedList.restCount));
</script>

<RetinueSheet {open} {onClose}>
	<div class="tones-sheet">
		<h3 class="sheet-title">Tones</h3>
		{#if error}
			<ErrorText message={error} />
		{/if}

		{#if locked}
			<p class="tone-lede">
				Themes and topics the table set to include or avoid. No one is told who set them. Locked
				since the Main Event began.
			</p>

			<div class="tone-groups">
				{#each lockedList.groups as group (group.status)}
					<section class="tone-group" data-status={group.status}>
						<h4 class="tone-group-head">
							<span class="swatch" aria-hidden="true"></span>
							<span>{group.label}</span>
						</h4>
						<ul class="tone-rows">
							{#each group.topics as topic (topic.id)}
								<li class="tone-row">{topic.topic}</li>
							{/each}
						</ul>
					</section>
				{/each}
				{#if lockedList.groups.length === 0}
					<p class="tone-none">No tones were set.</p>
				{/if}
				{#if rest}
					<p class="tone-rest">{rest}</p>
				{/if}
			</div>
		{:else}
			<p class="tone-lede">
				Themes and topics the table wants to include or avoid. Set as many as you like. No one is
				told who set each.
			</p>

			<!-- Sticky, because this is the key to a 41-tile grid and unstuck it was
			     gone after one flick on a phone — leaving you tapping colours with
			     no way left on screen to read them. It sticks to the scrollport of
			     RetinueSheet's `.sheet-body`, so it must stay a DIRECT child of
			     `.tones-sheet`: wrapping it with the title in a header div would
			     bound the stick to that div and it would scroll away again. -->
			<div class="tone-legend" aria-label="Legend">
				<span class="tone-legend-lead">Tap to cycle</span>
				<div class="tone-legend-items">
					<span class="tone-legend-item" data-status="default"><span class="swatch"></span>No opinion</span>
					<span class="tone-legend-item" data-status="include"><span class="swatch"></span>Include</span>
					<span class="tone-legend-item" data-status="avoid_detail"><span class="swatch"></span>Avoid detail</span>
					<span class="tone-legend-item" data-status="never"><span class="swatch"></span>Never</span>
				</div>
			</div>

			<div class="tone-grid">
				{#each topics as topic (topic.id)}
					<button
						type="button"
						class="tone-tile"
						data-status={topic.status}
						onclick={() => onCycle(topic)}
						aria-label={`${topic.topic}: ${STATUS_WORDS[topic.status] ?? 'Never'}. Tap to cycle.`}
					>
						<span class="tone-tile-topic">{topic.topic}</span>
					</button>
				{/each}
			</div>

			<form
				class="tone-add-row"
				onsubmit={(e) => { e.preventDefault(); void submitNewTopic(); }}
			>
				<input
					type="text"
					class="tone-add-input"
					placeholder="Add a custom topic…"
					bind:value={newTopicText}
					maxlength={TEXT_LIMITS.TONE_TOPIC}
					aria-label="Add a custom topic"
				/>
				<button
					type="submit"
					class="action-btn primary tone-add-button"
					disabled={!newTopicText.trim() || addingTopic}
				>
					{addingTopic ? '…' : 'Add'}
				</button>
			</form>
		{/if}
	</div>
</RetinueSheet>

<style>
	/* What the sheet is, in words: plain secondary text. It is a note about the
	   sheet, not something to press, so it has no box, no fill and no rule. */
	.tone-lede {
		margin: 0 0 var(--space-3);
		font-size: var(--text-small);
		line-height: 1.5;
		color: var(--color-text-secondary);
	}

	/* ── Locked: a list of what was set ─────────────────────────────────── */

	.tone-groups {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.tone-group { display: flex; flex-direction: column; }

	/* The group's name in small capitals, led by the tone's own swatch (a picture
	   of the tile it used to be). The word is always there beside the colour. */
	.tone-group-head {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0;
		padding-bottom: var(--space-1);
		font-size: var(--text-label);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--color-text-secondary);
	}

	.tone-group .swatch,
	.tone-legend-item .swatch {
		flex: none;
		width: 12px;
		height: 12px;
		border-radius: var(--radius-piece);
		background: var(--color-neutral);
	}
	.tone-group[data-status='include'] .swatch { background: var(--color-tone-include); }
	.tone-group[data-status='avoid_detail'] .swatch { background: var(--color-tone-avoid); }
	.tone-group[data-status='never'] .swatch { background: var(--color-danger-muted); }

	/* Not pressed, so not a tap target: a line of text with a hairline above. */
	.tone-rows { margin: 0; padding: 0; list-style: none; }
	.tone-row {
		display: flex;
		align-items: center;
		min-height: 36px;
		border-top: 1px solid var(--color-border);
		font-size: var(--text-body);
		color: var(--color-text);
	}

	.tone-rest,
	.tone-none {
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-text-faint);
	}

	/* ── Before the Main Event: the grid, to set tones on ───────────────── */

	.tone-legend {
		position: sticky;
		/* NOT 0. The offset is measured from the sheet body's content-box top,
		   which sits one --space-2 below the sheet's inner edge: lifting the pin
		   by exactly that padding parks the bar flush against the edge. Coupled
		   to RetinueSheet's `.sheet-body` padding. */
		top: calc(-1 * var(--space-2));
		z-index: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-label);
		/* Opaque, and padded on its own account: the body's top padding scrolls
		   away with the content, so the bar owns the whole band it occupies or
		   tiles show through above and touch it below. Matches the sheet. */
		background: var(--color-surface);
		padding: var(--space-4) 0 var(--space-2);
		margin: 0 0 var(--space-3);
		border-bottom: 1px solid var(--color-border);
		color: var(--color-text-secondary);
	}

	.tone-legend-lead {
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	/* Wraps rather than overflows if a fallback font ever runs wider. */
	.tone-legend-items {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: center;
		gap: var(--space-1) var(--space-2);
	}

	.tone-legend-item {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}

	.tone-legend-item[data-status='include']      .swatch { background: var(--color-tone-include); }
	.tone-legend-item[data-status='avoid_detail'] .swatch { background: var(--color-tone-avoid); }
	.tone-legend-item[data-status='never']        .swatch { background: var(--color-danger-muted); }

	/* Always 3 across: a column is drawn at phone width, where 4–5 tiles never fit. */
	.tone-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: var(--space-2);
		margin-bottom: var(--space-3);
	}

	.tone-tile {
		min-height: var(--size-tap);
		padding: var(--space-1);
		border-radius: var(--radius-control);
		/* A tile is a control, so it takes the control border. It used to be a faint
		   white wash; no token is a translucent edge, and the nearest is this one. */
		border: 1px solid var(--color-border-control);
		background: var(--color-neutral);
		color: var(--white);
		font-size: var(--text-small);
		font-weight: 500;
		text-align: center;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		transition: background-color 120ms ease, transform 80ms ease;
		word-break: break-word;
		hyphens: auto;
	}

	.tone-tile:active { transform: scale(0.97); }
	.tone-tile:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	/* Reduced motion (docs/STYLE_GUIDE.md "Motion & the deck"): drop the
	   press-down squish, keep the colour fade — the background *is* the tone's
	   status, so it is feedback, not motion. */
	@media (prefers-reduced-motion: reduce) {
		.tone-tile { transition: background-color 120ms ease; }
		.tone-tile:active { transform: none; }
	}

	.tone-tile[data-status='default']      { background: var(--color-neutral); }
	.tone-tile[data-status='include']      { background: var(--color-tone-include); }
	.tone-tile[data-status='avoid_detail'] { background: var(--color-tone-avoid); }
	.tone-tile[data-status='never']        { background: var(--color-danger-muted); }

	.tone-tile-topic { line-height: 1.2; }

	.tone-add-row {
		display: flex;
		gap: var(--space-2);
		align-items: stretch;
	}

	/* A control standing on the sheet's raised surface, so it takes the
	   raised-twice fill and a control's border. 16px, like every input. */
	.tone-add-input {
		flex: 1 1 auto;
		min-width: 0;
		min-height: var(--size-tap);
		padding: var(--space-2) var(--space-3);
		background: var(--color-surface-2);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		color: var(--color-text);
		font-family: inherit;
		font-size: var(--text-body);
	}
	.tone-add-input::placeholder { color: var(--color-text-faint); }
	.tone-add-input:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}

	/* The sheet's one main button, greyed out until there is a topic to add
	   (actionButton.css), the same way Send is. */
	.tone-add-button {
		flex: 0 0 auto;
		align-self: auto;
		min-width: 88px;
	}
</style>
