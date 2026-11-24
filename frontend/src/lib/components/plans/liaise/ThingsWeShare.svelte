<!-- Liaise/ThingsWeShare.svelte
  Phase 3: Things We Share. Each seat picks one option; picks are revealed once both
  players have submitted.

  Every option targets the PARTNER's assets (the rules are second-person — "your
  partner's …"). update_peer / break_peer target the partner's MEETING PEER specifically
  (fixed — the peer they brought to the liaison); take_gift a partner NON-peer; look /
  leverage any partner asset.
-->
<script lang="ts">
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { shareChoice, type Asset, type Plan } from '$lib/api';
	import { destructionWarning } from '$lib/assetRisk';
	import { hiddenCount } from '$lib/secretCounts';
	import { useSecretCounts } from '$lib/secretCountsContext';
	import EndsNote from '../shared/EndsNote.svelte';
	import CardPicker from '../CardPicker.svelte';
	import { playerName, ownerIntactAssets } from '../shared';
	import type { PlanContext } from '../types';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import FormField from '../FormField.svelte';
	import Chip from '$lib/components/shared/Chip.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import { clStateOf, type Seats } from './shared';

	let { ctx, plan, seats, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		seats: Seats;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	const clState = $derived(clStateOf(plan));
	/** The first of the viewer's seats that hasn't made a Things We Share pick yet. */
	const activeShareSeat = $derived(seats.activeShareSeat);

	// In share-choice sub-forms, the target is "the other participant" — the other
	// SEAT from the one being filled.
	const otherParticipantID = $derived(
		activeShareSeat == null
			? null
			: (activeShareSeat === plan.preparer_id ? clState.partnerID : plan.preparer_id),
	);
	const otherParticipantAssets = $derived(ownerIntactAssets(assets, otherParticipantID));
	// look_at_secret only makes sense on an asset that still holds a secret the
	// picker can't already read — otherwise the chip reveals nothing. The
	// existence count is public; "known" is viewer-scoped via the shared lookup.
	const secretCounts = useSecretCounts();
	const hasUnknownSecret = (a: Asset): boolean =>
		hiddenCount(a, secretCounts?.known(a.id) ?? 0) > 0;
	// update_peer / break_peer target the partner's MEETING PEER specifically —
	// the peer they brought to this liaison. From the preparer's seat that's
	// partnerPeerID; from the partner's seat it's preparerPeerID.
	const partnerMeetingPeerID = $derived(
		activeShareSeat === plan.preparer_id
			? clState.partnerPeerID
			: clState.preparerPeerID,
	);
	const partnerMeetingPeer = $derived(
		assets.find(a => a.id === partnerMeetingPeerID) ?? null,
	);
	// The meeting peer is targetable only if it still exists and isn't destroyed.
	const meetingPeerLive = $derived(
		partnerMeetingPeer != null && !partnerMeetingPeer.is_destroyed,
	);
	// Marginalia on the meeting peer that can still be torn (for break_peer).
	const meetingPeerBreakableMarginalia = $derived(
		(partnerMeetingPeer?.marginalia ?? []).filter(m => !m.is_torn),
	);

	// Destruction warning: tearing a peer's last intact marginalia destroys it.
	const shareBreakWarn = $derived(destructionWarning(partnerMeetingPeer));

	const SHARE_OPTIONS: { key: string; label: string; hint: string }[] = [
		{ key: 'look_at_secret',   label: "Look at partner's secrets",
			hint: "Pick one of their assets — you'll learn its secrets." },
		{ key: 'update_peer',      label: "Update partner's meeting peer",
			hint: '' },
		{ key: 'break_peer',       label: "Break partner's meeting peer",
			hint: '' },
		{ key: 'take_gift',        label: 'Take a gift from partner',
			hint: "Pick one of their non-peer assets — it transfers to you." },
		{ key: 'leverage_partner', label: "Leverage partner's asset, bank a die",
			hint: "Pick one of their assets; you bank a die for a future roll." },
	];
	// look / take_gift / leverage pick a partner asset from a list; update_peer /
	// break_peer have a FIXED target (the meeting peer) so they show no asset
	// picker — both need a marginalia chosen on that fixed peer (break_peer
	// tears it; update_peer additionally needs the rewritten text).
	const SHARE_NEEDS_PICKER = new Set(['look_at_secret', 'take_gift', 'leverage_partner']);
	const SHARE_TARGETS_MEETING_PEER = new Set(['update_peer', 'break_peer']);
	const SHARE_NEEDS_MARG = new Set(['update_peer', 'break_peer']);
	const SHARE_NEEDS_TEXT = new Set(['update_peer']);

	let shareChoiceKey = $state<string | null>(null);
	let shareAssetID = $state<number | null>(null);
	let shareMargID = $state<number | null>(null);
	let shareUpdateText = $state('');
	let shareBusy = $state(false);

	// Reset per-plan draft state on plan change — and on a change of ACTIVE seat,
	// for a viewer holding both: the next seat's form must not inherit the last
	// seat's half-made pick (it targets the other side's assets).
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			shareChoiceKey = null;
			shareAssetID = null;
			shareMargID = null;
			shareUpdateText = '';
		}
	});
	$effect(() => {
		void activeShareSeat;
		shareChoiceKey = null;
		shareAssetID = null;
		shareMargID = null;
		shareUpdateText = '';
	});

	// The asset this choice actually targets: a fixed meeting peer for
	// update/break, otherwise the player's picked partner asset.
	const shareEffectiveAssetID = $derived(
		shareChoiceKey != null && SHARE_TARGETS_MEETING_PEER.has(shareChoiceKey)
			? partnerMeetingPeerID
			: shareAssetID,
	);

	const shareSubmittable = $derived.by(() => {
		if (shareChoiceKey == null) return false;
		if (SHARE_NEEDS_PICKER.has(shareChoiceKey) && shareAssetID == null) return false;
		if (SHARE_TARGETS_MEETING_PEER.has(shareChoiceKey) && !meetingPeerLive) return false;
		if (SHARE_NEEDS_MARG.has(shareChoiceKey) && shareMargID == null) return false;
		if (SHARE_NEEDS_TEXT.has(shareChoiceKey) && !shareUpdateText.trim()) return false;
		return true;
	});

	async function onShare(p: Plan) {
		if (!shareSubmittable || shareBusy || shareChoiceKey == null) return;
		const needsAsset = SHARE_NEEDS_PICKER.has(shareChoiceKey)
			|| SHARE_TARGETS_MEETING_PEER.has(shareChoiceKey);
		shareBusy = true; error = '';
		try {
			await shareChoice(p.id, {
				choice: shareChoiceKey,
				target_asset_id: needsAsset ? shareEffectiveAssetID : null,
				target_marginalia_id: SHARE_NEEDS_MARG.has(shareChoiceKey) ? shareMargID : null,
				update_text: SHARE_NEEDS_TEXT.has(shareChoiceKey) ? shareUpdateText.trim() : undefined,
			});
			// iShared now reflects resolution_data; the refetch surfaces it.
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not submit share choice.';
		} finally { shareBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="ft-prompt">
		Pick one option. Picks are revealed once both players have submitted.
	</p>
	{#if seats.amLockedOut}
		<p class="choices-note"><PlayerName {players} id={seats.preparerSeatHolder} /> is making your pick for you.</p>
	{:else if seats.iShared}
		<p class="choices-note">You've submitted. Waiting for your partner…</p>
	{:else}
		{#if seats.actingForPreparerSeat}
			<p class="ft-prompt">
				You hold <PlayerName {players} id={plan.preparer_id} />'s side of this meeting —
				choose for them, from their partner's assets.
			</p>
		{/if}
		<FormField label="Pick one">
			<div class="chip-row">
				{#each SHARE_OPTIONS as opt}
					<Chip
						selected={shareChoiceKey === opt.key}
						onclick={() => {
							shareChoiceKey = shareChoiceKey === opt.key ? null : opt.key;
							shareAssetID = null;
							shareMargID = null;
							shareUpdateText = '';
						}}
					>{opt.label}</Chip>
				{/each}
			</div>
			{#if shareChoiceKey}
				{@const activeOpt = SHARE_OPTIONS.find(o => o.key === shareChoiceKey)}
				{#if activeOpt}
					<p class="choices-note muted">{activeOpt.hint}</p>
				{/if}
			{/if}
		</FormField>

		{#if shareChoiceKey && SHARE_TARGETS_MEETING_PEER.has(shareChoiceKey)}
			<!-- Target is fixed: the partner's MEETING PEER. No picker. -->
			{@const isUpdate = shareChoiceKey === 'update_peer'}
			{#if !meetingPeerLive}
				<p class="choices-note muted">
					{isUpdate ? 'Update' : 'Break'} is unavailable —
					your partner's meeting peer no longer exists. Pick another option.
				</p>
			{:else}
				<p class="ft-prompt">
					{isUpdate ? 'Update' : 'Break'}
					<AssetName name={partnerMeetingPeer?.name} /> —
					choose which marginalia to {isUpdate ? 'rewrite' : 'tear'}:
				</p>
				{#if meetingPeerBreakableMarginalia.length === 0}
					<p class="choices-note muted">
						This peer has no intact marginalia to {isUpdate ? 'rewrite' : 'tear'}.
					</p>
				{:else}
					<CardPicker
						label={isUpdate ? 'Marginalia to rewrite' : 'Marginalia to tear'}
						items={partnerMeetingPeer ? [partnerMeetingPeer] : []}
						{players}
						emptyMessage={isUpdate ? 'No intact marginalia to rewrite.' : 'No intact marginalia to tear.'}
						marginaliaMode
						selectedMarginaliaID={shareMargID}
						onSelectMarginalia={(mID) => { shareMargID = mID; }}
					/>
					{#if isUpdate && shareMargID != null}
						<textarea rows={2} class="form-textarea" bind:value={shareUpdateText}
							placeholder="The rewritten marginalia…" maxlength={TEXT_LIMITS.MARGINALIA}></textarea>
					{/if}
				{/if}
				{#if !isUpdate && shareBreakWarn}<Notice tone="warning">{shareBreakWarn}</Notice>{/if}
			{/if}
		{:else if shareChoiceKey && SHARE_NEEDS_PICKER.has(shareChoiceKey)}
			{@const candidates = otherParticipantAssets.filter(a => {
				if (shareChoiceKey === 'take_gift') return a.asset_type !== 'peer';
				// look_at_secret: only assets holding a secret you can't read.
				if (shareChoiceKey === 'look_at_secret') return hasUnknownSecret(a);
				return true;
			})}
			<CardPicker
				label={shareChoiceKey === 'take_gift' ? "Partner's gift" : "Partner's asset"}
				items={candidates}
				emptyMessage={shareChoiceKey === 'look_at_secret'
					? "Your partner has no assets with secrets you can't already read."
					: 'No eligible assets.'}
				{players}
				selected={shareAssetID}
				onSelect={(id) => (shareAssetID = id)}
			/>
		{/if}

		<button class="action-btn primary"
			onclick={() => onShare(plan)}
			disabled={shareBusy || !shareSubmittable}>
			{shareBusy ? '…' : 'Submit share choice'}
		</button>
		<EndsNote text="When you have both chosen, When Will I See You Again? begins. It is the last step." />
	{/if}
</div>
