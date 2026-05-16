<!-- ProposeDuelPanel.svelte
  Prep + resolve UI for Propose Duel (Tier 3, Esteem, delay 5).

  Resolution is driven by resolution_data.duel_phase:
    setup    — champion election (initiative-gated) + stake-count reveal.
    staking  — each duelist picks which assets to stake.
    bouts    — declarer/responder bout loop with hidden dice.
    roll     — standard dice roll with accumulated bout dice pre-loaded.
    done     — winner has claimed stakes; preparer completes.

  Tied dice carry over to the winner of the next non-tie bout (per the rules).
  The backend handles the carryover; this component mirrors the same logic
  when computing the "accumulated dice" display.
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount, onDestroy } from 'svelte';
	import {
		preparePlan, makeChoice, completePlan,
		electChampion, stakeReveal, selectStakes,
		boutDeclare, boutRespond,
		getDuelState,
		type Plan, type Asset, type Player, type Ranking, type DiceRoll,
		type DuelStake, type DuelBout, type DuelStateResponse,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import AssetCardSelectable from '../AssetCardSelectable.svelte';
	import { playerColor } from '$lib/playerColor';
	import { playerName, assetName, parseResolutionData } from './shared';

	import type { PlanPanelProps } from './types';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const rankings = $derived(ctx.rankings);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const isFocusPlayer = $derived(ctx.isFocusPlayer);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// ── Prep ─────────────────────────────────────────────────────────────────
	let prepTargetPlayerID = $state<number | null>(null);
	let prepDuelType = $state<'arms' | 'wits'>('arms');
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(players.filter(p => p.id !== currentPlayerID));

	async function submitPrep() {
		if (prepBusy) return;
		if (prepTargetPlayerID == null) { prepError = 'Pick a challenger.'; return; }
		if (!prepNotes.trim()) { prepError = 'Describe the location of the duel.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'propose_duel',
				target_player_id: prepTargetPlayerID,
				duel_type: prepDuelType,
				preparation_notes: prepNotes.trim(),
			});
			prepTargetPlayerID = null;
			prepDuelType = 'arms';
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── Resolve: parse resolution_data ───────────────────────────────────────
	type DuelRes = {
		duelType: string;
		phase: string;
		initiativeID: number | null;
		prepChampID: number | null;
		targChampID: number | null;
		prepChampDeclared: boolean;
		targChampDeclared: boolean;
		prepStakeCount: number;
		targStakeCount: number;
		currentBout: number;
		choices: string[];
	};
	const duelRes = $derived.by<DuelRes>(() => {
		const rd = parseResolutionData(plan);
		return {
			duelType: rd.duel_type ?? '',
			phase: rd.duel_phase ?? '',
			initiativeID: rd.initiative_player_id ?? null,
			prepChampID: rd.preparer_champion_id ?? null,
			targChampID: rd.target_champion_id ?? null,
			prepChampDeclared: rd.preparer_champion_declared ?? false,
			targChampDeclared: rd.target_champion_declared ?? false,
			prepStakeCount: rd.preparer_stake_count ?? 0,
			targStakeCount: rd.target_stake_count ?? 0,
			currentBout: rd.current_bout ?? 0,
			choices: rd.choices ?? [],
		};
	});

	// ── Participant identity helpers ─────────────────────────────────────────
	const amPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amTarget   = $derived(
		plan != null && plan.target_player_id != null && currentPlayerID === plan.target_player_id,
	);
	const amParticipant = $derived(amPreparer || amTarget);
	const opponentID = $derived(
		plan == null ? null
			: amPreparer ? plan.target_player_id
			: amTarget   ? plan.preparer_id
			: null,
	);

	function esteemRank(playerID: number | null): number | null {
		if (playerID == null) return null;
		const r = rankings.find(x => x.category === 'esteem' && x.player_id === playerID);
		return r?.rank ?? null;
	}
	function statusOf(playerID: number | null): number {
		const r = esteemRank(playerID);
		if (r == null) return 0;
		return Math.max(6 - r, 0);
	}
	// Max stakes = 1 + status, per rules.
	const myMaxStakes = $derived(1 + statusOf(currentPlayerID));

	// ── Duel-state fetch + live refresh ──────────────────────────────────────
	let duelState = $state<DuelStateResponse | null>(null);
	let duelStateError = $state('');
	let lastFetchedPlanID = $state<number | null>(null);

	async function refreshDuelState() {
		if (!plan) return;
		try {
			duelState = await getDuelState(plan.id);
			duelStateError = '';
		} catch (e) {
			duelStateError = e instanceof Error ? e.message : 'Could not load duel state.';
		}
	}

	function onDuelEvent(e: Event) {
		const detail = (e as CustomEvent<{ plan_id: number }>).detail;
		if (plan && detail?.plan_id === plan.id) refreshDuelState();
	}

	onMount(() => {
		if (mode === 'resolve' && plan) {
			lastFetchedPlanID = plan.id;
			refreshDuelState();
		}
		window.addEventListener('uneasy:duel.champion_elected', onDuelEvent);
		window.addEventListener('uneasy:duel.stakes_revealed', onDuelEvent);
		window.addEventListener('uneasy:duel.bout_resolved', onDuelEvent);
		window.addEventListener('uneasy:duel.bouts_complete', onDuelEvent);
	});
	onDestroy(() => {
		window.removeEventListener('uneasy:duel.champion_elected', onDuelEvent);
		window.removeEventListener('uneasy:duel.stakes_revealed', onDuelEvent);
		window.removeEventListener('uneasy:duel.bout_resolved', onDuelEvent);
		window.removeEventListener('uneasy:duel.bouts_complete', onDuelEvent);
	});

	// Re-fetch when the plan changes (e.g. another duel resolves).
	$effect(() => {
		if (mode === 'resolve' && plan && plan.id !== lastFetchedPlanID) {
			lastFetchedPlanID = plan.id;
			refreshDuelState();
		}
	});
	// Also re-fetch whenever the plan's resolution_data changes (phase advance).
	$effect(() => {
		// Touch the phase to register the dependency, then refresh.
		void duelRes.phase;
		if (mode === 'resolve' && plan) refreshDuelState();
	});

	const stakes = $derived<DuelStake[]>(duelState?.stakes ?? []);
	const bouts  = $derived<DuelBout[]>(duelState?.bouts ?? []);

	const preparerStakes = $derived(
		plan == null ? [] : stakes.filter(s => s.player_id === plan.preparer_id),
	);
	const targetStakes = $derived(
		plan == null ? [] : stakes.filter(s => s.player_id === plan.target_player_id),
	);
	const myStakes = $derived(stakes.filter(s => s.player_id === currentPlayerID));
	const myUnresolvedStakes = $derived(myStakes.filter(s => !s.is_resolved));

	// ── Accumulated dice (matches backend's carryover logic) ─────────────────
	const accumulated = $derived.by(() => {
		const prep: number[] = [];
		const targ: number[] = [];
		let pending: number[] = [];
		if (!plan) return { prep, targ, pending };
		for (const b of bouts) {
			if (b.declarer_die == null || b.responder_die == null) continue;
			if (b.is_match) {
				pending.push(b.declarer_die, b.responder_die);
				continue;
			}
			if (b.winner_id == null) continue;
			const gained = [b.declarer_die, b.responder_die, ...pending];
			pending = [];
			if (b.winner_id === plan.preparer_id) prep.push(...gained);
			else targ.push(...gained);
		}
		return { prep, targ, pending };
	});

	// ── Phase: setup (champion + stake count) ────────────────────────────────
	let championAssetID = $state<number | null>(null);
	let championBusy = $state(false);
	let championError = $state('');

	// Peers I own, available as champion.
	const myPeerAssets = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID
				&& a.asset_type === 'peer'
				&& !a.is_destroyed),
	);

	const iHaveChampionDeclared = $derived(
		amPreparer ? duelRes.prepChampDeclared
		: amTarget ? duelRes.targChampDeclared
		: false,
	);
	const iHaveInitiative = $derived(
		currentPlayerID != null && duelRes.initiativeID === currentPlayerID,
	);
	const initiativeDeclared = $derived(
		plan == null ? false
			: duelRes.initiativeID === plan.preparer_id ? duelRes.prepChampDeclared
			: duelRes.initiativeID === plan.target_player_id ? duelRes.targChampDeclared
			: false,
	);
	const canElectNow = $derived(
		amParticipant && !iHaveChampionDeclared
		&& (iHaveInitiative || initiativeDeclared),
	);

	async function submitChampion(assetID: number | null) {
		if (!plan || championBusy) return;
		championBusy = true; championError = '';
		try {
			await electChampion(plan.id, assetID);
			championAssetID = null;
			onPlansChanged();
		} catch (e) {
			championError = e instanceof Error ? e.message : 'Could not elect champion.';
		} finally { championBusy = false; }
	}

	// Stake count reveal.
	let stakeCountPicked = $state<number | null>(null);
	let stakeCountBusy = $state(false);
	let stakeCountError = $state('');
	const iSubmittedStakeCount = $derived.by(() => {
		if (currentPlayerID == null) return false;
		return duelRes.choices.some(c => c.startsWith(`stake_count:${currentPlayerID}:`));
	});
	async function submitStakeCount() {
		if (!plan || stakeCountPicked == null || stakeCountBusy) return;
		stakeCountBusy = true; stakeCountError = '';
		try {
			await stakeReveal(plan.id, stakeCountPicked);
			onPlansChanged();
		} catch (e) {
			stakeCountError = e instanceof Error ? e.message : 'Could not submit stake count.';
		} finally { stakeCountBusy = false; }
	}

	// ── Phase: staking ───────────────────────────────────────────────────────
	const myStakeCount = $derived(
		amPreparer ? duelRes.prepStakeCount
		: amTarget ? duelRes.targStakeCount
		: 0,
	);
	const iHaveStaked = $derived(myStakes.length > 0);
	let stakeSelectionIDs = $state<number[]>([]);
	let stakeSubmitBusy = $state(false);
	let stakeSubmitError = $state('');

	function toggleStakeSelection(id: number) {
		stakeSelectionIDs = stakeSelectionIDs.includes(id)
			? stakeSelectionIDs.filter(x => x !== id)
			: [...stakeSelectionIDs, id];
	}

	async function submitStakes() {
		if (!plan || stakeSubmitBusy) return;
		if (stakeSelectionIDs.length !== myStakeCount) {
			stakeSubmitError = `Pick exactly ${myStakeCount} asset${myStakeCount === 1 ? '' : 's'}.`;
			return;
		}
		stakeSubmitBusy = true; stakeSubmitError = '';
		try {
			await selectStakes(plan.id, stakeSelectionIDs);
			stakeSelectionIDs = [];
			onPlansChanged();
			refreshDuelState();
		} catch (e) {
			stakeSubmitError = e instanceof Error ? e.message : 'Could not select stakes.';
		} finally { stakeSubmitBusy = false; }
	}

	// My stake-eligible assets: peer assets I own, unleveraged, not destroyed.
	// (Per the backend: stakes cannot be already-leveraged.)
	const myStakeableAssets = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID
				&& a.asset_type === 'peer'
				&& !a.is_destroyed
				&& !a.is_leveraged),
	);

	// ── Phase: bouts ─────────────────────────────────────────────────────────
	const latestBout = $derived(bouts.length === 0 ? null : bouts[bouts.length - 1]);
	const boutInProgress = $derived(latestBout != null && latestBout.resolved_at == null);

	// Whose turn is it? If bout is in progress, the responder. Else the player
	// holding initiative is the declarer.
	const currentActorID = $derived(
		boutInProgress ? (latestBout?.responder_id ?? null)
			: duelRes.initiativeID,
	);
	const isMyTurn = $derived(
		amParticipant && currentPlayerID != null && currentActorID === currentPlayerID,
	);

	let pickedStakeID = $state<number | null>(null);
	let pickedDeclaration = $state<'high' | 'low'>('high');
	let boutBusy = $state(false);
	let boutError = $state('');

	async function submitDeclare() {
		if (!plan || boutBusy || pickedStakeID == null) return;
		boutBusy = true; boutError = '';
		try {
			await boutDeclare(plan.id, pickedStakeID, pickedDeclaration);
			pickedStakeID = null;
			onPlansChanged();
			refreshDuelState();
		} catch (e) {
			boutError = e instanceof Error ? e.message : 'Could not declare bout.';
		} finally { boutBusy = false; }
	}
	async function submitRespond() {
		if (!plan || boutBusy || pickedStakeID == null) return;
		boutBusy = true; boutError = '';
		try {
			await boutRespond(plan.id, pickedStakeID);
			pickedStakeID = null;
			onPlansChanged();
			refreshDuelState();
		} catch (e) {
			boutError = e instanceof Error ? e.message : 'Could not respond.';
		} finally { boutBusy = false; }
	}

	// ── Phase: done / post-roll winner-picks-N ───────────────────────────────
	// On make: winner = preparer, takes N = result from target's stakes.
	// On mar:  winner = target,   takes N = difficulty from preparer's stakes.
	const takeCount = $derived.by(() => {
		if (!activeRoll || rollOutcome == null) return 0;
		if (rollOutcome === 'make') return activeRoll.result ?? 0;
		return activeRoll.adjusted_difficulty ?? activeRoll.difficulty;
	});
	const winnerID = $derived(
		plan == null || rollOutcome == null ? null
			: rollOutcome === 'make' ? plan.preparer_id : plan.target_player_id,
	);
	const loserID = $derived(
		plan == null || rollOutcome == null ? null
			: rollOutcome === 'make' ? plan.target_player_id : plan.preparer_id,
	);
	const amWinner = $derived(winnerID != null && currentPlayerID === winnerID);
	const loserStakes = $derived(stakes.filter(s => s.player_id === loserID));
	// Effective count: can't take more than exist.
	const effectiveTake = $derived(Math.min(takeCount, loserStakes.length));
	const choicesApplied = $derived(duelRes.phase === 'done');

	let takeSelectionIDs = $state<number[]>([]);
	let takeBusy = $state(false);
	let takeError = $state('');
	function toggleTakeSelection(assetID: number) {
		takeSelectionIDs = takeSelectionIDs.includes(assetID)
			? takeSelectionIDs.filter(x => x !== assetID)
			: [...takeSelectionIDs, assetID];
	}
	async function submitTake() {
		if (!plan || takeBusy) return;
		if (takeSelectionIDs.length !== effectiveTake) {
			takeError = `Pick exactly ${effectiveTake} asset${effectiveTake === 1 ? '' : 's'}.`;
			return;
		}
		takeBusy = true; takeError = '';
		try {
			await makeChoice(
				plan.id,
				rollOutcome!,
				takeSelectionIDs.map(id => String(id)),
			);
			takeSelectionIDs = [];
			onPlansChanged();
		} catch (e) {
			takeError = e instanceof Error ? e.message : 'Could not apply duel result.';
		} finally { takeBusy = false; }
	}

	// Complete (preparer / focus player only).
	let completeBusy = $state(false);
	async function onComplete() {
		if (!plan || completeBusy) return;
		completeBusy = true;
		try {
			await completePlan(plan.id);
			onPlansChanged();
		} catch (e) {
			takeError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { completeBusy = false; }
	}

	// Reset per-plan state when the plan changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			championAssetID = null;
			stakeCountPicked = null;
			stakeSelectionIDs = [];
			pickedStakeID = null;
			pickedDeclaration = 'high';
			takeSelectionIDs = [];
		}
	});

	// ── Display helpers ──────────────────────────────────────────────────────
	function stakeLabel(s: DuelStake): string {
		const nm = assetName(assets, s.asset_id);
		if (s.is_resolved) {
			// The stake's die is in some resolved bout; find it.
			for (const b of bouts) {
				if (b.declarer_stake_id === s.id && b.declarer_die != null) {
					return `${nm} — ${b.declarer_die}${b.is_match ? ' (set aside)' : ''}`;
				}
				if (b.responder_stake_id === s.id && b.responder_die != null) {
					return `${nm} — ${b.responder_die}${b.is_match ? ' (set aside)' : ''}`;
				}
			}
			return `${nm} — resolved`;
		}
		if (s.hidden_die != null) {
			return `${nm} — hidden d${s.hidden_die}`;
		}
		return `${nm} — hidden`;
	}
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<div class="form-label">
			<span class="form-label-text">Challenger:</span>
			<PlayerChips
				players={otherPlayers}
				isActive={(p) => prepTargetPlayerID === p.id}
				onSelect={(p) => (prepTargetPlayerID = prepTargetPlayerID === p.id ? null : p.id)}
			/>
		</div>
		<div class="form-label">
			<span class="form-label-text">Duel of:</span>
			<div class="chip-row">
				<button
					type="button"
					class="chip-btn"
					class:active={prepDuelType === 'arms'}
					onclick={() => (prepDuelType = 'arms')}
				>Arms</button>
				<button
					type="button"
					class="chip-btn"
					class:active={prepDuelType === 'wits'}
					onclick={() => (prepDuelType = 'wits')}
				>Wits / Trial</button>
			</div>
		</div>
		<label class="form-label">
			Location:
			<textarea rows={2} bind:value={prepNotes} class="form-textarea"
				placeholder="Where will the duel take place?"></textarea>
		</label>
		<div style="text-align: center;">
			<button class="action-btn primary" onclick={submitPrep}
				disabled={prepBusy || prepTargetPlayerID == null}>
				{prepBusy ? '…' : 'Prepare Plan'}
			</button>
		</div>
	</div>

{:else if plan}
	<ResolvingCard {plan} {players} error={duelStateError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID} />

		<!-- Context: duel type + initiative -->
		<p class="choices-note">
			{duelRes.duelType === 'wits' ? 'Duel of wits' : duelRes.duelType === 'arms' ? 'Duel of arms' : 'Duel'}
			· initiative: <strong>{playerName(players, duelRes.initiativeID)}</strong>
		</p>

		<!-- ═══ Phase: setup ═════════════════════════════════════════════════ -->
		{#if duelRes.phase === 'setup' || duelRes.phase === ''}
			<!-- Champions -->
			<div class="choices-section">
				<p class="choices-header">Champions</p>
				<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
					<li>
						{playerName(players, plan.preparer_id)}:
						{#if duelRes.prepChampDeclared}
							{#if duelRes.prepChampID != null}
								fights through <strong>{assetName(assets, duelRes.prepChampID)}</strong>
							{:else}
								fights in person
							{/if}
						{:else}
							<span class="muted">(not yet declared)</span>
						{/if}
					</li>
					<li>
						{playerName(players, plan.target_player_id)}:
						{#if duelRes.targChampDeclared}
							{#if duelRes.targChampID != null}
								fights through <strong>{assetName(assets, duelRes.targChampID)}</strong>
							{:else}
								fights in person
							{/if}
						{:else}
							<span class="muted">(not yet declared)</span>
						{/if}
					</li>
				</ul>

				{#if amParticipant && !iHaveChampionDeclared}
					{#if canElectNow}
						<div class="plan-form" style="margin-top:0.5rem;">
							<p class="choices-note">
								{iHaveInitiative
									? 'You have initiative — choose first.'
									: 'Your opponent has declared. Make your choice.'}
							</p>
							{#if myPeerAssets.length > 0}
								<div class="peer-cards">
									{#each myPeerAssets as a (a.id)}
										<AssetCardSelectable
											asset={a}
											ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
											selectable
											selected={championAssetID === a.id}
											onToggle={() => (championAssetID = championAssetID === a.id ? null : a.id)}
										/>
									{/each}
								</div>
							{:else}
								<p class="choices-note muted">You have no peers available as champion.</p>
							{/if}
							{#if championError}<p class="res-error">{championError}</p>{/if}
							<div style="display:flex;gap:0.5rem;">
								<button class="action-btn primary"
									onclick={() => submitChampion(championAssetID)}
									disabled={championBusy || championAssetID == null}>
									{championBusy ? '…' : 'Elect as champion'}
								</button>
								<button class="action-btn"
									onclick={() => submitChampion(null)}
									disabled={championBusy}>
									Fight yourself
								</button>
							</div>
						</div>
					{:else}
						<p class="choices-note muted" style="margin-top:0.5rem;">
							Waiting for {playerName(players, duelRes.initiativeID)} to declare first.
						</p>
					{/if}
				{/if}
			</div>

			<!-- Stake-count reveal -->
			<div class="choices-section">
				<p class="choices-header">Stake count</p>
				<p class="choices-note">
					Each duelist secretly commits to a number of assets to stake
					(min 1, max {myMaxStakes} for you). Revealed once both submit.
				</p>
				{#if amParticipant}
					{#if iSubmittedStakeCount}
						<p class="choices-note">You've submitted. Waiting for your opponent…</p>
					{:else}
						<div class="chip-row" style="margin:0.5rem 0;">
							{#each Array.from({ length: myMaxStakes }, (_, i) => i + 1) as n}
								<button
									type="button"
									class="chip-btn"
									class:active={stakeCountPicked === n}
									onclick={() => (stakeCountPicked = n)}
								>
									{n}
								</button>
							{/each}
						</div>
						{#if stakeCountError}<p class="res-error">{stakeCountError}</p>{/if}
						<button class="action-btn primary"
							onclick={submitStakeCount}
							disabled={stakeCountBusy || stakeCountPicked == null}>
							{stakeCountBusy ? '…' : 'Submit stake count'}
						</button>
					{/if}
				{:else}
					<p class="choices-note muted">
						{duelRes.prepStakeCount > 0 && duelRes.targStakeCount > 0
							? `Counts revealed: ${playerName(players, plan.preparer_id)} ${duelRes.prepStakeCount}, `
							  + `${playerName(players, plan.target_player_id)} ${duelRes.targStakeCount}.`
							: 'Counts not yet revealed.'}
					</p>
				{/if}
			</div>

		<!-- ═══ Phase: staking ═══════════════════════════════════════════════ -->
		{:else if duelRes.phase === 'staking'}
			<div class="choices-section">
				<p class="choices-header">
					Selecting stakes
					({playerName(players, plan.preparer_id)}: {duelRes.prepStakeCount},
					{playerName(players, plan.target_player_id)}: {duelRes.targStakeCount})
				</p>

				{#if amParticipant}
					{#if iHaveStaked}
						<p class="choices-note">
							You've staked {myStakes.length} asset{myStakes.length === 1 ? '' : 's'}.
							Waiting for your opponent…
						</p>
						<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
							{#each myStakes as s}
								<li>{stakeLabel(s)}</li>
							{/each}
						</ul>
					{:else}
						<p class="choices-note">
							Pick exactly {myStakeCount} peer asset{myStakeCount === 1 ? '' : 's'} to stake.
							A hidden die will be tucked under each.
						</p>
						{#if myStakeableAssets.length === 0}
							<p class="choices-note muted">You have no unleveraged peers available.</p>
						{:else}
							<div class="peer-cards">
								{#each myStakeableAssets as a (a.id)}
									<AssetCardSelectable
										asset={a}
										ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
										selectable
										selected={stakeSelectionIDs.includes(a.id)}
										onToggle={() => toggleStakeSelection(a.id)}
									/>
								{/each}
							</div>
							{#if stakeSubmitError}<p class="res-error">{stakeSubmitError}</p>{/if}
							<button class="action-btn primary"
								onclick={submitStakes}
								disabled={stakeSubmitBusy || stakeSelectionIDs.length !== myStakeCount}>
								{stakeSubmitBusy ? '…' : `Stake ${stakeSelectionIDs.length}/${myStakeCount}`}
							</button>
						{/if}
					{/if}
				{:else}
					<p class="choices-note muted">The duelists are selecting their stakes.</p>
				{/if}
			</div>

		<!-- ═══ Phase: bouts ═════════════════════════════════════════════════ -->
		{:else if duelRes.phase === 'bouts'}
			<div class="choices-section">
				<p class="choices-header">
					Bout {duelRes.currentBout + (boutInProgress ? 0 : 1)}
					· to act: <strong>{playerName(players, currentActorID)}</strong>
					{#if boutInProgress}(responding){:else}(declaring){/if}
				</p>

				<!-- Side-by-side stake columns -->
				<div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin:0.5rem 0;">
					<div>
						<p class="choices-note"><strong>{playerName(players, plan.preparer_id)}</strong></p>
						<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
							{#each preparerStakes as s}
								<li class:muted={s.is_resolved}>{stakeLabel(s)}</li>
							{/each}
						</ul>
						<p class="choices-note">
							accumulated dice: {accumulated.prep.length === 0
								? '—'
								: accumulated.prep.join(', ')}
						</p>
					</div>
					<div>
						<p class="choices-note"><strong>{playerName(players, plan.target_player_id)}</strong></p>
						<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
							{#each targetStakes as s}
								<li class:muted={s.is_resolved}>{stakeLabel(s)}</li>
							{/each}
						</ul>
						<p class="choices-note">
							accumulated dice: {accumulated.targ.length === 0
								? '—'
								: accumulated.targ.join(', ')}
						</p>
					</div>
				</div>

				{#if accumulated.pending.length > 0}
					<p class="choices-note">
						Pending tied dice (go to next bout winner): {accumulated.pending.join(', ')}
					</p>
				{/if}

				<!-- Latest bout summary (once resolved) -->
				{#if latestBout && latestBout.resolved_at != null}
					<p class="choices-note">
						Last bout:
						{playerName(players, latestBout.declarer_id)}
						declared <strong>{latestBout.declaration}</strong>
						({latestBout.declarer_die}) vs
						{playerName(players, latestBout.responder_id)} ({latestBout.responder_die})
						{#if latestBout.is_match}
							→ tie, dice set aside
						{:else if latestBout.winner_id != null}
							→ <strong>{playerName(players, latestBout.winner_id)}</strong> wins
						{/if}
					</p>
				{/if}

				<!-- My turn: declare or respond -->
				{#if isMyTurn && myUnresolvedStakes.length > 0}
					<div class="plan-form" style="margin-top:0.5rem;">
						<p class="choices-note">
							{boutInProgress ? 'Pick one of your stakes to respond.' : 'Pick a stake and declare high or low.'}
						</p>
						<div class="peer-cards">
							{#each myUnresolvedStakes as s (s.id)}
								{@const a = assets.find(x => x.id === s.asset_id)}
								{#if a}
									<AssetCardSelectable
										asset={a}
										ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
										ownerLabel={s.hidden_die != null ? `hidden d${s.hidden_die}` : 'hidden'}
										selectable
										selected={pickedStakeID === s.id}
										onToggle={() => (pickedStakeID = pickedStakeID === s.id ? null : s.id)}
									/>
								{/if}
							{/each}
						</div>
						{#if !boutInProgress}
							<div class="form-label">
								<span class="form-label-text">Declare:</span>
								<div class="chip-row">
									<button
										type="button"
										class="chip-btn"
										class:active={pickedDeclaration === 'high'}
										onclick={() => (pickedDeclaration = 'high')}
									>High</button>
									<button
										type="button"
										class="chip-btn"
										class:active={pickedDeclaration === 'low'}
										onclick={() => (pickedDeclaration = 'low')}
									>Low</button>
								</div>
							</div>
						{/if}
						{#if boutError}<p class="res-error">{boutError}</p>{/if}
						<button class="action-btn primary"
							onclick={boutInProgress ? submitRespond : submitDeclare}
							disabled={boutBusy || pickedStakeID == null}>
							{boutBusy ? '…' : boutInProgress ? 'Respond' : 'Declare'}
						</button>
					</div>
				{:else if amParticipant}
					<p class="choices-note muted">
						Waiting for {playerName(players, currentActorID)}…
					</p>
				{/if}
			</div>

		<!-- ═══ Phase: roll ══════════════════════════════════════════════════ -->
		{:else if duelRes.phase === 'roll'}
			<div class="choices-section">
				<p class="choices-header">The final roll</p>
				<p class="choices-note">
					Accumulated dice from the bouts feed into the plan's dice roll.
					{playerName(players, plan.preparer_id)}'s {accumulated.prep.length}
					{accumulated.prep.length === 1 ? 'die' : 'dice'} form the actor pool;
					{playerName(players, plan.target_player_id)}'s {accumulated.targ.length}
					{accumulated.targ.length === 1 ? 'die' : 'dice'} form interference.
				</p>
				{#if rollActive}
					<p class="choices-note muted">Dice roll in progress — resolve above.</p>
				{:else if rollOutcome != null && !choicesApplied}
					<!-- Winner picks N stakes -->
					<p class="choices-header">
						Result:
						<strong class="outcome-{rollOutcome}">
							{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
						</strong>
					</p>
					<p class="choices-note">
						{playerName(players, winnerID)} takes {effectiveTake}
						{effectiveTake === 1 ? 'stake' : 'stakes'} from
						{playerName(players, loserID)}.
					</p>
					{#if amWinner}
						{#if effectiveTake === 0}
							<p class="choices-note muted">
								Nothing to take. Applying result automatically.
							</p>
							<button class="action-btn primary"
								onclick={() => { takeSelectionIDs = []; submitTake(); }}
								disabled={takeBusy}>
								{takeBusy ? '…' : 'Apply result'}
							</button>
						{:else}
							<div class="choice-list">
								{#each loserStakes as s}
									<label class="choice-item" style="display:flex;align-items:center;gap:0.5rem;">
										<input type="checkbox"
											checked={takeSelectionIDs.includes(s.asset_id)}
											onchange={() => toggleTakeSelection(s.asset_id)} />
										<span>{assetName(assets, s.asset_id)}</span>
									</label>
								{/each}
							</div>
							{#if takeError}<p class="res-error">{takeError}</p>{/if}
							<button class="action-btn primary"
								onclick={submitTake}
								disabled={takeBusy || takeSelectionIDs.length !== effectiveTake}>
								{takeBusy ? '…' : `Take ${takeSelectionIDs.length}/${effectiveTake}`}
							</button>
						{/if}
					{:else}
						<p class="choices-note muted">
							Waiting for {playerName(players, winnerID)} to claim stakes…
						</p>
					{/if}
				{/if}
			</div>

		<!-- ═══ Phase: done ══════════════════════════════════════════════════ -->
		{:else if duelRes.phase === 'done'}
			<div class="complete-section">
				<p class="choices-applied">
					Duel complete. All staked assets are leveraged.
				</p>
				{#if takeError}<p class="res-error">{takeError}</p>{/if}
				{#if isFocusPlayer}
					<button class="action-btn primary"
						onclick={onComplete} disabled={completeBusy}>
						{completeBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>

		{:else}
			<p class="ft-prompt muted">Phase: {duelRes.phase || '(unknown)'}</p>
		{/if}

	</ResolvingCard>
{/if}
