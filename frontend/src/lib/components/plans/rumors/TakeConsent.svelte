<!-- Rumors/TakeConsent.svelte
  A take-asset request is open: everything holds until the victim (the asset owner)
  agrees or disagrees. The victim decides; everyone else reads who is being waited on.
-->
<script lang="ts">
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { respondTakeConsent, type Plan } from '$lib/api';
	import type { TakeConsentRequest } from '$lib/plans/resolutionData/spread_rumors';
	import EndsNote from '../shared/EndsNote.svelte';
	import { playerName, assetName } from '../shared';
	import type { PlanContext } from '../types';
	import { countIn, type RumorScene } from './shared';
	import type { RumorPicks } from './picks.svelte';

	let { ctx, plan, scene, pendingConsent, picks, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: RumorScene;
		pendingConsent: TakeConsentRequest;
		picks: RumorPicks;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	const isConsentVictim = $derived(currentPlayerID === pendingConsent.victim_id);

	async function onRespondConsent(p: Plan, agree: boolean) {
		if (picks.busy) return;
		picks.busy = true; error = '';
		try {
			await respondTakeConsent(p.id, agree);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not respond.';
		} finally { picks.busy = false; }
	}
</script>

{#if isConsentVictim}
	{@const takeCount = pendingConsent.asset_ids.length}
	{@const breakCount = countIn(pendingConsent.choices, 'break_target')}
	{@const leverageCount = countIn(pendingConsent.choices, 'leverage_target')}
	{@const aboutName = scene.mar
		? (scene.mcAsset?.name ?? null)
		: plan.target_asset_id != null ? assetName(assets, plan.target_asset_id) : null}

	<!-- Secondary effects hit the rumor's subject (the target asset on a make,
	     the preparer's main character on a mar) but need no
	     consent — kept out of the callout as a plain FYI line. -->
	{#if (breakCount > 0 || leverageCount > 0) && aboutName}
		<p class="choices-note">
			No consent on these — {scene.mar ? 'their counter-rumor' : 'this rumor'}
			also hits <AssetName name={aboutName} />:
		</p>
		<ul class="effect-fyi">
			{#if breakCount > 0}<li>Tears a marginalia{#if breakCount > 1}{' '}(×{breakCount}){/if}</li>{/if}
			{#if leverageCount > 0}<li>Leverages it{#if leverageCount > 1}{' '}(×{leverageCount}){/if}</li>{/if}
		</ul>
	{/if}

	<!-- The callout: the one thing the victim actually decides. -->
	<div class="consent-group">
		<p class="choices-header">Consent needed</p>
		<p class="choices-note">
			<PlayerName {players} id={pendingConsent.requested_by} /> wants to take
			{takeCount === 1 ? 'this asset' : `these ${takeCount} assets`} from you:
		</p>
		<ul class="consent-assets">
			{#each pendingConsent.asset_ids as aid}
				<li><AssetName {assets} id={aid} /></li>
			{/each}
		</ul>
		<div class="form-actions start">
			<button class="action-btn primary"
				onclick={() => onRespondConsent(plan, true)} disabled={picks.busy}>
				{picks.busy ? '…' : 'Agree'}
			</button>
			<button class="action-btn secondary"
				onclick={() => onRespondConsent(plan, false)} disabled={picks.busy}>
				Disagree
			</button>
		</div>
		<EndsNote
			show={countIn(pendingConsent.choices, 'break_target') === 0 && countIn(pendingConsent.choices, 'hide_source') === 0}
			text="If you agree, that is the last step: Spread Rumors resolves. If you disagree, the preparer picks again." />
	</div>
{:else}
	<p class="ft-prompt muted">
		Waiting for <PlayerName {players} id={pendingConsent.victim_id} /> to agree to give up
		{pendingConsent.asset_ids.length === 1
			? assetName(assets, pendingConsent.asset_ids[0])
			: `${pendingConsent.asset_ids.length} assets`}…
	</p>
{/if}

<style>
	/* The take-asset consent, shown to the victim: a group in the open step, not a
	   box. Its heading says what is asked; the group only needs space around it. */
	.consent-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.consent-assets {
		margin: 0;
		padding-left: var(--space-4);
		font-size: var(--text-small);
	}

	/* Secondary (no-consent) effects listed above the consent. A rule, not a result. */
	.effect-fyi {
		margin: 0;
		padding-left: var(--space-4);
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		line-height: 1.4;
	}
</style>
