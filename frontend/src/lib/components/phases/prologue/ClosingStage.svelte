<!-- ClosingStage.svelte
  "The Stage is Set" — the prologue's closing step (game.prologue_ranking_step
  === 'closing'; adr/PROLOGUE_CLOSING_STAGE_PLAN.md). Reads recap-first,
  checklist-second (the ceremony framing): a review of the prologue's outcome
  (final standings, laws & rumors, retinue tallies), then the checklist of hard
  items (must complete before Ready) and soft nudges, then the ready
  roster/toggle that drives the all-ready auto-advance into the Main Event.
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
		myAtRiskCount,
		blankAssets,
		retinueTallies,
		RETINUE_TYPE_ORDER,
		type RetinueTally,
	} from '$lib/prologue/closing';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import { playerColorByID } from '$lib/playerColor';
	import TrackBoard from './TrackBoard.svelte';
	import AssetTypeIcon from '$lib/components/AssetTypeIcon.svelte';
	import AssetCreationForm from '$lib/components/AssetCreationForm.svelte';
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

	// The row's aria-label restates the counts because a labelled button hides
	// its inner text from assistive tech.
	const TALLY_TYPE_LABELS: Record<Asset['asset_type'], string> = {
		peer: 'Peers',
		artifact: 'Artifacts',
		resource: 'Resources',
		holding: 'Holdings',
	};
	function tallyRowLabel(t: RetinueTally): string {
		const counts = RETINUE_TYPE_ORDER.map((type) => `${TALLY_TYPE_LABELS[type]} ${t.counts[type]}`).join(', ');
		const you = t.playerID === currentPlayerID ? ' (you)' : '';
		const taken = t.takenFromOthers > 0 ? `; ${t.takenFromOthers} taken from others` : '';
		return `View ${playerName(t.playerID)}'s retinue${you} — ${counts}${taken}`;
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
			<ul class="recap-retinue">
				{#each tallies as t (t.playerID)}
					<li>
						<button
							type="button"
							class="retinue-row"
							onclick={() => onOpenRetinue?.(t.playerID)}
							aria-label={tallyRowLabel(t)}
						>
							<span class="retinue-name">
								<span class="retinue-dot" style:background={playerColorByID(t.playerID, players)} aria-hidden="true"></span>
								<span class="retinue-name-text">{playerName(t.playerID)}</span>
								{#if t.playerID === currentPlayerID}<span class="retinue-you">you</span>{/if}
							</span>
							<span class="retinue-counts">
								{#each RETINUE_TYPE_ORDER as type}
									<span class="retinue-count" class:zero={t.counts[type] === 0}>
										<AssetTypeIcon {type} size={14} />
										<span class="retinue-count-num">{t.counts[type]}</span>
									</span>
								{/each}
							</span>
							{#if t.takenFromOthers > 0}
								<span class="retinue-taken" title="Assets taken from other players during the prologue">
									{t.takenFromOthers} taken
								</span>
							{/if}
						</button>
					</li>
				{/each}
			</ul>
		</div>
	</section>

	<h3 class="section-heading">Put your house in order</h3>

	<div class="checklist">
		<section class="check-card" class:done={mcNamed}>
			<div class="check-head">
				<span class="check-mark" aria-hidden="true">{mcNamed ? '✓' : '○'}</span>
				<h3 class="check-title">Name your main character</h3>
			</div>
			{#if mcNamed}
				<p class="check-body">{myMainCharacter?.name}</p>
			{:else}
				<p class="check-body">Currently: {myMainCharacter?.name ?? '—'}</p>
				<div class="check-form">
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
			{/if}
		</section>

		{#if needsPeer}
			<section class="check-card" class:done={myExtraPeer != null}>
				<div class="check-head">
					<span class="check-mark" aria-hidden="true">{myExtraPeer ? '✓' : '○'}</span>
					<h3 class="check-title">Create an extra peer</h3>
				</div>
				{#if myExtraPeer}
					<p class="check-body">You created your extra peer: {myExtraPeer.title_name}.</p>
				{:else}
					<p class="check-body">
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
				{/if}
			</section>
		{/if}

		<section class="check-card" class:done={myBlankAssets.length === 0}>
			<div class="check-head">
				<span class="check-mark" aria-hidden="true">{myBlankAssets.length === 0 ? '✓' : '○'}</span>
				<h3 class="check-title">Give every asset a marginalia</h3>
			</div>
			{#if myBlankAssets.length === 0}
				<p class="check-body">Everything in your retinue carries a note.</p>
			{:else}
				<p class="check-body">
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
			{/if}
		</section>

		{#if riskCount > 0}
			<section class="check-card soft at-risk">
				<div class="check-head">
					<h3 class="check-title-soft">Shore up at-risk assets</h3>
				</div>
				<p class="check-body">
					{riskCount} of your assets {riskCount === 1 ? 'is' : 'are'} one tear from destruction.
				</p>
				<button class="action-btn secondary small" onclick={() => onOpenRetinue?.()}>Open Retinue</button>
			</section>
		{/if}

		<section class="check-card soft tones">
			<div class="check-head">
				<h3 class="check-title-soft">Tones — last chance</h3>
			</div>
			<p class="check-body">Tones lock when the Main Event begins.</p>
			<button class="action-btn secondary small" onclick={onOpenTones}>Open Tones</button>
		</section>
	</div>

	<h3 class="ready-heading">Ready roster</h3>
	<ul class="ready-roster">
		{#each players as p}
			{@const ready = isReady(closingReady, p.id)}
			<li class:ready>
				<span class="ready-roster-name">{p.display_name}</span>
				{#if ready}
					<span class="ready-roster-status done">✓ ready</span>
				{:else}
					<span class="ready-roster-status pending">waiting…</span>
				{/if}
			</li>
		{/each}
	</ul>

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

	/* Retinue tallies */
	.recap-retinue {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.recap-retinue li {
		display: flex;
	}
	.retinue-row {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-height: 44px;
		padding: 0.3rem 0.5rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-surface-2);
		border-radius: 4px;
		font-family: inherit;
		font-size: 0.82rem;
		color: inherit;
		text-align: left;
		cursor: pointer;
	}
	.retinue-row:hover {
		background: color-mix(in srgb, var(--color-surface-sunken) 92%, white);
		border-color: color-mix(in srgb, var(--color-surface-2) 75%, white);
	}
	.retinue-row:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
	.retinue-name {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		flex: 1 1 6rem;
		min-width: 0;
	}
	.retinue-dot {
		width: 0.5rem;
		height: 0.5rem;
		border-radius: 50%;
		flex: none;
	}
	.retinue-name-text {
		color: var(--color-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.retinue-you {
		flex: none;
		color: var(--color-text-muted);
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.retinue-counts {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex: none;
	}
	.retinue-count {
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
		color: var(--color-text-secondary);
	}
	.retinue-count.zero {
		opacity: 0.4;
	}
	.retinue-count-num {
		font-weight: 600;
		font-size: 0.78rem;
		min-width: 0.6rem;
	}
	/* Blue chip trio, matching the choosing view's steal ring — a take is an
	   opportunity, not a warning (this chip was orange to match the ring's old
	   colour, and followed it here when the ring moved on 2026-08-01). */
	.retinue-taken {
		flex: none;
		background: var(--color-chip-blue-bg);
		border: 1px solid var(--color-chip-blue-border);
		color: var(--color-chip-blue-text);
		font-size: 0.68rem;
		padding: 0.1rem 0.4rem;
		border-radius: 999px;
		white-space: nowrap;
	}

	.section-heading {
		color: var(--color-accent);
		font-size: 1rem;
		margin: 0.35rem 0 0;
	}

	.checklist {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.check-card {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		padding: 0.65rem 0.7rem;
	}
	/* Hard items (name your MC / extra peer): default border until complete,
	   then the same success-green the declare-track done-btn uses. */
	.check-card.done { border-color: var(--color-chip-green-border); }

	/* Soft items: red for the at-risk nudge (style guide — "red is danger,
	   which includes the at-risk game state"), orange for Tones (the one
	   "careful now, this locks soon" family). */
	.check-card.soft.at-risk {
		border-color: var(--color-danger-muted);
		background: color-mix(in srgb, var(--color-danger-muted) 12%, var(--color-surface-sunken));
	}
	.check-card.soft.tones {
		border-color: var(--color-warning-border);
		background: color-mix(in srgb, var(--color-warning-border) 12%, var(--color-surface-sunken));
	}

	.check-head {
		display: flex;
		align-items: baseline;
		gap: 0.45rem;
	}
	.check-mark {
		color: var(--color-text-muted);
		font-size: 0.95rem;
		line-height: 1;
	}
	.check-card.done .check-mark { color: var(--color-success); }
	.check-title {
		margin: 0;
		color: var(--color-accent);
		font-size: 0.95rem;
	}
	.check-title-soft {
		margin: 0;
		color: var(--color-text);
		font-size: 0.95rem;
	}
	.check-body {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.85rem;
		line-height: 1.4;
	}
	.check-form {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: end;
	}
	.check-form input {
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

	.ready-heading { color: var(--color-accent); font-size: 1rem; margin: 0; }

	.ready-roster {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		max-width: 24rem;
	}
	.ready-roster li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
		padding: 0.35rem 0.5rem;
		min-height: 44px;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-surface-2);
		border-radius: 4px;
		font-size: 0.85rem;
	}
	.ready-roster li.ready { border-color: var(--color-chip-green-border); }
	.ready-roster-name { color: var(--color-text); }
	.ready-roster-status.done { color: var(--color-success); font-size: 0.8rem; }
	.ready-roster-status.pending { color: var(--color-text-faint); font-size: 0.8rem; font-style: italic; }

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
