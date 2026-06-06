import { apiFetch } from './client';
import type { Asset, AssetType, Marginalium, Secret } from './types';

export function listAssets(gameID: string | number): Promise<{ assets: Asset[] }> {
	return apiFetch(`/tables/${gameID}/assets`);
}

export function createAsset(
	gameID: string | number,
	params: {
		asset_type: AssetType;
		name: string;
		is_main_character?: boolean;
		marginalia?: string[];
	}
): Promise<{ asset: Asset }> {
	return apiFetch(`/tables/${gameID}/assets`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

export function updateAsset(
	assetID: number,
	params: { name?: string; is_main_character?: boolean; tear_position?: number }
): Promise<{ asset: Asset }> {
	return apiFetch(`/assets/${assetID}`, {
		method: 'PUT',
		body: JSON.stringify(params)
	});
}

// ── Marginalia ────────────────────────────────────────────────────────────────

export function addMarginalia(
	assetID: number,
	text: string
): Promise<{ marginalia: Marginalium }> {
	return apiFetch(`/assets/${assetID}/marginalia`, {
		method: 'POST',
		body: JSON.stringify({ text })
	});
}

export function updateMarginalia(
	assetID: number,
	position: number,
	text: string
): Promise<{ marginalia: Marginalium }> {
	return apiFetch(`/assets/${assetID}/marginalia/${position}`, {
		method: 'PUT',
		body: JSON.stringify({ text })
	});
}

export function tearMarginalia(
	assetID: number,
	position: number
): Promise<{ torn: boolean; destroyed: boolean }> {
	return apiFetch(`/assets/${assetID}/marginalia/${position}`, {
		method: 'DELETE'
	});
}

// ── Leverage / Refresh / Take ─────────────────────────────────────────────────

export function leverageAsset(assetID: number): Promise<{ leveraged: boolean }> {
	return apiFetch(`/assets/${assetID}/leverage`, { method: 'POST' });
}

export function refreshAsset(assetID: number): Promise<{ leveraged: boolean }> {
	return apiFetch(`/assets/${assetID}/refresh`, { method: 'POST' });
}

export function takeAsset(assetID: number): Promise<{ asset: Asset }> {
	return apiFetch(`/assets/${assetID}/take`, { method: 'POST' });
}

// ── Secrets ───────────────────────────────────────────────────────────────────

export function writeSecret(assetID: number, text: string): Promise<{ secret: Secret }> {
	return apiFetch(`/assets/${assetID}/secrets`, {
		method: 'POST',
		body: JSON.stringify({ text })
	});
}

export function getSecrets(assetID: number): Promise<{ secrets: Secret[] }> {
	return apiFetch(`/assets/${assetID}/secrets`);
}

/** All secrets in this game that the caller can see (one query for the retinue). */
export function getVisibleSecrets(gameID: string | number): Promise<{ secrets: Secret[] }> {
	return apiFetch(`/tables/${gameID}/secrets/visible`);
}
