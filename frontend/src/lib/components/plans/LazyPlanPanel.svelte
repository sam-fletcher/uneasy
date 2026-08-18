<!-- Resolves a plan panel's chunk on demand, then renders it.
     Pairs with registry.ts, whose entries are dynamic imports rather than
     component references (see the note there for why). -->
<script lang="ts">
	import type { Component } from 'svelte';
	import type { PlanType, Plan } from '$lib/api';
	import type { PlanContext, PlanMode, PlanPanelProps } from './types';
	import { REGISTRY } from './registry';

	interface Props {
		planType: PlanType;
		ctx: PlanContext;
		plan?: Plan;
		mode: PlanMode;
	}
	let { planType, ctx, plan, mode }: Props = $props();

	let Comp = $state<Component<PlanPanelProps> | null>(null);
	let failed = $state(false);

	// Keyed on planType so switching plans loads the new panel. The token guard
	// drops a slow load whose plan is no longer the one on screen — without it,
	// opening B while A is still in flight can render A over B.
	let token = 0;
	$effect(() => {
		const wanted = planType;
		const mine = ++token;
		const entry = REGISTRY[wanted];
		if (!entry) { Comp = null; failed = true; return; }
		failed = false;
		// Keep the previous panel on screen while the next one loads rather than
		// blanking: swapping straight to a spinner reads as the panel closing.
		entry.load()
			.then((c) => { if (mine === token) Comp = c; })
			.catch(() => { if (mine === token) { Comp = null; failed = true; } });
	});
</script>

{#if Comp}
	{@const Panel = Comp}
	<Panel {ctx} {plan} {mode} />
{:else if failed}
	<!-- The chunk could not be fetched — nearly always a stale tab across a
	     redeploy, where the hashed asset no longer exists (see
	     reference_stale_tab_after_redeploy). A reload fixes it. -->
	<div class="plan-form">
		<p class="form-hint">
			This plan's panel could not be loaded. Refresh the page to try again.
		</p>
	</div>
{:else}
	<div class="plan-form">
		<p class="form-hint">Loading…</p>
	</div>
{/if}
