<!-- ChoicesApplied.svelte
  Shared "Choices applied:" summary for plans whose make/mar resolution is a
  repeatable pick from a fixed option list (Seek Answers, Spread Rumors,
  Chronicle Histories, …). Renders a labelled bullet list of each option that
  was picked, with its count. Panels with bespoke completion text (e.g.
  "Law enacted.") keep their own .choices-applied <p> — this is only for the
  option-count shape.
-->
<script lang="ts">
	interface Option { key: string; label: string }
	interface Props {
		/** The applied choice keys (resolution_data.make_mar_choices → option). */
		choices: string[];
		/** The plan's option catalogue, in display order. */
		options: readonly Option[];
		/** Heading above the list. */
		label?: string;
	}
	let { choices, options, label = 'Choices applied:' }: Props = $props();

	const applied = $derived(
		options
			.map(o => ({ label: o.label, count: choices.filter(c => c === o.key).length }))
			.filter(o => o.count > 0),
	);
</script>

{#if applied.length > 0}
	<div class="choices-summary">
		<p class="choices-summary-label">{label}</p>
		<ul class="choices-summary-list">
			{#each applied as entry}
				<li>{entry.label} × {entry.count}</li>
			{/each}
		</ul>
	</div>
{/if}

<style>
	.choices-summary { margin: 0; }
	.choices-summary-label {
		margin: 0;
		font-size: 0.82rem;
		font-weight: 600;
		color: var(--color-text-muted);
	}
	.choices-summary-list {
		margin: 0.25rem 0 0 1.1rem;
		padding: 0;
		list-style: disc;
		font-size: 0.82rem;
		color: var(--color-text-muted);
	}
	.choices-summary-list li { margin: 0.15rem 0; }
</style>
