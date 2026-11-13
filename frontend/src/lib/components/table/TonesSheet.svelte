<!--
  TonesSheet.svelte — the table's Tones, as a sheet.

  Moved out of the table page as it was, so the Chat piece can redesign it in
  place. It opens from the "Tones · N set" chip in the chat header and rises over
  the Chat column. What changed in the move is the numbers its styles are written
  in (they use the type, gap and corner scales now) and the surface behind the
  sticky key (the sheet is the raised surface); the layout, the wording and the
  behaviour are the old ones.

  The page owns the data and the writes (they are shared with the live-update
  handler, which drops this client's own echoes); this owns the sheet and the
  add-a-topic box.
-->
<script lang="ts">
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
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
		default: 'No Opinion',
		avoid_detail: 'Avoid detail',
		include: 'Include',
		never: 'Never',
	};
</script>

<RetinueSheet {open} {onClose}>
	<div class="tones-sheet">
		<h3 class="sheet-title">Tones</h3>
		{#if error}
			<ErrorText message={error} />
		{/if}
		<!-- The lede is the app's "read this" device: a fill and a gold left rule. It
		     carries only the *what*; the *how* ("tap to cycle") lives in the key,
		     where the mechanic it describes actually is.

		     The claim that no one is told who set a tone is load-bearing and true:
		     tone_topics is keyed on the game with no player column, and
		     handler/tone.go broadcasts a WS event without emitting an action-log
		     post. The tile does change colour for everyone — that is the point —
		     it just carries no name. Don't add attribution or a log entry here
		     without rewriting this line. -->
		<div class="tone-lede" class:locked>
			{#if locked}
				<p>
					Themes and topics the table set to include or avoid. No one is told
					who set them.
				</p>
				<p class="tone-lede-locked">Locked — the main event has begun.</p>
			{:else}
				<p>
					Themes and topics the table wants to include or avoid. Set as many as you like.
					No one is told who set each.
				</p>
			{/if}
		</div>

		<!-- Sticky, because this is the key to a 41-tile grid and unstuck it was
		     gone after one flick on a phone — leaving you tapping colours with
		     no way left on screen to read them. It sticks to the scrollport of
		     RetinueSheet's `.sheet-body`, so it must stay a DIRECT child of
		     `.tones-sheet`: wrapping it with the title in a header div would
		     bound the stick to that div and it would scroll away again. -->
		<div class="tone-legend" aria-label="Legend">
			<span class="tone-legend-lead">{locked ? 'Key' : 'Tap to cycle'}</span>
			<div class="tone-legend-items">
				<span class="tone-legend-item" data-status="default"><span class="swatch"></span>No Opinion</span>
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
					disabled={locked}
					onclick={() => onCycle(topic)}
					aria-label={`${topic.topic}: ${STATUS_WORDS[topic.status] ?? 'Never'}.${locked ? '' : ' Tap to cycle.'}`}
				>
					<span class="tone-tile-topic">{topic.topic}</span>
				</button>
			{/each}
		</div>

		{#if !locked}
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
					class="tone-add-button"
					disabled={!newTopicText.trim() || addingTopic}
				>
					{addingTopic ? '…' : '+ Add'}
				</button>
			</form>
		{/if}
	</div>
</RetinueSheet>

<style>
	.tone-lede {
		background: var(--color-surface-active);
		border: 1px solid var(--color-border-warm);
		border-left: 3px solid var(--color-accent);
		border-radius: var(--radius-piece);
		padding: var(--space-2) var(--space-3);
		/* Short bottom margin on purpose: the legend below re-pays the sheet
		   body's padding as padding-top of its own, so the visible gap between
		   the two is that plus this. */
		margin: var(--space-2) 0 var(--space-1);
	}

	/* Locked, the sheet is reference material rather than something to act on,
	   so the callout goes cold. */
	.tone-lede.locked {
		background: var(--color-bg);
		border: 1px solid var(--color-surface-2);
		border-left: 3px solid var(--color-border-strong);
	}
	.tone-lede p {
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-text);
		line-height: 1.4;
	}

	/* Higher specificity than `.tone-lede p` above, so no !important needed. */
	.tone-lede p.tone-lede-locked {
		margin-top: var(--space-1);
		font-style: italic;
		color: var(--color-text-secondary);
	}

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
		border-bottom: 1px solid var(--color-border-warm);
		color: var(--color-text-secondary);
	}

	.tone-legend-lead {
		color: var(--color-accent);
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

	.tone-legend-item .swatch {
		width: 0.9rem;
		height: 0.9rem;
		border-radius: var(--radius-piece);
		border: 1px solid rgba(255, 255, 255, 0.1);
	}

	.tone-legend-item[data-status='default']      .swatch { background: var(--color-neutral); }
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
		border: 1px solid rgba(255, 255, 255, 0.08);
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

	.tone-add-input {
		flex: 1 1 auto;
		min-width: 0;
		padding: var(--space-2) var(--space-3);
		background: var(--color-surface-2);
		border: 1px dashed rgba(255, 255, 255, 0.35);
		border-radius: var(--radius-control);
		color: var(--color-text);
		font-family: inherit;
		font-size: var(--text-body);
	}
	.tone-add-input::placeholder { color: color-mix(in srgb, var(--color-text) 50%, transparent); }
	.tone-add-input:focus {
		outline: none;
		border-style: solid;
		border-color: rgba(255, 255, 255, 0.6);
	}

	.tone-add-button {
		flex: 0 0 auto;
		min-width: 5.5rem;
		min-height: var(--size-tap);
		padding: 0 var(--space-4);
		/* -ink, not --color-info: this is a filled button with white label text,
		   and --color-info is a *fill against dark grounds*, not a ground for
		   light text — white on it managed only 3.3:1. The dark step holds 6.4:1. */
		background: var(--color-highlight-ink);
		color: var(--white);
		border: 1px solid rgba(255, 255, 255, 0.12);
		border-radius: var(--radius-control);
		font-size: var(--text-small);
		cursor: pointer;
	}
	.tone-add-button:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
