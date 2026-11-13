<!-- plans/shared/EndsNote.svelte
  The line under a submit button that ends a plan or a sub-flow, so the player
  who presses it knows it is the last step before the view changes under them
  (principle 3's third limit: a removed click's punctuation is a sentence, not
  another tap). Make Introductions' "The last arrival ends the plan." is the
  pattern. Renders nothing when `show` is false, so a panel can pass the
  condition straight in.

  Write the text as a plain statement of what happens, not a warning:
  "This is the last step: Seek Answers resolves when you submit."
-->
<script lang="ts">
	let { show = true, text }: { show?: boolean; text: string } = $props();
</script>

{#if show}
	<p class="ends-note" role="note">{text}</p>
{/if}

<style>
	.ends-note {
		margin: 0;
		font-size: var(--text-small);
		line-height: 1.35;
		color: var(--color-text-secondary);
	}
</style>
