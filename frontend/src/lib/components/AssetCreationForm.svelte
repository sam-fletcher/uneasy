<!-- AssetCreationForm.svelte
  Shared "name + one marginalia" authoring form for every player-created
  asset. A blank asset (no marginalia) is unbreakable and indestructible, so
  every creation route requires exactly one marginalia alongside the name —
  see adr/ASSET_CREATION_MARGINALIA_PLAN.md.

  Renders a live preview card on top (type badge, name, the one marginalia
  slot) followed by two SuggestionPicker sections. No internal submit button
  and no forced fill order — the caller owns the CTA and gates it on
  `name.trim() && marginalia.trim()`.
-->
<script lang="ts">
	import './shared/marginaliaTile.css';
	import type { AssetType } from '$lib/api';
	import { getAssetSuggestions } from '$lib/api';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import AssetTypeIcon from './AssetTypeIcon.svelte';
	import SuggestionPicker from './SuggestionPicker.svelte';

	interface Props {
		gameID: number | string;
		assetType: AssetType;
		name: string;
		marginalia: string;
		disabled?: boolean;
		nameLabel?: string;
		marginaliaLabel?: string;
	}

	let {
		gameID,
		assetType,
		name = $bindable(''),
		marginalia = $bindable(''),
		disabled = false,
		nameLabel = '1 · Name',
		marginaliaLabel = '2 · First marginalia',
	}: Props = $props();

	const typeLabels: Record<AssetType, string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};

	let nameSuggestions = $state<string[]>([]);
	let nameSuggLoading = $state(true);
	let marginaliaSuggestions = $state<string[]>([]);
	let marginaliaSuggLoading = $state(true);

	// Fetched once per mount — an assetType change (rare; callers generally
	// mount a fresh form per creation flow) simply keeps the first pool.
	let suggFetched = false;
	$effect(() => {
		if (suggFetched) return;
		suggFetched = true;
		getAssetSuggestions(gameID, assetType, 'name')
			.then((res) => { nameSuggestions = res.suggestions; })
			.catch(() => { nameSuggestions = []; })
			.finally(() => { nameSuggLoading = false; });
		getAssetSuggestions(gameID, assetType, 'marginalia')
			.then((res) => { marginaliaSuggestions = res.suggestions; })
			.catch(() => { marginaliaSuggestions = []; })
			.finally(() => { marginaliaSuggLoading = false; });
	});
</script>

<div class="acf">
	<div class="acf-preview">
		<div class="acf-badge">
			<AssetTypeIcon type={assetType} size={14} />
			<span>{typeLabels[assetType]}</span>
		</div>
		<div class="acf-name" class:filled={!!name.trim()}>
			{name.trim() || 'Unnamed'}
		</div>
		<div class="m-tile" class:empty={!marginalia.trim()}>
			{marginalia.trim() || '—'}
		</div>
	</div>

	<section class="acf-section">
		<p class="acf-label">{nameLabel}</p>
		<SuggestionPicker
			suggestions={nameSuggestions}
			bind:value={name}
			loading={nameSuggLoading}
			customPlaceholder="Name your new asset…"
			maxlength={TEXT_LIMITS.NAME}
			{disabled}
		/>
	</section>

	<section class="acf-section">
		<p class="acf-label">{marginaliaLabel}</p>
		<SuggestionPicker
			suggestions={marginaliaSuggestions}
			bind:value={marginalia}
			loading={marginaliaSuggLoading}
			customPlaceholder="A trait, tie, or detail…"
			maxlength={280}
			multiline
			{disabled}
		/>
	</section>
</div>

<style>
	.acf {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}

	.acf-preview {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.4rem;
		padding: 0.7rem;
		background: var(--color-surface);
		border: 1px solid var(--color-border-warm);
		border-radius: 8px;
	}

	.acf-badge {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
	}

	.acf-name {
		min-height: 44px;
		width: 100%;
		max-width: 320px;
		box-sizing: border-box;
		display: flex;
		align-items: center;
		justify-content: center;
		text-align: center;
		padding: 0.4rem 0.8rem;
		border: 1px dashed var(--color-border-strong);
		border-radius: 6px;
		font-style: italic;
		color: var(--color-text-muted);
	}
	.acf-name.filled {
		border: 1px solid var(--color-accent);
		font-style: normal;
		color: var(--color-text);
	}

	.acf-preview .m-tile {
		width: 100%;
		max-width: 320px;
		box-sizing: border-box;
		justify-content: center;
		text-align: center;
	}

	.acf-label {
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-accent);
		margin: 0 0 0.35rem;
	}
</style>
