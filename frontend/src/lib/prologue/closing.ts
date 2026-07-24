// closing.ts — pure helpers for the Prologue "closing" stage ("The Stage is
// Set"; adr/PROLOGUE_CLOSING_STAGE_PLAN.md).

import type {
	Asset,
	AssetType,
	ClosingReady,
	ExtraPeer,
	PrologueChoice,
	PrologueClaim,
	PrologueSheet,
} from '$lib/api';
import { isNeedlesslyAtRisk } from '$lib/assetRisk';

/** Mirrors model.MainCharacterPlaceholder (Go) — the name every player's
 *  main-character peer is created with before they choose a real one. */
export const MAIN_CHARACTER_PLACEHOLDER = '[Main Character]';

export function findMainCharacter(assets: Asset[], playerID: number | null): Asset | null {
	if (playerID == null) return null;
	return assets.find((a) => a.owner_id === playerID && a.is_main_character) ?? null;
}

export function isMcNamed(mc: Asset | null): boolean {
	return mc != null && mc.name.trim() !== '' && mc.name !== MAIN_CHARACTER_PLACEHOLDER;
}

/** ≤3 player games are the only ones that need an extra peer (S1 ruling). */
export function needsExtraPeer(playerCount: number): boolean {
	return playerCount <= 3;
}

export function findExtraPeer(extraPeers: ExtraPeer[], playerID: number | null): ExtraPeer | null {
	if (playerID == null) return null;
	return extraPeers.find((p) => p.player_id === playerID) ?? null;
}

/** Title-sheet boxes still open to claim as an extra peer: not claimed
 *  through the ordinary turn-taking flow, and not already taken by another
 *  player's extra peer. */
export function unclaimedTitles(
	titlesSheet: PrologueSheet | undefined,
	claims: PrologueClaim[],
	extraPeers: ExtraPeer[]
): PrologueChoice[] {
	if (!titlesSheet) return [];
	const claimedNames = new Set(
		claims.filter((c) => c.sheet_type === 'titles').map((c) => c.choice_name)
	);
	const extraClaimedNames = new Set(extraPeers.map((p) => p.title_name));
	return titlesSheet.choices.filter(
		(c) => !claimedNames.has(c.name) && !extraClaimedNames.has(c.name)
	);
}

/** The viewer's live assets carrying no marginalia at all. A blank asset is
 *  invulnerable — there is nothing to tear, so it can never be broken toward
 *  destruction — which is why clearing them is a hard gate on Ready
 *  (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md D2). Mirrors the server's
 *  CountBlankAssetsByOwner; the server re-checks it itself.
 *
 *  Note this is "no notes at all", NOT isNeedlesslyAtRisk's "≤1 intact note" —
 *  a blank asset is a subset of the at-risk set, and the two items sit side by
 *  side in the checklist (one hard, one soft). */
export function blankAssets(assets: Asset[], playerID: number | null): Asset[] {
	if (playerID == null) return [];
	// Same defence as assetRisk.ts: some WS payloads carry a marginalia-less
	// asset row, and a throw inside a derived breaks Svelte's reactivity batch.
	return assets.filter(
		(a) => a.owner_id === playerID && !a.is_destroyed && (a.marginalia ?? []).length === 0
	);
}

/** Server-authoritative gate (handler/prologue_closing.go) mirrored here only
 *  to disable/explain the Ready toggle client-side; the server re-checks every
 *  condition itself. Null once the viewer may ready up. Condition order matches
 *  closingReadyGateFailure so the client and server surface the same reason. */
export function readyBlockedReason(
	mcNamed: boolean,
	playerCount: number,
	hasExtraPeer: boolean,
	blankAssetCount = 0
): string | null {
	if (!mcNamed) return 'Name your main character first.';
	if (needsExtraPeer(playerCount) && !hasExtraPeer) return 'Create your extra peer first.';
	if (blankAssetCount > 0) return 'Give every asset at least one marginalia first.';
	return null;
}

export function isReady(closingReady: ClosingReady[], playerID: number | null): boolean {
	if (playerID == null) return false;
	return closingReady.find((c) => c.player_id === playerID)?.ready ?? false;
}

/** IDs of players who have not yet marked themselves ready. */
export function notReadyPlayerIDs(
	players: { id: number }[],
	closingReady: ClosingReady[]
): number[] {
	return players.filter((p) => !isReady(closingReady, p.id)).map((p) => p.id);
}

/** Count of the viewer's own needlessly-at-risk assets (soft nudge item). */
export function myAtRiskCount(assets: Asset[], playerID: number | null): number {
	if (playerID == null) return 0;
	return assets.filter((a) => a.owner_id === playerID && isNeedlesslyAtRisk(a)).length;
}

// ── Recap tallies (S3) ──────────────────────────────────────────────────────

/** One player's end-of-prologue retinue, tallied by asset type, for the recap
 *  section. `takenFromOthers` counts assets whose current owner differs from
 *  their creator (a proxy for "won from another player" — the plan's v1 scope;
 *  a true steal ledger is deferred to v2). Destroyed assets are excluded — a
 *  retinue is what the player still holds. */
export interface RetinueTally {
	playerID: number;
	counts: Record<AssetType, number>;
	total: number;
	takenFromOthers: number;
}

/** Fixed display order for the four asset types in the recap tally, matching
 *  the choosing-view suit legend (peer, artifact, resource, holding). */
export const RETINUE_TYPE_ORDER: AssetType[] = ['peer', 'artifact', 'resource', 'holding'];

export function retinueTallies(players: { id: number }[], assets: Asset[]): RetinueTally[] {
	return players.map((p) => {
		const counts: Record<AssetType, number> = { peer: 0, artifact: 0, resource: 0, holding: 0 };
		let takenFromOthers = 0;
		let total = 0;
		for (const a of assets) {
			if (a.owner_id !== p.id || a.is_destroyed) continue;
			counts[a.asset_type] += 1;
			total += 1;
			if (a.owner_id !== a.creator_id) takenFromOthers += 1;
		}
		return { playerID: p.id, counts, total, takenFromOthers };
	});
}
