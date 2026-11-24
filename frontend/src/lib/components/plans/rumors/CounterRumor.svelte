<!-- Rumors/CounterRumor.svelte
  Mar: the counter-rumor. "The target player describes the rumor that spreads about
  you instead." The text is written first, server-gated; every option after it is
  aimed at the preparer's main character (pinned server-side once the options apply).

  The panel mounts this for the whole step, whoever is looking, so what the actor has
  typed stays through a refetch; only the actor is shown the form.
-->
<script lang="ts">
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { writeCounterRumor, type Plan } from '$lib/api';
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import type { RumorScene } from './shared';
	import { TEXT_LIMITS } from '$lib/textLimits';

	let { ctx, plan, scene, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: RumorScene;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const players = $derived(ctx.players);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	let counterText = $state('');
	let counterBusy = $state(false);
	async function submitCounterRumor(p: Plan) {
		if (counterBusy || !counterText.trim()) return;
		counterBusy = true; error = '';
		try {
			await writeCounterRumor(p.id, counterText.trim());
			counterText = '';
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not write the counter-rumor.';
		} finally { counterBusy = false; }
	}
</script>

<!-- The mar opens with the counter-rumor: its author is the target player,
     and the table reads it before the options land. -->
{#if scene.isActor}
	<div class="plan-form">
		<p class="choices-header">The rumor backfires</p>
		<p class="choices-note">
			Describe the rumor that spreads about <AssetName name={scene.mcName} />,
			<PlayerName {players} id={plan.preparer_id} />'s main character, instead.
		</p>
		<label class="form-label">
			<span class="form-label-text">The rumor that spreads instead</span>
			<textarea rows={3} bind:value={counterText} class="form-textarea"
				maxlength={TEXT_LIMITS.NARRATIVE} disabled={counterBusy}
				placeholder="What is the court whispering about them now?"></textarea>
		</label>
		<p class="choices-note muted">The table reads this before you choose what it does.</p>
		<button class="action-btn primary" onclick={() => submitCounterRumor(plan)}
			disabled={counterBusy || !counterText.trim()}>
			{counterBusy ? '…' : 'Spread it'}
		</button>
	</div>
{/if}
