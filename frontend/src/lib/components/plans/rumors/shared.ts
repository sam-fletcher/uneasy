// shared.ts
// The facts about one Spread Rumors plan that more than one step reads: whose turn it
// is to act, which asset the rumor is about, which asset the break and leverage land
// on, what the options are called. Worked out once, from the plan context and the
// plan, and handed to each step as `scene`, rather than each step repeating the same
// dozen lookups.
//
// Plain data, no runes: the panel calls it inside a template `{@const}`, so it is
// recomputed whenever the context or the plan changes, as the `$derived`s it replaces
// were.

import type { Plan } from '$lib/api';
import type { BuffetTab } from '$lib/components/plans/shared/Buffet.svelte';
import type { SpreadRumorsResolutionData } from '$lib/plans/resolutionData/spread_rumors';
import { breakableAssets, breaksAvailable, playerName, resolutionActorID } from '../shared';
import type { PlanContext } from '../types';

export interface RumorOption {
	key: string;
	label: string;
	/** Viewer-neutral wording where `label` speaks to the picker ("Hide yourself…"). */
	summary?: string;
}

/**
 * The book's make list. (d) and (e) are its words verbatim: (e) is GLOATING —
 * "Relish in its glory" is the point — not a change of attribution, which is why
 * it carries the whole sentence. The labels read for the branch in play: on a
 * mar every option is aimed at the preparer's main character and the preparer's
 * assets, not at the rumor's original subject.
 */
function rumorOptions(
	mar: boolean,
	names: { mc: string; preparer: string; targetOwner: string },
): RumorOption[] {
	const mc = mar ? names.mc : '';
	return [
		{ key: 'break_target', label: mar
			? `Break ${mc} — tear one of its marginalia`
			: 'Break the target asset — tear a marginalia' },
		{ key: 'leverage_target', label: mar ? `Leverage ${mc}` : 'Leverage the target asset' },
		{ key: 'take_asset', label: mar
			? `Take an asset from ${names.preparer} — they must consent`
			: `Take an asset from ${names.targetOwner} — they must consent` },
		{ key: 'hide_source', label: 'Hide yourself as the source of the rumor. Add it as a secret to one of your assets.',
			summary: 'The source of the rumor is hidden — it becomes a secret on one of their assets.' },
		{ key: 'reveal_source', label: 'Reveal yourself as the source of the rumor. Relish in its glory.',
			summary: 'The source of the rumor is revealed, and relishes in its glory.' },
	];
}

export function rumorScene(ctx: PlanContext, plan: Plan, srData: SpreadRumorsResolutionData) {
	const { assets, players, currentPlayerID, plans, rollOutcome } = ctx;
	const mar = rollOutcome === 'mar';

	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = currentPlayerID === plan.preparer_id;
	// Post-roll steps belong to the perform_steps winner when one holds the plan,
	// else the preparer (mirrors the server's actsForPreparer). Pre-roll steps
	// stay on isPreparer: perform_steps is post-roll only.
	const amChoiceActor = currentPlayerID != null
		&& currentPlayerID === resolutionActorID(plan, plans);

	// ── What the rumor is about ──────────────────────────────────────────────
	const targetAsset = plan.target_asset_id != null
		? assets.find(a => a.id === plan.target_asset_id) ?? null
		: null;
	const targetAssetOwnerID = targetAsset?.owner_id ?? null;

	// On a mar, the rumor that spreads is about the preparer: its subject is their
	// main character (pinned server-side once the options apply), and every option
	// is aimed at it.
	const liveMainCharacter = assets.find(
		a => a.owner_id === plan.preparer_id && a.is_main_character && !a.is_destroyed,
	) ?? null;
	const mcAsset = srData.counter_target_asset_id != null
		? assets.find(a => a.id === srData.counter_target_asset_id) ?? null
		: liveMainCharacter;
	const mcName = mcAsset?.name ?? 'their main character';

	// ── Actor routing ────────────────────────────────────────────────────────
	// On make the preparer drives the picker and sub-flows. On mar the
	// target-asset owner does. `isActor` captures whichever applies.
	const isActor = currentPlayerID == null
		? false
		: mar
			? currentPlayerID === targetAssetOwnerID
			// make (or unresolved): preparer drives — or the perform_steps winner
			// of a demand against this plan, when one applies.
			: amChoiceActor;

	// The player whose deeds these are: the preparer on make, the target-asset
	// owner driving the counter-rumor on mar.
	const actorName = mar && targetAssetOwnerID != null
		? playerName(players, targetAssetOwnerID)
		: playerName(players, resolutionActorID(plan, plans));
	// Whose assets shelter the secret: the preparer's on a make (whoever is acting),
	// the counter-rumor's author on a mar.
	const shelterName = mar ? actorName : playerName(players, plan.preparer_id);

	// ── Break target ─────────────────────────────────────────────────────────
	// Candidate assets for the break-target picker.
	//  - make: only the plan's target asset (one card).
	//  - mar:  only the preparer's main character (one card).
	// AssetCardSelectable's marginalia-pick mode renders the per-line
	// checkboxes; asset identity is derived from the chosen marginalia (or,
	// for a blank asset, from the card itself — see breakableAssets).
	const btMarginaliaAssets = mar
		// Only the counter-rumor's subject: the actor picks WHICH marginalia, not
		// which asset (ruling Q2).
		? (mcAsset ? breakableAssets([mcAsset]) : [])
		: (targetAsset ? breakableAssets([targetAsset]) : []);
	// Each break_target pick spends one break, so the live cap is the total
	// available — picking more would commit a pick with nothing left to tear,
	// which then wedges resolution (CanComplete blocks until every break_target
	// pick is performed or forfeited via sr-forfeit-step). Mirrors the server's
	// srEligibleBreakTargets.
	const btMarginaliaCap = btMarginaliaAssets.reduce((sum, a) => sum + breaksAvailable(a), 0);

	// ── The options, as this viewer's branch calls them ──────────────────────
	const optionsFor = (forMar: boolean) => rumorOptions(forMar, {
		mc: mcName,
		preparer: playerName(players, plan.preparer_id),
		targetOwner: playerName(players, targetAssetOwnerID),
	});
	const options = optionsFor(mar);

	// Read-only "what can happen?" reference (shared Buffet). Viewer-neutral
	// wording, since everyone at the table reads it.
	const buffet: BuffetTab[] = [
		{
			key: 'make', label: 'Make', intro: 'Pick options equal to the dice result (repeatable):',
			opts: optionsFor(false).map(o => ({ key: o.key, label: o.summary ?? o.label })),
		},
		{
			key: 'mar', label: 'Mar', intro: 'Pick options equal to (difficulty − result), aimed at the preparer’s main character (repeatable):',
			opts: optionsFor(true).map(o => ({ key: o.key, label: o.summary ?? o.label })),
		},
	];

	return {
		srData, mar, isPreparer, isActor,
		targetAsset, targetAssetOwnerID, mcAsset, mcName,
		actorName, shelterName,
		btMarginaliaAssets, btMarginaliaCap,
		options, buffet,
	};
}

export type RumorScene = ReturnType<typeof rumorScene>;

/** How many of the picked choices are `key`. */
export function countIn(choices: string[], key: string): number {
	return choices.filter(c => c === key).length;
}

/** Said under the button whose press leaves nothing owed. */
export const LAST_STEP_TEXT = 'This is the last step: Spread Rumors resolves when you submit.';

/** The picked options as the flat list the server takes: each key once per pick, in option order. */
export function flatChoices(options: readonly RumorOption[], counts: Record<string, number>): string[] {
	const flat: string[] = [];
	for (const opt of options) {
		for (let i = 0; i < (counts[opt.key] ?? 0); i++) flat.push(opt.key);
	}
	return flat;
}
