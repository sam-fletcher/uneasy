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
		children: Snippet;
	}
	let { plan, players, error = '', children }: Props = $props();
</script>

<div class="plan-panel resolving">
	<div class="plan-header">
		<span class="plan-badge resolving-badge">Resolving</span>
		<strong class="plan-title">{PLAN_SHORT[plan.plan_type] ?? plan.plan_type}</strong>
		<span class="plan-preparer">by {playerName(players, plan.preparer_id)}</span>
	</div>

	{#if plan.preparation_notes}
		<p class="plan-notes">"{plan.preparation_notes}"</p>
	{/if}

	{#if error}
		<p class="res-error">{error}</p>
	{/if}

	{@render children()}
</div>
