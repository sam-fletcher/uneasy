<!-- HostFestivityPanel.svelte
  Prep + resolve UI for Host Festivity (Tier 3, Esteem, delay 6).

  Resolution phases (driven by resolution_data.festivity_phase):
    socializing   — guests join, then take turns (lower esteem first, host last).
    host_choosing — host picks a make option for each guest who marred / opted out.
    done          — host clicks Complete.

  Each guest creates their own dice roll via /guest-roll. The active roll is
  surfaced through the parent's <DiceRollPanel> using the standard
  activeRoll/rollOutcome props. Once the roll resolves, the guest submits a
  make/mar choice through /guest-choice (or /challenge-duel for duels).
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount, onDestroy } from 'svelte';
	import {
		preparePlan, completePlan,
		joinFestivity, guestRoll, guestChoice, hostChoice,
		challengeDuel, respondChallenge, insistHostMar,
		getRoll,
		type Plan, type Asset, type Player, type Ranking, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { playerName, assetName, parseResolutionData } from './shared';

	interface Props {
		mode: 'prep' | 'resolve';
		gameID: number;
		assets: Asset[];
		players: Player[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		plans?: Plan[];
		plan?: Plan | null;
		isFocusPlayer?: boolean;
		rollActive?: boolean;
		rollOutcome?: 'make' | 'mar' | null;
		activeRoll?: DiceRoll | null;
		onPlansChanged?: () => void;
		onPlanPrepared?: () => void;
	}

	let {
		mode, gameID, assets, players, rankings, currentPlayerID, plans = [],
		plan = null, isFocusPlayer = false,
		rollActive = false, rollOutcome = null, activeRoll = null,
		onPlansChanged = () => {},
		onPlanPrepared = () => {},
	}: Props = $props();

	// ── Prep ─────────────────────────────────────────────────────────────────
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Describe the event.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'host_festivity',
				preparation_notes: prepNotes.trim(),
			});
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── Resolve: parse resolution_data ───────────────────────────────────────
	type FestRes = {
		phase: string;
		guests: number[];
		outcomes: Record<string, string>;       // pid → "make"|"mar"|"opt_out"
		guestMakes: Record<string, string>;
		guestMars: Record<string, string>;
		hostChoices: Record<string, string>;
		guestRollIDs: Record<string, number>;
		guestIOUs: number[];
		hostMarInsists: string[];
		acceptDuels: number[];
		pendingDuelPlanID: number | null;
		pendingChallenge: { challenger_id: number; target_id: number; notes?: string } | null;
		centeredAssetIDs: number[];
	};
	const fest = $derived.by<FestRes>(() => {
		const rd = parseResolutionData(plan);
		return {
			phase: rd.festivity_phase ?? '',
			guests: rd.guest_player_ids ?? [],
			outcomes: rd.guest_outcomes ?? {},
			guestMakes: rd.guest_make_choices ?? {},
			guestMars: rd.guest_mar_choices ?? {},
			hostChoices: rd.host_guest_choices ?? {},
			guestRollIDs: rd.guest_roll_ids ?? {},
			guestIOUs: rd.guest_ious ?? [],
			hostMarInsists: rd.host_mar_insists ?? [],
			acceptDuels: rd.accept_duels_player_ids ?? [],
			pendingDuelPlanID: rd.pending_duel_plan_id ?? null,
			pendingChallenge: rd.pending_challenge ?? null,
			centeredAssetIDs: rd.centered_asset_ids ?? [],
		};
	});

	const meKey = $derived(currentPlayerID == null ? '' : String(currentPlayerID));
	const amHost = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const iAmGuest = $derived(currentPlayerID != null && fest.guests.includes(currentPlayerID));
	const myOutcome = $derived(meKey ? fest.outcomes[meKey] ?? null : null);
	const myRollID = $derived(meKey ? fest.guestRollIDs[meKey] ?? null : null);
	const iHaveIOU = $derived(currentPlayerID != null && fest.guestIOUs.includes(currentPlayerID));

	// ── Esteem-ordered guest list (lower esteem first, host last) ────────────
	function esteemRank(playerID: number | null): number {
		if (playerID == null) return 999;
		const r = rankings.find(x => x.category === 'esteem' && x.player_id === playerID);
		return r?.rank ?? 999;
	}
	const orderedGuests = $derived.by<number[]>(() => {
		if (plan == null) return [];
		const hostID = plan.preparer_id;
		const others = fest.guests.filter(id => id !== hostID);
		// Higher rank number = lower esteem; lower-esteem players go first.
		others.sort((a, b) => esteemRank(b) - esteemRank(a));
		return fest.guests.includes(hostID) ? [...others, hostID] : others;
	});

	// Status string per guest, for the side panel.
	function guestStatus(pid: number): string {
		const k = String(pid);
		const oc = fest.outcomes[k];
		if (oc === 'make') return `make → ${fest.guestMakes[k] ?? '?'}`;
		if (oc === 'mar') return `mar → ${fest.guestMars[k] ?? '?'}`;
		if (oc === 'opt_out') return 'opted out';
		if (fest.guestRollIDs[k]) return 'rolling';
		return 'waiting';
	}

	// "Whose turn" — the first ordered guest who hasn't acted yet.
	const currentTurnID = $derived.by<number | null>(() => {
		for (const id of orderedGuests) {
			if (!(String(id) in fest.outcomes)) return id;
		}
		return null;
	});

	// ── Active roll outcome (fallback fetch) ─────────────────────────────────
	// If our own roll has resolved but the parent's activeRoll is gone (e.g. it
	// was cleared), fetch the roll once to recover the outcome.
	let fetchedRollOutcome = $state<'make' | 'mar' | null>(null);
	let fetchedForRollID = $state<number | null>(null);
	$effect(() => {
		const haveOwnActive = activeRoll && myRollID != null && activeRoll.id === myRollID;
		if (haveOwnActive) {
			fetchedRollOutcome = rollOutcome ?? null;
			fetchedForRollID = myRollID;
			return;
		}
		if (myRollID != null && myOutcome == null && fetchedForRollID !== myRollID) {
			fetchedForRollID = myRollID;
			getRoll(myRollID)
				.then(r => { fetchedRollOutcome = r.roll.outcome ?? null; })
				.catch(() => { fetchedRollOutcome = null; });
		}
	});
	const myEffectiveOutcome = $derived<'make' | 'mar' | null>(
		(activeRoll && myRollID != null && activeRoll.id === myRollID
			? rollOutcome
			: fetchedRollOutcome) ?? null,
	);

	// ── Live refresh ─────────────────────────────────────────────────────────
	function onFestEvent(e: Event) {
		const d = (e as CustomEvent<{ plan_id: number }>).detail;
		if (plan && d?.plan_id === plan.id) onPlansChanged();
	}
	const FEST_EVENTS = [
		'uneasy:festivity.guest_joined',
		'uneasy:festivity.guest_rolled',
		'uneasy:festivity.guest_chose',
		'uneasy:festivity.host_chose',
		'uneasy:festivity.insist_host_mar',
		'uneasy:festivity.phase_changed',
		'uneasy:festivity.challenge_issued',
		'uneasy:festivity.challenge_declined',
		'uneasy:festivity.duel_triggered',
	];
	onMount(() => { for (const ev of FEST_EVENTS) window.addEventListener(ev, onFestEvent); });
	onDestroy(() => { for (const ev of FEST_EVENTS) window.removeEventListener(ev, onFestEvent); });

	// ── Join / opt-out / roll ────────────────────────────────────────────────
	let actionBusy = $state(false);
	let actionError = $state('');

	async function onJoin() {
		if (!plan || actionBusy) return;
		actionBusy = true; actionError = '';
		try { await joinFestivity(plan.id); onPlansChanged(); }
		catch (e) { actionError = e instanceof Error ? e.message : 'Could not join.'; }
		finally { actionBusy = false; }
	}
	async function onRoll() {
		if (!plan || actionBusy) return;
		actionBusy = true; actionError = '';
		try { await guestRoll(plan.id, 'roll'); onPlansChanged(); }
		catch (e) { actionError = e instanceof Error ? e.message : 'Could not roll.'; }
		finally { actionBusy = false; }
	}
	async function onOptOut() {
		if (!plan || actionBusy) return;
		actionBusy = true; actionError = '';
		try { await guestRoll(plan.id, 'opt_out'); onPlansChanged(); }
		catch (e) { actionError = e instanceof Error ? e.message : 'Could not opt out.'; }
		finally { actionBusy = false; }
	}

	// ── My make / mar choice picker ──────────────────────────────────────────
	const MAKE_OPTS = [
		{ key: 'spread_rumor',     label: 'Spread a new rumor (notecard added to public record)' },
		{ key: 'introduce_peer',   label: 'Introduce a new peer' },
		{ key: 'take_center_peer', label: 'Take a peer from the center of the table' },
		{ key: 'challenge_duel',   label: 'Challenge somebody to a duel' },
	];
	const MAR_OPTS = [
		{ key: 'rumor_about_you', label: 'A rumor spreads about you' },
		{ key: 'disagreement',    label: 'Get into a disagreement with one of your peers (sets them in the center)' },
		{ key: 'accept_duels',    label: 'You must accept any duel challenges during the event' },
		{ key: 'break_self',      label: 'Break yourself (tear a marginalia on your main character)' },
	];

	let pickedChoice = $state<string | null>(null);
	let rumorText = $state('');
	let peerName = $state('');
	let pickedAssetID = $state<number | null>(null);
	let pickedDuelTargetID = $state<number | null>(null);
	let pickerBusy = $state(false);
	let pickerError = $state('');

	const myCenterPeerCandidates = $derived(
		assets.filter(a => fest.centeredAssetIDs.includes(a.id) && !a.is_destroyed),
	);
	const myOwnPeers = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID
				&& a.asset_type === 'peer'
				&& !a.is_destroyed),
	);
	const otherGuests = $derived(
		fest.guests.filter(id => id !== currentPlayerID),
	);

	function resetPicker() {
		pickedChoice = null;
		rumorText = '';
		peerName = '';
		pickedAssetID = null;
		pickedDuelTargetID = null;
		pickerError = '';
	}

	async function submitMyChoice() {
		if (!plan || pickerBusy || !pickedChoice) return;
		pickerBusy = true; pickerError = '';
		try {
			if (myEffectiveOutcome === 'make' && pickedChoice === 'challenge_duel') {
				if (pickedDuelTargetID == null) {
					pickerError = 'Pick a target.';
					return;
				}
				await challengeDuel(plan.id, pickedDuelTargetID);
			} else {
				const body: { choice: string; rumor_text?: string; peer_name?: string; asset_id?: number } = {
					choice: pickedChoice,
				};
				if (pickedChoice === 'spread_rumor' || pickedChoice === 'rumor_about_you') {
					body.rumor_text = rumorText.trim();
				}
				if (pickedChoice === 'introduce_peer') {
					body.peer_name = peerName.trim() || 'New peer';
				}
				if (pickedChoice === 'take_center_peer' || pickedChoice === 'disagreement') {
					if (pickedAssetID == null) { pickerError = 'Pick an asset.'; return; }
					body.asset_id = pickedAssetID;
				}
				await guestChoice(plan.id, body);
			}
			resetPicker();
			onPlansChanged();
		} catch (e) {
			pickerError = e instanceof Error ? e.message : 'Could not submit choice.';
		} finally { pickerBusy = false; }
	}

	// ── Host's per-guest make picker (host_choosing phase) ───────────────────
	const pendingHostGuests = $derived(
		fest.guests.filter(id => {
			const k = String(id);
			const oc = fest.outcomes[k];
			if (oc !== 'mar' && oc !== 'opt_out') return false;
			return !(k in fest.hostChoices);
		}),
	);

	let hostPickerGuestID = $state<number | null>(null);
	let hostPickedChoice = $state<string | null>(null);
	let hostRumor = $state('');
	let hostPeerName = $state('');
	let hostAssetID = $state<number | null>(null);
	let hostPickerBusy = $state(false);
	let hostPickerError = $state('');

	// Host can choose any make option except challenge_duel for the guest.
	const HOST_MAKE_OPTS = MAKE_OPTS.filter(o => o.key !== 'challenge_duel');

	function resetHostPicker() {
		hostPickerGuestID = null;
		hostPickedChoice = null;
		hostRumor = '';
		hostPeerName = '';
		hostAssetID = null;
		hostPickerError = '';
	}

	async function submitHostChoice() {
		if (!plan || hostPickerBusy || hostPickerGuestID == null || !hostPickedChoice) return;
		hostPickerBusy = true; hostPickerError = '';
		try {
			const body: { target_player_id: number; choice: string; rumor_text?: string; peer_name?: string; asset_id?: number } = {
				target_player_id: hostPickerGuestID,
				choice: hostPickedChoice,
			};
			if (hostPickedChoice === 'spread_rumor') body.rumor_text = hostRumor.trim();
			if (hostPickedChoice === 'introduce_peer') body.peer_name = hostPeerName.trim() || 'New peer';
			if (hostPickedChoice === 'take_center_peer') {
				if (hostAssetID == null) { hostPickerError = 'Pick a centered peer.'; return; }
				body.asset_id = hostAssetID;
			}
			await hostChoice(plan.id, body);
			resetHostPicker();
			onPlansChanged();
		} catch (e) {
			hostPickerError = e instanceof Error ? e.message : 'Could not submit host choice.';
		} finally { hostPickerBusy = false; }
	}

	// ── IOU insist (force a mar on the host) ─────────────────────────────────
	let insistOpen = $state(false);
	let insistChoice = $state<string | null>(null);
	let insistRumor = $state('');
	let insistAssetID = $state<number | null>(null);
	let insistBusy = $state(false);
	let insistError = $state('');

	async function submitInsist() {
		if (!plan || insistBusy || !insistChoice) return;
		insistBusy = true; insistError = '';
		try {
			const body: { mar_option: string; rumor_text?: string; asset_id?: number } = {
				mar_option: insistChoice,
			};
			if (insistChoice === 'rumor_about_you') body.rumor_text = insistRumor.trim();
			if (insistChoice === 'disagreement') {
				if (insistAssetID == null) { insistError = 'Pick a peer.'; return; }
				body.asset_id = insistAssetID;
			}
			await insistHostMar(plan.id, body);
			insistOpen = false;
			insistChoice = null;
			insistRumor = '';
			insistAssetID = null;
			onPlansChanged();
		} catch (e) {
			insistError = e instanceof Error ? e.message : 'Could not insist.';
		} finally { insistBusy = false; }
	}

	// ── Pending challenge response (target only) ─────────────────────────────
	let respondBusy = $state(false);
	let respondError = $state('');
	const challengeIsForMe = $derived(
		fest.pendingChallenge != null
		&& currentPlayerID != null
		&& fest.pendingChallenge.target_id === currentPlayerID,
	);
	const mustAccept = $derived(
		currentPlayerID != null && fest.acceptDuels.includes(currentPlayerID),
	);

	async function onRespond(accept: boolean) {
		if (!plan || respondBusy) return;
		respondBusy = true; respondError = '';
		try { await respondChallenge(plan.id, accept); onPlansChanged(); }
		catch (e) { respondError = e instanceof Error ? e.message : 'Could not respond.'; }
		finally { respondBusy = false; }
	}

	// ── Complete (host) ──────────────────────────────────────────────────────
	let completeBusy = $state(false);
	let completeError = $state('');
	async function onComplete() {
		if (!plan || completeBusy) return;
		completeBusy = true; completeError = '';
		try { await completePlan(plan.id); onPlansChanged(); }
		catch (e) { completeError = e instanceof Error ? e.message : 'Could not complete plan.'; }
		finally { completeBusy = false; }
	}

	// Reset picker when plan changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			resetPicker();
			resetHostPicker();
		}
	});
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<label class="form-label">
			Event:
			<textarea rows={3} bind:value={prepNotes} class="form-textarea"
				placeholder="Where are you, and what sort of event is planned?"></textarea>
		</label>
		<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy}>
			{prepBusy ? '…' : 'Prepare Host Festivity'}
		</button>
	</div>

{:else if plan}
	<ResolvingCard {plan} {players}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID} />

		<p class="choices-note">
			Phase: <strong>{fest.phase || '(starting)'}</strong>
			· host: <strong>{playerName(players, plan.preparer_id)}</strong>
		</p>

		<!-- ── Guest list ─────────────────────────────────────────────────── -->
		<div class="choices-section">
			<p class="choices-header">Guests</p>
			{#if orderedGuests.length === 0}
				<p class="choices-note muted">No guests yet.</p>
			{:else}
				<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
					{#each orderedGuests as gid (gid)}
						{@const isHost = gid === plan.preparer_id}
						{@const isTurn = gid === currentTurnID}
						<li class:muted={!isTurn && (String(gid) in fest.outcomes)}>
							<strong>{playerName(players, gid)}</strong>
							{#if isHost}<em> (host)</em>{/if}
							— {guestStatus(gid)}
							{#if isTurn && fest.phase === 'socializing'}
								<span class="muted"> · their turn</span>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}

			{#if fest.phase === 'socializing' && !iAmGuest && currentPlayerID != null && !amHost}
				{#if actionError}<p class="res-error">{actionError}</p>{/if}
				<button class="action-btn"
					onclick={onJoin}
					disabled={actionBusy}>
					{actionBusy ? '…' : 'Join as guest'}
				</button>
			{/if}
		</div>

		<!-- ── Pending challenge banner ───────────────────────────────────── -->
		{#if fest.pendingChallenge}
			<div class="choices-section">
				<p class="choices-header">
					Duel challenge:
					<strong>{playerName(players, fest.pendingChallenge.challenger_id)}</strong>
					→ <strong>{playerName(players, fest.pendingChallenge.target_id)}</strong>
				</p>
				{#if fest.pendingChallenge.notes}
					<p class="plan-notes">"{fest.pendingChallenge.notes}"</p>
				{/if}
				{#if challengeIsForMe}
					{#if respondError}<p class="res-error">{respondError}</p>{/if}
					<div style="display:flex;gap:0.5rem;">
						<button class="action-btn primary"
							onclick={() => onRespond(true)}
							disabled={respondBusy}>
							{respondBusy ? '…' : 'Accept challenge'}
						</button>
						<button class="action-btn"
							onclick={() => onRespond(false)}
							disabled={respondBusy || mustAccept}
							title={mustAccept ? 'You took accept_duels and cannot decline' : ''}>
							Decline
						</button>
					</div>
				{:else}
					<p class="choices-note muted">
						Awaiting the target's response. All festivity actions are paused.
					</p>
				{/if}
			</div>
		{/if}

		<!-- ── My turn (socializing) ─────────────────────────────────────── -->
		{#if fest.phase === 'socializing' && iAmGuest && !fest.pendingChallenge && myOutcome == null}
			<div class="choices-section">
				<p class="choices-header">Your turn</p>
				{#if currentTurnID !== currentPlayerID && currentTurnID != null}
					<p class="choices-note muted">
						{playerName(players, currentTurnID)} should go before you (lower esteem first).
						You may still act if they insist on going later.
					</p>
				{/if}

				{#if myRollID == null}
					<!-- Decide to roll or opt out -->
					{#if actionError}<p class="res-error">{actionError}</p>{/if}
					<div style="display:flex;gap:0.5rem;">
						<button class="action-btn primary" onclick={onRoll} disabled={actionBusy}>
							{actionBusy ? '…' : 'Roll'}
						</button>
						<button class="action-btn" onclick={onOptOut} disabled={actionBusy}>
							Opt out
						</button>
					</div>
				{:else if myEffectiveOutcome == null}
					<!-- Roll exists but unresolved — DiceRollPanel handles input. -->
					<p class="choices-note muted">Rolling… resolve the dice above.</p>
				{:else}
					<!-- Outcome known: pick a make/mar option -->
					{@const opts = myEffectiveOutcome === 'make' ? MAKE_OPTS : MAR_OPTS}
					<p class="choices-header">
						Result:
						<strong class="outcome-{myEffectiveOutcome}">
							{myEffectiveOutcome === 'make' ? '✓ Make' : '✗ Mar'}
						</strong>
						— pick one option:
					</p>
					<div class="choice-list">
						{#each opts as o}
							<label class="choice-item" style="display:flex;gap:0.5rem;align-items:flex-start;">
								<input type="radio" name="myc-{plan.id}"
									value={o.key}
									checked={pickedChoice === o.key}
									onchange={() => (pickedChoice = o.key)} />
								<span>{o.label}</span>
							</label>
						{/each}
					</div>

					<!-- Sub-forms by choice -->
					{#if pickedChoice === 'spread_rumor' || pickedChoice === 'rumor_about_you'}
						<label class="form-label">
							Rumor text:
							<textarea rows={2} bind:value={rumorText} class="form-textarea"
								placeholder="What does the rumor say?"></textarea>
						</label>
					{:else if pickedChoice === 'introduce_peer'}
						<label class="form-label">
							New peer's name:
							<input type="text" bind:value={peerName} class="form-textarea" style="height:auto;"
								placeholder="Name of the new peer" />
						</label>
					{:else if pickedChoice === 'take_center_peer'}
						{#if myCenterPeerCandidates.length === 0}
							<p class="choices-note muted">No peers in the center of the table.</p>
						{:else}
							<div class="choice-list">
								{#each myCenterPeerCandidates as a}
									<label class="choice-item" style="display:flex;gap:0.5rem;align-items:center;">
										<input type="radio" name="myasset-{plan.id}"
											value={a.id}
											checked={pickedAssetID === a.id}
											onchange={() => (pickedAssetID = a.id)} />
										<span>{a.name}</span>
									</label>
								{/each}
							</div>
						{/if}
					{:else if pickedChoice === 'disagreement'}
						{#if myOwnPeers.length === 0}
							<p class="choices-note muted">You have no peers to set in the center.</p>
						{:else}
							<div class="choice-list">
								{#each myOwnPeers as a}
									<label class="choice-item" style="display:flex;gap:0.5rem;align-items:center;">
										<input type="radio" name="myasset-{plan.id}"
											value={a.id}
											checked={pickedAssetID === a.id}
											onchange={() => (pickedAssetID = a.id)} />
										<span>{a.name}</span>
									</label>
								{/each}
							</div>
						{/if}
					{:else if pickedChoice === 'challenge_duel'}
						<label class="form-label">
							Challenge:
							<select bind:value={pickedDuelTargetID} class="form-textarea" style="height:auto;">
								<option value={null}>— pick a target —</option>
								{#each otherGuests as gid}
									<option value={gid}>
										{playerName(players, gid)}
										{fest.acceptDuels.includes(gid) ? ' (must accept)' : ''}
									</option>
								{/each}
							</select>
						</label>
					{/if}

					{#if pickerError}<p class="res-error">{pickerError}</p>{/if}
					<button class="action-btn primary"
						onclick={submitMyChoice}
						disabled={pickerBusy || !pickedChoice}>
						{pickerBusy ? '…' : 'Submit choice'}
					</button>
				{/if}
			</div>
		{/if}

		<!-- ── IOU: insist host take a mar ────────────────────────────────── -->
		{#if iHaveIOU && fest.phase !== 'done' && !fest.pendingChallenge}
			<div class="choices-section">
				<p class="choices-header">You hold an IOU</p>
				<p class="choices-note">
					As a guest who rolled make, you may force the host to take one mar option.
				</p>
				{#if !insistOpen}
					<button class="action-btn" onclick={() => (insistOpen = true)}>
						Insist on a mar
					</button>
				{:else}
					<div class="choice-list">
						{#each MAR_OPTS as o}
							<label class="choice-item" style="display:flex;gap:0.5rem;align-items:flex-start;">
								<input type="radio" name="insist-{plan.id}"
									value={o.key}
									checked={insistChoice === o.key}
									onchange={() => (insistChoice = o.key)} />
								<span>{o.label}</span>
							</label>
						{/each}
					</div>
					{#if insistChoice === 'rumor_about_you'}
						<label class="form-label">
							Rumor text (about the host):
							<textarea rows={2} bind:value={insistRumor} class="form-textarea"></textarea>
						</label>
					{:else if insistChoice === 'disagreement'}
						{@const hostPeers = assets.filter(a =>
							a.owner_id === plan.preparer_id && a.asset_type === 'peer' && !a.is_destroyed)}
						{#if hostPeers.length === 0}
							<p class="choices-note muted">The host has no peers to set in the center.</p>
						{:else}
							<div class="choice-list">
								{#each hostPeers as a}
									<label class="choice-item" style="display:flex;gap:0.5rem;align-items:center;">
										<input type="radio" name="insist-asset-{plan.id}"
											value={a.id}
											checked={insistAssetID === a.id}
											onchange={() => (insistAssetID = a.id)} />
										<span>{a.name}</span>
									</label>
								{/each}
							</div>
						{/if}
					{/if}
					{#if insistError}<p class="res-error">{insistError}</p>{/if}
					<div style="display:flex;gap:0.5rem;">
						<button class="action-btn primary"
							onclick={submitInsist}
							disabled={insistBusy || !insistChoice}>
							{insistBusy ? '…' : 'Insist'}
						</button>
						<button class="action-btn" onclick={() => { insistOpen = false; insistChoice = null; }}>
							Cancel
						</button>
					</div>
				{/if}
				{#if fest.hostMarInsists.length > 0}
					<p class="choices-note muted">
						Forced on host so far: {fest.hostMarInsists.join(', ')}
					</p>
				{/if}
			</div>
		{/if}

		<!-- ── Host choosing phase ────────────────────────────────────────── -->
		{#if fest.phase === 'host_choosing'}
			<div class="choices-section">
				<p class="choices-header">Host's make picks</p>
				{#if pendingHostGuests.length === 0}
					<p class="choices-note">All host choices have been made.</p>
				{:else if !amHost}
					<p class="choices-note muted">
						Waiting for the host to choose for:
						{pendingHostGuests.map(id => playerName(players, id)).join(', ')}
					</p>
				{:else}
					<p class="choices-note">
						Pick a make option for each guest who rolled mar or opted out.
					</p>
					<label class="form-label">
						Guest:
						<select bind:value={hostPickerGuestID} class="form-textarea" style="height:auto;">
							<option value={null}>— pick a guest —</option>
							{#each pendingHostGuests as gid}
								<option value={gid}>
									{playerName(players, gid)}
									({fest.outcomes[String(gid)]})
								</option>
							{/each}
						</select>
					</label>
					{#if hostPickerGuestID != null}
						<div class="choice-list">
							{#each HOST_MAKE_OPTS as o}
								<label class="choice-item" style="display:flex;gap:0.5rem;align-items:flex-start;">
									<input type="radio" name="hostc-{plan.id}"
										value={o.key}
										checked={hostPickedChoice === o.key}
										onchange={() => (hostPickedChoice = o.key)} />
									<span>{o.label}</span>
								</label>
							{/each}
						</div>
						{#if hostPickedChoice === 'spread_rumor'}
							<label class="form-label">
								Rumor text:
								<textarea rows={2} bind:value={hostRumor} class="form-textarea"></textarea>
							</label>
						{:else if hostPickedChoice === 'introduce_peer'}
							<label class="form-label">
								New peer's name:
								<input type="text" bind:value={hostPeerName}
									class="form-textarea" style="height:auto;"
									placeholder="Name of the new peer" />
							</label>
						{:else if hostPickedChoice === 'take_center_peer'}
							{#if myCenterPeerCandidates.length === 0}
								<p class="choices-note muted">No peers in the center.</p>
							{:else}
								<div class="choice-list">
									{#each myCenterPeerCandidates as a}
										<label class="choice-item" style="display:flex;gap:0.5rem;align-items:center;">
											<input type="radio" name="host-asset-{plan.id}"
												value={a.id}
												checked={hostAssetID === a.id}
												onchange={() => (hostAssetID = a.id)} />
											<span>{a.name}</span>
										</label>
									{/each}
								</div>
							{/if}
						{/if}
					{/if}
					{#if hostPickerError}<p class="res-error">{hostPickerError}</p>{/if}
					<button class="action-btn primary"
						onclick={submitHostChoice}
						disabled={hostPickerBusy || hostPickerGuestID == null || !hostPickedChoice}>
						{hostPickerBusy ? '…' : 'Submit pick'}
					</button>
				{/if}

				{#if Object.keys(fest.hostChoices).length > 0}
					<p class="choices-note muted" style="margin-top:0.5rem;">
						Done so far:
						{Object.entries(fest.hostChoices)
							.map(([pid, c]) => `${playerName(players, Number(pid))} → ${c}`)
							.join('; ')}
					</p>
				{/if}
			</div>
		{/if}

		<!-- ── Centered peers + rumor summary ─────────────────────────────── -->
		{#if fest.centeredAssetIDs.length > 0}
			<p class="choices-note muted">
				Center of the table:
				{fest.centeredAssetIDs.map(id => assetName(assets, id)).join(', ')}
			</p>
		{/if}

		<!-- ── Done ──────────────────────────────────────────────────────── -->
		{#if fest.phase === 'done'}
			<div class="complete-section">
				<p class="choices-applied">The festivity has wound down.</p>
				{#if completeError}<p class="res-error">{completeError}</p>{/if}
				{#if isFocusPlayer}
					<button class="action-btn primary" onclick={onComplete} disabled={completeBusy}>
						{completeBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>
		{/if}

	</ResolvingCard>
{/if}
