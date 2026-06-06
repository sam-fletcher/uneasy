import { apiFetch } from './client';
import type { Law, Rumor } from './types';

export function listLaws(gameID: number): Promise<{ laws: Law[] }> {
	return apiFetch(`/tables/${gameID}/laws`);
}

export function updateLaw(
	lawID: number,
	patch: { text: string; addendum?: string | null }
): Promise<{ law: Law }> {
	return apiFetch(`/laws/${lawID}`, {
		method: 'PATCH',
		body: JSON.stringify(patch),
	});
}

export function listRumors(gameID: number): Promise<{ rumors: Rumor[] }> {
	return apiFetch(`/tables/${gameID}/rumors`);
}

export function updateRumor(rumorID: number, text: string): Promise<{ rumor: Rumor }> {
	return apiFetch(`/rumors/${rumorID}`, {
		method: 'PATCH',
		body: JSON.stringify({ text }),
	});
}
