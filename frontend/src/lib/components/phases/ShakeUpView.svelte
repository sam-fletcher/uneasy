<!-- ShakeUpView.svelte
  Phase 4c — Shake-Up endgame UI.

  Two sub-modes driven by game.shake_up_step, both reverse-rank turn order
  (lowest status first):
    1 (rolling): each player rolls in turn through the real dice-roll stage
      machine (DiceRollPanel, in its shake-up mode) — the server creates the
      open roll for whoever is up next. After the last roller resolves, the
      server advances to step 2.
    2 (spending): players take turns in reverse rank order. The active
      spender announces an option (deducts 1 token immediately), other
      players post ±1 adjustments or explicitly pass (1 token each to
      adjust; passing is free), and the spender commits — once every other
      token-holding player has reacted — to lock in the final cost and
      trigger the mechanical effect.

  Looks (the redesign): nothing is framed. The acts, the token pools and each
  step are set apart by headings and gaps, and only what a player can press has
  an outline. The options and the titles are choices, so they are chips (the
  house chip, the one picked in bright gold) and the notes to tear are small
  game pieces; the pay, commit, abandon and react buttons are the house
  buttons. A raised cost is a notice (a mark and coloured text), not a tinted
  strip.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import '$lib/components/shared/marginaliaTile.css';
	import '../plans/planPanel.css';
	import { onMount, onDestroy } from 'svelte';
	import {
		getShakeUp, shakeUpSpend, shakeUpAdjust, shakeUpCommit, shakeUpPass,
	} from '$lib/api';
	import type {
		Game, Player, Asset, AssetType, Ranking, DiceRoll, DiceRollDie, RollParticipant,
		ShakeUpOptionInfo, ShakeUpSpend, ShakeUpAdjustmentRow, ShakeUpPassRow, ShakeUpTokensRow,
		ClaimableTitle,
	} from '$lib/api';
	import AssetCardSelectable from '../AssetCardSelectable.svelte';
	import CrownGlyph from '../CrownGlyph.svelte';
	import DiceRollPanel from '../DiceRollPanel.svelte';
	import CardPicker from '../plans/CardPicker.svelte';
	import FormField from '../plans/FormField.svelte';
	import Buffet, { type BuffetTab } from '../plans/shared/Buffet.svelte';
	import { isBlankAsset } from '$lib/assetRisk';
	import { playerColor, playerColorByID } from '$lib/playerColor';
	import { shakeUpWaitingOn, type WaitingOnState } from '$lib/waitingOn';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import Chip from '$lib/components/shared/Chip.svelte';
	import ChecklistRow from '$lib/components/shared/ChecklistRow.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import TableRoster from '$lib/components/shared/TableRoster.svelte';
	import ToneMark from '$lib/components/shared/ToneMark.svelte';

	interface Props {
		gameID: string;
		game: Game;
		players: Player[];
		assets: Asset[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		activeRoll: DiceRoll | null;
		activeRollDice: DiceRollDie[];
		activeRollParticipants: RollParticipant[];
		waitingOn: WaitingOnState;
	}
	let {
		gameID, game, players, assets, rankings, currentPlayerID,
		activeRoll = $bindable(), activeRollDice = $bindable(), activeRollParticipants = $bindable(),
		waitingOn = $bindable(),
	}: Props = $props();

	let tokens = $state<ShakeUpTokensRow[]>([]);
	let options = $state<ShakeUpOptionInfo[]>([]);
	let claimableTitles = $state<ClaimableTitle[]>([]);
	let openSpend = $state<{
		spend: ShakeUpSpend;
		adjustments: ShakeUpAdjustmentRow[];
		passes: ShakeUpPassRow[];
		pending_reactor_ids: number[];
		commit_ready: boolean;
	} | null>(null);
	let currentActor = $state<number | null>(null);
	let currentRollerID = $state<number | null>(null);
	// Split by lifetime (adr/ERROR_HANDLING_PLAN.md): loadError explains stale
	// shake-up state and is owned by refresh(), which runs on every WS event;
	// actionError explains one control the player just used. Sharing one
	// variable meant a refresh could erase a message the player hadn't read.
	let loadError = $state('');
	let actionError = $state('');
	let busy = $state(false);

	// Local, ephemeral: the reason shown when a tap on the (aria-disabled, not
	// dead) reduce-cost button is blocked by the floor guard — Make Demands
	// eligibility pattern (PlanPanel's ineligibleNotice). Cleared on refresh
	// so a stale reason doesn't linger onto a different spend.
	let reduceBlockedReason = $state('');

	async function refresh() {
		try {
			const data = await getShakeUp(gameID);
			tokens = data.tokens;
			options = data.options ?? [];
			claimableTitles = data.claimable_titles ?? [];
			openSpend = data.open_spend ?? null;
			currentActor = data.current_actor ?? null;
			currentRollerID = data.current_roller_id ?? null;
			reduceBlockedReason = '';
			// Cleared on success, not on entry, so a failed refresh leaves the
			// last message up rather than blanking it mid-flight.
			loadError = '';
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load shake-up state.';
		}
	}

	onMount(refresh);

	function onShakeUpEvent() { refresh(); }
	onMount(() => {
		for (const t of [
			'shake_up.step_changed', 'shake_up.rolled', 'shake_up.spend_opened',
			'shake_up.adjusted', 'shake_up.spend_committed', 'shake_up.spend_abandoned',
			'shake_up.passed', 'shake_up.ended',
		]) window.addEventListener(`uneasy:${t}`, onShakeUpEvent);
	});
	onDestroy(() => {
		for (const t of [
			'shake_up.step_changed', 'shake_up.rolled', 'shake_up.spend_opened',
			'shake_up.adjusted', 'shake_up.spend_committed', 'shake_up.spend_abandoned',
			'shake_up.passed', 'shake_up.ended',
		]) window.removeEventListener(`uneasy:${t}`, onShakeUpEvent);
	});

	const myTokens = $derived(
		tokens.find(t => t.id === currentPlayerID)?.shake_up_tokens ?? 0
	);
	// Whose turn it is to announce a spend (reverse-rank order, server-driven).
	const isMyTurn = $derived(currentActor != null && currentActor === currentPlayerID);
	const playerNameMap = $derived(new Map(players.map(p => [p.id, p.display_name])));
	function playerName(id: number | null): string {
		if (id == null) return '?';
		return players.find(p => p.id === id)?.display_name ?? '?';
	}

	// ── Act tracker ──────────────────────────────────────────────────────────
	const CATEGORIES = ['esteem', 'knowledge', 'power'] as const;
	const CATEGORY_LABELS: Record<(typeof CATEGORIES)[number], string> = {
		esteem: 'Esteem', knowledge: 'Knowledge', power: 'Power',
	};
	const currentCategoryIndex = $derived(
		CATEGORIES.indexOf((game.shake_up_category ?? 'esteem') as (typeof CATEGORIES)[number])
	);
	const stepLabel = $derived(
		game.shake_up_step === 1 ? 'Rolling' : game.shake_up_step === 2 ? 'Spending' : '—'
	);

	// Phase intro: open by default only in the first act. ChecklistRow's
	// `defaultOpen` is a one-shot seed (like Buffet's `open` prop /
	// AssetCardSelectable's `defaultExpanded`) — once the player toggles it,
	// their choice sticks even as the category advances.

	// Read-only reference of every option across all three acts (Buffet).
	// Wording follows SHAKEUP_RULES.md's "List of Options" verbatim.
	const BUFFET_ALWAYS =
		'Each option costs 1 token. Others may spend their own tokens to raise or lower the cost by 1 per token. Turn order loops until everyone is out.';
	const buffetTabs: BuffetTab[] = [
		{
			key: 'esteem', label: 'Esteem', always: BUFFET_ALWAYS,
			opts: [
				{ key: 'take_peer', label: 'Take a peer asset' },
				{ key: 'take_artifact', label: 'Take an artifact asset' },
				{ key: 'break_resource', label: 'Break a resource asset' },
				{ key: 'bump_knowledge', label: 'Bump up on Knowledge' },
			],
		},
		{
			key: 'knowledge', label: 'Knowledge', always: BUFFET_ALWAYS,
			opts: [
				{ key: 'take_resource', label: 'Take a resource asset' },
				{ key: 'break_holding', label: 'Break a holding asset' },
				{ key: 'break_peer', label: 'Break a peer asset' },
				{ key: 'bump_power', label: 'Bump up on Power' },
			],
		},
		{
			key: 'power', label: 'Power', always: BUFFET_ALWAYS,
			opts: [
				{ key: 'take_holding', label: 'Take a holding asset' },
				{ key: 'break_artifact', label: 'Break an artifact asset' },
				{ key: 'claim_title', label: 'Claim a new title' },
				{ key: 'bump_esteem', label: 'Bump up on Esteem' },
			],
		},
	];

	// ── Turn order & token pools (shared by both steps) ─────────────────────
	// Reverse rank (lowest status first), mirroring gamepkg.ShakeUpTurnOrder.
	// Dummy slots (player_id null) don't take turns, so they're skipped.
	const turnOrder = $derived.by<number[]>(() => {
		const cat = game.shake_up_category;
		if (!cat) return [];
		return rankings
			.filter(r => r.category === cat && r.player_id !== null)
			.sort((a, b) => b.rank - a.rank)
			.map(r => r.player_id as number);
	});
	// The token pools list, in turn order, as the Players the shared roster draws.
	const turnPlayers = $derived(
		turnOrder.flatMap((id) => players.find((p) => p.id === id) ?? [])
	);
	function tokensOf(id: number): number {
		return tokens.find((t) => t.id === id)?.shake_up_tokens ?? 0;
	}

	// ── Step 2: announce spend ──────────────────────────────────────────────
	let pickedOption = $state<string>('');
	let pickedAssetID = $state<number | ''>('');
	// Break options tear one marginalia, so the breaker also picks which.
	let pickedMarginaliaID = $state<number | ''>('');
	// Claim-a-title picks a title id + the peer (one of mine, with a free slot)
	// that receives it + optional freeform marginalia flavor.
	let pickedTitleID = $state<string>('');
	let titleFlavor = $state<string>('');
	const pickedOptionInfo = $derived(options.find(o => o.Key === pickedOption));
	const isClaimTitle = $derived(pickedOptionInfo?.Key === 'claim_title');

	// Take/break asset-type filters, keyed off the option key (matches the
	// backend's expectedTakeType/expectedBreakType in handler/shake_up.go).
	const TAKE_ASSET_TYPE: Record<string, AssetType> = {
		take_peer: 'peer', take_artifact: 'artifact', take_resource: 'resource', take_holding: 'holding',
	};
	const BREAK_ASSET_TYPE: Record<string, AssetType> = {
		break_resource: 'resource', break_holding: 'holding', break_peer: 'peer', break_artifact: 'artifact',
	};
	const isTakeOption = $derived(pickedOption in TAKE_ASSET_TYPE);
	const targetAssetType = $derived<AssetType | null>(
		TAKE_ASSET_TYPE[pickedOption] ?? BREAK_ASSET_TYPE[pickedOption] ?? null
	);
	// Take options exclude the spender's own assets (ruling 8 — a no-op);
	// break options may target any owner's asset, including the spender's own.
	const targetableAssets = $derived(
		targetAssetType == null
			? []
			: assets.filter(a =>
				a.asset_type === targetAssetType &&
				!a.is_destroyed &&
				(!isTakeOption || a.owner_id !== currentPlayerID))
	);
	function selectTargetAsset(id: number | null) {
		pickedAssetID = id ?? '';
		pickedMarginaliaID = '';
	}

	// My own peers with a free marginalia slot (positions 1–4; torn marginalia
	// still occupy their slot) — the only valid recipients of a new title.
	const titleablePeers = $derived(
		assets.filter(a => a.owner_id === currentPlayerID && a.asset_type === 'peer'
			&& !a.is_destroyed && a.marginalia.length < 4)
	);
	// Intact (un-torn) marginalia on the chosen asset — the breakable choices.
	const pickedAsset = $derived(
		pickedAssetID === '' ? null : assets.find(a => a.id === pickedAssetID) ?? null
	);
	const breakableMarginalia = $derived(
		(pickedAsset?.marginalia ?? []).filter(m => !m.is_torn)
	);
	// A blank asset names no marginalia because it has none: the break destroys
	// it outright (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md, D3), and the announce
	// omits target_marginalia_id. Since the last tear always destroys, a live
	// asset with no intact marginalia is necessarily blank.
	const pickedIsBlank = $derived(pickedAsset != null && isBlankAsset(pickedAsset));
	// A break announce is only ready once a marginalia is chosen (or the target
	// is blank); a claim-title announce needs both a title and one of my peers to
	// bear it.
	const announceReady = $derived(
		!!pickedOption &&
		(!pickedOptionInfo?.NeedsAsset || pickedAssetID !== '') &&
		(!pickedOptionInfo?.NeedsMarginalia || pickedMarginaliaID !== '' || pickedIsBlank) &&
		(!isClaimTitle || (pickedTitleID !== '' && pickedAssetID !== ''))
	);

	async function announce() {
		if (!announceReady || busy) return;
		busy = true; actionError = '';
		try {
			const body: {
				option_key: string;
				target_asset_id?: number;
				target_marginalia_id?: number;
				target_title_id?: string;
				title_flavor?: string;
			} = { option_key: pickedOption };
			if (pickedOptionInfo?.NeedsAsset && pickedAssetID !== '') {
				body.target_asset_id = pickedAssetID as number;
			}
			if (pickedOptionInfo?.NeedsMarginalia && pickedMarginaliaID !== '') {
				body.target_marginalia_id = pickedMarginaliaID as number;
			}
			if (isClaimTitle) {
				body.target_title_id = pickedTitleID;
				body.target_asset_id = pickedAssetID as number;
				if (titleFlavor.trim()) body.title_flavor = titleFlavor.trim();
			}
			await shakeUpSpend(gameID, body);
			pickedOption = '';
			pickedAssetID = '';
			pickedMarginaliaID = '';
			pickedTitleID = '';
			titleFlavor = '';
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not announce spend.';
		} finally {
			busy = false;
		}
	}

	async function adjust(direction: 1 | -1) {
		if (!openSpend || busy) return;
		busy = true; actionError = ''; reduceBlockedReason = '';
		try {
			await shakeUpAdjust(gameID, openSpend.spend.id, direction);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not adjust.';
		} finally {
			busy = false;
		}
	}

	// The reduce button stays clickable (aria-disabled, not native-disabled)
	// even at the cost floor, so a tap always surfaces why — mirrors the Make
	// Demands ineligible-card pattern rather than silently swallowing the tap.
	function onReduceClick() {
		if (atCostFloor) {
			reduceBlockedReason = "The cost can't go below 1 token.";
			return;
		}
		adjust(-1);
	}

	async function pass() {
		if (!openSpend || busy) return;
		busy = true; actionError = '';
		try {
			await shakeUpPass(gameID, openSpend.spend.id);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not pass.';
		} finally {
			busy = false;
		}
	}

	// ADR-008 "pay or abandon": intent is required once the cost was raised
	// (extra > 0); the plain no-intent form is only valid when extra === 0,
	// where the spend auto-commits regardless of what's passed.
	async function commit(intent?: 'pay' | 'abandon') {
		if (!openSpend || busy) return;
		busy = true; actionError = '';
		try {
			await shakeUpCommit(gameID, openSpend.spend.id, intent);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not commit.';
		} finally {
			busy = false;
		}
	}

	const isMySpend = $derived(openSpend?.spend.player_id === currentPlayerID);
	const adjustmentTotal = $derived(
		(openSpend?.adjustments ?? []).reduce((sum, a) => sum + a.adjustment, 0)
	);
	// The floor guard (ADR-008 §1) keeps this at ≥ 1 server-side; Math.max
	// here just guards the brief window before a stale client catches up.
	const runningCost = $derived(openSpend ? Math.max(openSpend.spend.base_cost + adjustmentTotal, 1) : 0);
	const atCostFloor = $derived(runningCost <= 1);
	// extra = final cost owed beyond the base — always ≥ 0 given the floor
	// guard, since base_cost is always 1.
	const extra = $derived(openSpend ? Math.max(runningCost - openSpend.spend.base_cost, 0) : 0);
	const affordable = $derived(myTokens >= extra);
	const pendingReactorIDs = $derived(openSpend?.pending_reactor_ids ?? []);
	const commitReady = $derived(openSpend?.commit_ready ?? true);
	const pendingReactorNames = $derived(pendingReactorIDs.map(playerName));
	const myPassed = $derived(
		currentPlayerID != null && (openSpend?.passes ?? []).some(p => p.player_id === currentPlayerID)
	);
	const spendOptionInfo = $derived.by(() => {
		const spend = openSpend;
		if (!spend) return undefined;
		return options.find(o => o.Key === spend.spend.option_key);
	});
	// The option phrase without its trailing period, echoing the backend's
	// shakeUpOptionPhrase (handler/system_posts.go) used for the chat log.
	const spendPhrase = $derived(
		(spendOptionInfo?.Description ?? openSpend?.spend.option_key ?? '').replace(/\.$/, '')
	);
	const spendTargetAsset = $derived.by(() => {
		const spend = openSpend;
		if (!spend || spend.spend.target_asset_id == null) return undefined;
		return assets.find(a => a.id === spend.spend.target_asset_id);
	});

	// ── Waiting-on derivation ────────────────────────────────────────────────
	// Both steps are strictly sequential turn order, so the pure function
	// (lib/waitingOn.ts) always names exactly one party (or, mid-reaction-
	// window, the set of pending reactors) — see its vitest coverage.
	const computedWaitingOn = $derived(shakeUpWaitingOn({
		step: game.shake_up_step,
		currentRollerID,
		openSpend: openSpend ? {
			spend: { player_id: openSpend.spend.player_id },
			pendingReactorIDs: openSpend.pending_reactor_ids,
			commitReady: openSpend.commit_ready,
		} : null,
		currentActor,
	}));
	$effect(() => { waitingOn = computedWaitingOn; });
	// The clock in the token pools marks whoever the game is waiting on, from the
	// same derivation as the player row's clocks, so the two can't disagree.
	const waitingPlayerIDs = $derived(
		new Set(computedWaitingOn.waitees.flatMap((w) => (w.kind === 'player' ? [w.playerID] : [])))
	);
</script>

<div class="shake-up">
	<header>
		<h2>Shake-Up</h2>
		<!-- The three acts as a track, not three boxes: the one in play is deep gold
		     with a gold line under it and names its step; the finished ones go
		     faint. Nothing here is pressable, so nothing is outlined. -->
		<ol class="act-tracker" role="list" aria-label="Shake-Up acts">
			{#each CATEGORIES as cat (cat)}
				<li
					class="act"
					class:current={cat === game.shake_up_category}
					class:done={CATEGORIES.indexOf(cat) < currentCategoryIndex}
					aria-current={cat === game.shake_up_category ? 'step' : undefined}
				>
					<span class="act-name">{CATEGORY_LABELS[cat]}</span>
					{#if cat === game.shake_up_category}
						<span class="status-word">{stepLabel}</span>
					{/if}
				</li>
			{/each}
		</ol>
		<p class="state">Your tokens: <span class="num">{myTokens}</span></p>
	</header>

	<ChecklistRow
		title="About the Shake-Up"
		glyph="help"
		action="expand"
		id="shake-up-intro"
		defaultOpen={game.shake_up_category === 'esteem'}
	>
		<p class="intro-text">
			This is the finale. Over three acts — Esteem, then Knowledge, then
			Power — each player rolls to earn tokens, then spends them in turn
			to take, break, rise, and claim. Take stock of everything that's
			happened over the last thirteen rows of the public record, and
			embrace this last moment: once Power is settled, the game ends.
		</p>
	</ChecklistRow>

	<!-- Load errors sit up here with the header: they explain the state below
	     being stale. The action slot is further down, under the spend
	     controls. -->
	{#if loadError}<ErrorText message={loadError} />{/if}

	<Buffet tabs={buffetTabs} defaultTab={game.shake_up_category ?? 'esteem'} />

	<section class="tokens-panel">
		<h3>Token pools</h3>
		<!-- Turn order is reverse rank — lowest status first. The clock is on whoever
		     the game is waiting on (bright gold on you). -->
		<TableRoster players={turnPlayers} {currentPlayerID} {waitingPlayerIDs}>
			{#snippet trailing(p)}
				<span class="pool-count">{tokensOf(p.id)}<span class="sr-only"> tokens</span></span>
			{/snippet}
		</TableRoster>
	</section>

	{#if game.shake_up_step === 1}
		<section class="step rolling-section">
			<h3>Rolling for {game.shake_up_category ?? 'this category'}</h3>
			<p class="muted-text">Each distinct die face earns one token.</p>
			{#if activeRoll}
				<DiceRollPanel
					bind:roll={activeRoll}
					bind:dice={activeRollDice}
					votes={[]}
					bind:participants={activeRollParticipants}
					bankedDice={[]}
					{assets}
					{currentPlayerID}
					{players}
					{playerNameMap}
				/>
			{:else}
				<p class="muted-text">Waiting for the next roll to start…</p>
			{/if}
		</section>

	{:else if game.shake_up_step === 2}
		{#if openSpend}
			<section class="step spend-panel">
				<h3>Open spend</h3>
				<p class="spend-phrase">
					<span class="spend-player" style:color={playerColorByID(openSpend.spend.player_id, players)}>
						{playerName(openSpend.spend.player_id)}
					</span>
					announced: {spendPhrase}
				</p>
				{#if spendTargetAsset}
					<AssetCardSelectable
						asset={spendTargetAsset}
						ownerColor={playerColor(players.find(pl => pl.id === spendTargetAsset.owner_id))}
						ownerLabel={spendTargetAsset.owner_id === currentPlayerID
							? 'Your own'
							: `Owned by ${playerName(spendTargetAsset.owner_id)}`}
					/>
				{/if}
				<p class="muted-text">
					Base cost {openSpend.spend.base_cost} · running cost: <span class="num">{runningCost}</span>
					{#if adjustmentTotal !== 0}
						({adjustmentTotal > 0 ? '+' : ''}{adjustmentTotal} so far)
					{/if}
				</p>
				{#if openSpend.adjustments.length > 0}
					<ul class="adjustment-history">
						{#each openSpend.adjustments as a (a.id)}
							<li>
								<span style:color={playerColorByID(a.player_id, players)}>{playerName(a.player_id)}</span>
								{a.adjustment > 0 ? 'raised' : 'lowered'} the cost by 1
							</li>
						{/each}
					</ul>
				{/if}

				{#if isMySpend}
					{#if !commitReady}
						<p class="muted-text">
							Others can still react — waiting on {pendingReactorNames.join(', ')} to react.
						</p>
					{:else if extra === 0}
						<div class="adjust-buttons">
							<button class="action-btn primary" disabled={busy} onclick={() => commit()}>
								{busy ? '…' : 'Commit'}
							</button>
						</div>
					{:else if affordable}
						<!-- ADR-008: the cost was raised and the spender can afford it —
						     Pay or Abandon is their real choice, not a forced completion.
						     A cost to weigh, so the careful (warning) tone. -->
						<Notice tone="warning">
							The cost was raised to {runningCost} ({extra} more than you committed to).
							Pay the extra, or abandon — either way the tokens already spent are gone.
						</Notice>
						<div class="adjust-buttons">
							<button class="action-btn primary" disabled={busy} onclick={() => commit('pay')}>
								{busy ? '…' : `Pay ${extra} more token${extra === 1 ? '' : 's'}`}
							</button>
							<button class="action-btn secondary" disabled={busy} onclick={() => commit('abandon')}>
								{busy ? '…' : 'Abandon'}
							</button>
						</div>
					{:else}
						<!-- Forced abandon: the raise exceeds what's left in the spender's
						     pool, so Pay isn't offered — only Abandon. The purchase is lost
						     with no say in it, so the loss (danger) tone. -->
						<Notice tone="danger">
							The cost was raised to {runningCost}, {extra} more than you committed to — you only
							have {myTokens} token{myTokens === 1 ? '' : 's'} left, so you can't afford it. The
							spend must be abandoned; the tokens already spent are gone either way.
						</Notice>
						<div class="adjust-buttons">
							<button class="action-btn primary" disabled={busy} onclick={() => commit('abandon')}>
								{busy ? '…' : 'Abandon'}
							</button>
						</div>
					{/if}
				{:else if myTokens > 0}
					<!-- Three ways to react. Each one acts the moment it is pressed, so
					     they are buttons, not a choice that waits for a second tap, and
					     none is the main one. The cost-floor button greys out through
					     aria-disabled (and stays pressable, to say why). -->
					<div class="adjust-buttons">
						<button class="action-btn secondary" disabled={busy} onclick={() => adjust(1)}>+1 (costs you 1 token)</button>
						<button
							class="action-btn secondary"
							disabled={busy}
							aria-disabled={atCostFloor}
							title={atCostFloor ? "The cost can't go below 1 token" : undefined}
							onclick={onReduceClick}
						>−1 (costs you 1 token)</button>
						<button class="action-btn secondary" disabled={busy || myPassed} onclick={pass}>
							{myPassed ? 'You let it stand' : 'Let it stand'}
						</button>
					</div>
					{#if reduceBlockedReason}
						<Notice tone="warning" live>{reduceBlockedReason}</Notice>
					{/if}
				{:else}
					<p class="muted-text">You have no tokens left to react with.</p>
				{/if}
			</section>
		{:else if isMyTurn}
			<section class="step">
				<h3>Announce a spend</h3>
				<div class="announce-form">
					<FormField label="Option">
						<div class="su-chip-row">
							{#each options as opt}
								<Chip
									selected={pickedOption === opt.Key}
									onclick={() => {
										pickedOption = pickedOption === opt.Key ? '' : opt.Key;
										pickedAssetID = '';
										pickedMarginaliaID = '';
										pickedTitleID = '';
										titleFlavor = '';
									}}
								>{opt.Description}</Chip>
							{/each}
						</div>
					</FormField>
					{#if pickedOptionInfo?.NeedsAsset}
						<CardPicker
							label="Target asset"
							items={targetableAssets}
							{players}
							ownerLabel={(a) => a.owner_id === currentPlayerID
								? 'Your own'
								: `Owned by ${playerName(a.owner_id)}`}
							selected={pickedAssetID === '' ? null : pickedAssetID}
							onSelect={selectTargetAsset}
						/>
					{/if}
					{#if pickedOptionInfo?.NeedsMarginalia && pickedAssetID !== ''}
						<FormField label="Marginalia to tear (breaking tears one)">
							{#if pickedIsBlank}
								<Notice tone="warning">
									This asset has no marginalia — breaking it destroys it outright.
								</Notice>
							{:else if breakableMarginalia.length === 0}
								<p class="muted-text">This asset has no intact marginalia to tear.</p>
							{:else}
								<!-- A note is a small game piece, so picking one is the piece's
								     selected state: the bright gold border and a tick. -->
								<div class="su-notes">
									{#each breakableMarginalia as m (m.id)}
										{@const picked = pickedMarginaliaID === m.id}
										<button
											type="button"
											class="m-tile su-note"
											class:picked
											aria-pressed={picked}
											onclick={() => (pickedMarginaliaID = picked ? '' : m.id)}
										>
											<span class="su-note-text">{m.text}</span>
											{#if picked}<span class="su-tick"><ToneMark tone="success" size={14} /></span>{/if}
										</button>
									{/each}
								</div>
							{/if}
						</FormField>
					{/if}
					{#if isClaimTitle}
						<FormField label="Title to claim (unclaimed this game)">
							{#if claimableTitles.length === 0}
								<p class="muted-text">Every title has already been claimed.</p>
							{:else}
								<div class="su-chip-row">
									{#each claimableTitles as t (t.id)}
										<Chip
											selected={pickedTitleID === t.id}
											title={t.description}
											onclick={() => (pickedTitleID = pickedTitleID === t.id ? '' : t.id)}
										>{t.name}{#if t.in_succession}<CrownGlyph mark={{ role: 'successor' }} size={14} />{/if}</Chip>
									{/each}
								</div>
							{/if}
						</FormField>
						{#if pickedTitleID !== ''}
							<FormField label="Peer to bear the title (must have a free marginalia slot)">
								{#if titleablePeers.length === 0}
									<p class="muted-text">You have no peer with a free marginalia slot.</p>
								{:else}
									<div class="su-peer-cards">
										{#each titleablePeers as a (a.id)}
											<AssetCardSelectable
												asset={a}
												ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
												selectable
												selected={pickedAssetID === a.id}
												onToggle={() => (pickedAssetID = pickedAssetID === a.id ? '' : a.id)}
											/>
										{/each}
									</div>
								{/if}
							</FormField>
							<label class="form-label">
								<span class="form-label-text">Marginalia text (optional — defaults to the title name):</span>
								<input
									type="text"
									maxlength={TEXT_LIMITS.NAME}
									placeholder="e.g. crowned at the Midwinter Accord"
									bind:value={titleFlavor}
								/>
							</label>
						{/if}
					{/if}
					<button class="action-btn primary" disabled={!announceReady || busy} onclick={announce}>
						{busy ? '…' : 'Announce (cost 1 token)'}
					</button>
				</div>
			</section>
		{:else if myTokens > 0}
			<p class="muted-text">
				It's not your turn yet — waiting on {playerName(currentActor)} to spend.
				Players take turns in reverse-rank order (lowest status first).
			</p>
		{:else}
			<p class="muted-text">You have no tokens. The category advances when everyone is at zero.</p>
		{/if}

		<!-- Below whichever spend branch is showing, so announce / adjust /
		     pass / commit all report next to the control that failed. -->
		{#if actionError}<ErrorText message={actionError} />{/if}
	{:else}
		<p class="muted-text">Shake-Up state unavailable.</p>
	{/if}
</div>

<style>
	/* A column of sections with no frames: a heading, then what it holds, with
	   24px between sections and 8–12px inside one. The side padding is the
	   view's own, since the play column gives its content none. */
	.shake-up {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		padding: var(--space-4) var(--space-3);
		flex: 1;
		min-height: 0;
		overflow-y: auto;
	}
	.shake-up h2,
	.shake-up h3 {
		margin: 0;
		color: var(--color-accent);
		font-size: var(--text-heading);
	}
	header {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.state {
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	/* A value read off the page, in the main colour so it stands out from the
	   secondary words around it, at regular weight like every other value. */
	.num {
		color: var(--color-text);
		font-variant-numeric: tabular-nums;
	}

	/* Act tracker: Esteem, Knowledge, Power as three cells on a track. The one in
	   play is deep gold with a gold line under it and its step named beneath
	   (the words are the signal that doesn't depend on colour). */
	.act-tracker {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: var(--space-2);
	}
	.act {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		padding-bottom: var(--space-2);
		border-bottom: 2px solid var(--color-border);
		color: var(--color-text-secondary);
		font-size: var(--text-small);
	}
	.act.done { color: var(--color-text-faint); }
	.act.current {
		border-bottom-color: var(--color-accent);
		color: var(--color-accent);
	}
	.act.current .status-word { color: inherit; }

	.intro-text {
		margin: 0;
		font-size: var(--text-small);
		line-height: 1.5;
		color: var(--color-text-secondary);
	}

	/* A step: its heading, then what it holds, a little closer together than the
	   steps are to each other. */
	.step {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.tokens-panel {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.pool-count {
		font-size: var(--text-small);
		font-variant-numeric: tabular-nums;
	}
	/* Words for a screen reader that the number says to the eye. Same recipe as
	   TableRoster's .sr-state. */
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	/* ── Announce a spend ─────────────────────────────────────────────────── */
	.announce-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}
	.su-chip-row { display: flex; flex-wrap: wrap; gap: var(--space-2); }
	.su-peer-cards { display: flex; flex-direction: column; gap: var(--space-2); }

	/* A marginalia to tear is a small game piece you pick, so it takes a piece's
	   selected state: a 2px bright gold border (the second pixel an inset shadow,
	   so picking never moves it) and a tick. The tile recipe supplies the warm
	   border, the corner and the surface. */
	.su-notes { display: flex; flex-direction: column; gap: var(--space-2); }
	.su-note {
		width: 100%;
		min-height: var(--size-tap);
		padding: var(--space-2) var(--space-3);
		justify-content: space-between;
		gap: var(--space-2);
		font: inherit;
		font-size: var(--text-small);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
	}
	.su-note:hover:not(.picked) {
		border-color: color-mix(in srgb, var(--color-border-piece) 75%, white);
	}
	.su-note.picked {
		border-color: var(--color-you);
		box-shadow: inset 0 0 0 1px var(--color-you);
	}
	.su-note:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	.su-note-text { min-width: 0; }
	.su-tick {
		display: inline-flex;
		flex: none;
		color: var(--color-you);
	}

	/* A text field is a control: raised surface, neutral border, the control
	   corner, 16px type so iOS doesn't zoom the page when it takes focus. */
	input {
		min-height: var(--size-tap);
		padding: 0 var(--space-3);
		font: inherit;
		font-size: var(--text-body);
		color: var(--color-text);
		background: var(--color-surface);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
	}
	input:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	input::placeholder { color: var(--color-text-faint); }

	/* ── Open spend ───────────────────────────────────────────────────────── */
	.spend-phrase { margin: 0; font-size: var(--text-body); }
	.adjustment-history {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	.adjust-buttons { display: flex; gap: var(--space-2); flex-wrap: wrap; }
</style>
