<!-- shared/Buffet.svelte
  Read-only "what can happen?" reference, shared across plans. Collapsible
  (collapsed by default) and fully interactive for everyone — switching tabs
  commits nothing. The actual choice is made via each plan's own picker; this is
  purely informational so players can weigh the consequences before they roll.

  Data-driven: pass `tabs`, each with an optional `always` line (an effect that
  applies on top of any chosen option), an optional `intro` line above the list,
  and an `opts` list of { label, desc }. A tab with only `always` (no opts)
  renders just that line (e.g. the festivity opt-out tab).
-->
<script lang="ts">
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

	// These props seed the component's own (uncontrolled) state once on mount.
	// svelte-ignore state_referenced_locally
	let isOpen = $state(open);
	// svelte-ignore state_referenced_locally
	let active = $state(defaultTab || tabs[0]?.key || '');
	const current = $derived(tabs.find((t) => t.key === active) ?? tabs[0]);
</script>

<div class="choices-section buffet" class:open={isOpen}>
	<button
		type="button"
		class="buffet-toggle"
		aria-expanded={isOpen}
		onclick={() => (isOpen = !isOpen)}
	>
		<span class="choices-header" style="margin:0;">{heading}</span>
		<span class="buffet-caret" aria-hidden="true">▾</span>
	</button>

	{#if isOpen}
	<div class="buffet-body">
		<div class="buffet-tabs" role="tablist">
			{#each tabs as t (t.key)}
				<button type="button" class="buffet-tab" class:active={active === t.key}
					role="tab" aria-selected={active === t.key} onclick={() => (active = t.key)}>{t.label}</button>
			{/each}
		</div>

		<div class="buffet-pane">
			{#if current?.always}
				<p class="choices-note buffet-always">{current.always}</p>
			{/if}
			{#if current?.opts && current.opts.length > 0}
				{#if current.intro}
					<p class="choices-note" style="margin:0 0 0.25rem;">{current.intro}</p>
				{/if}
				<ul class="buffet-list">
					{#each current.opts as o (o.key)}
						<li>{o.label}{#if o.desc} <span class="muted">{o.desc}</span>{/if}</li>
					{/each}
				</ul>
			{/if}
		</div>
	</div>
	{/if}
</div>

<style>
	/* Override the parent .choices-section gap so the header bar and the
	   expanded body read as one connected accordion unit. */
	.buffet { gap: 0; }

	.buffet-toggle {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		min-height: 44px;
		padding: 0.55rem 0.75rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		cursor: pointer;
		color: inherit;
		font: inherit;
		text-align: left;
	}
	/* When open, the bar joins the body below: square the bottom corners,
	   drop the dividing border, and outline the whole unit in gold. */
	.buffet.open .buffet-toggle {
		border-color: var(--color-accent);
		border-bottom-color: transparent;
		border-bottom-left-radius: 0;
		border-bottom-right-radius: 0;
	}
	.buffet-caret {
		flex-shrink: 0;
		color: var(--color-accent);
		font-size: 0.75rem;
		/* Points right when collapsed; rotates down to ▾ on open. */
		transform: rotate(-90deg);
		transition: transform 0.15s ease;
	}
	.buffet.open .buffet-caret { transform: rotate(0); }

	.buffet-body {
		border: 1px solid var(--color-accent);
		border-top: none;
		border-bottom-left-radius: 8px;
		border-bottom-right-radius: 8px;
		padding: 0.55rem 0.7rem;
	}
	.buffet-tabs {
		display: flex;
		gap: 6px;
		margin: 0 0 0.5rem;
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
