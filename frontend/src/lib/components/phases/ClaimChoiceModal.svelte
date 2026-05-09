<!--
  Step-by-step authoring modal for a Prologue choice. The player drafts:
    1. The sheet-derived asset's text (prefilled with [choice text]).
    2. (titles) A title marginalium for their main character, prefilled.
    2. (laws_rumors) A public-record entry, prefilled.
    3. For each fresh card: a multiple-choice picker (3 unused examples + Custom).
       Cards already owned by another player become takes — no text needed.

  Submits everything in a single choosePrologue call.
-->
<script lang="ts">
	import {
		choosePrologue,
		getPrologueCardSuggestions,
		type PrologueSheet,
		type PrologueClaim,
		type PrologueSheetType,
		type PlayerCardRow,
	} from '$lib/api';

	interface Props {
		gameID: string;
		sheet: PrologueSheet;
		choice: PrologueSheet['choices'][number];
		cards: PlayerCardRow[];
		onClose: () => void;
		onSubmitted: () => void;
	}

	let { gameID, sheet, choice, cards, onClose, onSubmitted }: Props = $props();

	const isTitles = $derived(sheet.type === 'titles');
	const isLawsRumors = $derived(sheet.type === 'laws_rumors');

	type CardSlot = {
		suit: string;
		value: string;
		isTake: boolean;
		suggestions: string[];
		picked: string;
		custom: string;
	};

	function isCardTaken(suit: string, value: string): boolean {
		return cards.some(c => c.card_suit === suit && c.card_value === value);
	}

	// Editable form state. Initialized empty and seeded by the effect below
	// so the seed re-runs if the parent ever reuses this modal for a
	// different choice (Svelte 5 was warning that $state(propValue) only
	// captures the initial prop value, not a reactive reference).
	let assetText = $state('');
	let marginaliumText = $state('');
	let lawOrRumorText = $state('');
	let cardSlots = $state<CardSlot[]>([]);

	// Reset the form whenever the choice changes (including on first mount).
	// Tracking the choice name lets us avoid clobbering user edits on
	// unrelated re-renders that pass the same choice through.
	let seededFor = '';
	$effect(() => {
		if (seededFor === choice.name) return;
		assetText = `[${choice.name}]`;
		marginaliumText = `[${choice.name}]`;
		lawOrRumorText = `[${choice.name}]`;
		cardSlots = choice.cards.map(c => ({
			suit: c.suit,
			value: c.value,
			isTake: isCardTaken(c.suit, c.value),
			suggestions: [],
			picked: '',
			custom: '',
		}));
		seededFor = choice.name;
	});

	let loadingSuggestions = $state(true);
	let submitting = $state(false);
	let error = $state('');

	async function loadSuggestions() {
		loadingSuggestions = true;
		try {
			for (const slot of cardSlots) {
				if (slot.isTake) continue;
				const res = await getPrologueCardSuggestions(gameID, slot.suit);
				slot.suggestions = res.suggestions;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load card suggestions.';
		} finally {
			loadingSuggestions = false;
		}
	}

	$effect(() => {
		loadSuggestions();
	});

	function cardLabel(suit: string, value: string): string {
		const s = suit === 'H' ? '♥' : suit === 'D' ? '♦' : suit === 'S' ? '♠' : '♣';
		return value + s;
	}

	function suitTypeLabel(suit: string): string {
		switch (suit) {
			case 'C': return 'holding';
			case 'D': return 'resource';
			case 'S': return 'artifact';
			case 'H': return 'peer';
			default:  return 'asset';
		}
	}

	const ready = $derived.by(() => {
		if (!assetText.trim()) return false;
		if (isTitles && !marginaliumText.trim()) return false;
		if (isLawsRumors && !lawOrRumorText.trim()) return false;
		for (const slot of cardSlots) {
			if (slot.isTake) continue;
			const text = slot.picked === '__custom__' ? slot.custom : slot.picked;
			if (!text.trim()) return false;
		}
		return true;
	});

	async function submit() {
		if (!ready || submitting) return;
		submitting = true;
		error = '';
		try {
			const card_assets = cardSlots
				.filter(s => !s.isTake)
				.map(s => ({
					suit: s.suit,
					value: s.value,
					text: (s.picked === '__custom__' ? s.custom : s.picked).trim(),
				}));
			await choosePrologue(gameID, {
				sheet_type: sheet.type as PrologueSheetType,
				choice_name: choice.name,
				asset_text: assetText.trim(),
				marginalium_text: isTitles ? marginaliumText.trim() : undefined,
				law_or_rumor_text: isLawsRumors ? lawOrRumorText.trim() : undefined,
				card_assets,
			});
			onSubmitted();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not submit your choice.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="backdrop" onclick={onClose} role="presentation"></div>
<div class="modal" role="dialog" aria-modal="true" aria-labelledby="claim-modal-heading">
	<header>
		<h3 id="claim-modal-heading">{choice.name}</h3>
		<button class="close" onclick={onClose} aria-label="Cancel">×</button>
	</header>

	{#if error}<p class="local-error">{error}</p>{/if}

	<section class="step">
		<label class="field">
			<span class="label">Your {sheet.choice_asset_type}</span>
			<textarea
				rows="2"
				bind:value={assetText}
				placeholder={`[${choice.name}]`}
			></textarea>
			<span class="hint">Replace the bracketed text with the name and feel of the {sheet.choice_asset_type}.</span>
		</label>
	</section>

	{#if isTitles}
		<section class="step">
			<label class="field">
				<span class="label">Title marginalium on your main character</span>
				<textarea
					rows="2"
					bind:value={marginaliumText}
					placeholder={`[${choice.name}]`}
				></textarea>
				<span class="hint">Adds a one-line note about this title to your main character.</span>
			</label>
		</section>
	{/if}

	{#if isLawsRumors}
		<section class="step">
			<label class="field">
				<span class="label">Public-record entry</span>
				<textarea
					rows="2"
					bind:value={lawOrRumorText}
					placeholder={`[${choice.name}]`}
				></textarea>
				<span class="hint">The exact text of this {choice.name.toLowerCase().includes('law') ? 'law' : 'rumor'}.</span>
			</label>
		</section>
	{/if}

	{#each cardSlots as slot, idx (slot.suit + slot.value)}
		<section class="step">
			<div class="card-head">
				<strong>{cardLabel(slot.suit, slot.value)}</strong>
				<span class="muted small">— {slot.isTake ? `take an existing ${suitTypeLabel(slot.suit)}` : `make a new ${suitTypeLabel(slot.suit)}`}</span>
			</div>

			{#if slot.isTake}
				<p class="muted small">This card is already in play. You'll take its asset; no new text needed.</p>
			{:else if loadingSuggestions}
				<p class="muted small">Loading suggestions…</p>
			{:else}
				<div class="suggestion-list">
					{#each slot.suggestions as suggestion}
						<label class="suggestion">
							<input type="radio" bind:group={cardSlots[idx].picked} value={suggestion} />
							<span>{suggestion}</span>
						</label>
					{/each}
					<label class="suggestion">
						<input type="radio" bind:group={cardSlots[idx].picked} value="__custom__" />
						<span>Custom…</span>
					</label>
					{#if slot.picked === '__custom__'}
						<input
							type="text"
							class="custom-input"
							bind:value={cardSlots[idx].custom}
							placeholder={`Name your ${suitTypeLabel(slot.suit)}`}
						/>
					{/if}
				</div>
			{/if}
		</section>
	{/each}

	<footer>
		<button class="secondary" onclick={onClose} disabled={submitting}>Cancel</button>
		<button class="primary" onclick={submit} disabled={!ready || submitting}>
			{submitting ? '…' : 'Claim'}
		</button>
	</footer>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0,0,0,0.6);
		z-index: 95;
	}
	.modal {
		position: fixed;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
		z-index: 96;
		width: min(560px, 94vw);
		max-height: 90dvh;
		overflow-y: auto;
		background: #1e1e1c;
		border: 1px solid #3a3a3a;
		border-radius: 12px;
		padding: 1rem 1.25rem 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
	}
	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	h3 { color: #c8a96e; margin: 0; font-size: 1.1rem; }
	.close {
		width: 36px; height: 36px;
		background: none; color: #aaa;
		font-size: 1.4rem; line-height: 1;
		border-radius: 6px;
	}
	.close:hover { background: #2a2a2a; color: #e8e4d9; }

	.local-error { color: #e07070; font-size: 0.85rem; margin: 0; }

	.step { background: #161614; border: 1px solid #2c2c2c; border-radius: 8px; padding: 0.65rem 0.75rem; }

	.field { display: flex; flex-direction: column; gap: 0.3rem; }
	.label { color: #c8a96e; font-size: 0.85rem; font-weight: 600; }
	.hint { font-size: 0.75rem; color: #888; }

	textarea, .custom-input {
		background: #2a2a2a; color: #e8e4d9;
		border: 1px solid #444; border-radius: 4px;
		padding: 0.4rem 0.5rem; font-size: 0.9rem;
		font-family: inherit;
	}

	.card-head { display: flex; gap: 0.4rem; align-items: baseline; margin-bottom: 0.3rem; }
	.muted { color: #999; }
	.muted.small { font-size: 0.8rem; }

	.suggestion-list { display: flex; flex-direction: column; gap: 0.25rem; }
	.suggestion { display: flex; align-items: center; gap: 0.5rem; font-size: 0.9rem; min-height: 28px; }
	.custom-input { margin-top: 0.25rem; }

	footer { display: flex; gap: 0.6rem; justify-content: flex-end; }

	.primary {
		background: #c8a96e; color: #1a1a1a; font-weight: 600;
		padding: 0.5rem 1rem; border-radius: 6px;
	}
	.primary:disabled { opacity: 0.4; cursor: not-allowed; }

	.secondary {
		background: #333; color: #e8e4d9; font-weight: 600;
		padding: 0.5rem 0.9rem; border-radius: 6px;
		border: 1px solid #555;
	}
</style>
