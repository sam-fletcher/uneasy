<!-- SeekAnswersPanel.svelte
  Prep + resolve UI for Seek Answers (Tier 1, Knowledge).

  Flow:
  - Prep: notes textarea (required).
  - Resolve: dice roll → pick option counts (repeatable, total = dice result)
    → submit via makeChoice → for each break_resource/reveal_secret pick,
    fill sub-form → complete.

  The option list is the same for make and mar (per spec). For mar, the
  server additionally auto-applies forced breaks on the preparer's own
  resources; the UI just reflects the resulting asset updates.
-->
<script lang="ts">
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		breakResource, revealSecret,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import { parseChoices, playerName, intactMarginalia } from './shared';

	interface Props {
		mode: 'prep' | 'resolve';
		gameID: number;
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		plan?: Plan | null;
		isFocusPlayer?: boolean;
		rollActive?: boolean;
		rollOutcome?: 'make' | 'mar' | null;
		onRollCreated?: (roll: DiceRoll) => void;
		onPlansChanged?: () => void;
		onPlanPrepared?: () => void;
	}

	let {
		mode, gameID, assets, players, currentPlayerID,
		plan = null, isFocusPlayer = false,
		rollActive = false, rollOutcome = null,
		onRollCreated: _or = () => {},
		onPlansChanged = () => {},
		onPlanPrepared = () => {},
	}: Props = $props();

	const OPTIONS = [
		{ key: 'break_resource', label: 'Break a resource (describe a flaw)' },
		{ key: 'declare_truth',  label: 'Declare something true (scene post)' },
		{ key: 'ask_question',   label: 'Ask a player a truthful question (scene post)' },
		{ key: 'reveal_secret',  label: "Reveal an asset's secrets to you" },
	] as const;

	// ── Prep ─────────────────────────────────────────────────────────────────
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Describe your research methods.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'seek_answers',
				preparation_notes: prepNotes.trim(),
			});
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── Resolve: option counts picker ────────────────────────────────────────
	let counts = $state<Record<string, number>>({
		break_resource: 0, declare_truth: 0, ask_question: 0, reveal_secret: 0,
	});
	let choicesBusy = $state(false);
	let resError = $state('');

	function bump(key: string, delta: number) {
		const next = Math.max(0, (counts[key] ?? 0) + delta);
		counts = { ...counts, [key]: next };
	}

	const totalPicked = $derived(Object.values(counts).reduce((a, b) => a + b, 0));

	async function onApplyChoices(p: Plan, outcome: 'make' | 'mar') {
		if (choicesBusy || totalPicked === 0) return;
		choicesBusy = true; resError = '';
		try {
			const flat: string[] = [];
			for (const opt of OPTIONS) {
				for (let i = 0; i < (counts[opt.key] ?? 0); i++) flat.push(opt.key);
			}
			await makeChoice(p.id, outcome, flat);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not apply choices.';
		} finally { choicesBusy = false; }
	}

	// ── Sub-flows: break_resource, reveal_secret ─────────────────────────────
	// We track remaining sub-actions locally: one per recorded pick, decremented
	// as the preparer submits each. The server is the source of truth; if the
	// page is reloaded mid-flow, the user may see remaining counts reset — they
	// should re-submit any outstanding actions.
	let breakDone = $state(0);
	let revealDone = $state(0);

	// Reset sub-flow counters whenever the plan id changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			breakDone = 0;
			revealDone = 0;
		}
	});

	// Sub-form selection state
	let brAssetID = $state<number | null>(null);
	let brMargPos = $state<number | null>(null);
	let brBusy = $state(false);

	let rsAssetID = $state<number | null>(null);
	let rsBusy = $state(false);

	// All non-destroyed resource assets across all players.
	const allResources = $derived(
		assets.filter(a => a.asset_type === 'resource' && !a.is_destroyed)
	);
	// All non-destroyed assets (reveal-secret can target any asset).
	const allAssets = $derived(assets.filter(a => !a.is_destroyed));

	const brAssetMarginalia = $derived(
		brAssetID == null ? [] : intactMarginalia(assets, assets.find(a => a.id === brAssetID)?.owner_id ?? null)
			.filter(m => m.assetID === brAssetID)
	);

	async function submitBreakResource(p: Plan) {
		if (brBusy || brAssetID == null || brMargPos == null) return;
		brBusy = true; resError = '';
		try {
			await breakResource(p.id, brAssetID, brMargPos);
			breakDone += 1;
			brAssetID = null; brMargPos = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break resource.';
		} finally { brBusy = false; }
	}

	async function submitRevealSecret(p: Plan) {
		if (rsBusy || rsAssetID == null) return;
		rsBusy = true; resError = '';
		try {
			await revealSecret(p.id, rsAssetID);
			revealDone += 1;
			rsAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not reveal secret.';
		} finally { rsBusy = false; }
	}

	// ── Complete ────────────────────────────────────────────────────────────
	let resBusy = $state(false);
	async function onComplete(p: Plan) {
		if (resBusy) return;
		resBusy = true; resError = '';
		try {
			await completePlan(p.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { resBusy = false; }
	}

	function countIn(choices: string[], key: string) {
		return choices.filter(c => c === key).length;
	}
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<label class="form-label">
			Research methods and topics:
			<textarea rows={3} bind:value={prepNotes} class="form-textarea"
				placeholder="What are you investigating, and how?"></textarea>
		</label>
		<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy}>
			{prepBusy ? '…' : 'Prepare Seek Answers'}
		</button>
	</div>

{:else if plan}
	{@const existingChoices = parseChoices(plan)}
	{@const choicesDone = existingChoices.length > 0}
	{@const brNeeded = countIn(existingChoices, 'break_resource')}
	{@const rsNeeded = countIn(existingChoices, 'reveal_secret')}
	{@const brRemaining = Math.max(0, brNeeded - breakDone)}
	{@const rsRemaining = Math.max(0, rsNeeded - revealDone)}
	{@const subflowsDone = brRemaining === 0 && rsRemaining === 0}

	<ResolvingCard {plan} {players} error={resError}>
		{#if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone && isFocusPlayer}
			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-{rollOutcome}">
						{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
					</strong>
				</p>
				<p class="choices-note">
					Pick options equal to your dice result (repeatable). For mar, the
					server also auto-applies breaks to your own resources.
				</p>
				{#each OPTIONS as opt}
					<div class="choice-item" style="display:flex;align-items:center;gap:0.5rem;">
						<button class="action-btn" onclick={() => bump(opt.key, -1)}
							disabled={(counts[opt.key] ?? 0) === 0}>−</button>
						<strong style="min-width:1.5rem;text-align:center;">{counts[opt.key] ?? 0}</strong>
						<button class="action-btn" onclick={() => bump(opt.key, 1)}>+</button>
						<span>{opt.label}</span>
					</div>
				{/each}
				<p class="choices-note">Total picks: <strong>{totalPicked}</strong></p>
				<button class="action-btn primary"
					onclick={() => onApplyChoices(plan, rollOutcome!)}
					disabled={choicesBusy || totalPicked === 0}>
					{choicesBusy ? '…' : 'Apply choices'}
				</button>
			</div>

		{:else if choicesDone && isFocusPlayer}
			<div class="complete-section">
				<p class="choices-applied">
					Choices applied:
					{#each OPTIONS as opt}
						{#if countIn(existingChoices, opt.key) > 0}
							<span>{opt.label} × {countIn(existingChoices, opt.key)}; </span>
						{/if}
					{/each}
				</p>

				{#if brRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Break a resource ({brRemaining} remaining)
						</p>
						<label class="form-label">
							Resource:
							<select bind:value={brAssetID} class="form-select">
								<option value={null}>Select a resource…</option>
								{#each allResources as a}
									<option value={a.id}>{a.name} (owner: {playerName(players, a.owner_id)})</option>
								{/each}
							</select>
						</label>
						{#if brAssetID != null}
							<label class="form-label">
								Marginalia to tear:
								<select bind:value={brMargPos} class="form-select">
									<option value={null}>Select a marginalia…</option>
									{#each brAssetMarginalia as m}
										<option value={m.position}>#{m.position}: {m.text}</option>
									{/each}
								</select>
							</label>
						{/if}
						<button class="action-btn primary"
							onclick={() => submitBreakResource(plan)}
							disabled={brBusy || brAssetID == null || brMargPos == null}>
							{brBusy ? '…' : 'Break resource'}
						</button>
					</div>
				{/if}

				{#if rsRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Reveal an asset's secrets ({rsRemaining} remaining)
						</p>
						<label class="form-label">
							Asset:
							<select bind:value={rsAssetID} class="form-select">
								<option value={null}>Select an asset…</option>
								{#each allAssets as a}
									<option value={a.id}>{a.name} (owner: {playerName(players, a.owner_id)})</option>
								{/each}
							</select>
						</label>
						<button class="action-btn primary"
							onclick={() => submitRevealSecret(plan)}
							disabled={rsBusy || rsAssetID == null}>
							{rsBusy ? '…' : 'Reveal secrets'}
						</button>
					</div>
				{/if}

				{#if subflowsDone}
					<p class="complete-note">
						Post any questions, truths, or follow-scene narration in the scene
						thread, then complete the plan.
					</p>
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>

		{:else if !isFocusPlayer}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Seek Answers…
			</p>
		{/if}
	</ResolvingCard>
{/if}

<style>
	:global(.form-select) {
		width: 100%;
		padding: 0.4rem;
		border: 1px solid var(--border, #ccc);
		border-radius: 4px;
	}
</style>
