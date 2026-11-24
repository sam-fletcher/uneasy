<!-- Rumors/TakeAssets.svelte
  Step 2 of a take: when "take asset" is picked, the aggressor first names the
  specific assets to take (those can be ANY of the victim's assets), and only then is
  the victim asked. Nothing commits until the victim agrees.

  Back to the picker keeps the counts: they live in the panel's `picks`.
-->
<script lang="ts">
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { requestTakeConsent, type Plan } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import { flatChoices, type RumorScene } from './shared';
	import type { RumorPicks } from './picks.svelte';

	let { ctx, plan, scene, picks, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: RumorScene;
		picks: RumorPicks;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const rollOutcome = $derived(ctx.rollOutcome);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	// The victim of a take: target-asset owner on make, preparer on mar. The
	// aggressor selects from this player's intact assets on the consent screen.
	const victimID = $derived(scene.mar ? plan.preparer_id : scene.targetAssetOwnerID);
	const victimAssets = $derived(
		victimID == null ? [] : assets.filter(a => a.owner_id === victimID && !a.is_destroyed),
	);

	// Submit the take-asset consent request: the aggressor's full picks plus the
	// chosen victim assets. Nothing commits until the victim agrees.
	async function onRequestConsent(p: Plan, outcome: 'make' | 'mar') {
		if (picks.busy) return;
		if (picks.takeSelected.length !== (picks.counts.take_asset ?? 0)) return;
		picks.busy = true; error = '';
		try {
			await requestTakeConsent(p.id, outcome, flatChoices(scene.options, picks.counts), picks.takeSelected);
			picks.takeStep = 'options';
			picks.takeSelected = [];
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not request consent.';
		} finally { picks.busy = false; }
	}

	const k = $derived(picks.counts.take_asset ?? 0);
</script>

<div class="choices-section">
	<p class="choices-header">Choose {k} asset{k === 1 ? '' : 's'} to take</p>
	<p class="choices-note">
		From <PlayerName {players} id={victimID} />'s assets — they'll be asked to consent.
	</p>
	<CardPicker
		label="Assets to take"
		items={victimAssets}
		{players}
		emptyMessage="This player has no assets to take."
		multi
		max={k}
		selectedMulti={picks.takeSelected}
		onSelectMulti={(ids) => (picks.takeSelected = ids)}
	/>
	<p class="choices-note">
		Selected: <strong>{picks.takeSelected.length}</strong> / {k}
	</p>
	<div class="form-actions start">
		<button class="action-btn primary"
			onclick={() => onRequestConsent(plan, rollOutcome!)}
			disabled={picks.busy || picks.takeSelected.length !== k}>
			{picks.busy ? '…' : 'Request consent'}
		</button>
		<button class="action-btn" onclick={() => { picks.takeStep = 'options'; }}>
			Back
		</button>
	</div>
</div>
