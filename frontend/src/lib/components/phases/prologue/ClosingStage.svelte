<!-- ClosingStage.svelte
  "The Stage is Set" — the prologue's closing step (game.prologue_ranking_step
  === 'closing'; adr/PROLOGUE_CLOSING_STAGE_PLAN.md). Reads recap-first,
  checklist-second (the ceremony framing): a review of the prologue's outcome
  (final standings, laws & rumors, retinue tallies), then the verdict + the
  checklist of hard items (must complete before Ready) and soft nudges, then
  the ready roster/toggle that drives the all-ready auto-advance into the Main
  Event.

  Rebuilt onto the house checklist grammar by
  adr/LOBBY_AND_CHECKLIST_PLAN.md D6, which this page shares with the lobby:
  ChecklistRow for every item, TableRoster for both player lists, and a verdict
  panel that says in a sentence what the reader's situation is. What it does
  NOT share is the accordion (R4): lobby items are optional invitations, so
  hiding them is free, but these gate the Ready button and a hidden blocker is
  a stuck table. So a done item collapses to one row that shows its answer, and
  a blocker stays open with its form.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { createExtraPeer, updateAsset, setClosingReady } from '$lib/api';
	import type {
		Player,
		Asset,
		Ranking,
		PlayerCardRow,
		CommittedHeart,
		TrackDone,
		PrologueSheet,
		PrologueClaim,
		ExtraPeer,
		ClosingReady,
		Law,
		Rumor,
	} from '$lib/api';
	import {
		findMainCharacter,
		isMcNamed,
		needsExtraPeer,
		findExtraPeer,
		unclaimedTitles,
		readyBlockedReason,
		isReady,
		notReadyPlayerIDs,
		myAtRiskCount,
		blankAssets,
		retinueTallies,
		RETINUE_TYPE_ORDER,
		type RetinueTally,
	} from '$lib/prologue/closing';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import TrackBoard from './TrackBoard.svelte';
	import AssetTypeIcon from '$lib/components/AssetTypeIcon.svelte';
	import AssetCreationForm from '$lib/components/AssetCreationForm.svelte';
	import ChecklistRow from '$lib/components/shared/ChecklistRow.svelte';
	import TableRoster from '$lib/components/shared/TableRoster.svelte';
	import FlagGlyph from '$lib/components/FlagGlyph.svelte';
	import LogMark from '$lib/components/LogMark.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	interface Props {
		gameID: string;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		closingReady: ClosingReady[];
		extraPeers: ExtraPeer[];
		sheets: PrologueSheet[];
		claims: PrologueClaim[];
		// Recap-only reference data (final standings via TrackBoard).
		rankings: Ranking[];
		cards: PlayerCardRow[];
		committed: CommittedHeart[];
		doneFlags: TrackDone[];
		laws?: Law[];
		rumors?: Rumor[];
		/** Tone topics the table has taken a position on — the tones row's chip. */
		tonesSetCount?: number;
		onReload: () => void;
		onResync?: () => void;
		onOpenTones?: () => void;
		/** Opens the retinue sheet — for `playerID` if given, else the current player. */
		onOpenRetinue?: (playerID?: number) => void;
		onOpenLaws?: () => void;
		onOpenRumors?: () => void;
	}

	let {
		gameID,
		players,
		assets = $bindable(),
		currentPlayerID,
		closingReady,
		extraPeers,
		sheets,
		claims,
		rankings,
		cards,
		committed,
		doneFlags,
		laws = [],
		rumors = [],
		tonesSetCount = 0,
		onReload,
		onResync,
		onOpenTones,
		onOpenRetinue,
		onOpenLaws,
		onOpenRumors,
	}: Props = $props();

	let error = $state('');

	function playerName(id: number): string {
		return players.find((p) => p.id === id)?.display_name ?? '?';
	}

	// ── Recap: retinue tallies ───────────────────────────────────────────────
	const tallies = $derived(retinueTallies(players, assets));
	function tallyFor(playerID: number): RetinueTally | undefined {
		return tallies.find((t) => t.playerID === playerID);
	}

	// The row's aria-label restates the counts because a labelled button hides
	// its inner text from assistive tech — the counts ride in TableRoster's
	// `trailing`, which is inside the button. `stateWords` is what the roster
	// would have appended on its own; passing it through keeps the presence
	// wording from being dropped here.
	const TALLY_TYPE_LABELS: Record<Asset['asset_type'], string> = {
		peer: 'Peers',
		artifact: 'Artifacts',
		resource: 'Resources',
		holding: 'Holdings',
	};
	function tallyRowLabel(p: Player, stateWords: string): string {
		const t = tallyFor(p.id);
		if (!t) return p.display_name;
		const counts = RETINUE_TYPE_ORDER.map((type) => `${TALLY_TYPE_LABELS[type]} ${t.counts[type]}`).join(', ');
		const you = t.playerID === currentPlayerID ? ' (you)' : '';
		const taken = t.takenFromOthers > 0 ? `; ${t.takenFromOthers} taken from others` : '';
		return `View ${playerName(t.playerID)}'s retinue${you} — ${counts}${taken}${stateWords}`;
	}

	// ── Name your main character (hard) ──────────────────────────────────────
	const myMainCharacter = $derived(findMainCharacter(assets, currentPlayerID));
	const mcNamed = $derived(isMcNamed(myMainCharacter));

	let mcRenameDraft = $state('');
	let savingMcRename = $state(false);
	async function submitMcRename() {
		const text = mcRenameDraft.trim();
		if (!myMainCharacter || !text || savingMcRename) return;
		savingMcRename = true;
		error = '';
		try {
			await updateAsset(myMainCharacter.id, { name: text });
			mcRenameDraft = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not rename your main character.';
		} finally {
			savingMcRename = false;
		}
	}

	// ── Create an extra peer (hard, ≤3p) ─────────────────────────────────────
	const needsPeer = $derived(needsExtraPeer(players.length));
	const myExtraPeer = $derived(findExtraPeer(extraPeers, currentPlayerID));
	const titlesSheet = $derived(sheets.find((s) => s.type === 'titles'));
	const openTitles = $derived(unclaimedTitles(titlesSheet, claims, extraPeers));

	// The claimed title's name, the peer's own name, and the wording of the
	// title marginalia the peer gets stamped with. The last one is seeded from
	// the title so the field is never a blank canvas — the player edits "The
	// Heretic" into their own phrasing rather than inventing one cold.
	let extraTitleName = $state('');
	let extraPeerText = $state('');
	let extraTitleText = $state('');
	let creatingExtra = $state(false);

	const selectedTitle = $derived(openTitles.find((t) => t.name === extraTitleName) ?? null);

	// Switching titles resets the wording to the new title's name, discarding
	// whatever was there. Owner's ruling (2026-08-09): picking a different
	// title *is* the signal that the old wording is unwanted — why else switch?
	// Preserving it only leaves a peer described as one title and stamped with
	// another.
	function pickTitle(name: string) {
		const next = extraTitleName === name ? '' : name;
		extraTitleName = next;
		extraTitleText = next;
	}

	// The done row's answer (D6): who they actually wrote, and which title it
	// spent — "you created your extra peer" is the one thing the reader can
	// already see from the tick.
	const extraPeerAnswer = $derived.by(() => {
		if (!myExtraPeer) return undefined;
		const asset = assets.find((a) => a.id === myExtraPeer.asset_id);
		return asset ? `${asset.name}, ${myExtraPeer.title_name}` : myExtraPeer.title_name;
	});

	async function submitExtraPeer() {
		if (!extraTitleName || !extraPeerText.trim() || !extraTitleText.trim() || creatingExtra) return;
		creatingExtra = true;
		error = '';
		try {
			const result = await createExtraPeer(
				gameID,
				extraTitleName,
				extraPeerText.trim(),
				extraTitleText.trim()
			);
			// The asset.created WS event may already have appended this peer (its
			// handler dedups by id); only append if it hasn't, so we don't end up
			// with two rows for one peer. See [[optimistic-append-ws-dup]].
			if (!assets.find((a) => a.id === result.asset.id)) {
				assets = [...assets, result.asset];
			}
			extraTitleName = '';
			extraPeerText = '';
			extraTitleText = '';
			onReload();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create extra peer.';
			onResync?.();
			onReload();
		} finally {
			creatingExtra = false;
		}
	}

	// ── Give every asset a marginalia (hard) ─────────────────────────────────
	const myBlankAssets = $derived(blankAssets(assets, currentPlayerID));
	// What the done row says instead of "everything is fine": how many things
	// it is actually vouching for (D6 — a finished item spends its line on the
	// answer). Same live-assets rule as blankAssets, so the two can't disagree.
	const myAssetCount = $derived(
		currentPlayerID == null
			? 0
			: assets.filter((a) => a.owner_id === currentPlayerID && !a.is_destroyed).length
	);

	// ── Shore up at-risk assets (soft) ────────────────────────────────────────
	// Blank assets are a subset of the at-risk set, and they already have their
	// own hard item above — so the soft nudge counts only the rest, otherwise
	// one asset would be scolded twice on the same page.
	const riskCount = $derived(myAtRiskCount(assets, currentPlayerID) - myBlankAssets.length);

	// ── Ready roster + toggle ────────────────────────────────────────────────
	const blockedReason = $derived(
		readyBlockedReason(mcNamed, players.length, myExtraPeer != null, myBlankAssets.length)
	);
	const myReady = $derived(isReady(closingReady, currentPlayerID));
	const notReadyIDs = $derived(new Set(notReadyPlayerIDs(players, closingReady)));
	const readyCount = $derived(players.length - notReadyIDs.size);

	// ── Verdict (D4) ─────────────────────────────────────────────────────────
	// The page's answer to "what is my situation", in a sentence, above the
	// rows. It replaced the blockedReason line as the primary signal — that
	// line stays under the button as the disabled tooltip's visible twin.
	const NUMBER_WORDS = ['no', 'one', 'two', 'three', 'four', 'five'];
	function numberWord(n: number): string {
		return NUMBER_WORDS[n] ?? String(n);
	}
	function sentenceCase(s: string): string {
		return s.charAt(0).toUpperCase() + s.slice(1);
	}
	/** "alice", "alice and bob", "alice, bob and carol". */
	function nameList(names: string[]): string {
		if (names.length <= 1) return names[0] ?? '';
		return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;
	}

	// The hard items still outstanding, one sentence each, in readyBlockedReason's
	// order — so the verdict and the button's reason can never name different
	// things first.
	const outstanding = $derived.by<string[]>(() => {
		const out: string[] = [];
		if (!mcNamed) out.push('Your main character needs a name.');
		if (needsPeer && myExtraPeer == null) out.push('Your court needs an extra peer.');
		const blank = myBlankAssets.length;
		if (blank > 0) {
			out.push(
				blank === 1
					? 'One asset still carries no marginalia.'
					: `${blank} of your assets still carry no marginalia.`
			);
		}
		return out;
	});

	// "both of you" beats "all two of you"; nothing else in the range 2–5 needs
	// a special case.
	const everyoneClause = $derived(
		players.length === 2 ? 'both of you' : `all ${numberWord(players.length)} of you`
	);
	const waitingNames = $derived(
		players.filter((p) => p.id !== currentPlayerID && notReadyIDs.has(p.id)).map((p) => p.display_name)
	);

	let savingReady = $state(false);
	async function toggleReady() {
		if (savingReady || (blockedReason && !myReady)) return;
		savingReady = true;
		error = '';
		try {
			await setClosingReady(gameID, !myReady);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update readiness.';
			onResync?.();
			onReload();
		} finally {
			savingReady = false;
		}
	}
</script>

<!-- Row glyphs. The tear is the log's own mark for a torn note — the exact
     thing these assets are one of away from destruction — borrowed by name
     the way the chat bar borrows `chat` (LogMark's doc comment). The flag is
     a component so the lobby's tones row and this one can't drift. -->
{#snippet tearGlyph()}
	<!-- LogMark fills whatever box it is given, so the box is here: 16px, the
	     size FlagGlyph draws itself at, or the two nudges' marks disagree by
	     2px in the same list. -->
	<span class="tear-mark"><LogMark family="tear" /></span>
{/snippet}
{#snippet flagGlyph()}
	<FlagGlyph />
{/snippet}

<div class="closing-stage">
	{#if error}
		<ErrorText message={error} />
	{/if}

	<h2 class="closing-title">The Stage is Set.</h2>
	<p class="closing-lede">
		The prologue draws to a close. Here is the court as it now stands — settle your
		affairs, then ready yourself for the Main Event.
	</p>

	<!-- ── Recap: the prologue in review (recap-first ceremony framing) ─────── -->
	<section class="recap" aria-label="Prologue recap">
		<div class="recap-block">
			<h4 class="recap-sub">Starting Ranks</h4>
			<TrackBoard
				{players}
				{cards}
				{rankings}
				{committed}
				{doneFlags}
				activeTrack={null}
				{currentPlayerID}
				showCards={false}
			/>
		</div>

		<div class="recap-block">
			<h4 class="recap-sub">Laws &amp; rumors</h4>
			{#if laws.length === 0 && rumors.length === 0}
				<p class="muted-text small" style="margin:0;">The record is quiet — no laws or rumors yet.</p>
			{:else}
				<div class="recap-lr">
					{#if laws.length > 0}
						<div class="recap-lr-group">
							<div class="recap-lr-head">
								<span class="recap-lr-title">Laws <span class="recap-count">{laws.length}</span></span>
								{#if onOpenLaws}
									<button type="button" class="recap-link" onclick={onOpenLaws}>View all</button>
								{/if}
							</div>
							<ul class="recap-lr-list">
								{#each laws as law (law.id)}
									<li class="recap-lr-item">{law.text}</li>
								{/each}
							</ul>
						</div>
					{/if}
					{#if rumors.length > 0}
						<div class="recap-lr-group">
							<div class="recap-lr-head">
								<span class="recap-lr-title">Rumors <span class="recap-count">{rumors.length}</span></span>
								{#if onOpenRumors}
									<button type="button" class="recap-link" onclick={onOpenRumors}>View all</button>
								{/if}
							</div>
							<ul class="recap-lr-list">
								{#each rumors as rumor (rumor.id)}
									<li class="recap-lr-item">{rumor.text}</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>
			{/if}
		</div>

		<div class="recap-block">
			<h4 class="recap-sub">Retinue</h4>
			<!-- The same seats as the ready roster below, so the two lists stop
			     being two different objects (D3). No waiting set here on
			     purpose: the recap is a review, and gold on this list would
			     compete with the roster that actually asks something. -->
			<TableRoster
				{players}
				{currentPlayerID}
				onSelect={(playerID) => onOpenRetinue?.(playerID)}
				rowLabel={tallyRowLabel}
			>
				{#snippet trailing(p)}
					{@const t = tallyFor(p.id)}
					{#if t}
						<span class="tally-counts">
							{#each RETINUE_TYPE_ORDER as type}
								<span class="tally-count" class:zero={t.counts[type] === 0}>
									<AssetTypeIcon {type} size={14} />
									<span class="tally-num">{t.counts[type]}</span>
								</span>
							{/each}
						</span>
						{#if t.takenFromOthers > 0}
							<span class="tally-taken" title="Assets taken from other players during the prologue">
								{t.takenFromOthers} taken
							</span>
						{/if}
					{/if}
				{/snippet}
			</TableRoster>
		</div>
	</section>

	<!-- ── Verdict (D4) ────────────────────────────────────────────────────
	     Local markup, not a shared component: the lobby's panel sits in the
	     same slot and shares not one word with this one. -->
	<section class="verdict" class:act={!myReady && outstanding.length > 0}>
		{#if myReady}
			{#if waitingNames.length === 0}
				<!-- All-ready auto-advances, so this is the half-second between
				     the last ready and the phase change (and the safety net if
				     the advance ever fails), not a state to design for. -->
				<h3>Everyone is ready.</h3>
				<p class="muted-text">The Main Event begins now.</p>
			{:else}
				<h3>You're ready. Waiting on {nameList(waitingNames)}.</h3>
				<p class="muted-text">
					Nothing else to do — the Main Event begins the moment the last of you is ready.
				</p>
			{/if}
		{:else if outstanding.length > 0}
			<h3>
				{sentenceCase(numberWord(outstanding.length))}
				{outstanding.length === 1 ? 'thing' : 'things'} left before you can ready up.
			</h3>
			<!-- Name the one thing; count the many. Listing three items here
			     when three blocker rows sit open directly below says everything
			     three times and costs the panel two lines to do it (decided in
			     session — the plan left the several-blockers case open). -->
			<p class="muted-text">
				{#if outstanding.length === 1}
					{outstanding[0]} Everything else is settled — the table advances as soon as
					{everyoneClause} are ready.
				{:else}
					Each is waiting below — the table advances as soon as {everyoneClause} are ready.
				{/if}
			</p>
		{:else}
			<!-- Not "your house is in order": it sits two lines above the heading
			     "Put your house in order", and the echo reads as a joke. -->
			<h3>Nothing left to settle.</h3>
			<p class="muted-text">
				Ready up when you're happy with your court — the table advances as soon as
				{everyoneClause} are ready.
			</p>
		{/if}
	</section>

	<h3 class="section-heading">Put your house in order</h3>

	<div class="checklist">
		<!-- Done items are one row carrying their answer; blockers stay open
		     with their form (R4). Neither is ever behind a caret. -->
		{#if mcNamed}
			<ChecklistRow
				title="Name your main character"
				subtitle={myMainCharacter?.name}
				tone="done"
				glyph="tick"
			/>
		{:else}
			<ChecklistRow title="Name your main character" tone="blocker" glyph="circle">
				<p class="item-copy">Currently: {myMainCharacter?.name ?? '—'}</p>
				<div class="item-form">
					<input
						type="text"
						bind:value={mcRenameDraft}
						placeholder="Name your main character"
						maxlength={TEXT_LIMITS.NAME}
						aria-label="New main character name"
					/>
					<button
						class="action-btn secondary small"
						onclick={submitMcRename}
						disabled={!mcRenameDraft.trim() || savingMcRename}
					>
						{savingMcRename ? '…' : 'Save name'}
					</button>
				</div>
			</ChecklistRow>
		{/if}

		{#if needsPeer}
			{#if myExtraPeer}
				<ChecklistRow
					title="Create an extra peer"
					subtitle={extraPeerAnswer}
					tone="done"
					glyph="tick"
				/>
			{:else}
				<ChecklistRow title="Create an extra peer" tone="blocker" glyph="circle">
					<p class="item-copy">
						A court this small is a thin cast. Take one title nobody claimed and give
						it a face.
					</p>
					<!-- Title first, then the authoring form: the description under the
					     chosen chip is the prompt the peer is written from, so there is
					     nothing to write until a title is picked. -->
					<div class="extra-title">
						<span class="extra-title-label">1 · Claim a title</span>
						{#if openTitles.length === 0}
							<p class="muted-text small" style="margin:0;">No titles remain.</p>
						{:else}
							<div class="title-chip-row">
								{#each openTitles as t}
									<button
										type="button"
										class="title-chip"
										class:active={extraTitleName === t.name}
										aria-pressed={extraTitleName === t.name}
										onclick={() => pickTitle(t.name)}
									>{t.name}</button>
								{/each}
							</div>
						{/if}
					</div>

					{#if selectedTitle}
						{#if selectedTitle.description}
							<p class="title-desc">{selectedTitle.description}</p>
						{/if}
						<AssetCreationForm
							{gameID}
							assetType="peer"
							bind:name={extraPeerText}
							bind:marginalia={extraTitleText}
							disabled={creatingExtra}
							nameLabel="2 · Name"
							marginaliaLabel="3 · The title, in your words"
							namePlaceholder="Name your peer…"
							marginaliaPlaceholder="How does this title sit on them?"
						/>
						<button
							class="action-btn secondary small"
							onclick={submitExtraPeer}
							disabled={!extraPeerText.trim() || !extraTitleText.trim() || creatingExtra}
						>
							{creatingExtra ? '…' : 'Create peer'}
						</button>
					{/if}
				</ChecklistRow>
			{/if}
		{/if}

		{#if myBlankAssets.length === 0}
			<ChecklistRow
				title="Give every asset a marginalia"
				subtitle={myAssetCount === 1
					? 'Your one asset carries a note.'
					: `All ${myAssetCount} carry a note.`}
				tone="done"
				glyph="tick"
			/>
		{:else}
			<ChecklistRow title="Give every asset a marginalia" tone="blocker" glyph="circle">
				<p class="item-copy">
					{myBlankAssets.length}
					{myBlankAssets.length === 1 ? 'asset has' : 'assets have'} nothing written on
					{myBlankAssets.length === 1 ? 'it' : 'them'} yet. An asset with no marginalia can never be
					broken — or lost.
				</p>
				<ul class="blank-list">
					{#each myBlankAssets as a (a.id)}
						<li class="blank-item">
							<AssetTypeIcon type={a.asset_type} size={14} />
							<span class="blank-name">{a.name}</span>
						</li>
					{/each}
				</ul>
				<button class="action-btn secondary small" onclick={() => onOpenRetinue?.()}>Open Retinue</button>
			</ChecklistRow>
		{/if}

		<!-- Both nudges open a panel elsewhere, so both take the arrow, never a
		     caret (D1). The count rides in the chip, which is what makes a
		     44px row enough where each of these used to spend ~180px. -->
		{#if riskCount > 0}
			<ChecklistRow
				title="Shore up at-risk assets"
				subtitle="One tear from destruction."
				tone="risk"
				glyph={tearGlyph}
				action="navigate"
				onSelect={() => onOpenRetinue?.()}
				state={{ text: `${riskCount} at risk`, tone: 'risk' }}
			/>
		{/if}

		<!-- The lobby offers the same item as an invitation; here it is a
		     deadline, and the copy is the only thing that says so (D6). -->
		<ChecklistRow
			title="Tones — last chance"
			subtitle="They lock when the Main Event begins."
			tone="warn"
			glyph={flagGlyph}
			action="navigate"
			onSelect={onOpenTones}
			state={{ text: tonesSetCount > 0 ? `${tonesSetCount} set` : 'none yet', tone: 'neutral' }}
		/>
	</div>

	<h3 class="section-heading">
		Ready roster <span class="count">{readyCount} of {players.length} ready</span>
	</h3>
	<!-- Gold on the seats we're waiting for, matching the header chips' rings —
	     the roster and the chips read from the same set, so they can't
	     disagree about who is holding the table up. -->
	<TableRoster {players} {currentPlayerID} waitingPlayerIDs={notReadyIDs}>
		{#snippet trailing(p)}
			{#if isReady(closingReady, p.id)}
				<span class="ready-chip done">✓ ready</span>
			{:else}
				<span class="ready-chip pending">waiting…</span>
			{/if}
		{/snippet}
	</TableRoster>

	<button
		class="action-btn primary done-btn"
		class:active={myReady}
		disabled={savingReady || (!myReady && blockedReason != null)}
		title={!myReady ? (blockedReason ?? undefined) : undefined}
		onclick={toggleReady}
	>
		{savingReady ? '…' : myReady ? 'Ready ✓ (tap to undo)' : "I'm ready"}
	</button>
	{#if !myReady && blockedReason}
		<p class="muted-text small">{blockedReason}</p>
	{/if}
</div>

<style>
	.closing-stage {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.closing-title {
		margin: 0;
		color: var(--color-text);
		font-size: 1.05rem;
		line-height: 1.45;
	}
	.closing-lede {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.9rem;
		line-height: 1.4;
	}

	/* ── Recap ─────────────────────────────────────────────────────────────── */
	.recap {
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
	}
	.recap-block {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.recap-sub {
		margin: 0;
		color: var(--color-text-muted);
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	/* Laws & rumors: two stacked groups on a phone; side by side once the
	   column has room (mirrors the record-content band). */
	.recap-lr {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	@container column (min-width: 420px) {
		.recap-lr { flex-direction: row; }
		.recap-lr-group { flex: 1 1 0; min-width: 0; }
	}
	.recap-lr-group {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.recap-lr-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.recap-lr-title {
		color: var(--color-text);
		font-size: 0.85rem;
	}
	.recap-count {
		color: var(--color-text-muted);
		font-weight: 600;
		font-size: 0.78rem;
	}
	.recap-link {
		background: none;
		border: none;
		padding: 0.25rem 0;
		min-height: 32px;
		color: var(--color-accent);
		font: inherit;
		font-size: 0.8rem;
		text-decoration: underline;
		cursor: pointer;
		flex: none;
	}
	.recap-link:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
	.recap-lr-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.recap-lr-item {
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-left: 3px solid var(--color-border-warm);
		border-radius: 4px;
		padding: 0.35rem 0.5rem;
		color: var(--color-text-secondary);
		font-size: 0.82rem;
		line-height: 1.35;
		/* Compact: at most two lines here — the full text lives behind View all. */
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	/* Retinue tallies: what rides in each TableRoster row's trailing slot. */
	.tally-counts {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex: none;
	}
	.tally-count {
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
		color: var(--color-text-secondary);
	}
	.tally-count.zero {
		opacity: 0.4;
	}
	.tally-num {
		font-weight: 600;
		font-size: 0.78rem;
		min-width: 0.6rem;
	}
	/* Blue chip trio, matching the choosing view's steal ring — a take is an
	   opportunity, not a warning (this chip was orange to match the ring's old
	   colour, and followed it here when the ring moved on 2026-08-01). */
	.tally-taken {
		flex: none;
		background: var(--color-chip-blue-bg);
		border: 1px solid var(--color-chip-blue-border);
		color: var(--color-chip-blue-text);
		font-size: 0.68rem;
		padding: 0.1rem 0.4rem;
		border-radius: 999px;
		white-space: nowrap;
	}

	.tear-mark {
		display: block;
		width: 16px;
		height: 16px;
	}

	/* ── Verdict ──────────────────────────────────────────────────────────── */
	/* Same frame as the lobby's panel — the two screens answer the same
	   question and a reader arriving from one should recognise the other. */
	.verdict {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		align-items: flex-start;
		padding: 0.85rem 0.8rem;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: 12px;
	}
	/* Warm frame only while something is owed, matching the lobby's
	   facilitator block: gold as the frame and the label, never as a fill. */
	.verdict.act { border-color: var(--color-border-warm-antique); }
	.verdict h3 {
		margin: 0;
		color: var(--color-accent);
		font-size: 1.05rem;
		line-height: 1.3;
	}
	.verdict .muted-text { line-height: 1.45; }

	.section-heading {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin: 0;
		color: var(--color-accent);
		font-size: 1rem;
	}
	.count {
		color: var(--color-text-muted);
		font-size: 0.8rem;
	}

	.checklist {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	/* ── Blocker bodies ───────────────────────────────────────────────────── */
	.item-copy {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.85rem;
		line-height: 1.4;
	}
	.item-form {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: end;
	}
	.item-form input {
		flex: 1 1 12rem;
		min-width: 0;
	}

	/* Blank-asset list: names the offending assets so the player knows what to
	   open the Retinue for. Asset names render italic app-wide. */
	.blank-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}
	.blank-item {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--color-text-secondary);
		font-size: 0.82rem;
	}
	.blank-name {
		font-style: italic;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}

	/* Ready roster: the seat's own trailing content. The state is a word, not
	   just the seat's gold — the two lists on this page would otherwise differ
	   only by a fill. */
	.ready-chip { font-size: 0.8rem; }
	.ready-chip.done { color: var(--color-success); }
	.ready-chip.pending { color: var(--color-text-faint); font-style: italic; }

	.done-btn.active { background: var(--color-success); }

	.extra-title {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	/* Numbered to match AssetCreationForm's own step labels, which this row
	   sits directly above — so the title pick reads as step 1 of one form
	   rather than a separate control. */
	.extra-title-label {
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-accent);
	}

	/* The claimed title's sheet description — the fiction the peer is written
	   from. Quoted-source treatment (rule + italic), not a form hint. */
	.title-desc {
		margin: 0;
		padding-left: 0.6rem;
		border-left: 2px solid var(--color-border-warm);
		color: var(--color-text-secondary);
		font-size: 0.85rem;
		font-style: italic;
		line-height: 1.4;
	}
	/* Fixed 3 columns, not wrapping pills. Twelve titles flowed 2-per-row at
	   the phone floor — six rows of chrome above the form. Measured in
	   Spectral at 360 (row 311.6px, so ~100px cells): the longest label,
	   "The Spymaster", is 94.6px at 0.9rem and 89.3px at 0.85rem, neither of
	   which clears a cell once the pill's padding is paid — and 344 takes
	   another 5px off the cell. So the label wraps to a second line instead,
	   and the grid keeps every tile the height of the tallest. Same accepted
	   cost as the choosing-phase tile grid, four rows instead of six. */
	.title-chip-row {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.35rem;
	}
	.title-chip {
		display: flex;
		align-items: center;
		justify-content: center;
		min-width: 0;
		min-height: 44px;
		padding: 0.3rem 0.4rem;
		border-radius: 6px;
		border: 1px solid var(--color-neutral);
		background: var(--color-surface-2);
		color: var(--color-text);
		font-size: 0.85rem;
		line-height: 1.15;
		text-align: center;
		cursor: pointer;
	}
	.title-chip.active {
		border-color: var(--color-accent);
		background: var(--color-chip-gold-bg);
	}
	.title-chip:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
</style>
