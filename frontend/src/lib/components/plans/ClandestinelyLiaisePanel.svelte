<!-- ClandestinelyLiaisePanel.svelte
  Prep + resolve UI for Clandestinely Liaise (Tier 2, Knowledge, variable delay).

  Unlike other plans, CL does not use a standard dice roll. Delay is set by a
  simultaneous reveal at prep time; resolution walks through four phases
  (together_at_last → secrets_we_keep → things_we_share →
  when_will_i_see_you_again → done).
-->
<script lang="ts">
	import './planPanel.css';
	import {
		preparePlan, completePlan,
		advanceLiaise, keepSecret, shareChoice,
		type Plan, type Asset, type Player, type KeptSecret,
	} from '$lib/api';
	import { onDestroy } from 'svelte';
	import { parseLiaiseData, type LiaisePhase } from '$lib/plans/resolutionData/liaise';
	import { destructionWarning } from '$lib/assetRisk';
	import { hiddenCount } from '$lib/secretCounts';
	import { useSecretCounts } from '$lib/secretCountsContext';
	import ResolvingCard from './ResolvingCard.svelte';
	import SimultaneousRevealInput from './SimultaneousRevealInput.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import CardPicker from './CardPicker.svelte';
	import {
		playerName,
		playersExcept, ownerIntactAssets,
	} from './shared';

	import type { PlanPanelProps } from './types';
	import FormField from './FormField.svelte';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as {
		partner_id?: number | null;
		notes?: string;
		preparer_peer_id?: number | null;
		partner_peer_id?: number | null;
	} | null);

	// ── Prep ─────────────────────────────────────────────────────────────────
	// A liaison is a meeting between two SPECIFIC peers — one from each player's
	// retinue — picked here. The preparer selects both (and is nudged to agree
	// the partner's pick in chat first).
	let clPartnerID = $state<number | null>(null);
	let clPreparerPeerID = $state<number | null>(null);
	let clPartnerPeerID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(playersExcept(players, currentPlayerID));

	// The peers each side could bring to the meeting (intact, owned).
	const myPeers = $derived(
		ownerIntactAssets(assets, currentPlayerID).filter(a => a.asset_type === 'peer'),
	);
	const prepPartnerPeers = $derived(
		ownerIntactAssets(assets, clPartnerID).filter(a => a.asset_type === 'peer'),
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
		if (readOnly || mode !== 'prep') return;
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

	// ── Resolve state ────────────────────────────────────────────────────────
	type CLState = {
		phase: LiaisePhase | '';
		partnerID: number | null;
		preparerPeerID: number | null;
		partnerPeerID: number | null;
		delayRevealID: number | null;
		redelayRevealID: number | null;
		keptSecrets: KeptSecret[];
		shareSubmitterIDs: number[];
	};
	const clState = $derived.by<CLState>(() => {
		const ld = parseLiaiseData(plan);
		return {
			phase: ld.phase ?? '',
			partnerID: ld.partner_id ?? null,
			preparerPeerID: ld.preparer_peer_id ?? null,
			partnerPeerID: ld.partner_peer_id ?? null,
			delayRevealID: ld.delay_reveal_id ?? null,
			redelayRevealID: ld.redelay_reveal_id ?? null,
			keptSecrets: ld.kept_secrets ?? [],
			shareSubmitterIDs: ld.share_submitter_ids ?? [],
		};
	});

	// The two specific peers brought to the meeting (by stored id, seat-agnostic),
	// for the delay-reveal play-area summary. Looked up from the shared asset list
	// so every viewer — participant or not — can see who is meeting whom.
	const preparerMeetingPeer = $derived(
		assets.find(a => a.id === clState.preparerPeerID) ?? null,
	);
	const partnerMeetingPeerAsset = $derived(
		assets.find(a => a.id === clState.partnerPeerID) ?? null,
	);

	const amPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amPartner = $derived(currentPlayerID != null && clState.partnerID === currentPlayerID);
	const amParticipant = $derived(amPreparer || amPartner);

	const participants = $derived.by(() => {
		if (!plan) return [];
		const out: { player_id: number; display_name: string }[] = [];
		out.push({
			player_id: plan.preparer_id,
			display_name: playerName(players, plan.preparer_id),
		});
		if (clState.partnerID != null) {
			out.push({
				player_id: clState.partnerID,
				display_name: playerName(players, clState.partnerID),
			});
		}
		return out;
	});

	// Keep-secret picker: the rule is "one of your assets" (THE_12_PLANS_RULES
	// §Secrets We Keep) — writing a secret on an asset's underside, which the
	// backend allows for ANY owned asset regardless of leverage. So this list
	// must NOT exclude leveraged assets (a stricter filter wrongly hid the main
	// character, which is the asset most often leveraged through play). Only
	// destroyed assets are excluded — you can't write on a torn-up asset.
	const myKeepSecretAssets = $derived(ownerIntactAssets(assets, currentPlayerID));
	// In share-choice sub-forms, the target is "the other participant".
	const otherParticipantID = $derived(
		amPreparer ? clState.partnerID : (plan?.preparer_id ?? null),
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
		amPreparer ? clState.partnerPeerID : clState.preparerPeerID,
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

	// Has the current player submitted keep-secret?
	const iKeptSecret = $derived(
		currentPlayerID != null
		&& clState.keptSecrets.some(ks => ks.player_id === currentPlayerID),
	);
	const keepSecretSubmittedIDs = $derived(
		new Set(clState.keptSecrets.map(ks => ks.player_id)),
	);

	let resError = $state('');

	// ── Advance (preparer only) ──────────────────────────────────────────────
	let advanceBusy = $state(false);
	async function onAdvance(p: Plan) {
		if (advanceBusy) return;
		advanceBusy = true; resError = '';
		try {
			await advanceLiaise(p.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not advance phase.';
		} finally { advanceBusy = false; }
	}

	// ── Phase 2: keep secret ─────────────────────────────────────────────────
	let keepSecretAssetID = $state<number | null>(null);
	let keepBusy = $state(false);
	async function onKeepSecret(p: Plan) {
		if (keepSecretAssetID == null || keepBusy) return;
		keepBusy = true; resError = '';
		try {
			await keepSecret(p.id, keepSecretAssetID);
			keepSecretAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not commit secret.';
		} finally { keepBusy = false; }
	}

	// ── Phase 3: share choice ────────────────────────────────────────────────
	// Every option targets the PARTNER's assets (the rules are second-person —
	// "your partner's …"). update_peer / break_peer target the partner's MEETING
	// PEER specifically (fixed — the peer they brought to the liaison); take_gift
	// a partner NON-peer; look / leverage any partner asset.
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
	// Whether the current player has submitted their Things We Share choice.
	// Server-authoritative (from resolution_data) so a refresh mid-phase doesn't
	// re-prompt — the submission is the canonical signal, never a local flag.
	const iShared = $derived(
		currentPlayerID != null && clState.shareSubmitterIDs.includes(currentPlayerID),
	);

	// Reset per-plan draft state on plan change.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			keepSecretAssetID = null;
			shareChoiceKey = null;
			shareAssetID = null;
			shareMargID = null;
			shareUpdateText = '';
		}
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
		shareBusy = true; resError = '';
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
			resError = e instanceof Error ? e.message : 'Could not submit share choice.';
		} finally { shareBusy = false; }
	}

	// ── Complete ─────────────────────────────────────────────────────────────
	let completeBusy = $state(false);
	async function onComplete(p: Plan) {
		if (completeBusy) return;
		completeBusy = true; resError = '';
		try {
			await completePlan(p.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { completeBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<p class="res-error">{prepError}</p>{/if}
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
			</p><p class="choices-note muted">
				Tip: discuss the choice with the other player in the chat first.
			</p>
			<CardPicker
				label="Your meeting peer"
				items={myPeers}
				{players}
				emptyMessage="You have no peer to bring to the meeting."
				selected={clPreparerPeerID}
				onSelect={(id) => (clPreparerPeerID = id)}
			/>
			{#if clPartnerID != null}
				<CardPicker
					label="Partner's meeting peer"
					items={prepPartnerPeers}
					{players}
					emptyMessage="Your partner has no peer to bring to the meeting."
					selected={clPartnerPeerID}
					onSelect={(id) => (clPartnerPeerID = id)}
				/>
			{:else}
				<p class="choices-note muted">Pick a partner to choose their meeting peer.</p>
			{/if}
			<label class="form-label">
				Details:
				<textarea rows={2} bind:value={prepNotes} class="form-textarea"
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

{:else if mode === 'delayReveal' && plan}
	<!-- Delay reveal — the plan is pending at row 0 until both faces are in. -->
	<!-- No panel header here — the page-level WaitingOnBar already names the
	     plan ("Clandestinely Liaise — delay reveal") and who we're waiting on. -->
	<div class="plan-panel pending">
		{#if plan.preparation_notes}
			<p class="plan-notes">"{plan.preparation_notes}"</p>
		{/if}
		{#if preparerMeetingPeer || partnerMeetingPeerAsset}
			<p class="cl-meeting-peers">
				<span class="cl-peer">
					{playerName(players, plan.preparer_id)}'s
					<strong>{preparerMeetingPeer?.name ?? '?'}</strong>
				</span>
				<span class="cl-peer-sep">meets</span>
				<span class="cl-peer">
					{playerName(players, clState.partnerID)}'s
					<strong>{partnerMeetingPeerAsset?.name ?? '?'}</strong>
				</span>
			</p>
		{/if}
		{#if amParticipant && clState.delayRevealID != null && currentPlayerID != null}
			<SimultaneousRevealInput
				revealID={clState.delayRevealID}
				{currentPlayerID}
				{participants}
				prompt="Pick a die face to set the delay:"
			/>
		{:else}
			<p class="choices-note muted">
				{playerName(players, plan.preparer_id)} and
				{playerName(players, clState.partnerID)} are settling on a time…
			</p>
		{/if}
	</div>

{:else if plan}
	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID} />
		{#if !amParticipant}
			<p class="ft-prompt muted">A clandestine liaison is underway.</p>

		<!-- Phase 1: Together At Last ─────────────────────────────── -->
		{:else if clState.phase === 'together_at_last'}
			<div class="choices-section">
				<p class="choices-header">Together at Last</p>
				<p class="choices-note">
					{playerName(players, plan.preparer_id)} and
					{playerName(players, clState.partnerID)} — set the scene of your
					meeting together in the chat. Where are you? What are you doing?
					Are you sharing a meal, meeting under a bridge under the cover
					of night, or perhaps something more intimate?
				</p>
				{#if amPreparer}
					<p class="choices-note muted">
						When you're both ready, advance the liaison.
					</p>
					<button class="action-btn primary"
						onclick={() => onAdvance(plan)} disabled={advanceBusy}>
						{advanceBusy ? '…' : 'Advance to Secrets We Keep'}
					</button>
				{:else}
					<p class="choices-note muted">
						Once you're both ready, {playerName(players, plan.preparer_id)}
						will advance the liaison.
					</p>
				{/if}
			</div>

		<!-- Phase 2: Secrets We Keep ──────────────────────────────── -->
		{:else if clState.phase === 'secrets_we_keep'}
			<div class="choices-section">
				<p class="choices-header">Secrets We Keep</p>
				<p class="choices-note">
					Each of you nominates one of your own assets to hold the secret of
					this meeting.
				</p>
				{#if iKeptSecret}
					{@const waitingFor = participants.filter(p => !keepSecretSubmittedIDs.has(p.player_id))}
					{#if waitingFor.length > 0}
						<p class="choices-note">
							You've committed your secret. Waiting for
							{waitingFor.map(p => p.display_name).join(', ')}…
						</p>
					{:else}
						<p class="choices-note">Both secrets are committed.</p>
					{/if}
				{:else}
					<CardPicker
						label="Asset to hold the secret"
						items={myKeepSecretAssets}
						{players}
						emptyMessage="You have no un-leveraged assets to bear the secret."
						selected={keepSecretAssetID}
						onSelect={(id) => (keepSecretAssetID = id)}
					/>
					{#if myKeepSecretAssets.length > 0}
						<button class="action-btn primary"
							onclick={() => onKeepSecret(plan)}
							disabled={keepBusy || keepSecretAssetID == null}>
							{keepBusy ? '…' : 'Keep this secret'}
						</button>
					{/if}
				{/if}
			</div>

		<!-- Phase 3: Things We Share ──────────────────────────────── -->
		{:else if clState.phase === 'things_we_share'}
			<div class="choices-section">
				<p class="choices-header">Things We Share</p>
				<p class="choices-note">
					Pick one option. Picks are revealed once both players have submitted.
				</p>
				{#if iShared}
					<p class="choices-note">You've submitted. Waiting for your partner…</p>
				{:else}
					<FormField label="Pick one">
						<div class="chip-row">
							{#each SHARE_OPTIONS as opt}
								<button
									type="button"
									class="chip-btn"
									class:active={shareChoiceKey === opt.key}
									onclick={() => {
										shareChoiceKey = shareChoiceKey === opt.key ? null : opt.key;
										shareAssetID = null;
										shareMargID = null;
										shareUpdateText = '';
									}}
								>{opt.label}</button>
							{/each}
						</div>
						{#if shareChoiceKey}
							{@const activeOpt = SHARE_OPTIONS.find(o => o.key === shareChoiceKey)}
							{#if activeOpt}
								<p class="choices-note muted" style="margin:0.25rem 0 0;">{activeOpt.hint}</p>
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
							<p class="choices-note">
								{isUpdate ? 'Update' : 'Break'}
								<strong>{partnerMeetingPeer?.name}</strong> —
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
										placeholder="The rewritten marginalia…" maxlength={280}></textarea>
								{/if}
							{/if}
							{#if !isUpdate && shareBreakWarn}<p class="res-warning">{shareBreakWarn}</p>{/if}
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
				{/if}
			</div>

		<!-- Phase 4: When Will I See You Again ────────────────────── -->
		{:else if clState.phase === 'when_will_i_see_you_again'}
			<div class="choices-section">
				<p class="choices-header">When will I see you again?</p>
				<p class="choices-note">
					Reveal a die face (1–6) to schedule another liaison, or 0 to
					part ways. If either of you picks 0, no follow-up is scheduled.
				</p>
				{#if clState.redelayRevealID != null && currentPlayerID != null}
					<SimultaneousRevealInput
						revealID={clState.redelayRevealID}
						{currentPlayerID}
						{participants}
						allowZero={true}
						prompt="Pick a face (0 = cancel)"
					/>
				{/if}
			</div>

		<!-- Done ──────────────────────────────────────────────────── -->
		{:else if clState.phase === 'done'}
			<div class="complete-section">
				<p class="choices-applied">The liaison is complete.</p>
				{#if amPreparer}
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={completeBusy}>
						{completeBusy ? '…' : 'Complete plan'}
					</button>
				{:else}
					<p class="choices-note muted">
						Waiting for {playerName(players, plan.preparer_id)} to close the plan…
					</p>
				{/if}
			</div>

		{:else}
			<p class="ft-prompt muted">Phase: {clState.phase || '(unknown)'}</p>
		{/if}
	</ResolvingCard>
{/if}

<style>
	/* The delay-reveal scene caption ("Under a bridge") is centred to match the
	   meeting-peers summary below it. Scoped to this component, so it only
	   affects the delay-reveal notes. */
	.plan-notes {
		text-align: center;
	}

	/* Meeting-peers summary in the delay-reveal play area: who is bringing whom
	   to the liaison. Stacked and centred, with "meets" on its own line. */
	.cl-meeting-peers {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
		gap: 0.15rem;
		/* Extra bottom margin separates the meeting summary from the reveal
		   input ("Pick a die face…") / waiting copy that follows. */
		margin: 0.35rem 0 1.5rem;
		font-size: 0.9rem;
		color: var(--color-text);
	}
	.cl-peer strong {
		color: var(--color-accent);
		font-weight: 600;
	}
	.cl-peer-sep {
		color: var(--color-text-muted);
		font-style: italic;
	}
</style>
