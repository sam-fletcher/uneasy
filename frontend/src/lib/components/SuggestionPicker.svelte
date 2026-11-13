<!-- SuggestionPicker.svelte
  Shared "write your own, with a nudge if you're stuck" control. The free-text
  field is always visible and is the only source of `value`; below it sits a
  single example and a reroll button. Tapping the example *fills the field*
  with editable text — it is a starting point, not a competing choice, so
  there is no picked/custom mode to reconcile.

  Used wherever a player authors asset text from scratch (Prologue claims,
  Retinue marginalia, peer/asset naming) — see asset_suggestions.go for the
  matching type-keyed example pools.

  The example row is a fixed height and never wraps, so spamming reroll can't
  jump the panel under the player's thumb. The endpoint hands back the whole
  unused pool in one response, so reroll walks it locally and only spends a
  request on the wrap-around — a tap never blocks on the network.

  Looks: the field is an input and the example and reroll are controls, so all
  three take the control corner and a neutral border, on the raised-twice fill
  (they stand on a raised panel or sheet). The example is DASHED, the one
  place a dashed control survives: it is an offer, not something chosen, and
  the outline says "there is nothing here yet". The reroll is a plain secondary
  button with an icon.
-->
<script lang="ts">
	import { TEXT_LIMITS } from '$lib/textLimits';

	interface Props {
		/** Example strings to offer, one at a time. Empty hides the example row. */
		suggestions: string[];
		/** The text the player ends up with. Always the field's contents. */
		value: string;
		/** Placeholder for the free-text field. */
		placeholder?: string;
		/** When true, hold the example row's space with a loading note. */
		loading?: boolean;
		/** Max length for the field. Defaults to the marginalia tier. */
		maxlength?: number;
		/** Render the field as a multi-line textarea (for marginalia). */
		multiline?: boolean;
		disabled?: boolean;
		/** Refetch this picker's suggestion pool, for when the player has
		 *  walked all the way through it. Must leave the existing pool alone
		 *  if the request fails. Omit to hide the reroll button; the example
		 *  then stays fixed at the first one fetched. */
		onReroll?: () => void | Promise<void>;
		/** True while a reroll is in flight; disables the button. */
		rerolling?: boolean;
	}

	let {
		suggestions,
		value = $bindable(''),
		placeholder = 'Write your own…',
		loading = false,
		maxlength = TEXT_LIMITS.MARGINALIA,
		multiline = false,
		disabled = false,
		onReroll = undefined,
		rerolling = false,
	}: Props = $props();

	// Which of the fetched examples is on show. Clamped rather than indexed
	// blind so a shorter refetch can't leave us pointing past the end.
	let shown = $state(0);
	const current = $derived(
		suggestions.length > 0 ? suggestions[Math.min(shown, suggestions.length - 1)] : '',
	);

	// Fill the field, replacing whatever's there — the example is only ever
	// tapped by someone who wants it. No focus() call: on mobile that throws
	// the keyboard up and shoves the layout the player is reading.
	const useSuggestion = () => {
		if (current) value = current;
	};

	// Walk the examples already in hand first. The endpoint sends the whole
	// unused pool, so in practice every tap is a local step and the refetch
	// below is the rare wrap-around.
	const reroll = async () => {
		if (!onReroll || rerolling) return;
		if (shown + 1 < suggestions.length) {
			shown += 1;
			return;
		}
		// Pool walked out. Refetch FIRST and go back to the top only once the
		// replacement is in hand — resetting before the await would put the
		// first example back on screen for the whole round trip, which reads
		// as the button undoing itself. If the fetch fails the parent keeps
		// the old pool and `shown` stays put, so the row simply doesn't move.
		await onReroll();
		shown = 0;
	};

	// Fit-to-width: the example must stay on one line at a fixed height, but
	// the pools run to 34 characters and the narrowest column leaves ~206px for
	// text (docs/STYLE_GUIDE.md "Layout widths"), where the full size would
	// overflow. Shrinking only the strings that need it keeps the typical one
	// legible.
	const BASE_PX = 14; // --text-small, the example's size
	const MIN_PX = 12;  // the smallest any text gets; below it, ellipsis is kinder than a squint
	let textEl = $state<HTMLElement | null>(null);

	// clientWidth/scrollWidth on the nowrap span give available vs natural
	// width directly, so no padding arithmetic is needed. Reset to base first —
	// a measurement taken at a previously-shrunk size would compound.
	function fitText() {
		const el = textEl;
		if (!el) return;
		el.style.fontSize = `${BASE_PX}px`;
		const avail = el.clientWidth;
		const natural = el.scrollWidth;
		if (avail > 0 && natural > avail) {
			el.style.fontSize = `${Math.max(MIN_PX, (BASE_PX * avail) / natural)}px`;
		}
	}

	$effect(() => {
		current; // refit whenever the example changes
		fitText();
	});

	// The column resizes on rotation and when the chat/record docks flip.
	$effect(() => {
		const el = textEl;
		if (!el || typeof ResizeObserver === 'undefined') return;
		const ro = new ResizeObserver(() => fitText());
		ro.observe(el);
		return () => ro.disconnect();
	});
</script>

<div class="sp">
	{#if multiline}
		<textarea
			class="sp-input"
			bind:value
			{placeholder}
			{maxlength}
			{disabled}
			rows={2}
		></textarea>
	{:else}
		<input
			type="text"
			class="sp-input"
			bind:value
			{placeholder}
			{maxlength}
			{disabled}
		/>
	{/if}

	{#if loading || suggestions.length > 0}
		<div class="sp-example-row">
			<button
				type="button"
				class="sp-example"
				title="Use this suggestion"
				aria-label={current ? `Use suggestion: ${current}` : 'Loading suggestion'}
				disabled={disabled || loading || rerolling || !current}
				onclick={useSuggestion}
			>
				<span class="sp-example-text" bind:this={textEl}>
					{loading ? 'Loading suggestions…' : current}
				</span>
			</button>
			{#if onReroll}
				<button
					type="button"
					class="sp-reroll"
					title="Another suggestion"
					aria-label="Another suggestion"
					disabled={disabled || loading || rerolling}
					onclick={reroll}
				>
					<svg
						viewBox="0 0 24 24"
						width="18"
						height="18"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="M20 11a8.1 8.1 0 0 0 -15.5 -2m-.5 -4v4h4" />
						<path d="M4 13a8.1 8.1 0 0 0 15.5 2m.5 4v-4h-4" />
					</svg>
				</button>
			{/if}
		</div>
	{/if}
</div>

<style>
	/* The example belongs to the field above it, so it sits closer to that
	   than to whatever follows the picker. */
	.sp { display: flex; flex-direction: column; gap: var(--space-1); }

	/* An input: control corner, neutral border, raised-twice fill, and the 16px
	   body size iOS needs so it doesn't zoom the page on focus. */
	.sp-input {
		width: 100%;
		box-sizing: border-box;
		background: var(--color-surface-2);
		color: var(--color-text);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-body);
		font-family: inherit;
	}
	.sp-input:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	/* Greyed out, not faded: faint words, no border. */
	.sp-input:disabled {
		color: var(--color-text-faint);
		border-color: transparent;
		cursor: not-allowed;
	}

	.sp-example-row {
		display: flex;
		gap: var(--space-2);
		min-width: 0;
	}

	/* Quiet and dashed: an example on offer, not a filled control the player
	   has chosen. Height is fixed so reroll never moves the panel. */
	.sp-example {
		flex: 1;
		min-width: 0;
		height: var(--size-tap);
		display: flex;
		align-items: center;
		padding: 0 var(--space-2);
		background: transparent;
		border: 1px dashed var(--color-border-control);
		border-radius: var(--radius-control);
		color: var(--color-text-secondary);
		font-family: inherit;
		font-style: italic;
		cursor: pointer;
		transition: background-color 120ms ease, border-color 120ms ease, color 120ms ease;
	}
	/* hover:hover only — on touch the fill sticks after a tap, which reads as
	   "selected" and reintroduces the very mode this control does without. */
	@media (hover: hover) {
		.sp-example:hover:not(:disabled) {
			background: var(--color-surface-hover);
			border-color: var(--color-border-control-hover);
			color: var(--color-text);
		}
	}
	.sp-example:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	.sp-example:disabled { cursor: default; }

	/* flex:1 + min-width:0 makes this the measurable box: clientWidth is the
	   space available, scrollWidth the width the text actually wants. */
	.sp-example-text {
		flex: 1;
		min-width: 0;
		text-align: center;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		font-size: var(--text-small);
	}

	/* The reroll is a secondary button (neutral border, raised-twice fill, main
	   text), square at the touch minimum, so it reads as an action and not a
	   choice. */
	.sp-reroll {
		flex: none;
		width: var(--size-tap);
		height: var(--size-tap);
		padding: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: var(--color-surface-2);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		color: var(--color-text);
		cursor: pointer;
		transition: border-color 120ms ease;
	}
	@media (hover: hover) {
		.sp-reroll:hover:not(:disabled) { border-color: var(--color-border-control-hover); }
	}
	.sp-reroll:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	.sp-reroll:disabled {
		color: var(--color-text-faint);
		border-color: transparent;
		cursor: not-allowed;
	}

	/* Reduced motion removes the travel, not the feedback: the colours still
	   change, they just don't fade. */
	@media (prefers-reduced-motion: reduce) {
		.sp-example,
		.sp-reroll { transition: none; }
	}
</style>
