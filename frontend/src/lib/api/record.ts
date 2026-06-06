import { apiFetch } from './client';
import type { RecordRow, SceneEntry } from './types';

export function getFullRecord(gameID: string | number): Promise<{ rows: RecordRow[] }> {
	return apiFetch(`/tables/${gameID}/record`);
}

export function createSceneEntry(
	gameID: string | number,
	rowNumber: number,
	body: string
): Promise<{ entry: SceneEntry }> {
	return apiFetch(`/tables/${gameID}/rows/${rowNumber}/summary`, {
		method: 'POST',
		body: JSON.stringify({ body })
	});
}
