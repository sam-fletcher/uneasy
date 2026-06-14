import { apiFetch } from './client';
import type {
	PrologueSheet, PrologueClaim, PlayerCardRow, PrologueSheetType, AssetType,
	PrologueRankingStep, Asset,
} from './types';

export function getPrologueSheets(
	gameID: string | number
): Promise<{
	sheets: PrologueSheet[];
	claims: PrologueClaim[];
	current_player_id: number | null;
	turn_number: number;
}> {
	return apiFetch(`/tables/${gameID}/prologue/sheets`);
}

export function getPrologueCards(
	gameID: string | number
): Promise<{ cards: PlayerCardRow[] }> {
	return apiFetch(`/tables/${gameID}/prologue/cards`);
}

export interface PrologueCardAssetText {
	suit: string;
	value: string;
	text: string;
}

export function choosePrologue(
	gameID: string | number,
	body: {
		sheet_type: PrologueSheetType;
		choice_name: string;
		asset_text: string;
		marginalia_text?: string;
		law_or_rumor_text?: string;
		card_assets: PrologueCardAssetText[];
	}
): Promise<{ sheet_type: PrologueSheetType; choice_name: string; turn_number: number }> {
	return apiFetch(`/tables/${gameID}/prologue/choose`, {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

export function getPrologueCardSuggestions(
	gameID: string | number,
	suit: string,
): Promise<{ suggestions: string[]; asset_type: string }> {
	return apiFetch(`/tables/${gameID}/prologue/card-suggestions?suit=${encodeURIComponent(suit)}`);
}

/**
 * Type-keyed example strings for player-authored asset text, with anything
 * already in play filtered out. kind='name' suggests asset names; 'marginalia'
 * suggests marginalia. Returns up to 3 (fewer when the unused pool is small).
 */
export function getAssetSuggestions(
	gameID: string | number,
	assetType: AssetType,
	kind: 'name' | 'marginalia',
): Promise<{ suggestions: string[]; asset_type: string }> {
	return apiFetch(
		`/tables/${gameID}/asset-suggestions?asset_type=${encodeURIComponent(assetType)}&kind=${kind}`,
	);
}

export function placePrologueSetAsides(
	gameID: string | number,
	ordering: number[]
): Promise<{ track: string; next_step: PrologueRankingStep | '' }> {
	return apiFetch(`/tables/${gameID}/prologue/place-set-asides`, {
		method: 'POST',
		body: JSON.stringify({ ordering })
	});
}

export function createExtraPeer(
	gameID: string | number,
	titleName: string,
	peerText: string,
): Promise<{ asset: Asset }> {
	return apiFetch(`/tables/${gameID}/prologue/extra-peer`, {
		method: 'POST',
		body: JSON.stringify({ title_name: titleName, peer_text: peerText })
	});
}

// ── Phase 4b: max-commitment prologue ranking ────────────────────────────────

export type PrologueTrack = 'power' | 'knowledge' | 'esteem';

export interface CommittedHeart {
	player_id: number;
	track: PrologueTrack;
	card_id: number;
	value: string;
	suit: 'H';
}

export interface TrackDone {
	player_id: number;
	track: PrologueTrack;
	done: boolean;
}

export interface ExtraPeer {
	player_id: number;
	title_name: string;
	asset_id: number;
}

export interface PrologueRankingState {
	committed: CommittedHeart[];
	done: TrackDone[];
	extra_peers: ExtraPeer[];
}

export function getPrologueRankingState(
	gameID: string | number
): Promise<PrologueRankingState> {
	return apiFetch(`/tables/${gameID}/prologue/ranking-state`);
}

export function commitTrackHearts(
	gameID: string | number,
	track: PrologueTrack,
	cardIDs: number[]
): Promise<{ track: PrologueTrack; card_ids: number[] }> {
	return apiFetch(`/tables/${gameID}/prologue/committed-hearts`, {
		method: 'POST',
		body: JSON.stringify({ track, card_ids: cardIDs })
	});
}

export function setPrologueDone(
	gameID: string | number,
	track: PrologueTrack,
	done: boolean
): Promise<{ track: PrologueTrack; done: boolean }> {
	return apiFetch(`/tables/${gameID}/prologue/done`, {
		method: 'POST',
		body: JSON.stringify({ track, done })
	});
}
