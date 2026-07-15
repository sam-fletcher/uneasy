<!--
  Retinue view for any player. Shows the player's assets as tiles with a
  2x2 marginalia sub-grid. The tile is the primary interactive surface;
  owner-only mutations (add/edit marginalia, rename, etc.) are hooked
  directly onto the tile parts.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/rankStrip.css';
	import '$lib/components/shared/cornerBadge.css';
	import '$lib/components/shared/statusText.css';
	import '$lib/components/shared/marginaliaTile.css';
	import { addMarginalia, updateMarginalia, updateAsset, writeSecret, getAssetSuggestions } from '$lib/api';
	import type { Asset, Player, PresenceMember, Marginalium, Secret, Ranking } from '$lib/api';
	import { isNeedlesslyAtRisk, firstEmptySlotIndex } from '$lib/assetRisk';
	import { knownCount, hiddenCount } from '$lib/secretCounts';
	import { useSuccession } from '$lib/successionContext';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import SuggestionPicker from './SuggestionPicker.svelte';
	import CrownGlyph from './CrownGlyph.svelte';

	// Line-of-succession crown lookup (ADR-007). Undefined when no provider is
	// mounted → no crowns render.
	const succession = useSuccession();

	let {
		playerId,
		players,
		members,
		assets,
		secrets = [],
		rankings = [],
		viewerPlayerId,
		focusPlayerId = null,
		onSecretsChanged,
	}: {
		playerId: number;
		players: Player[];
		members: PresenceMember[];
		assets: Asset[];
		secrets?: Secret[];
		rankings?: Ranking[];
		viewerPlayerId: number | null;
		/** The player currently holding focus (their turn). Surfaced as a badge,
		    visible to all viewers, on that player's retinue. */
		focusPlayerId?: number | null;
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

	// Type-keyed marginalia suggestions, fetched when the add editor opens.
	let addSuggestions = $state<string[]>([]);
	let suggestionsLoading = $state(false);

	async function startAdd(asset: Asset) {
		editingAssetId = asset.id;
		editingPosition = null;
		draftText = '';
		editError = null;
		addSuggestions = [];
		suggestionsLoading = true;
		try {
			const res = await getAssetSuggestions(asset.game_id, asset.asset_type, 'marginalia');
			// Ignore if the editor was closed or moved on while we were loading.
			if (editingAssetId === asset.id && editingPosition == null) {
				addSuggestions = res.suggestions;
			}
		} catch {
			addSuggestions = [];
		} finally {
			if (editingAssetId === asset.id) suggestionsLoading = false;
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
			<div class="header-title-row">
				<h2>{isSelf ? 'Your Retinue' : `${player.display_name}'s Retinue`}</h2>
				<span class="dot" class:online={presence?.online}></span>
				<span class="status">{presence?.online ? 'online' : 'offline'}</span>
				<!-- {#if player.is_facilitator}
					<span class="tag">facilitator</span>
				{/if} -->
			</div>
			<div class="header-badges">
				{#if isFocusPlayer}
					<span class="focus-badge" title="Focus player — they'll set the next scene and then prepare another plan">
						Sets next scene
					</span>
				{/if}
				<span
					class="leveraged-badge"
					class:zero={leveragedCount === 0}
					title={leveragedCount === 0
						? 'No assets leveraged'
						: `${leveragedCount} leveraged ${leveragedCount === 1 ? 'asset' : 'assets'} — leveraged for a roll until refreshed`}
				>
					<span class="leveraged-count">{leveragedCount}</span> leveraged {leveragedCount === 1 ? 'asset' : 'assets'}
				</span>
			</div>
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
			<p class="empty">No assets yet.</p>
		{:else}
			<ul class="asset-grid">
				{#each displayAssets as asset (asset.id)}
					<!-- A destroyed asset renders as a read-only "tombstone": all edit
					     affordances are suppressed (canEdit), the card is greyed and
					     stamped with an X. It's visual-only — it affects no counts. -->
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
						{#if dead}
							<span class="destroyed-badge">Destroyed</span>
							<svg class="tombstone-x" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
								<line x1="6" y1="6" x2="94" y2="94" />
								<line x1="94" y1="6" x2="6" y2="94" />
							</svg>
						{/if}
						<div class="tile-head">
							{#if renamingAssetId === asset.id}
								<input
									class="rename-input"
									type="text"
									bind:value={renameDraft}
									disabled={renameSaving}
									maxlength={80}
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
							<!-- Status glyphs + type word in one right-aligned cluster,
							     generously separated from the name (see .tile-head-meta). -->
							<div class="tile-head-meta">
							{#if asset.is_main_character}
								<!-- Main character: filled star, sharing the status-icon
								     footprint. Passive — the MC is changed by the ☆ toggle on
								     another peer, not by un-setting it here. -->
								<span class="hi main-star" title="Main character" aria-label="Main character">
									<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor" stroke="none" aria-hidden="true"><path d="M12 17.75l-6.172 3.245l1.179 -6.873l-5 -4.867l6.9 -1l3.086 -6.253l3.086 6.253l6.9 1l-5 4.867l1.179 6.873z" /></svg>
								</span>
							{:else if canEdit && asset.asset_type === 'peer' && renamingAssetId !== asset.id && mcSwapTo == null}
								<!-- Make-main-character toggle: outline star (reads as "set
								     this"), same footprint as the filled star. -->
								<button
									type="button"
									class="hi mc-toggle"
									onclick={() => startMcSwap(asset)}
									disabled={mcSwapSaving}
									aria-label={`Make ${asset.name} the main character`}
									title="Make main character"
								>
									<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 17.75l-6.172 3.245l1.179 -6.873l-5 -4.867l6.9 -1l3.086 -6.253l3.086 6.253l6.9 1l-5 4.867l1.179 6.873z" /></svg>
								</button>
							{/if}
							{#if asset.is_leveraged}
								<!-- Leveraged status glyph — the sole "exhausted" cue now that
								     the tile is neither dimmed nor border-tinted for it. -->
								<span class="hi lev-badge" title="Leveraged — spent for a roll until refreshed" aria-label="Leveraged">
									<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
										<rect x="3" y="3" width="18" height="18" rx="3" />
										<circle cx="8" cy="8" r="1.2" fill="currentColor" stroke="none" />
										<circle cx="16" cy="8" r="1.2" fill="currentColor" stroke="none" />
										<circle cx="12" cy="12" r="1.2" fill="currentColor" stroke="none" />
										<circle cx="8" cy="16" r="1.2" fill="currentColor" stroke="none" />
										<circle cx="16" cy="16" r="1.2" fill="currentColor" stroke="none" />
									</svg>
								</span>
							{/if}
							{#if !dead && (isSelf || secretsForAsset(asset.id).length > 0)}
								<!-- Open (known) eye. Always gold. On your own assets it's the
								     write-a-secret affordance, prefixed with an inline "+" (the
								     same add convention as the marginalia "+" tiles). On others'
								     assets it's read-only and only appears when you can actually
								     read something. Unlike the single-glyph icons this is a
								     comfortable rectangle so the "+" and eye both breathe; the
								     readable count rides the corner. -->
								<button
									type="button"
									class="secrets-btn"
									class:active={secretsOpenForAssetId === asset.id}
									onclick={() => toggleSecrets(asset.id)}
									aria-label={isSelf
										? (secretsForAsset(asset.id).length > 0 ? `Write or view ${secretsForAsset(asset.id).length} secret(s)` : 'Write a secret')
										: `View ${secretsForAsset(asset.id).length} secret(s)`}
									title={isSelf ? 'Write a secret' : 'Secrets you can read'}
								>
									{#if isSelf}<span class="secrets-plus" aria-hidden="true">+</span>{/if}
									<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
										<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
										<circle cx="12" cy="12" r="3" />
									</svg>
									{#if secretsForAsset(asset.id).length > 0}<span class="corner-badge known">{secretsForAsset(asset.id).length}</span>{/if}
								</button>
							{:else if dead && secretsForAsset(asset.id).length > 0}
								<!-- Tombstone: the readable-secret count survives as a passive,
								     non-clickable record of secrets lost with the asset. -->
								<span
									class="hi secrets-static"
									title={`${secretsForAsset(asset.id).length} secret${secretsForAsset(asset.id).length === 1 ? '' : 's'} lost with this asset`}
									aria-label={`${secretsForAsset(asset.id).length} secrets lost with this asset`}
								>
									<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
										<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
										<circle cx="12" cy="12" r="3" />
									</svg>
									<span class="corner-badge known">{secretsForAsset(asset.id).length}</span>
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
									<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
										<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
										<circle cx="12" cy="12" r="3" />
										<line x1="3" y1="21" x2="21" y2="3" />
									</svg>
									<span class="corner-badge hidden">{hiddenSecretsFor(asset)}</span>
								</span>
							{/if}
							<span class="asset-type">{assetTypeLabels[asset.asset_type]}</span>
							</div>
						</div>
						{#if renamingAssetId === asset.id && renameError}
							<p class="error-text">{renameError}</p>
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
								{#if mcSwapError}<p class="error-text">{mcSwapError}</p>{/if}
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
										customPlaceholder="Write a marginalia…"
										multiline
										disabled={saving}
									/>
								{:else}
									<textarea
										class="m-editor-input"
										placeholder="Write a marginalia…"
										bind:value={draftText}
										disabled={saving}
										rows={3}
										maxlength={280}
										onkeydown={(e) => {
											if (e.key === 'Escape') { e.preventDefault(); cancelEdit(); }
											else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); saveEdit(); }
										}}
										use:focusOnMount
									></textarea>
								{/if}
								{#if editError}<p class="error-text">{editError}</p>{/if}
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
									<p class="empty small">No secrets visible to you.</p>
								{:else}
									<ul class="secrets-list">
										{#each secretsForAsset(asset.id) as s (s.id)}
											<li class="secret-row" class:authored={s.author_id === viewerPlayerId}>
												<span class="secret-text">{s.text}</span>
												{#if s.author_id === viewerPlayerId}<span class="secret-mine">yours</span>{/if}
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
									{#if secretError}<p class="error-text">{secretError}</p>{/if}
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
		<p class="empty">Player not found.</p>
	{/if}
</div>

<style>
	.retinue-view {
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
	}

	.retinue-header {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	/* Row 1: title + online status inline. min-width:0 lets the h2 ellipsize
	   instead of wrapping, since display names can run up to 40 chars. */
	.header-title-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
	}
	.header-title-row h2 {
		color: var(--color-accent);
		font-size: 1.1rem;
		margin: 0;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Row 2: right-aligned badge cluster (Focus Player + leveraged count). */
	.header-badges {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 0.4rem;
	}

	/* Focus-player marker, visible to all viewers on the focus player's retinue.
	   Filled gold to read as the dominant "it's their turn" cue, contrasting the
	   outlined leveraged badge beside it. */
	.focus-badge {
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-bg);
		background: var(--color-accent);
		border-radius: 999px;
		padding: 0.15rem 0.55rem;
		line-height: 1.2;
		white-space: nowrap;
	}

	/* Spent-but-refreshable counter, top-right across from online status.
	   Outlined (not filled) to echo the leveraged asset-card border. */
	.leveraged-badge {
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-leveraged);
		border: 1px solid var(--color-leveraged);
		border-radius: 999px;
		padding: 0.15rem 0.5rem;
		line-height: 1.2;
		white-space: nowrap;
	}
	.leveraged-count {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
	/* Nothing committed — keep it present but quiet so it reads as a resting state. */
	.leveraged-badge.zero {
		color: var(--color-text-faint);
		border-color: var(--color-border-strong);
	}

	.status {
		flex-shrink: 0;
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--color-neutral);
	}
	.dot.online { background: var(--color-success); }

	.empty {
		color: var(--color-text-faint);
		font-size: 0.9rem;
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
		gap: 0.6rem;
	}

	.asset-tile {
		background: var(--color-surface-warm);
		border: 0.5px solid var(--color-accent);
		border-radius: 10px;
		padding: 0.6rem 0.7rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	/* Tombstone: a destroyed asset kept visible as a read-only record. Greyed
	   and desaturated (text stays legible — we don't crush opacity), stamped
	   with a large X, and inert to interaction. All edit affordances are
	   already suppressed in markup via canEdit; pointer-events is a backstop. */
	.asset-tile.destroyed {
		position: relative;
		filter: grayscale(1);
		opacity: 0.72;
		border-color: var(--color-text-faint, #8a8a8a);
		border-style: dashed;
		pointer-events: none;
	}
	.asset-tile.destroyed .tombstone-x {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		stroke: var(--color-text-faint, #8a8a8a);
		stroke-width: 2;
		opacity: 0.55;
		pointer-events: none;
	}
	.asset-tile.destroyed .destroyed-badge {
		position: absolute;
		top: 0.4rem;
		left: 50%;
		transform: translateX(-50%);
		z-index: 1;
		font-size: 0.62rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--color-text-faint, #8a8a8a);
		border: 0.5px solid var(--color-text-faint, #8a8a8a);
		border-radius: 4px;
		padding: 0.1rem 0.4rem;
		background: var(--color-surface-warm);
	}
	/* The main character is distinguished by the filled star in its tile head, so
	   the gold border is shared by every asset tile. Leveraged tiles carry the
	   .lev-badge die in the tile head as their only "exhausted" cue. */

	.tile-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}

	/* Trailing icons (☆ / die / eyes) + the type chip, kept together and pushed
	   to the right edge. padding-left guarantees a generous gap from the name
	   even when a long name fills the row. */
	.tile-head-meta {
		display: flex;
		align-items: center;
		gap: 1.0rem;
		flex-shrink: 0;
		padding-left: 0.75rem;
	}

	.asset-name {
		font-size: 0.95rem;
		color: var(--color-text);
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	button.asset-name.editable {
		background: none;
		border: 1px solid transparent;
		padding: 0.15rem 0.35rem;
		margin: -0.15rem -0.35rem;
		border-radius: 4px;
		font-family: inherit;
		text-align: left;
		cursor: pointer;
		max-width: 100%;
	}
	button.asset-name.editable:hover { border-color: var(--color-border-warm-hover); background: var(--color-surface-warm-hover); }
	button.asset-name.editable:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	.rename-input {
		flex: 1;
		min-width: 0;
		font-family: inherit;
		font-size: 0.95rem;
		color: var(--color-text);
		background: var(--color-surface-warm-sunken);
		border: 1px solid var(--color-border-warm-hover);
		border-radius: 4px;
		padding: 0.25rem 0.4rem;
	}
	.rename-input:focus { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	/* ── Tile-head status icons ──────────────────────────────────────────────
	   Every item in the cluster — icons AND the type chip — is sized to hug its
	   own content, so the flex `gap` on .tile-head-meta is the single, uniform
	   spacer between all of them (no element carries extra box padding that
	   would inflate its neighbours' gaps). The interactive ones (mc-toggle,
	   secrets) get their hover/active "box" from a box-shadow spread + an
	   ::after hit area — both layout-neutral, so they never change the spacing. */
	.hi {
		position: relative;
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	button.hi,
	.secrets-btn {
		padding: 0;
		background: none;
		border: none;
		border-radius: 4px;
		cursor: pointer;
	}
	/* ~26px tap area centred on the glyph; -4px meets the neighbour's edge at the
	   0.5rem gap without overlapping it. */
	button.hi::after,
	.secrets-btn::after { content: ''; position: absolute; inset: -4px; }
	button.hi:focus-visible,
	.secrets-btn:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 3px; }
	button.hi:disabled { opacity: 0.45; cursor: not-allowed; }

	/* Main character: filled star (changed elsewhere via the ☆ toggle). */
	.main-star { color: var(--color-accent); }

	/* Make-main-character toggle: outline star, brightening to gold with a soft
	   box on hover (box-shadow spread = padded look, zero layout footprint). */
	.mc-toggle { color: var(--color-accent-dim); }
	.mc-toggle:hover { color: var(--color-accent); background: var(--color-surface-warm-hover); box-shadow: 0 0 0 4px var(--color-surface-warm-hover); }

	.lev-badge { color: var(--color-leveraged); }

	/* Writable/readable secrets eye — an inline "+" beside the eye. No outer
	   padding, so it sits in the row on the same gap as everything else. */
	.secrets-btn {
		position: relative;
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		gap: 0.15rem;
		color: var(--color-accent);
	}
	.secrets-btn:hover { color: var(--color-accent-hover); background: var(--color-surface-warm-hover); box-shadow: 0 0 0 4px var(--color-surface-warm-hover); }
	.secrets-btn.active { background: var(--color-surface-warm-active); box-shadow: 0 0 0 4px var(--color-surface-warm-active); }
	/* Inline "+" prefix on the writable (own-asset) eye. */
	.secrets-plus { font-size: 1rem; font-weight: 600; line-height: 1; }

	/* Tombstone's passive readable-secret eye — same gold as the live eye, but
	   a plain span (no hover/press), a record rather than an affordance. */
	.secrets-static { color: var(--color-accent); }

	/* Struck eye: muted to read as "not available to you" against the gold eye. */
	.hidden-secrets { color: var(--color-text-muted); }

	/* Secrets panel (expanded under the marginalia area) */
	.secrets-panel {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--color-border);
	}

	.secrets-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.secret-row {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 0.5rem;
		padding: 0.4rem 0.55rem;
		background: var(--color-surface-warm-sunken);
		border: 1px solid var(--color-border-warm-faint);
		border-radius: 5px;
		font-size: 0.85rem;
		line-height: 1.4;
		color: var(--color-text-secondary);
	}
	.secret-row.authored { border-left: 2px solid var(--color-accent); }

	.secret-text { flex: 1; word-break: break-word; }
	.secret-mine {
		flex-shrink: 0;
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-accent);
	}

	.empty.small { font-size: 0.82rem; padding: 0.3rem 0; }

	/* Pick-a-marginalia-to-break picker (replaces grid on old MC during swap) */
	.mc-picker {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.mc-picker-list {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.mc-picker-item {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		text-align: left;
		min-height: 44px;
		padding: 0.5rem 0.6rem;
		background: var(--color-surface-warm-sunken);
		border: 1px solid color-mix(in srgb, var(--color-danger-muted) 40%, var(--color-surface));
		border-radius: 5px;
		font-family: inherit;
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		cursor: pointer;
	}
	.mc-picker-item:hover { background: color-mix(in srgb, var(--color-danger-muted) 12%, var(--color-surface)); border-color: var(--color-danger-muted); color: var(--color-danger); }
	.mc-picker-item:focus-visible { outline: 2px solid var(--color-danger-muted); outline-offset: 1px; }
	.mc-picker-item:disabled { opacity: 0.5; cursor: not-allowed; }

	.mc-picker-item .m-pos {
		flex-shrink: 0;
		color: var(--color-accent);
	}

	.asset-type {
		font-size: 0.7rem;
		background: var(--color-border-warm);
		color: var(--color-accent);
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		flex-shrink: 0;
	}

	/* ── Marginalia 2×2 grid ─────────────────────────────────────────────── */

	.m-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.35rem;
	}

	/* Base look (background/border/color/torn/titled/empty) comes from
	   shared/marginaliaTile.css; this is just the real grid's tap-target
	   sizing and clipping, which the static HelpContent replica doesn't need. */
	.m-tile {
		min-height: 44px;
		overflow: hidden;
	}

	.m-tile-text {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		word-break: break-word;
	}

	/* "+" add affordance on owner empty slots */
	.m-tile.empty.add {
		justify-content: center;
		color: var(--color-text-faint-warm);
		cursor: pointer;
		font-family: inherit;
		font-size: inherit;
	}
	.m-tile.empty.add:hover { color: var(--color-accent); border-color: var(--color-border-warm-hover); }
	.m-tile.empty.add:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.add-plus { font-size: 1.4rem; line-height: 1; }

	/* Needlessly-at-risk nudge: solid red border on the next fillable slot,
	   matching the red header-chip risk badge. Hover/focus keep the red so the
	   warning doesn't disappear mid-interaction. */
	.m-tile.empty.add.at-risk {
		border-style: solid;
		border-color: var(--color-at-risk-border);
		color: var(--color-at-risk-text);
	}
	.m-tile.empty.add.at-risk:hover { color: var(--color-danger); border-color: var(--color-at-risk-border-hover); }
	.m-tile.empty.add.at-risk:focus-visible { outline-color: var(--color-at-risk-border); }

	/* Owner edit affordance on filled (untorn) slots */
	.m-tile.filled {
		text-align: left;
		font-family: inherit;
		font-size: 0.78rem;
		color: var(--color-text-secondary);
		cursor: pointer;
	}
	.m-tile.filled:hover { background: var(--color-surface-warm-hover); border-color: var(--color-border-warm-hover); }
	.m-tile.filled:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	/* ── Inline marginalia editor (replaces grid while active) ───────────── */

	.m-editor {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.m-editor-label {
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-accent);
		margin: 0;
	}

	.m-editor-input {
		width: 100%;
		font-family: inherit;
		font-size: 0.88rem;
		line-height: 1.4;
		padding: 0.5rem 0.6rem;
		background: var(--color-surface-warm-sunken);
		color: var(--color-text);
		border: 1px solid var(--color-border-warm-hover);
		border-radius: 6px;
		resize: vertical;
		min-height: 84px;
	}
	.m-editor-input:focus { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	.m-editor-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}

</style>
