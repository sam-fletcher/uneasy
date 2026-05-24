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
	import { parseLiaiseData, type LiaisePhase } from '$lib/plans/resolutionData/liaise';
	import ResolvingCard from './ResolvingCard.svelte';
	import SimultaneousRevealInput from './SimultaneousRevealInput.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import CardPicker from './CardPicker.svelte';
	import D6Face from './D6Face.svelte';
	import {
		playerName,
		playersExcept, ownerUnleveragedAssets, ownerIntactAssets,
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

	// ── Prep ─────────────────────────────────────────────────────────────────
	let clPartnerID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(playersExcept(players, currentPlayerID));

	async function submitPrep() {
		if (prepBusy) return;
		if (clPartnerID == null) { prepError = 'Pick a partner.'; return; }
		if (!prepNotes.trim()) { prepError = 'Preparation notes are required.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'clandestinely_liaise',
				target_player_id: clPartnerID,
				preparation_notes: prepNotes.trim(),
			});
			clPartnerID = null; prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── Resolve state ────────────────────────────────────────────────────────
	type CLState = {
		phase: LiaisePhase | '';
		partnerID: number | null;
		delayRevealID: number | null;
		redelayRevealID: number | null;
		keptSecrets: KeptSecret[];
	};
	const clState = $derived.by<CLState>(() => {
		const ld = parseLiaiseData(plan);
		return {
			phase: ld.phase ?? '',
			partnerID: ld.partner_id ?? null,
			delayRevealID: ld.delay_reveal_id ?? null,
			redelayRevealID: ld.redelay_reveal_id ?? null,
			keptSecrets: ld.kept_secrets ?? [],
		};
	});

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

	const myUnleveragedAssets = $derived(ownerUnleveragedAssets(assets, currentPlayerID));
	// In share-choice sub-forms, the target is "the other participant".
	const otherParticipantID = $derived(
		amPreparer ? clState.partnerID : (plan?.preparer_id ?? null),
	);
	const otherParticipantAssets = $derived(ownerIntactAssets(assets, otherParticipantID));

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
	const SHARE_OPTIONS: { key: string; label: string; hint: string }[] = [
		{ key: 'look_at_secret',   label: "Look at partner's secrets",
			hint: "Pick one of their assets — you'll see its secrets." },
		{ key: 'update_peer',      label: 'Update your own peer',
			hint: 'Narrative only — edit the peer via its asset card later.' },
		{ key: 'break_peer',       label: 'Break your own peer',
			hint: 'Server tears one marginalia on your first intact peer.' },
		{ key: 'take_gift',        label: 'Take a gift from partner',
			hint: "Pick one of their non-peer assets — it transfers to you." },
		{ key: 'leverage_partner', label: "Leverage partner's asset, bank a die",
			hint: 'Pick one of their assets plus a die face (1–6) to bank.' },
	];
	const SHARE_NEEDS_ASSET = new Set(['look_at_secret', 'take_gift', 'leverage_partner']);
	const SHARE_NEEDS_FACE = new Set(['leverage_partner']);

	let shareChoiceKey = $state<string | null>(null);
	let shareAssetID = $state<number | null>(null);
	let shareDieFace = $state<number | null>(null);
	let shareBusy = $state(false);
	let iShared = $state(false);

	// Reset per-plan state on plan change.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			keepSecretAssetID = null;
			shareChoiceKey = null;
			shareAssetID = null;
			shareDieFace = null;
			iShared = false;
		}
	});

	const shareSubmittable = $derived.by(() => {
		if (shareChoiceKey == null) return false;
		if (SHARE_NEEDS_ASSET.has(shareChoiceKey) && shareAssetID == null) return false;
		if (SHARE_NEEDS_FACE.has(shareChoiceKey)
			&& (shareDieFace == null || shareDieFace < 1 || shareDieFace > 6)) return false;
		return true;
	});

	async function onShare(p: Plan) {
		if (!shareSubmittable || shareBusy || shareChoiceKey == null) return;
		shareBusy = true; resError = '';
		try {
			await shareChoice(p.id, {
				choice: shareChoiceKey,
				target_asset_id: SHARE_NEEDS_ASSET.has(shareChoiceKey) ? shareAssetID : null,
				die_face: SHARE_NEEDS_FACE.has(shareChoiceKey) ? shareDieFace : null,
			});
			iShared = true;
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
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<FormField label="Partner">
			<PlayerChips
				players={otherPlayers}
				isActive={(p) => clPartnerID === p.id}
				onSelect={(p) => (clPartnerID = clPartnerID === p.id ? null : p.id)}
			/>
		</FormField>
		<label class="form-label">
			Details:
			<textarea rows={2} bind:value={prepNotes} class="form-textarea"
				placeholder="Which peers are meeting? Where? Will you share a meal, meet under a bridge, or something more intimate?" required></textarea>
		</label>
		<p class="choices-note muted">
			Once prepared, you and your partner each reveal a die to set
			the delay (average rounded up).
		</p>
		<div class="form-actions">
			<button class="action-btn primary" onclick={submitPrep}
				disabled={prepBusy || clPartnerID == null || !prepNotes.trim()}>
				{prepBusy ? '…' : 'Prepare Plan'}
			</button>
		</div>
	</div>

{:else if mode === 'alwaysOn' && plan}
	<!-- Delay reveal — the plan is pending at row 0 until both faces are in. -->
	<div class="plan-panel pending">
		<div class="plan-header">
			<span class="plan-badge pending-badge">Pending — delay reveal</span>
			<strong class="plan-title">Clandestinely Liaise</strong>
			<span class="plan-preparer">by {playerName(players, plan.preparer_id)}</span>
		</div>
		{#if plan.preparation_notes}
			<p class="plan-notes">"{plan.preparation_notes}"</p>
		{/if}
		{#if amParticipant && clState.delayRevealID != null && currentPlayerID != null}
			<SimultaneousRevealInput
				revealID={clState.delayRevealID}
				{currentPlayerID}
				{participants}
				prompt="Pick a die face to set the delay"
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
				<p class="choices-header">Together at last</p>
				<p class="choices-note">
					Set the scene of your meeting in the posts below. When you're ready
					to move on, the preparer advances the liaison.
				</p>
				{#if amPreparer}
					<button class="action-btn primary"
						onclick={() => onAdvance(plan)} disabled={advanceBusy}>
						{advanceBusy ? '…' : 'Advance to Secrets We Keep'}
					</button>
				{:else}
					<p class="choices-note muted">
						Waiting for {playerName(players, plan.preparer_id)} to advance…
					</p>
				{/if}
			</div>

		<!-- Phase 2: Secrets We Keep ──────────────────────────────── -->
		{:else if clState.phase === 'secrets_we_keep'}
			<div class="choices-section">
				<p class="choices-header">Secrets we keep</p>
				<p class="choices-note">
					Each of you nominates one of your own assets to hold the secret of
					this meeting. Picks are revealed only once both have submitted.
				</p>
				{#if iKeptSecret}
					<p class="choices-note">
						You've committed your secret. Waiting for
						{#each participants.filter(p => !keepSecretSubmittedIDs.has(p.player_id)) as p, i}
							{i > 0 ? ', ' : ''}{p.display_name}
						{/each}…
					</p>
				{:else}
					<CardPicker
						label="Asset to hold the secret"
						items={myUnleveragedAssets}
						{players}
						emptyMessage="You have no un-leveraged assets to bear the secret."
						selected={keepSecretAssetID}
						onSelect={(id) => (keepSecretAssetID = id)}
					/>
					{#if myUnleveragedAssets.length > 0}
						<button class="action-btn primary"
							onclick={() => onKeepSecret(plan)}
							disabled={keepBusy || keepSecretAssetID == null}>
							{keepBusy ? '…' : 'Keep this secret'}
						</button>
					{/if}
				{/if}

				{#if amPreparer && keepSecretSubmittedIDs.size >= 2}
					<button class="action-btn primary" style="margin-top:0.5rem;"
						onclick={() => onAdvance(plan)} disabled={advanceBusy}>
						{advanceBusy ? '…' : 'Advance to Things We Share'}
					</button>
				{/if}
			</div>

		<!-- Phase 3: Things We Share ──────────────────────────────── -->
		{:else if clState.phase === 'things_we_share'}
			<div class="choices-section">
				<p class="choices-header">Things we share</p>
				<p class="choices-note">
					Pick one option. Both picks are revealed once both have submitted.
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
										shareDieFace = null;
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

					{#if shareChoiceKey && SHARE_NEEDS_ASSET.has(shareChoiceKey)}
						{@const candidates = otherParticipantAssets.filter(a =>
							shareChoiceKey === 'take_gift' ? a.asset_type !== 'peer' : true,
						)}
						<CardPicker
							label={shareChoiceKey === 'take_gift' ? "Partner's gift" : "Partner's asset"}
							items={candidates}
							{players}
							selected={shareAssetID}
							onSelect={(id) => (shareAssetID = id)}
						/>
					{/if}

					{#if shareChoiceKey && SHARE_NEEDS_FACE.has(shareChoiceKey)}
						<FormField label="Die face to bank">
							<div class="chip-row">
								{#each [1, 2, 3, 4, 5, 6] as face}
									<button
										type="button"
										class="chip-btn face-chip"
										class:active={shareDieFace === face}
										aria-label={`Bank ${face}`}
										onclick={() => (shareDieFace = face)}
									>
										<D6Face value={face} size={28} />
									</button>
								{/each}
							</div>
						</FormField>
					{/if}

					<button class="action-btn primary"
						onclick={() => onShare(plan)}
						disabled={shareBusy || !shareSubmittable}>
						{shareBusy ? '…' : 'Submit share choice'}
					</button>
				{/if}

				{#if amPreparer}
					<button class="action-btn" style="margin-top:0.5rem;"
						onclick={() => onAdvance(plan)} disabled={advanceBusy}>
						{advanceBusy ? '…' : 'Advance to "When will I see you again?"'}
					</button>
					<p class="choices-note muted">
						(Advance once both players have submitted above.)
					</p>
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
