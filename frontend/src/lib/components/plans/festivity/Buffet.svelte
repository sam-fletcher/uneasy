<!-- Festivity/Buffet.svelte
  Read-only "what can happen?" reference for the festivity. Collapsible
  (open by default) and fully interactive for everyone — switching tabs
  commits nothing. The actual choice is made via the separate picker in
  SocializingTurn; this is purely informational so guests can weigh the
  make / mar / opt-out consequences before they roll.
-->
<script lang="ts">
	import {
		MAKE_OPTS, MAR_OPTS, MAKE_ALWAYS, MAR_ALWAYS, OPT_OUT_EFFECT,
	} from './options';

	type Tab = 'make' | 'mar' | 'opt';
	let open = $state(true);
	let tab = $state<Tab>('make');
</script>

<div class="choices-section buffet">
	<button
		type="button"
		class="buffet-toggle"
		aria-expanded={open}
		onclick={() => (open = !open)}
	>
		<span class="choices-header" style="margin:0;">What can happen?</span>
		<span class="muted">{open ? '−' : '+'}</span>
	</button>

	{#if open}
		<div class="buffet-tabs" role="tablist">
			<button type="button" class="buffet-tab" class:active={tab === 'make'}
				role="tab" aria-selected={tab === 'make'} onclick={() => (tab = 'make')}>Make</button>
			<button type="button" class="buffet-tab" class:active={tab === 'mar'}
				role="tab" aria-selected={tab === 'mar'} onclick={() => (tab = 'mar')}>Mar</button>
			<button type="button" class="buffet-tab" class:active={tab === 'opt'}
				role="tab" aria-selected={tab === 'opt'} onclick={() => (tab = 'opt')}>Opt out</button>
		</div>

		<div class="buffet-pane">
			{#if tab === 'make'}
				<p class="choices-note buffet-always">{MAKE_ALWAYS}</p>
				<p class="choices-note" style="margin:0 0 0.25rem;">Plus, choose one:</p>
				<ul class="buffet-list">
					{#each MAKE_OPTS as o (o.key)}
						<li>{o.label} <span class="muted">{o.desc}</span></li>
					{/each}
				</ul>
			{:else if tab === 'mar'}
				<p class="choices-note buffet-always">{MAR_ALWAYS}</p>
				<p class="choices-note" style="margin:0 0 0.25rem;">Plus, choose one:</p>
				<ul class="buffet-list">
					{#each MAR_OPTS as o (o.key)}
						<li>{o.label} <span class="muted">{o.desc}</span></li>
					{/each}
				</ul>
			{:else}
				<p class="choices-note buffet-always">{OPT_OUT_EFFECT}</p>
			{/if}
		</div>
	{/if}
</div>

<style>
	.buffet-toggle {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		background: none;
		border: none;
		padding: 0;
		cursor: pointer;
		color: inherit;
		font: inherit;
	}
	.buffet-tabs {
		display: flex;
		gap: 6px;
		margin: 0.5rem 0;
	}
	.buffet-tab {
		flex: 1;
		min-height: 36px;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		color: var(--color-text-muted);
		font: inherit;
		cursor: pointer;
	}
	.buffet-tab.active {
		background: var(--color-surface-2);
		border-color: var(--color-accent);
		color: var(--color-text);
	}
	.buffet-pane {
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		padding: 0.6rem 0.75rem;
	}
	.buffet-always {
		margin: 0 0 0.5rem;
		color: var(--color-accent);
		font-size: 0.88em;
	}
	.buffet-list {
		margin: 0;
		padding-left: 1.1rem;
	}
	.buffet-list li {
		margin: 0.2rem 0;
	}
</style>
