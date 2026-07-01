<!-- ResolvingCard.svelte
  Outer wrapper for a resolving plan: header badge, title, preparer,
  preparation notes, and a slot for plan-specific body.
-->
<script lang="ts">
	import './planPanel.css';
	import type { Snippet } from 'svelte';
	import type { Plan, Player } from '$lib/api';
	import { PLAN_SHORT, playerName } from './shared';

	interface Props {
		plan: Plan;
		players: Player[];
		error?: string;
		/** Set false to suppress the generic preparation-notes line and render
		    a custom version in the body (e.g. Spread Rumors' quoted rumor). */
		showNotes?: boolean;
		children: Snippet;
	}
	let { plan, players, error = '', showNotes = true, children }: Props = $props();
</script>

<div class="plan-panel resolving">
	<div class="plan-header">
		<span class="plan-badge resolving-badge">Resolving</span>
		<span class="plan-title">{PLAN_SHORT[plan.plan_type] ?? plan.plan_type}</span>
		<span class="plan-preparer">by {playerName(players, plan.preparer_id)}</span>
	</div>

	{#if showNotes && plan.preparation_notes}
		<p class="plan-notes">"{plan.preparation_notes}"</p>
	{/if}

	{#if error}
		<p class="res-error">{error}</p>
	{/if}

	{@render children()}
</div>
