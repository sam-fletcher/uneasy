<!--
	Quiet character counter for a length-capped field.

	Stays hidden until the value is within `warnWithin` of the limit, so a
	field nobody is stressing looks like a plain field — the counter is
	feedback, not decoration. It turns warning-coloured on the last few
	characters and at the limit, which is the moment `maxlength` starts
	silently swallowing keystrokes; without this the input just stops
	responding and reads as broken.

	Counts UTF-16 code units via .length, matching what `maxlength` enforces
	in the browser. The server counts runes, so an astral-plane character
	(emoji) can be two here and one there — the mismatch only ever makes the
	client stricter, so honest input can't be rejected by surprise.
-->
<script lang="ts">
	interface Props {
		/** Current field value. */
		value: string;
		/** The field's maxlength. */
		max: number;
		/** Show the counter once this many characters remain. */
		warnWithin?: number;
	}

	const { value, max, warnWithin = 5 }: Props = $props();

	const used = $derived(value.length);
	const remaining = $derived(max - used);
	const visible = $derived(remaining <= warnWithin);
</script>

{#if visible}
	<!-- aria-live so a screen reader hears the field filling up; "polite" so
	     it waits for a pause instead of interrupting every keystroke. -->
	<span class="char-counter" class:at-limit={remaining <= 0} aria-live="polite">
		{used}/{max}
	</span>
{/if}

<style>
	.char-counter {
		font-size: 0.75rem;
		font-variant-numeric: tabular-nums;
		color: var(--color-text-muted);
		white-space: nowrap;
		flex-shrink: 0;
	}
	.char-counter.at-limit { color: var(--color-warning); }
</style>
