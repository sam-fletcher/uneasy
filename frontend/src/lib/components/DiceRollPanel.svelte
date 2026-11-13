<!-- DiceRollPanel.svelte
  The dice roll, as one of a plan's steps. The server owns the stages
  (decide_vote → voting → leverage → resolved); clients call action endpoints
  and observe stage_changed / ready_changed / intent_set / vote_resolved /
  resolved over WS.

  OPEN, it is the open step: the heading (the current ask), one sentence on
  what is being rolled, the dice split into "For" and "Against", then the
  controls for the stage. No box and no wash: the heading and the space around
  it make it a step.
  RESOLVED, it is a finished step: one line (the result) that opens in place
  to show the dice. It sits in its place among the plan's steps, above the
  step that acts on the result.

  MainEventView draws it either inside the plan's card (so it lands between the
  finished steps and the open one) or alone when no plan is on screen (a
  scene's roll). Shake-Up draws it too, with no heading.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import type {
		DiceRoll, DiceRollDie, VoteView, RollParticipant, RollIntent,
		Asset, Player, BankedDie,
	} from '$lib/api';
	import {
		leverageRoll, useBankedDie, callVote, skipVote, voteOnRoll,
		setRollIntent, setRollReady,
	} from '$lib/api';
	import { untrack } from 'svelte';
	import type { StepHeading as StepHeadingData } from '$lib/stepHeading';
	import { useHeadingSlot } from './plans/shared/headingSlot';
	import { rollDraft } from '$lib/rollDraft.svelte';
	import AssetCardSelectable from './AssetCardSelectable.svelte';
	import { playerColorByID } from '$lib/playerColor';
	import { useSecretCounts } from '$lib/secretCountsContext';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import Chip from '$lib/components/shared/Chip.svelte';
	import DieMark from '$lib/components/shared/DieMark.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import StepHeading from '$lib/components/shared/StepHeading.svelte';
	import DieTile from './plans/shared/DieTile.svelte';
	import FinishedStep from './plans/shared/FinishedStep.svelte';

	interface Props {
		roll: DiceRoll;
		dice: DiceRollDie[];
		votes: VoteView[];
		participants: RollParticipant[];
		bankedDice: BankedDie[];
		assets: Asset[];
		currentPlayerID: number | null;
		players: Player[];
		playerNameMap: Map<number, string>;
		/** True when the actor cannot leverage their own assets (Make Demands
		 *  control_leverage winner has authority). */
		actorLeverageBlocked?: boolean;
		/** The open step's heading ("Your move — aid or interfere"). Null where
		 *  the surrounding screen already says it (Shake-Up). */
		heading?: StepHeadingData | null;
	}

	let {
		roll = $bindable(),
		dice = $bindable(),
		votes = $bindable(),
		participants = $bindable(),
		bankedDice = $bindable(),
		assets,
		currentPlayerID,
		players,
		playerNameMap,
		actorLeverageBlocked = false,
		heading = null,
	}: Props = $props();

	// Per-viewer known-secret counts (undefined outside a provider → no eyes).
	const secretCounts = useSecretCounts();

	// An open roll draws the heading at the top of its own step, so it claims the
	// heading from the page for as long as it does (a resolved roll is a finished
	// line and draws none). `drawsHeading` is a boolean, so a new heading object
	// each time the wait changes does not let go and re-claim.
	const headingSlot = useHeadingSlot();
	const drawsHeading = $derived(roll.stage !== 'resolved' && heading != null);
	$effect(() => {
		if (!drawsHeading) return;
		return untrack(() => headingSlot?.claim());
	});

	const isActor = $derived(currentPlayerID === roll.actor_id);
	const stage = $derived(roll.stage);
	const me = $derived(participants.find(p => p.player_id === currentPlayerID) ?? null);
	const myReady = $derived(me?.is_ready ?? false);

	// A non-actor's intent is locked once they've committed any die for
	// this roll. Non-actors never have automatic base dice, so every die
	// belonging to them is a committed asset or banked-die spend.
	// (Intent is irrelevant for the actor — they're implicitly aiding —
	// so this check doesn't need to special-case the actor's base dice.)
	const intentLocked = $derived(
		dice.some(d => d.player_id === currentPlayerID)
	);

	const myAssets = $derived(
		assets.filter(a => a.owner_id === currentPlayerID && !a.is_destroyed)
	);
	const myUnleveragedAssets = $derived(myAssets.filter(a => !a.is_leveraged));
	const myUnspentBanked = $derived(bankedDice.filter(b => b.used_at == null));
	const canCommit = $derived(myUnleveragedAssets.length > 0 || myUnspentBanked.length > 0);
	const myColor = $derived(playerColorByID(currentPlayerID, players));

	// ── Draft state (Option A: nothing commits until Ready) ───────────────────
	// Intent and dice picks live client-side until the player presses Ready,
	// which reveals + commits them all at once. Until then nothing is broadcast,
	// so there's no information for opponents to react to.
	//
	// They live in a store keyed by the roll (lib/rollDraft), not in this
	// component: the roll is drawn inside the plan's card or alone on the page and
	// can move from the one to the other (a plan panel that was still loading when
	// the roll opened), and a component's own state is lost when it does.
	// A genuinely new roll starts a fresh draft; the frequent WS roll
	// reassignments during a leverage stage keep it, since it is keyed on roll.id.
	const draft = rollDraft;
	draft.begin(roll.id);
	$effect(() => {
		const id = roll.id;
		untrack(() => draft.begin(id));
	});
	const draftIntent = $derived(draft.intent);
	const draftAssetIds = $derived(draft.assetIds);
	const draftBankedIds = $derived(draft.bankedIds);

	// The actor always aids; non-actors fall back to any server intent (set on
	// a prior submit) so a forced-unready keeps showing the right side.
	const effectiveIntent = $derived<RollIntent | 'aid' | null>(
		isActor ? 'aid' : (intentLocked ? (me?.intent ?? null) : (draftIntent ?? me?.intent ?? null))
	);

	// Asset ids I've already committed a die for on this roll (locked).
	const myCommittedAssetIds = $derived(
		new Set(
			dice
				.filter(d => d.player_id === currentPlayerID && d.leveraged_asset_id != null)
				.map(d => d.leveraged_asset_id as number)
		)
	);

	// Assets to show in the leverage picker: unleveraged assets (selectable) plus
	// any I've already committed for THIS roll (locked, but kept visible so the
	// player can see what they've spent). Assets leveraged on a *prior* roll are
	// hidden — they can't be selected and only lengthen the list.
	const visibleAssets = $derived(
		myAssets.filter(a => !a.is_leveraged || myCommittedAssetIds.has(a.id))
	);

	const draftCount = $derived(draftAssetIds.size + draftBankedIds.size);
	const hasDraft = $derived(draftCount > 0);
	// Non-actors must choose a side before their drafted dice can commit.
	const needsIntent = $derived(!isActor && effectiveIntent == null && hasDraft);
	// Dice I could still add after this submit. If zero AND I'm committing
	// something, the server auto-readies me on the final commit — so we skip the
	// explicit ready call to avoid racing an auto-resolve.
	const remainingAfterSubmit = $derived(
		myUnleveragedAssets.filter(a => !draftAssetIds.has(a.id)).length +
		myUnspentBanked.filter(b => !draftBankedIds.has(b.id)).length
	);

	// Split dice into the two sides: the dice for the roll, and the dice against it.
	const actorPool = $derived(dice.filter(d => !d.is_interference));
	const intPool = $derived(dice.filter(d => d.is_interference));

	const effectiveDifficulty = $derived(roll.adjusted_difficulty ?? roll.difficulty);

	// Result scoring counts DISTINCT uncancelled actor-pool faces, so a repeat
	// of a face already showing is "free" — it doesn't raise the score. On a
	// resolved roll, flag those redundant copies (the first die to show a face
	// is kept; later matches are duplicates) so the count is legible. Cancelled
	// dice are skipped entirely: interference already removed them, and they
	// don't reserve a face for the distinct-count.
	const duplicateDieIds = $derived.by(() => {
		const dups = new Set<number>();
		if (stage !== 'resolved') return dups;
		const seenFaces = new Set<number>();
		for (const d of actorPool) {
			if (d.is_cancelled || d.face == null) continue;
			if (seenFaces.has(d.face)) dups.add(d.id);
			else seenFaces.add(d.face);
		}
		return dups;
	});

	// What a die is, in words: whose it is and why it isn't counting. The tile's
	// accessible name and its tooltip (the visible cue is its edge and face).
	function dieLabel(d: DiceRollDie): string {
		const who = playerNameMap.get(d.player_id) ?? '?';
		if (d.is_interference) return `${who}'s die, interfering${d.is_cancelled ? ', cancelled' : ''}`;
		const src = d.leveraged_asset_id
			? assets.find(a => a.id === d.leveraged_asset_id)?.name ?? 'asset'
			: 'base or banked die';
		if (d.is_cancelled) return `${who}'s die (${src}), cancelled by interference`;
		if (duplicateDieIds.has(d.id)) return `${who}'s die (${src}), repeats a face already counted`;
		return `${who}'s die (${src})`;
	}

	// ── Shake-Up mode ─────────────────────────────────────────────────────────
	// Derived entirely from roll.is_shake_up — shake-up rolls are actor-only
	// (no interference/voting), banked dice aren't spendable on them (ruling 3),
	// and a "result" is tokens earned (distinct faces), not a make/mar outcome.
	const isShakeUp = $derived(roll.is_shake_up);
	const shakeUpCategoryLabel = $derived(
		roll.shake_up_category
			? roll.shake_up_category[0].toUpperCase() + roll.shake_up_category.slice(1)
			: 'their category'
	);
	const actorName = $derived(playerNameMap.get(roll.actor_id) ?? '?');

	// One sentence on what is being rolled, so the dice below it have a subject.
	// Numbers of dice are words, as a person would say them.
	const NUMBER_WORDS = ['no', 'one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight', 'nine', 'ten'];
	const word = (n: number) => NUMBER_WORDS[n] ?? String(n);
	const dieNoun = (n: number) => (n === 1 ? 'die' : 'dice');
	const brief = $derived.by(() => {
		// Shake-Up says this itself, in the screen around the roll.
		if (isShakeUp) return '';
		let text = `${actorName} rolls ${word(actorPool.length)} ${dieNoun(actorPool.length)} against difficulty ${effectiveDifficulty}`;
		if (roll.adjusted_difficulty != null && roll.adjusted_difficulty !== roll.difficulty) {
			text += `, ${roll.adjusted_difficulty > roll.difficulty ? 'up' : 'down'} from ${roll.difficulty} by the vote`;
		}
		if (intPool.length > 0) {
			text += `, with ${word(intPool.length)} ${dieNoun(intPool.length)} rolled against them. Each interfering die cancels one matching die.`;
		} else {
			text += '.';
		}
		return text;
	});

	let busy = $state(false);
	let error = $state('');
	const setErr = (e: unknown) => {
		error = e instanceof Error ? e.message : 'Action failed.';
	};
	async function run(fn: () => Promise<unknown>) {
		if (busy) return;
		busy = true; error = '';
		try { await fn(); } catch (e) { setErr(e); } finally { busy = false; }
	}

	// ── decide_vote actions ───────────────────────────────────────────────────
	const onCallVote = () => run(() => callVote(roll.id));
	const onSkipVote = () => run(() => skipVote(roll.id));

	// ── voting actions ────────────────────────────────────────────────────────
	const myVote = $derived(votes.find(v => v.player_id === currentPlayerID));
	const voteCount = $derived(votes.length);
	const onVote = (v: 1 | -1) => run(() => voteOnRoll(roll.id, v));

	// ── leverage actions (draft-then-submit) ──────────────────────────────────
	// Intent and asset/banked picks only mutate local draft state. The Ready
	// button submits the whole draft, then readies.
	function setDraftIntent(intent: RollIntent) {
		if (intentLocked || busy) return;
		draft.intent = intent;
	}
	function toggleDraftAsset(asset: Asset) {
		if (myReady || busy) return;
		const next = new Set(draftAssetIds);
		next.has(asset.id) ? next.delete(asset.id) : next.add(asset.id);
		draft.assetIds = next;
	}
	function toggleDraftBanked(b: BankedDie) {
		if (myReady || busy) return;
		const next = new Set(draftBankedIds);
		next.has(b.id) ? next.delete(b.id) : next.add(b.id);
		draft.bankedIds = next;
	}

	// Append a freshly committed die, guarding against the roll.leverage_added
	// WS broadcast for the SAME die having already landed during our await. The
	// server broadcasts to every client including us, and that handler appends
	// too; without this dedup a race produces two dice rows sharing one id, and
	// the keyed {#each ... (die.id)} pools throw each_key_duplicate — which
	// freezes the interference list until a manual refresh reloads clean dice.
	function addDie(die: DiceRollDie) {
		if (!dice.some(d => d.id === die.id)) dice = [...dice, die];
	}

	// Commit the draft (one API call per leverage — preserves one chat-log
	// entry per asset), then ready. Each leverageRoll/useBankedDie reveals a
	// die publicly; this burst is the player's reveal-on-Ready.
	// `commit = false` is a pass: ready without the draft (nothing is committed,
	// and the draft is dropped).
	const submit = (commit: boolean) => run(async () => {
		if (commit && needsIntent) return;
		// Snapshot everything reactive up front — the assets/bankedDice props
		// mutate via WS as commits land, so capture the plan before any await.
		const assetIds = commit ? [...draftAssetIds] : [];
		const bankedIds = commit ? [...draftBankedIds] : [];
		const intentToSet = !isActor && !intentLocked ? (draftIntent ?? me?.intent ?? null) : null;
		const committingAny = assetIds.length > 0 || bankedIds.length > 0;
		// If this submit empties my hand, the server auto-readies me on the
		// final commit (and may auto-resolve) — so skip the explicit ready to
		// avoid racing a resolved roll.
		const willAutoReady = committingAny && remainingAfterSubmit === 0;

		// Clear the draft immediately; we hold local copies for the API calls.
		draft.clearPicks();

		// 1. Intent (non-actor, not yet locked, changed from server value).
		if (intentToSet && intentToSet !== me?.intent) {
			await setRollIntent(roll.id, intentToSet);
		}
		// 2. Leverage each drafted asset.
		for (const id of assetIds) {
			const { die } = await leverageRoll(roll.id, id);
			addDie(die);
		}
		// 3. Spend each drafted banked die.
		for (const id of bankedIds) {
			const { die } = await useBankedDie(roll.id, id);
			addDie(die);
			bankedDice = bankedDice.map(x =>
				x.id === id ? { ...x, used_at: new Date().toISOString(), used_roll_id: roll.id } : x);
		}
		// 4. Ready (unless the auto-ready already covered it).
		if (!willAutoReady) {
			await setRollReady(roll.id, true);
		}
	});
	const onReadySubmit = () => submit(true);
	const onPass = () => submit(false);
	const onUnready = () => run(() => setRollReady(roll.id, false));

	// The main button names what it commits. It is the one filled thing in the
	// step; with nothing drafted it is the pass (or, for the roller, the roll).
	const mainLabel = $derived.by(() => {
		if (isShakeUp) return hasDraft ? `Commit ${draftCount} and roll` : 'Roll the dice';
		if (isActor) return hasDraft ? 'Commit and roll' : 'Roll';
		if (!hasDraft) return 'Pass';
		return effectiveIntent === 'interfere' ? 'Commit interference'
			: effectiveIntent === 'aid' ? 'Commit aid' : 'Commit';
	});

	// What the draft will spend, in a notice: leveraging an asset is a cost.
	const draftedAssetNames = $derived(
		visibleAssets.filter(a => draftAssetIds.has(a.id)).map(a => a.name)
	);
	const joinNames = (names: string[]) =>
		names.length <= 1 ? (names[0] ?? '')
			: `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;

	// ── Player roster ────────────────────────────────────────────────────────
	// A locked-ready participant (no dice to add) is called out explicitly so
	// the actor can see at a glance who's been auto-readied vs. who's still
	// thinking. The actor's "Ready" click can resolve the roll the instant
	// every other participant is ready; surfacing locked-ready prevents
	// surprise resolutions.
	function rosterStatus(p: RollParticipant): string {
		const ready = p.is_ready ? 'ready' : 'not ready';
		if (p.player_id === roll.actor_id) return `actor · ${ready}`;
		if (p.is_ready && p.intent == null) return 'no dice · ready';
		return `${p.intent ?? 'choosing…'} · ${ready}`;
	}
	const playerFor = (id: number) => players.find(p => p.id === id);

	// ── Footer tally ─────────────────────────────────────────────────────────
	// Counts PLAYERS by side, in the same unit as "N of M ready" beside it and
	// the roster above. It used to count dice in each pool, so the
	// actor's two base dice read as "2 aid" and a player who had picked
	// interfere but not yet committed a die read as "0 interfere". The dice
	// themselves are already shown in the pools. The actor always aids; a
	// player who hasn't picked a side counts on neither.
	const aidCount = $derived(
		participants.filter(p => p.player_id === roll.actor_id || p.intent === 'aid').length
	);
	const interfereCount = $derived(
		participants.filter(p => p.player_id !== roll.actor_id && p.intent === 'interfere').length
	);

	// ── Commit feed ──────────────────────────────────────────────────────────
	// Show every die EXCEPT the actor's two automatic base dice (no asset,
	// not interference). Banked-die spends and leveraged assets both show up.
	function isActorBaseDie(d: DiceRollDie): boolean {
		return d.player_id === roll.actor_id
			&& d.leveraged_asset_id == null
			&& !d.is_interference;
	}
	const commitFeed = $derived(
		dice
			.filter(d => !isActorBaseDie(d))
			.map(d => {
				const asset = d.leveraged_asset_id != null
					? assets.find(a => a.id === d.leveraged_asset_id) : undefined;
				return {
					id: d.id,
					player: playerFor(d.player_id),
					playerName: playerNameMap.get(d.player_id) ?? '?',
					source: asset ? asset.name : 'banked die',
					isInterference: d.is_interference,
				};
			})
	);
</script>

{#snippet dieGroup(label: string | null, group: DiceRollDie[])}
	<div class="dice-group">
		{#if label}<span class="dice-label">{label}</span>{/if}
		<div class="dice-row">
			{#each group as die (die.id)}
				<DieTile
					face={die.face}
					color={playerColorByID(die.player_id, players)}
					label={dieLabel(die)}
					cancelled={die.is_cancelled}
					duplicate={duplicateDieIds.has(die.id)}
				/>
			{/each}
		</div>
	</div>
{/snippet}

{#snippet diceGroups()}
	<div class="dice-groups">
		<!-- Labelled only when there is a second group to tell it from. -->
		{@render dieGroup(intPool.length > 0 ? 'For' : null, actorPool)}
		{#if intPool.length > 0}
			{@render dieGroup('Against', intPool)}
		{/if}
	</div>
{/snippet}

{#if stage === 'resolved'}
	<!-- A finished step: one line, and the dice a tap away. -->
	<FinishedStep>
		{#snippet mark()}<DieMark size={16} />{/snippet}
		{#if isShakeUp}
			Rolled <span class="status-word">+{roll.result ?? 0} token{(roll.result ?? 0) === 1 ? '' : 's'}</span>
			· {roll.result ?? 0} distinct face{(roll.result ?? 0) === 1 ? '' : 's'}
		{:else}
			Rolled
			<span class="status-word" class:success={roll.outcome === 'make'} class:danger={roll.outcome === 'mar'}>{roll.outcome === 'make' ? 'Make' : 'Mar'}</span>
			· {roll.result} distinct face{roll.result === 1 ? '' : 's'}, difficulty {effectiveDifficulty}
		{/if}
		{#snippet detail()}{@render diceGroups()}{/snippet}
	</FinishedStep>
{:else}
	<section class="roll-step" aria-label="Dice roll">
		<StepHeading {heading} />
		{#if brief}<p class="roll-brief">{brief}</p>{/if}

		{#if error}
			<ErrorText message={error} />
		{/if}

		<!-- Dice (always visible): the dice for the roll, and the dice against it. -->
		{@render diceGroups()}

		<!-- ── Stage: decide_vote ──────────────────────────────────────────────── -->
		{#if stage === 'decide_vote'}
			{#if isActor}
				<div class="step-actions">
					<button class="action-btn primary" onclick={onCallVote} disabled={busy}>
						Call difficulty vote
					</button>
					<button class="action-btn secondary" onclick={onSkipVote} disabled={busy}>
						Skip vote
					</button>
				</div>
			{/if}
			<!-- Everyone else has nothing to do but wait: the heading already says on whom. -->
		{/if}

		<!-- ── Stage: voting ───────────────────────────────────────────────────── -->
		{#if stage === 'voting'}
			<p class="muted-text">
				Each vote moves the difficulty by one. {voteCount} of {players.length} have voted.
			</p>
			<!-- A vote is a choice, so it wears no meaning colour: the words and the
			     order say which is which. Once cast it shows as picked and locked. -->
			<div class="choice-row">
				<Chip
					selected={myVote?.vote === -1}
					disabled={busy || myVote != null}
					onclick={() => onVote(-1)}
				>
					Easier (−1)
				</Chip>
				<Chip
					selected={myVote?.vote === 1}
					disabled={busy || myVote != null}
					onclick={() => onVote(1)}
				>
					Harder (+1)
				</Chip>
			</div>
			{#if myVote}
				<p class="muted-text">
					You voted {myVote.vote === 1 ? '+1 (harder)' : '−1 (easier)'}. The votes reveal together.
				</p>
			{/if}
		{/if}

		<!-- ── Stage: leverage ─────────────────────────────────────────────────── -->
		{#if stage === 'leverage'}
			{#if isShakeUp && !isActor}
				<!-- Shake-up rolls are actor-only; a non-roller has nothing to do but
				     watch the dice fill in. -->
				<p class="muted-text">{actorName} is rolling for {shakeUpCategoryLabel}…</p>
			{:else}
				{#if !isShakeUp && !myReady}
					<p class="roll-brief">
						{isActor
							? 'Leverage assets to add dice for the roll, then roll.'
							: 'Leverage an asset to add a die for the roll or against it, or pass.'}
					</p>
				{/if}

				<!-- Aid or interfere. The actor always aids, so has no choice to make.
				     Once a die is committed the side is locked: it stays shown as
				     picked, and can't be pressed. -->
				{#if !isActor}
					<div class="choice-row">
						<Chip
							selected={effectiveIntent === 'aid'}
							disabled={busy || intentLocked}
							onclick={() => setDraftIntent('aid')}
						>
							Aid
						</Chip>
						<Chip
							selected={effectiveIntent === 'interfere'}
							disabled={busy || intentLocked}
							onclick={() => setDraftIntent('interfere')}
						>
							Interfere
						</Chip>
					</div>
				{/if}

				<!-- Banked dice (once a side is chosen, or I'm the actor). Rare and free,
				     so they lead the leverage choices above the asset list. Not offered
				     on shake-up rolls — banked dice aren't spendable there (ruling 3). -->
				{#if !isShakeUp && (isActor || effectiveIntent != null) && myUnspentBanked.length > 0}
					<div class="leverage-group">
						<span class="section-label">Banked dice ({myUnspentBanked.length})</span>
						<div class="choice-row">
							{#each myUnspentBanked as b (b.id)}
								<Chip
									selected={draftBankedIds.has(b.id)}
									disabled={busy || myReady}
									title="Spend this banked die on Ready (random face at resolution)"
									onclick={() => toggleDraftBanked(b)}
								>
									<DieMark size={16} />
									{draftBankedIds.has(b.id) ? 'Selected' : 'Spend a banked die'}
								</Chip>
							{/each}
						</div>
					</div>
				{/if}

				<!-- My assets (once a side is chosen, or I'm the actor). Assets leveraged
				     on a prior roll are filtered out; current-roll commits stay visible. -->
				{#if (isActor || effectiveIntent != null) && visibleAssets.length > 0}
					<div class="leverage-group">
						<span class="section-label">Leverage from your retinue</span>
						{#each visibleAssets as asset (asset.id)}
							<AssetCardSelectable
								{asset}
								ownerColor={myColor}
								knownSecretCount={secretCounts?.known(asset.id)}
								leverageMode
								leverageDrafted={draftAssetIds.has(asset.id)}
								leverageDisabled={myReady || (isActor && actorLeverageBlocked)}
								onToggleLeverage={toggleDraftAsset}
							/>
						{/each}
					</div>
				{/if}

				<!-- What the pick will cost. Leveraging an asset turns it, so it is
				     said before the button that commits it. -->
				{#if draftedAssetNames.length > 0}
					<Notice tone="warning">{joinNames(draftedAssetNames)} will be leveraged.</Notice>
				{/if}
				{#if draftBankedIds.size > 0}
					<Notice tone="warning">
						{draftBankedIds.size === 1 ? 'A banked die' : `${draftBankedIds.size} banked dice`} will be spent.
					</Notice>
				{/if}

				<!-- The step's one main button names what it commits; the pass sits
				     beside it. Already ready: just the way back. -->
				{#if myReady}
					<div class="step-actions">
						<button
							class="action-btn secondary"
							onclick={onUnready}
							disabled={busy || !canCommit}
						>
							{canCommit ? 'Unready' : (isShakeUp ? 'Rolled' : 'Ready (nothing left to add)')}
						</button>
					</div>
					{#if !canCommit}
						<p class="muted-text">You have no dice left to add, so you are ready automatically.</p>
					{/if}
				{:else}
					<div class="step-actions">
						<button
							class="action-btn primary"
							onclick={onReadySubmit}
							disabled={busy || needsIntent}
						>
							{mainLabel}
						</button>
						{#if hasDraft && !isActor && !isShakeUp}
							<button class="action-btn secondary" onclick={onPass} disabled={busy}>
								Pass
							</button>
						{/if}
					</div>
					{#if needsIntent}
						<p class="muted-text">Pick aid or interfere first.</p>
					{/if}
				{/if}
				{#if !isShakeUp}
					<p class="muted-text small">Opposing leverages will unready you.</p>
				{/if}

				{#if !isShakeUp}
					<!-- Public commit feed -->
					{#if commitFeed.length > 0}
						<div class="leverage-group">
							<span class="section-label">Commits this roll</span>
							<ul class="plain-list">
								{#each commitFeed as c (c.id)}
									<li>
										<PlayerShape player={c.player} size={12} />
										<span>{c.playerName} · {c.source} · {c.isInterference ? 'interfere' : 'aid'}</span>
									</li>
								{/each}
							</ul>
						</div>
					{/if}

					<!-- Where everyone stands: plain lines, not chips, since none of them
					     can be pressed. -->
					<ul class="plain-list roster">
						{#each participants as p (p.player_id)}
							<li>
								<PlayerShape player={playerFor(p.player_id)} size={12} />
								<span>{playerNameMap.get(p.player_id) ?? '?'} · {rosterStatus(p)}</span>
							</li>
						{/each}
					</ul>

					<!-- Footer summary -->
					<p class="footer-summary">
						{aidCount} aid · {interfereCount} interfere ·
						{participants.filter(p => p.is_ready).length} of {participants.length} ready
					</p>
				{/if}
			{/if}
		{/if}
	</section>
{/if}

<style>
	/* The open roll is a step: a column with the steps' gaps, and nothing around it. */
	.roll-step {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		flex-shrink: 0;
	}

	.roll-brief {
		margin: 0;
		font-size: var(--text-body);
		line-height: 1.5;
		color: var(--color-text);
	}

	/* Dice: two labelled groups side by side, wrapping on a narrow column. */
	.dice-groups {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3) var(--space-5);
	}
	.dice-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-width: 0;
	}
	.dice-label {
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}
	.dice-row {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}

	/* A row of choices: Aid / Interfere, easier / harder, a banked die. Two equal
	   halves for the pair, and they wrap if a third arrives. */
	.choice-row {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.choice-row > :global(.chip) { flex: 1 1 8rem; }

	/* The step's buttons: the main one takes the room, the pass sits beside it. */
	.step-actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.step-actions > :global(.action-btn) { align-self: stretch; }
	.step-actions > :global(.action-btn.primary) { flex: 2 1 10rem; }
	.step-actions > :global(.action-btn.secondary) { flex: 1 1 6rem; }

	/* A cluster of controls under a small label ("Leverage from your retinue"). */
	.leverage-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.section-label {
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}

	/* Who has committed what, and where everyone stands: lines with the player's
	   shape in front, never chips. */
	.plain-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	.plain-list li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.footer-summary {
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
</style>
