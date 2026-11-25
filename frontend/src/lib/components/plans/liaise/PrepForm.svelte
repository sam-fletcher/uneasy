<!-- Liaise/PrepForm.svelte
  Prep: a liaison is a meeting between two SPECIFIC peers — one from each player's
  retinue — picked here. The preparer selects both (and is nudged to agree the
  partner's pick in chat first), and says where they meet.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import { preparePlan } from '$lib/api';
	import PlayerChips from '../PlayerChips.svelte';
	import CardPicker from '../CardPicker.svelte';
	import { playerName, playersExcept, ownerIntactAssets } from '../shared';
	import type { PlanContext } from '../types';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import FormField from '../FormField.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { ctx }: { ctx: PlanContext } = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as {
		partner_id?: number | null;
		notes?: string;
		preparer_peer_id?: number | null;
		partner_peer_id?: number | null;
	} | null);

	let clPartnerID = $state<number | null>(null);
	let clPreparerPeerID = $state<number | null>(null);
	let clPartnerPeerID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	// The player whose prep form this is. The preparer's own form is the viewer's;
	// a watcher's read-only mirror is the focus player's, so the lists and labels
	// name the preparer, not the watcher.
	const draftPreparerID = $derived(readOnly ? ctx.focusPlayerID : currentPlayerID);
	const draftPreparerName = $derived(playerName(players, draftPreparerID));
	const otherPlayers = $derived(playersExcept(players, draftPreparerID));

	// The peers the preparer could bring to the meeting (intact, owned).
	const draftPreparerPeers = $derived(
		ownerIntactAssets(assets, draftPreparerID).filter(a => a.asset_type === 'peer'),
	);
	const prepPartnerPeers = $derived(
		ownerIntactAssets(assets, clPartnerID).filter(a => a.asset_type === 'peer'),
	);
	// "Your …" is the preparer's own wording. A watcher reads the same form, so
	// there it names whose peer each list is.
	const meetingPeerLabel = $derived(
		readOnly ? `${draftPreparerName}'s meeting peer` : 'Your meeting peer',
	);
	const meetingPeerEmpty = $derived(
		readOnly
			? `${draftPreparerName} has no peer to bring to the meeting.`
			: 'You have no peer to bring to the meeting.',
	);
	const partnerPeerEmpty = $derived(
		readOnly
			? `${playerName(players, clPartnerID)} has no peer to bring to the meeting.`
			: 'Your partner has no peer to bring to the meeting.',
	);

	// Drop a partner-peer pick if the partner changes (it belonged to the old one).
	$effect(() => {
		void clPartnerID;
		if (clPartnerPeerID != null && !prepPartnerPeers.some(p => p.id === clPartnerPeerID)) {
			clPartnerPeerID = null;
		}
	});

	const prepSubmittable = $derived(
		clPartnerID != null && clPreparerPeerID != null && clPartnerPeerID != null
		&& !!prepNotes.trim(),
	);

	async function submitPrep() {
		if (prepBusy) return;
		if (clPartnerID == null) { prepError = 'Pick a partner.'; return; }
		if (clPreparerPeerID == null) { prepError = 'Pick your meeting peer.'; return; }
		if (clPartnerPeerID == null) { prepError = "Pick your partner's meeting peer."; return; }
		if (!prepNotes.trim()) { prepError = 'Preparation notes are required.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'clandestinely_liaise',
				target_player_id: clPartnerID,
				preparer_peer_id: clPreparerPeerID,
				partner_peer_id: clPartnerPeerID,
				preparation_notes: prepNotes.trim(),
			});
			clPartnerID = null; clPreparerPeerID = null; clPartnerPeerID = null; prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	$effect(() => {
		if (!readOnly) return;
		clPartnerID = prepDraft?.partner_id ?? null;
		clPreparerPeerID = prepDraft?.preparer_peer_id ?? null;
		clPartnerPeerID = prepDraft?.partner_peer_id ?? null;
		prepNotes = prepDraft?.notes ?? '';
	});
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly) return;
		void clPartnerID; void clPreparerPeerID; void clPartnerPeerID; void prepNotes;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft({
				partner_id: clPartnerID,
				preparer_peer_id: clPreparerPeerID,
				partner_peer_id: clPartnerPeerID,
				notes: prepNotes,
			});
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });
</script>

<fieldset class="plan-form-fieldset" disabled={readOnly}>
	<div class="plan-form">
		{#if prepError}<ErrorText message={prepError} variant="panel" />{/if}
		<FormField label="Partner">
			<PlayerChips
				players={otherPlayers}
				isActive={(p) => clPartnerID === p.id}
				onSelect={(p) => (clPartnerID = clPartnerID === p.id ? null : p.id)}
				{readOnly}
			/>
		</FormField>
		<p class="choices-note muted">
			A liaison is a meeting between two specific peers, one from each of you.
		</p>
		<p class="choices-note muted">
			Tip: discuss the choice with the other player in the chat first.
		</p>
		<CardPicker
			label={meetingPeerLabel}
			items={draftPreparerPeers}
			{players}
			emptyMessage={meetingPeerEmpty}
			selected={clPreparerPeerID}
			onSelect={(id) => (clPreparerPeerID = id)}
		/>
		{#if clPartnerID != null}
			<CardPicker
				label="Partner's meeting peer"
				items={prepPartnerPeers}
				{players}
				emptyMessage={partnerPeerEmpty}
				selected={clPartnerPeerID}
				onSelect={(id) => (clPartnerPeerID = id)}
			/>
		{:else}
			<p class="choices-note muted">Pick a partner to choose their meeting peer.</p>
		{/if}
		<label class="form-label">
			Details
			<textarea rows={2} bind:value={prepNotes} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
				placeholder="Where do the two peers meet? Will they share a meal, meet under a bridge, or something more intimate?" required></textarea>
		</label>
		<p class="choices-note muted">
			Once prepared, you and your partner each reveal
			a die face to set the delay (average rounded up).
		</p>
		{#if !readOnly}
			<div class="form-actions">
				<button class="action-btn primary" onclick={submitPrep}
					disabled={prepBusy || !prepSubmittable}>
					{prepBusy ? '…' : 'Prepare Plan'}
				</button>
			</div>
		{/if}
	</div>
</fieldset>
