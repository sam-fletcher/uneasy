<!-- Rumors/PrepForm.svelte
  Prep: target asset (any asset at the table) + the rumor text (a DRAFT, and
  optionally kept secret for now). An open rumor's text may be left blank and stated
  at the pre-roll; a secret one needs its words.

  The rumor's text never goes into the live draft (lib/plans/prepDraft.ts): the draft
  is read by the whole table as it is typed, and the box that keeps the rumor secret
  can be ticked after the words are. A watcher's mirror says what is going on instead.
-->
<script lang="ts">
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { onDestroy } from 'svelte';
	import { preparePlan } from '$lib/api';
	import { spreadRumorsPrepDraft } from '$lib/plans/prepDraft';
	import PlayerChips from '../PlayerChips.svelte';
	import CardPicker from '../CardPicker.svelte';
	import { playerName, assetName } from '../shared';
	import type { PlanContext } from '../types';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import FormField from '../FormField.svelte';
	import Checkbox from '$lib/components/shared/Checkbox.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { ctx }: { ctx: PlanContext } = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as {
		filter_owner_id?: number | null;
		target_asset_id?: number | null;
		keep_secret?: boolean;
		secret_asset_id?: number | null;
	} | null);

	// Whether the preparer is keeping the rumor secret, as a watcher's mirror has it. Read
	// straight from the draft (not from prepKeepSecret, which an effect copies in) so it
	// holds on first paint.
	const mirrorKeptSecret = $derived(prepDraft?.keep_secret === true);

	let prepTargetAssetID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	// "Keep it secret for now": the rumor text is written on the underside of one
	// of the preparer's own assets instead of announced openly. prepSecretAssetID
	// is which own asset holds it.
	let prepKeepSecret = $state(false);
	let prepSecretAssetID = $state<number | null>(null);

	const intactAssets = $derived(assets.filter(a => !a.is_destroyed));

	// The preparer's own intact assets — candidate holders for the secret. In
	// prep mode the focus player drives the form, so currentPlayerID is the
	// preparer (read-only viewers don't pick; they see a summary line instead).
	const ownIntactAssets = $derived(
		intactAssets.filter(a => a.owner_id === currentPlayerID)
	);
	const secretHolderName = $derived(
		prepSecretAssetID == null ? '' : assetName(assets, prepSecretAssetID)
	);

	// Owner filter for the asset picker. Rumours about your own assets are
	// allowed (preserves the dropdown's old behaviour), so every player —
	// including the current one — appears as a chip.
	let prepFilterOwnerID = $state<number | null>(null);
	const ownersWithAssets = $derived(
		players.filter(p => intactAssets.some(a => a.owner_id === p.id))
	);
	const filteredIntactAssets = $derived(
		prepFilterOwnerID == null
			? []
			: intactAssets.filter(a => a.owner_id === prepFilterOwnerID)
	);

	// Pre-select the first owner so cards appear without an extra tap.
	// Skip in read-only — the draft sync below is authoritative for viewers.
	$effect(() => {
		if (readOnly) return;
		if (prepFilterOwnerID == null && ownersWithAssets.length > 0) {
			prepFilterOwnerID = ownersWithAssets[0].id;
		}
	});

	function selectRumorOwner(pid: number) {
		if (prepFilterOwnerID === pid) return;
		prepFilterOwnerID = pid;
		prepTargetAssetID = null;
	}


	async function submitPrep() {
		if (prepBusy) return;
		if (prepTargetAssetID == null) { prepError = 'Pick the asset the rumor is about.'; return; }
		// An open rumor may be left unwritten: it is stated at the pre-roll (book p.59;
		// rulings, Spread Rumors, 2026-11-26). A secret one always has words, so the
		// server refuses it blank too.
		if (prepKeepSecret && !prepNotes.trim()) { prepError = 'Write the rumor you are keeping secret.'; return; }
		if (prepKeepSecret && prepSecretAssetID == null) {
			prepError = 'Pick one of your assets to hide the rumor under.'; return;
		}
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'spread_rumors',
				target_asset_id: prepTargetAssetID,
				preparation_notes: prepNotes.trim(),
				secret_asset_id: prepKeepSecret ? prepSecretAssetID : undefined,
			});
			prepTargetAssetID = null; prepNotes = '';
			prepKeepSecret = false; prepSecretAssetID = null;
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	$effect(() => {
		if (!readOnly) return;
		prepFilterOwnerID = prepDraft?.filter_owner_id ?? null;
		prepTargetAssetID = prepDraft?.target_asset_id ?? null;
		prepKeepSecret = prepDraft?.keep_secret ?? false;
		prepSecretAssetID = prepDraft?.secret_asset_id ?? null;
	});
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly) return;
		// Not prepNotes: the draft never carries the rumor's text (lib/plans/prepDraft.ts), so
		// typing it sends nothing.
		void prepFilterOwnerID; void prepTargetAssetID;
		void prepKeepSecret; void prepSecretAssetID;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft(spreadRumorsPrepDraft({
				filterOwnerID: prepFilterOwnerID,
				targetAssetID: prepTargetAssetID,
				keepSecret: prepKeepSecret,
				secretAssetID: prepSecretAssetID,
			}));
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });
</script>

<fieldset class="plan-form-fieldset" disabled={readOnly}>
	<div class="plan-form">
		{#if prepError}<ErrorText message={prepError} variant="panel" />{/if}
		<FormField label="Asset owner">
			<PlayerChips
				players={ownersWithAssets}
				isActive={(p) => prepFilterOwnerID === p.id}
				onSelect={(p) => selectRumorOwner(p.id)}
				{readOnly}
			/>
		</FormField>

		{#if prepFilterOwnerID != null}
			<CardPicker
				label="Asset the rumor is about"
				items={filteredIntactAssets}
				{players}
				emptyMessage="This player has no intact assets."
				selected={prepTargetAssetID}
				onSelect={(id) => (prepTargetAssetID = id)}
				{readOnly}
			/>
		{/if}
		{#if readOnly}
			<!-- A watcher is never sent the rumor's text, kept secret or not: the draft is read by
			     the whole table, and the box can be ticked after the words are typed. An empty box
			     would read as a rumor not yet written, so say what is going on instead. -->
			<p class="form-hint">
				{#if mirrorKeptSecret}
					Kept secret for now. Only <PlayerName {players} id={ctx.focusPlayerID} /> can read it.
				{:else}
					<PlayerName {players} id={ctx.focusPlayerID} /> is preparing the rumor.
				{/if}
			</p>
		{:else}
			<label class="form-label">
				Rumor
				<textarea rows={3} bind:value={prepNotes} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
					placeholder={prepKeepSecret
						? 'Write the rumor — only you will see it until you spread it.'
						: 'What are people starting to say? You can leave it until the roll.'}></textarea>
			</label>
		{/if}

		<Checkbox
			bind:checked={prepKeepSecret}
			disabled={readOnly}
			label="Keep it secret for now"
			description="Hide the rumor under one of your own assets instead of announcing it. Others won't see the text until you spread it."
		/>

		{#if prepKeepSecret}
			{#if readOnly}
				<p class="plan-notes">
					Hidden under: <AssetName name={secretHolderName || '—'} />
				</p>
			{:else}
				<CardPicker
					label="Hide the rumor under"
					items={ownIntactAssets}
					{players}
					emptyMessage="You have no intact assets to hide it under."
					selected={prepSecretAssetID}
					onSelect={(id) => (prepSecretAssetID = id)}
					{readOnly}
				/>
			{/if}
		{/if}

		{#if !readOnly}
			<div class="form-actions">
				<button class="action-btn primary" onclick={submitPrep}
					disabled={prepBusy || prepTargetAssetID == null
						|| (prepKeepSecret && (!prepNotes.trim() || prepSecretAssetID == null))}>
					{prepBusy ? '…' : 'Prepare Plan'}
				</button>
			</div>
		{/if}
	</div>
</fieldset>
