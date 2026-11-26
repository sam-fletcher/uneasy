<!-- shared/Buffet.svelte
  Read-only "what can happen?" reference, shared across plans. It is the app's
  explainer row (shared/HelpDisclosure: the deep-gold "?", the title, a chevron
  and one closing line that moves under the open reference), collapsed by
  default and fully interactive for everyone — switching tabs commits nothing.
  The tabs are chips and the reference itself is plain text: only the things
  you press have boxes. The actual choice is made via each plan's own picker;
  this is purely informational so players can weigh the consequences before
  they roll.

  Data-driven: pass `tabs`, each with an optional `always` line (an effect that
  applies on top of any chosen option), an optional `intro` line above the list,
  and an `opts` list of { label, desc }. A tab with only `always` (no opts)
  renders just that line (e.g. the festivity opt-out tab).
-->
<script lang="ts">
	import '$lib/components/shared/chip.css';
	import HelpDisclosure from '$lib/components/shared/HelpDisclosure.svelte';

	export type BuffetOpt = { key: string; label: string; desc?: string };
	export type BuffetTab = {
		key: string;
		label: string;
		always?: string;
		intro?: string;
		opts?: BuffetOpt[];
	};

	let {
		tabs,
		heading = 'What can happen?',
		defaultTab = '',
		open = false,
	}: {
		tabs: BuffetTab[];
		heading?: string;
		defaultTab?: string;
		open?: boolean;
	} = $props();

	// One per mounted Buffet: a table can show two (Exchange Courtiers), and the
	// body's id is what the row's aria-controls points at.
	const uid = $props.id();
	// `defaultTab` seeds the component's own (uncontrolled) state once on mount.
	// svelte-ignore state_referenced_locally
	let active = $state(defaultTab || tabs[0]?.key || '');
	const current = $derived(tabs.find((t) => t.key === active) ?? tabs[0]);
</script>

<div class="choices-section buffet">
	<HelpDisclosure title={heading} id={`buffet-${uid}`} defaultOpen={open}>
		<div class="buffet-body">
			<div class="buffet-tabs" role="tablist">
				{#each tabs as t (t.key)}
					<button type="button" class="chip compact buffet-tab" class:selected={active === t.key}
						role="tab" aria-selected={active === t.key} onclick={() => (active = t.key)}>{t.label}</button>
				{/each}
			</div>

			<div class="buffet-pane">
				{#if current?.always}
					<p class="choices-note buffet-always">{current.always}</p>
				{/if}
				{#if current?.opts && current.opts.length > 0}
					{#if current.intro}
						<p class="choices-note">{current.intro}</p>
					{/if}
					<ul class="buffet-list">
						{#each current.opts as o (o.key)}
							<li>{o.label}{#if o.desc}{' '}<span class="muted">{o.desc}</span>{/if}</li>
						{/each}
					</ul>
				{/if}
			</div>
		</div>
	</HelpDisclosure>
</div>

<style>
	.buffet { gap: 0; }

	/* The row, its mark and the closing line are HelpDisclosure's; this is what
	   it holds. */
	.buffet-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.buffet-tabs {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.buffet-tab { flex: 1 1 0; }
	.buffet-pane {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.buffet-always {
		margin: 0;
		color: var(--color-text);
	}
	.buffet-list {
		margin: 0;
		padding-left: var(--space-4);
		font-size: var(--text-small);
	}
	.buffet-list li {
		margin: var(--space-1) 0;
	}
</style>
