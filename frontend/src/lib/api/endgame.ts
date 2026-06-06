import { apiFetch } from './client';

export type EndgameMode = 'smooth_landing' | 'explosive_finale';

export function setEndgameMode(
	gameID: string | number,
	mode: EndgameMode
): Promise<{ mode: EndgameMode }> {
	return apiFetch(`/tables/${gameID}/endgame`, {
		method: 'POST',
		body: JSON.stringify({ mode })
	});
}
