<!--
  Retinue view for any player. Shows the player's assets as tiles with a
  2x2 marginalia sub-grid. The tile is the primary interactive surface;
  owner-only mutations (add/edit marginalia, rename, etc.) are hooked
  directly onto the tile parts.

  Every tile is a game piece (the warm border, the piece corner, the raised
  surface) and so is each marginalia tile inside it; that one level of nesting
  is all there is. Everything else is a heading, a line of words or spacing:
  the status of an asset (its type, "Leveraged", "Destroyed") is small capitals
  under its name, never a badge, so a line that does nothing when tapped is not
  drawn like a button.

  The AT-RISK COUNT lives here, not on the player row: a notice at the top
  says how many assets are one tear from destruction and still have a slot to
  fill, in words, for you and for anyone looking at your retinue. The empty
  slot to fill is still marked on the tile itself.

  The owner's shape sits beside the title, once, since every tile on this
  sheet has the same owner.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/rankStrip.css';
	import '$lib/components/shared/cornerBadge.css';
	import '$lib/components/shared/statusText.css';
	import '$lib/components/shared/marginaliaTile.css';
	import '$lib/components/shared/modalShell.css';
	import { addMarginalia, updateMarginalia, updateAsset, writeSecret, getAssetSuggestions } from '$lib/api';
	import type {
		Asset, Player, PresenceMember, PlayerActivity, Marginalium, Secret, Ranking,
	} from '$lib/api';
	import { lastActiveLabel, reminderLine, activityFor } from '$lib/playerActivity';
	import { isNeedlesslyAtRisk, firstEmptySlotIndex } from '$lib/assetRisk';
	import { atRiskCountByPlayer } from '$lib/tableHeader';
	import { knownCount, hiddenCount } from '$lib/secretCounts';
	import { useSuccession } from '$lib/successionContext';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import SuggestionPicker from './SuggestionPicker.svelte';
	import CrownGlyph from './CrownGlyph.svelte';
	import LogMark from './LogMark.svelte';
	import DieMark from '$lib/components/shared/DieMark.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import StarMark from '$lib/components/shared/StarMark.svelte';

	// Line-of-succession crown lookup (ADR-007). Undefined when no provider is
	// mounted → no crowns render.
	const succession = useSuccession();

	let {
		playerId,
		players,
		members,
		playerActivity = [],
		assets,
		secrets = [],
		rankings = [],
		viewerPlayerId,
		focusPlayerId = null,
		isWaitedOn = false,
		onSecretsChanged,
	}: {
		playerId: number;
		players: Player[];
		members: PresenceMember[];
		/** Durable presence + reminder reachability per seat. Empty until the
		    state fetch lands, which just means the status line reads from live
		    socket presence alone. */
		playerActivity?: PlayerActivity[];
		assets: Asset[];
		secrets?: Secret[];
		rankings?: Ranking[];
		viewerPlayerId: number | null;
		/** The player currently holding focus (their turn). Surfaced as a badge,
		    visible to all viewers, on that player's retinue. */
		focusPlayerId?: number | null;
		/** True when the game is waiting on this player. Gates the reminder
		    line: whether a ping can reach someone only changes what anyone
		    would *do* while they owe the table a move, and a permanent
		    "reminders on" label on every seat is noise. */
		isWaitedOn?: boolean;
		/** Called after the viewer writes a secret so the parent can refetch. */
		onSecretsChanged?: () => void;
	} = $props();

	function rankFor(category: 'power' | 'knowledge' | 'esteem'): number | null {
		const r = rankings.find(r => r.category === category && r.player_id === playerId);
		return r ? r.rank : null;
	}
	const playerRanks = $derived({
		power: rankFor('power'),
		knowledge: rankFor('knowledge'),
		esteem: rankFor('esteem'),
	});
	// Status is the inverse of rank (status = 6 − rank); it's the difficulty
	// others face when targeting this player, where rank is their own difficulty
	// when acting. The rules use both, so we surface both.
	function statusFromRank(rank: number | null): number | null {
		return rank == null ? null : 6 - rank;
	}
	const playerStatuses = $derived({
		power: statusFromRank(playerRanks.power),
		knowledge: statusFromRank(playerRanks.knowledge),
		esteem: statusFromRank(playerRanks.esteem),
	});
	const hasRanks = $derived(
		playerRanks.power != null || playerRanks.knowledge != null || playerRanks.esteem != null
	);

	const player = $derived(players.find(p => p.id === playerId) ?? null);
	const presence = $derived(members.find(m => m.id === playerId) ?? null);
	const isSelf = $derived(viewerPlayerId === playerId);

	// ── Presence & reminder lines ─────────────────────────────────────────────
	// `now` is captured once when the sheet opens rather than ticking: these are
	// hour-and-day buckets, and the sheet is a transient overlay, so a live
	// timer would burn a re-render a second to change nothing.
	const now = Date.now();
	const activity = $derived(activityFor(playerActivity, playerId));
	const presenceLabel = $derived(
		lastActiveLabel(
			{ lastActiveAt: activity?.last_active_at ?? null, online: presence?.online ?? false },
			now
		)
	);
	// Suppressed on your own retinue: you know perfectly well whether you are
	// here, and "reminders off" reads as a scold when it's aimed at yourself —
	// the Profile page is where you'd change it, and it says so there.
	const reminder = $derived(
		isWaitedOn && !isSelf && activity
			? reminderLine(
					{ reminder: activity.reminder, reminderDueAt: activity.reminder_due_at },
					now
				)
			: null
	);
	const isFocusPlayer = $derived(focusPlayerId != null && focusPlayerId === playerId);

	// Live assets only. Every count, MC-swap, and at-risk computation below
	// reads from this, so destroyed assets never affect gameplay-facing state.
	const ownedAssets = $derived(
		assets.filter(a => a.owner_id === playerId && !a.is_destroyed)
	);

	// Destroyed assets, grouped to the bottom of the list as read-only
	// "tombstone" cards. Purely visual — they feed nothing but the render loop.
	const destroyedAssets = $derived(
		assets
			.filter(a => a.owner_id === playerId && a.is_destroyed)
			.sort((a, b) => (a.destroyed_at ?? '').localeCompare(b.destroyed_at ?? ''))
	);

	// Render order: live assets first (created order), then tombstones.
	const displayAssets = $derived([...ownedAssets, ...destroyedAssets]);

	const leveragedCount = $derived(ownedAssets.filter(a => a.is_leveraged).length);

	// How many live assets are needlessly at risk: one tear from destruction
	// with a slot still free to fill (see isNeedlesslyAtRisk). This is where the
	// player row's old count went, and it comes from the same helper the row
	// used (atRiskCountByPlayer, which skips destroyed assets), so the two
	// can't disagree.
	const atRiskCount = $derived(atRiskCountByPlayer(ownedAssets).get(playerId) ?? 0);

	const assetTypeLabels: Record<Asset['asset_type'], string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};

	// Build a 4-slot marginalia array (filled by 1-indexed position, padded with null).
	function slotsFor(asset: Asset): (Marginalium | null)[] {
		const slots: (Marginalium | null)[] = [null, null, null, null];
		for (const m of asset.marginalia) {
			if (m.position >= 1 && m.position <= 4) slots[m.position - 1] = m;
		}
		return slots;
	}

	// ── Add / edit marginalia (owner only) ──────────────────────────────────
	// When set, the matching asset's marginalia grid is replaced by an editor.
	// editingPosition === null → adding new; otherwise editing that position.
	let editingAssetId = $state<number | null>(null);
	let editingPosition = $state<number | null>(null);
	let draftText = $state('');
	let saving = $state(false);
	let editError = $state<string | null>(null);

	// Type-keyed marginalia suggestions, fetched when the add editor opens and
	// refetched by the picker's reroll button.
	let addSuggestions = $state<string[]>([]);
	let suggestionsLoading = $state(false);
	let addRerolling = $state(false);

	async function loadAddSuggestions(asset: Asset) {
		try {
			const res = await getAssetSuggestions(asset.game_id, asset.asset_type, 'marginalia');
			// Ignore if the editor was closed or moved on while we were loading.
			if (editingAssetId === asset.id && editingPosition == null) {
				addSuggestions = res.suggestions;
			}
		} catch {
			// Leave the existing pool alone — clearing it collapses the example
			// row and takes the reroll button with it, permanently.
		}
	}

	async function startAdd(asset: Asset) {
		editingAssetId = asset.id;
		editingPosition = null;
		draftText = '';
		editError = null;
		addSuggestions = [];
		suggestionsLoading = true;
		try {
			await loadAddSuggestions(asset);
		} finally {
			if (editingAssetId === asset.id) suggestionsLoading = false;
		}
	}

	async function rerollAddSuggestions(asset: Asset) {
		addRerolling = true;
		try {
			await loadAddSuggestions(asset);
		} finally {
			addRerolling = false;
		}
	}

	function startEdit(asset: Asset, m: Marginalium) {
		editingAssetId = asset.id;
		editingPosition = m.position;
		draftText = m.text;
		editError = null;
	}

	function cancelEdit() {
		editingAssetId = null;
		editingPosition = null;
		draftText = '';
		editError = null;
	}

	async function saveEdit() {
		const assetId = editingAssetId;
		const text = draftText.trim();
		if (assetId == null || !text || saving) return;
		saving = true;
		editError = null;
		try {
			if (editingPosition == null) {
				await addMarginalia(assetId, text);
			} else {
				await updateMarginalia(assetId, editingPosition, text);
			}
			// WS events (MarginaliaAdded / MarginaliaUpdated) update the asset prop.
			cancelEdit();
		} catch (e) {
			editError = e instanceof Error ? e.message : 'Could not save.';
			saving = false;
			return;
		}
		saving = false;
	}

	// ── Rename asset (owner only) ───────────────────────────────────────────
	let renamingAssetId = $state<number | null>(null);
	let renameDraft = $state('');
	let renameSaving = $state(false);
	let renameError = $state<string | null>(null);

	function startRename(asset: Asset) {
		renamingAssetId = asset.id;
		renameDraft = asset.name;
		renameError = null;
	}

	function cancelRename() {
		renamingAssetId = null;
		renameDraft = '';
		renameError = null;
	}

	async function saveRename(asset: Asset) {
		const text = renameDraft.trim();
		if (renamingAssetId !== asset.id || !text || renameSaving) return;
		if (text === asset.name) { cancelRename(); return; }
		renameSaving = true;
		renameError = null;
		try {
			await updateAsset(asset.id, { name: text });
			cancelRename();
		} catch (e) {
			renameError = e instanceof Error ? e.message : 'Could not rename.';
		} finally {
			renameSaving = false;
		}
	}

	// ── Set as main character (owner, peers only) ──────────────────────────
	// Two-step flow: tap ☆ on a non-main peer → if old MC has untorn marginalia,
	// that tile shows a picker; tapping a marginalia fires the atomic swap.
	let mcSwapTo = $state<number | null>(null); // new MC asset id (target of swap)
	let mcSwapSaving = $state(false);
	let mcSwapError = $state<string | null>(null);

	const currentMC = $derived(ownedAssets.find(a => a.is_main_character) ?? null);

	function cancelMcSwap() {
		mcSwapTo = null;
		mcSwapSaving = false;
		mcSwapError = null;
	}

	async function startMcSwap(target: Asset) {
		if (mcSwapSaving) return;
		mcSwapError = null;
		// Edge case: no current MC, or current MC has no untorn marginalia → no
		// picker needed, fire directly.
		const old = currentMC;
		const untorn = old ? old.marginalia.filter(m => !m.is_torn) : [];
		if (!old || untorn.length === 0) {
			mcSwapSaving = true;
			try {
				await updateAsset(target.id, { is_main_character: true });
				cancelMcSwap();
			} catch (e) {
				mcSwapError = e instanceof Error ? e.message : 'Could not set main character.';
				mcSwapSaving = false;
			}
			return;
		}
		mcSwapTo = target.id;
	}

	async function confirmMcSwap(tearPosition: number) {
		if (mcSwapTo == null || mcSwapSaving) return;
		mcSwapSaving = true;
		mcSwapError = null;
		try {
			await updateAsset(mcSwapTo, { is_main_character: true, tear_position: tearPosition });
			cancelMcSwap();
		} catch (e) {
			mcSwapError = e instanceof Error ? e.message : 'Could not swap main character.';
			mcSwapSaving = false;
		}
	}

	// ── Secrets (any viewer, any asset) ─────────────────────────────────────
	let secretsOpenForAssetId = $state<number | null>(null);
	let newSecretText = $state('');
	let secretSaving = $state(false);
	let secretError = $state<string | null>(null);

	function secretsForAsset(assetId: number): Secret[] {
		return secrets.filter(s => s.asset_id === assetId);
	}

	// Secrets that exist on the asset but whose content this viewer can't read:
	// the public total minus the ones they can see. The open-eye button already
	// carries the readable count; this feeds the passive struck-eye beside it.
	function hiddenSecretsFor(asset: Asset): number {
		return hiddenCount(asset, knownCount(secrets, asset.id));
	}

	function toggleSecrets(assetId: number) {
		if (secretsOpenForAssetId === assetId) {
			secretsOpenForAssetId = null;
			newSecretText = '';
			secretError = null;
		} else {
			secretsOpenForAssetId = assetId;
			newSecretText = '';
			secretError = null;
		}
	}

	async function saveSecret(asset: Asset) {
		const text = newSecretText.trim();
		if (!text || secretSaving) return;
		secretSaving = true;
		secretError = null;
		try {
			await writeSecret(asset.id, text);
			newSecretText = '';
			onSecretsChanged?.();
		} catch (e) {
			secretError = e instanceof Error ? e.message : 'Could not write secret.';
		} finally {
			secretSaving = false;
		}
	}

	// Reset edit state when the player being viewed changes.
	$effect(() => {
		void playerId;
		cancelEdit();
		cancelRename();
		cancelMcSwap();
	});

	// Action: focus the textarea when it mounts (replaces autofocus attribute,
	// which Svelte's a11y rules discourage).
	function focusOnMount(node: HTMLElement) {
		node.focus();
	}
</script>

<div class="retinue-view">
	{#if player}
		<header class="retinue-header">
			<h2 class="sheet-title">
				<!-- Whose retinue: their shape, in their colour. The name is right
				     beside it, so the shape is decoration. -->
				<PlayerShape {player} size={16} />
				<span class="title-text">{isSelf ? 'Your Retinue' : `${player.display_name}'s Retinue`}</span>
			</h2>
			<p class="presence">
				<span class="dot" class:online={presence?.online}></span>
				<span class="status">{presenceLabel}</span>
			</p>
			{#if reminder}
				{#if reminder.unreachable}
					<Notice tone="warning">{reminder.text}</Notice>
				{:else}
					<p class="reminder-line">{reminder.text}</p>
				{/if}
			{/if}
			{#if isFocusPlayer || leveragedCount > 0}
				<p class="header-status">
					{#if isFocusPlayer}
						<span class="status-word" title="Focus player: they'll set the next scene and then prepare another plan">Sets next scene</span>
					{/if}
					{#if leveragedCount > 0}
						<span
							class="status-word lev-word"
							title={`${leveragedCount} leveraged ${leveragedCount === 1 ? 'asset' : 'assets'}: spent for a roll until refreshed`}
						>
							<DieMark size={14} />{leveragedCount} leveraged {leveragedCount === 1 ? 'asset' : 'assets'}
						</span>
					{/if}
				</p>
			{/if}
			{#if atRiskCount > 0}
				<!-- The at-risk count. Red is "something could be lost"; the words
				     say it too. The tear is the house mark for a note about to go. -->
				<Notice tone="danger">
					{#snippet mark()}<LogMark family="tear" />{/snippet}
					{#if isSelf}
						{atRiskCount} {atRiskCount === 1 ? 'asset needs' : 'assets need'} marginalia. Fill an empty slot, or one break could destroy {atRiskCount === 1 ? 'it' : 'them'}.
					{:else}
						{atRiskCount} {atRiskCount === 1 ? 'asset has' : 'assets have'} too few marginalia. {player.display_name} can still shore {atRiskCount === 1 ? 'it' : 'them'} up.
					{/if}
				</Notice>
			{/if}
		</header>

		{#if hasRanks}
			<section class="rank-strip" aria-label="Track rankings">
				{#each [
					{ label: 'Power', rank: playerRanks.power, status: playerStatuses.power },
					{ label: 'Knowledge', rank: playerRanks.knowledge, status: playerStatuses.knowledge },
					{ label: 'Esteem', rank: playerRanks.esteem, status: playerStatuses.esteem },
				] as track (track.label)}
					<div class="rank-cell">
						<span class="rank-label">{track.label}</span>
						<div class="rank-pair">
							<span class="rank-stat">
								<span class="rank-num">{track.rank ?? '—'}</span>
								<span class="rank-sublabel">Rank</span>
							</span>
							<span class="rank-stat">
								<span class="rank-num">{track.status ?? '—'}</span>
								<span class="rank-sublabel">Status</span>
							</span>
						</div>
					</div>
				{/each}
			</section>
		{/if}

		{#if displayAssets.length === 0}
			<p class="empty-note">No assets yet.</p>
		{:else}
			<ul class="asset-grid">
				{#each displayAssets as asset (asset.id)}
					<!-- A destroyed asset renders as a read-only "tombstone": all edit
					     affordances are suppressed (canEdit), the tile is outlined with a
					     dashed hairline, its words go faint and it says "Destroyed" in red.
					     It's visual-only — it affects no counts. -->
					{@const dead = asset.is_destroyed}
					{@const canEdit = isSelf && !dead}
					<!-- Owner-only nudge: if this asset is one tear from destruction
					     but a slot is still fillable, flag the first empty slot to fix. -->
					{@const atRiskSlot = canEdit && isNeedlesslyAtRisk(asset) ? firstEmptySlotIndex(asset) : null}
					<li
						class="asset-tile"
						class:main-char={asset.is_main_character}
						class:leveraged={asset.is_leveraged}
						class:destroyed={dead}
						aria-label={dead ? `${asset.name} — destroyed` : undefined}
					>
						<div class="tile-head">
							{#if renamingAssetId === asset.id}
								<input
									class="rename-input"
									type="text"
									bind:value={renameDraft}
									disabled={renameSaving}
									maxlength={TEXT_LIMITS.NAME}
									onblur={() => saveRename(asset)}
									onkeydown={(e) => {
										if (e.key === 'Escape') { e.preventDefault(); cancelRename(); }
										else if (e.key === 'Enter') { e.preventDefault(); saveRename(asset); }
									}}
									use:focusOnMount
								/>
							{:else if canEdit}
								<button type="button" class="asset-name editable" onclick={() => startRename(asset)} aria-label={`Rename ${asset.name}`}>
									{asset.name}
								</button>
							{:else}
								<span class="asset-name">
									{asset.name}
								</span>
							{/if}
							<!-- The controls and the secret indicators, together at the right
							     edge. Every control is a 44px square; the passive ones are not. -->
							<div class="tile-head-meta">
							{#if asset.is_main_character}
								<!-- Main character: the filled star. Passive — the MC is changed
								     by the outline-star toggle on another peer, not by un-setting
								     it here. -->
								<span class="hi main-star" title="Main character">
									<StarMark size={18} label="Main character" />
								</span>
							{:else if canEdit && asset.asset_type === 'peer' && renamingAssetId !== asset.id && mcSwapTo == null}
								<!-- Make-main-character toggle: the outline star (reads as "set
								     this"), the same size as the filled one. -->
								<button
									type="button"
									class="hi ctl mc-toggle"
									onclick={() => startMcSwap(asset)}
									disabled={mcSwapSaving}
									aria-label={`Make ${asset.name} the main character`}
									title="Make main character"
								>
									<StarMark size={18} outline />
								</button>
							{/if}
							{#if !dead && (isSelf || secretsForAsset(asset.id).length > 0)}
								<!-- Open (known) eye. On your own assets it's the
								     write-a-secret control, prefixed with an inline "+" (the
								     same add convention as the marginalia "+" tiles). On others'
								     assets it's read-only and only appears when you can actually
								     read something. The readable count rides the eye's corner. -->
								<button
									type="button"
									class="hi ctl secrets-btn"
									class:active={secretsOpenForAssetId === asset.id}
									onclick={() => toggleSecrets(asset.id)}
									aria-label={isSelf
										? (secretsForAsset(asset.id).length > 0 ? `Write or view ${secretsForAsset(asset.id).length} secret(s)` : 'Write a secret')
										: `View ${secretsForAsset(asset.id).length} secret(s)`}
									title={isSelf ? 'Write a secret' : 'Secrets you can read'}
								>
									{#if isSelf}<span class="secrets-plus" aria-hidden="true">+</span>{/if}
									<span class="eye">
										<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
											<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
											<circle cx="12" cy="12" r="3" />
										</svg>
										{#if secretsForAsset(asset.id).length > 0}<span class="corner-badge known">{secretsForAsset(asset.id).length}</span>{/if}
									</span>
								</button>
							{:else if dead && secretsForAsset(asset.id).length > 0}
								<!-- Tombstone: the readable-secret count survives as a passive,
								     non-clickable record of secrets lost with the asset. -->
								<span
									class="hi secrets-static"
									title={`${secretsForAsset(asset.id).length} secret${secretsForAsset(asset.id).length === 1 ? '' : 's'} lost with this asset`}
									aria-label={`${secretsForAsset(asset.id).length} secrets lost with this asset`}
								>
									<span class="eye">
										<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
											<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
											<circle cx="12" cy="12" r="3" />
										</svg>
										<span class="corner-badge known">{secretsForAsset(asset.id).length}</span>
									</span>
								</span>
							{/if}
							{#if hiddenSecretsFor(asset) > 0}
								<!-- Struck eye: secrets that exist here but are hidden from
								     this viewer (total − readable). Passive; its count rides the
								     corner like the open eye's, so both eyes share one footprint. -->
								<span
									class="hi hidden-secrets"
									title={`${hiddenSecretsFor(asset)} secret${hiddenSecretsFor(asset) === 1 ? '' : 's'} hidden from you`}
									aria-label={`${hiddenSecretsFor(asset)} secrets hidden from you`}
								>
									<span class="eye">
										<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
											<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
											<circle cx="12" cy="12" r="3" />
											<line x1="3" y1="21" x2="21" y2="3" />
										</svg>
										<span class="corner-badge hidden">{hiddenSecretsFor(asset)}</span>
									</span>
								</span>
							{/if}
							</div>
						</div>
						<!-- What it is and where it stands, as words: the type, then
						     "Leveraged" in orange beside the die, or "Destroyed" in red. -->
						<p class="tile-status">
							<span class="status-word">{assetTypeLabels[asset.asset_type]}</span>
							{#if asset.is_leveraged}
								<span class="status-word lev-word" title="Spent for a roll until refreshed">
									<DieMark size={14} />Leveraged
								</span>
							{/if}
							{#if dead}
								<span class="status-word danger">Destroyed</span>
							{/if}
						</p>
						{#if renamingAssetId === asset.id && renameError}
							<ErrorText message={renameError} />
						{/if}
						{#if mcSwapTo != null && currentMC && asset.id === currentMC.id}
							<div class="mc-picker">
								<p class="m-editor-label">
									Pick a marginalia to break on {asset.name}
								</p>
								<div class="mc-picker-list">
									{#each slotsFor(asset) as slot, i (i)}
										{#if slot && !slot.is_torn}
											<button type="button" class="mc-picker-item" disabled={mcSwapSaving} onclick={() => confirmMcSwap(slot.position)}>
												<span class="m-pos">{i + 1}.</span>
												<span class="m-tile-text">{slot.text}</span>
											</button>
										{/if}
									{/each}
								</div>
								{#if mcSwapError}<ErrorText message={mcSwapError} />{/if}
								<div class="m-editor-actions">
									<button type="button" class="action-btn secondary" onclick={cancelMcSwap} disabled={mcSwapSaving}>Cancel</button>
								</div>
							</div>
						{:else if editingAssetId === asset.id}
							<div class="m-editor">
								<p class="m-editor-label">
									{editingPosition == null ? 'Adding marginalia' : `Editing marginalia ${editingPosition} of 4`}
								</p>
								{#if editingPosition == null}
									<!-- Add: offer type-keyed examples, or write your own. -->
									<SuggestionPicker
										suggestions={addSuggestions}
										bind:value={draftText}
										loading={suggestionsLoading}
										placeholder="Write a marginalia…"
										multiline
										onReroll={() => rerollAddSuggestions(asset)}
										rerolling={addRerolling}
										disabled={saving}
									/>
								{:else}
									<textarea
										class="m-editor-input"
										placeholder="Write a marginalia…"
										bind:value={draftText}
										disabled={saving}
										rows={3}
										maxlength={TEXT_LIMITS.MARGINALIA}
										onkeydown={(e) => {
											if (e.key === 'Escape') { e.preventDefault(); cancelEdit(); }
											else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); saveEdit(); }
										}}
										use:focusOnMount
									></textarea>
								{/if}
								{#if editError}<ErrorText message={editError} />{/if}
								<div class="m-editor-actions">
									<button type="button" class="action-btn secondary" onclick={cancelEdit} disabled={saving}>Cancel</button>
									<button type="button" class="action-btn primary" onclick={saveEdit} disabled={saving || !draftText.trim()}>
										{saving ? '…' : 'Save'}
									</button>
								</div>
							</div>
						{:else}
							<div class="m-grid">
								{#each slotsFor(asset) as slot, i (i)}
									{#if slot}
										{@const crown = slot.title ? succession?.crown(slot.id) : undefined}
										{#if canEdit && !slot.is_torn}
											<button type="button" class="m-tile filled" class:titled={!!crown} onclick={() => startEdit(asset, slot)} aria-label={`Edit marginalia ${slot.position}`}>
												<span class="m-tile-text">{slot.text}</span>
												{#if crown}<CrownGlyph mark={crown} size={14} />{/if}
											</button>
										{:else}
											<div class="m-tile" class:torn={slot.is_torn} class:titled={!!crown}>
												<span class="m-tile-text">{slot.text}</span>
												{#if crown}<CrownGlyph mark={crown} size={14} />{/if}
											</div>
										{/if}
									{:else if canEdit}
										<button
											type="button"
											class="m-tile empty add"
											class:at-risk={i === atRiskSlot}
											onclick={() => startAdd(asset)}
											aria-label={i === atRiskSlot ? `Add marginalia to ${asset.name} — it has too few notes and could be destroyed` : 'Add marginalia'}
											title={i === atRiskSlot ? 'Too few notes — add one so a single break can’t destroy this asset' : undefined}
										>
											<span class="add-plus" aria-hidden="true">+</span>
										</button>
									{:else}
										<div class="m-tile empty" aria-label="empty marginalia slot"></div>
									{/if}
								{/each}
							</div>
						{/if}
						{#if secretsOpenForAssetId === asset.id}
							<div class="secrets-panel">
								<p class="m-editor-label">Secrets</p>
								{#if secretsForAsset(asset.id).length === 0}
									<p class="empty-note small">No secrets visible to you.</p>
								{:else}
									<ul class="secrets-list">
										{#each secretsForAsset(asset.id) as s (s.id)}
											<li class="secret-row">
												<span class="secret-text">{s.text}</span>
												{#if s.author_id === viewerPlayerId}<span class="status-word">yours</span>{/if}
											</li>
										{/each}
									</ul>
								{/if}
								{#if isSelf}
									<textarea
										class="m-editor-input"
										placeholder="Write a secret on this asset. Secrets can be anything, as long as:
	1. It doesn't contradict anything already established
	2. Your character has the ability to know or make true
	3. It doesn't take away agency from another player's character."
										bind:value={newSecretText}
										disabled={secretSaving}
										rows={4}
										maxlength={TEXT_LIMITS.NARRATIVE}
										onkeydown={(e) => {
											if (e.key === 'Escape') { e.preventDefault(); toggleSecrets(asset.id); }
											else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); saveSecret(asset); }
										}}
									></textarea>
									{#if secretError}<ErrorText message={secretError} />{/if}
								{/if}
								<div class="m-editor-actions">
									<button type="button" class="action-btn secondary" onclick={() => toggleSecrets(asset.id)} disabled={secretSaving}>Close</button>
									{#if isSelf}
										<button type="button" class="action-btn primary" onclick={() => saveSecret(asset)} disabled={secretSaving || !newSecretText.trim()}>
											{secretSaving ? '…' : 'Add secret'}
										</button>
									{/if}
								</div>
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	{:else}
		<p class="empty-note">Player not found.</p>
	{/if}
</div>

<style>
	.retinue-view {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.retinue-header {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* The shared sheet title (deep gold, heading size, 44px tall, clear of the
	   close button) with the owner's shape in front. The text is what gives way
	   when a name runs long: display names can run to 40 characters. */
	.retinue-header .sheet-title {
		gap: var(--space-2);
		min-width: 0;
	}
	.title-text {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Who is here: a status dot and the words beside it. The dot is the one
	   circle on the sheet (a circle means "status", never a player). */
	.presence {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0;
	}
	.status {
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	.dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: var(--radius-dot);
		background: var(--color-neutral);
	}
	.dot.online { background: var(--color-success); }

	/* Reminder reachability, rendered only while the table is waiting on this
	   player (see the `reminder` derivation). Two tiers, because the two cases
	   want different weights: "reminder due in ~6h" is reassurance and stays in
	   plain secondary text, while "reminders off" / "no device" is the one thing
	   on this sheet that asks the reader to go do something outside the app, so
	   it is a warning notice (orange: careful, not lost). */
	.reminder-line {
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}

	/* The retinue's status words: "Sets next scene" and the leveraged count.
	   Small capitals, no box. */
	.header-status {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		column-gap: var(--space-4);
		row-gap: var(--space-1);
		margin: 0;
	}

	/* Leveraged: orange, beside the die and its words. The status-word recipe
	   with the leveraged colour. */
	.lev-word {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		color: var(--color-leveraged);
	}

	/* Italic prose placeholders ("No assets yet."). Named -note, not .empty:
	   as .empty it collided with `.m-tile.empty`, and the marginalia tiles were
	   silently inheriting `font-style: italic` — which slanted the "+" in every
	   empty slot. Same class of leak as the `.muted` one the July 2026 UI audit
	   caught; keep placeholder-prose classes off single-word names that a state
	   modifier might also want. */
	.empty-note {
		color: var(--color-text-faint);
		font-size: var(--text-small);
		font-style: italic;
		margin: 0;
	}

	/* ── Asset tiles ─────────────────────────────────────────────────────── */

	.asset-grid {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}

	/* An asset is a game piece: the warm border, the piece corner, the raised
	   surface. */
	.asset-tile {
		background: var(--color-surface);
		border: 1px solid var(--color-border-piece);
		border-radius: var(--radius-piece);
		padding: var(--space-3);
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* Tombstone: a destroyed asset kept visible as a read-only record. It drops
	   to a dashed neutral hairline and faint words (text stays legible: nothing
	   is faded with opacity) and carries the word "Destroyed" in red. The big
	   cross it used to wear ran through that word and the notes, so it is gone.
	   All edit affordances are already suppressed in markup via canEdit;
	   pointer-events is a backstop. */
	.asset-tile.destroyed {
		border: 1px dashed var(--color-border);
		color: var(--color-text-faint);
		pointer-events: none;
	}
	.asset-tile.destroyed .asset-name { color: var(--color-text-faint); }
	.asset-tile.destroyed .m-tile { border-color: var(--color-border); }
	/* The main character is distinguished by the filled star in its tile head, so
	   the warm border is shared by every asset tile. Leveraged tiles carry the
	   die and "Leveraged" under the name as their only "exhausted" cue. */

	.tile-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--space-2);
	}
	/* A head that holds a control is as tall as the control (the 44px touch
	   minimum); a read-only head is only as tall as the name. */
	.tile-head:has(.ctl, button.asset-name, .rename-input) { min-height: var(--size-tap); }

	/* The controls and secret indicators, kept together and pushed to the right
	   edge. */
	.tile-head-meta {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		flex-shrink: 0;
	}

	.asset-name {
		font-size: var(--text-body);
		color: var(--color-text);
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* The name is a button for its owner (tap to rename). It has no box at rest,
	   so it reads as the title it is; the border appears on hover and the ring on
	   focus. 44px tall like every pressable thing. */
	button.asset-name.editable {
		background: none;
		border: 1px solid transparent;
		padding: 0 var(--space-2);
		margin: 0 calc(-1 * var(--space-2));
		min-height: var(--size-tap);
		border-radius: var(--radius-control);
		font-family: inherit;
		text-align: left;
		cursor: pointer;
		max-width: 100%;
	}
	button.asset-name.editable:hover { border-color: var(--color-border-control-hover); background: var(--color-surface-hover); }
	button.asset-name.editable:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }

	/* The type, "Leveraged" and "Destroyed", as words under the name. */
	.tile-status {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		column-gap: var(--space-3);
		row-gap: 0;
		margin: 0;
	}

	/* ── Tile-head controls and indicators ───────────────────────────────────
	   Every pressable thing is a 44px square: the outline star (make main
	   character) and the eye (write or read secrets). They are controls, so they
	   take a control's look: the neutral border, the control corner and the
	   raised-twice fill (they stand on a raised tile). The passive ones (the
	   filled star, the struck eye) are plain glyphs with no box, so nothing that
	   can't be pressed looks like something that can. */
	.hi {
		position: relative;
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.ctl {
		min-width: var(--size-tap);
		min-height: var(--size-tap);
		padding: 0;
		box-sizing: border-box;
		background: var(--color-surface-2);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		cursor: pointer;
	}
	.ctl:hover:not(:disabled) { border-color: var(--color-border-control-hover); }
	.ctl:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }
	.ctl:disabled { color: var(--color-text-faint); cursor: not-allowed; }
	/* The corner badge rides the glyph, not the 44px box around it. */
	.eye { position: relative; display: inline-flex; }

	/* Main character: the filled star, deep gold (the game's colour). */
	.main-star { color: var(--color-accent); }

	/* Make-main-character toggle: the outline star, quiet at rest. */
	.mc-toggle { color: var(--color-text-secondary); }
	.mc-toggle:hover:not(:disabled) { color: var(--color-accent); }

	/* Writable/readable secrets eye — an inline "+" beside the eye. */
	.secrets-btn {
		gap: var(--space-1);
		padding: 0 var(--space-2);
		color: var(--color-accent);
	}
	.secrets-btn:hover { color: var(--color-accent-hover); }
	/* The open panel: the control stays pressed, shown by its border. */
	.secrets-btn.active { border-color: var(--color-border-control-hover); }
	/* Inline "+" prefix on the writable (own-asset) eye. */
	.secrets-plus { font-size: var(--text-body); font-weight: 600; line-height: 1; }

	/* Tombstone's passive readable-secret eye — same gold as the live eye, but
	   a plain span (no box, no hover), a record rather than a control. */
	.secrets-static { color: var(--color-accent); }

	/* Struck eye: quiet, to read as "not available to you" against the gold eye. */
	.hidden-secrets { color: var(--color-text-secondary); }

	/* Secrets panel (expanded under the marginalia area): a line above, a heading
	   and plain entries separated by lines. Read-only entries are not boxes. */
	.secrets-panel {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding-top: var(--space-2);
		border-top: 1px solid var(--color-border);
	}

	.secrets-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
	}

	.secret-row {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: var(--space-2);
		padding: var(--space-2) 0;
		border-bottom: 1px solid var(--color-border);
		font-size: var(--text-small);
		line-height: 1.4;
		color: var(--color-text-secondary);
	}
	.secret-row:last-child { border-bottom: 0; }

	.secret-text { flex: 1; word-break: break-word; }

	.empty-note.small { font-size: var(--text-small); padding: var(--space-1) 0; }

	/* Pick-a-marginalia-to-break picker (replaces grid on old MC during swap).
	   Which note to break is a choice, so it wears no meaning colour: the red
	   it used to take on hover is gone, and the label above says what it does. */
	.mc-picker {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.mc-picker-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.mc-picker-item {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
		text-align: left;
		min-height: var(--size-tap);
		padding: var(--space-2) var(--space-3);
		background: var(--color-surface);
		border: 1px solid var(--color-border-piece);
		border-radius: var(--radius-piece);
		font-family: inherit;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		cursor: pointer;
	}
	.mc-picker-item:hover:not(:disabled) { background: var(--color-surface-hover); }
	.mc-picker-item:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }
	.mc-picker-item:disabled { color: var(--color-text-faint); cursor: not-allowed; }

	.mc-picker-item .m-pos {
		flex-shrink: 0;
		color: var(--color-accent);
	}

	/* ── Marginalia 2×2 grid ─────────────────────────────────────────────── */

	.m-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-2);
	}

	/* Base look (background/border/color/torn/titled/empty) comes from
	   shared/marginaliaTile.css; this is just the real grid's tap-target
	   sizing and clipping, which the static HelpContent replica doesn't need. */
	.m-tile {
		min-height: var(--size-tap);
		overflow: hidden;
	}

	/* Three lines: at the 14px note size a tile holds about 16 characters a
	   line, so two lines cut off the notes players actually write. */
	.m-tile-text {
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
		word-break: break-word;
	}

	/* "+" add affordance on owner empty slots */
	.m-tile.empty.add {
		justify-content: center;
		color: var(--color-text-faint);
		cursor: pointer;
		font-family: inherit;
		font-size: inherit;
	}
	.m-tile.empty.add:hover { color: var(--color-text); background: var(--color-surface-hover); }
	.m-tile.empty.add:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }
	.add-plus { font-size: var(--text-title); line-height: 1; }

	/* Needlessly-at-risk nudge: solid red rim + red "+" on the next fillable
	   slot. Red is "something could be lost"; the notice at the top of the sheet
	   says it in words. Hover/focus keep the red so the warning doesn't disappear
	   mid-interaction. */
	.m-tile.empty.add.at-risk {
		border-style: solid;
		border-color: var(--color-danger-muted);
		color: var(--color-danger);
	}
	.m-tile.empty.add.at-risk:hover { color: var(--color-danger); }

	/* Owner edit affordance on filled (untorn) slots */
	.m-tile.filled {
		text-align: left;
		font-family: inherit;
		color: var(--color-text-secondary);
		cursor: pointer;
	}
	.m-tile.filled:hover { background: var(--color-surface-hover); }
	.m-tile.filled:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }

	/* ── Inline marginalia editor (replaces grid while active) ───────────── */

	.m-editor {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* A small-capitals label: the heading of whatever opens under the tile. */
	.m-editor-label {
		font-size: var(--text-label);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-accent);
		margin: 0;
	}

	/* An input: the control corner, a neutral border and the raised-twice fill
	   (it stands on a raised tile), at the 16px body size iOS needs to not zoom. */
	.m-editor-input,
	.rename-input {
		font-family: inherit;
		font-size: var(--text-body);
		color: var(--color-text);
		background: var(--color-surface-2);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
	}
	.m-editor-input {
		width: 100%;
		box-sizing: border-box;
		line-height: 1.4;
		padding: var(--space-2) var(--space-3);
		resize: vertical;
		min-height: 84px;
	}
	.rename-input {
		flex: 1;
		min-width: 0;
		min-height: var(--size-tap);
		padding: var(--space-1) var(--space-3);
	}
	.m-editor-input:focus,
	.rename-input:focus { outline: 2px solid var(--color-you); outline-offset: 2px; }

	.m-editor-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
	}

</style>
