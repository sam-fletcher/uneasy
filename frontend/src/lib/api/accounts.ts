import { ApiError, apiFetch } from './client';
import type { Account, MyTable } from './types';

export function createAccount(body: {
	username: string;
	password: string;
	email?: string | null;
}): Promise<Account> {
	return apiFetch<Account>('/accounts', {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

export function login(username: string, password: string): Promise<Account> {
	return apiFetch<Account>('/sessions', {
		method: 'POST',
		body: JSON.stringify({ username, password })
	});
}

// Returns 204 No Content. Used to be a raw fetch that never checked res.ok,
// which made a failed logout indistinguishable from a successful one — the
// caller navigated away and the session cookie stayed live. apiFetch handles
// the empty body now, so this can report failure like everything else.
export async function logout(): Promise<void> {
	await apiFetch<void>('/sessions', { method: 'DELETE' });
}

/** Null means "not logged in" — a 401 here is the ordinary logged-out answer,
 *  not a failure, and every caller treats it as one. Anything else throws. */
export async function getMe(): Promise<Account | null> {
	try {
		return await apiFetch<Account>('/accounts/me');
	} catch (e) {
		if (e instanceof ApiError && e.status === 401) return null;
		throw e;
	}
}

export function updateMe(patch: {
	username?: string;
	email?: string | null;
	password?: string;
	notify_cadence_hours?: number | null;
}): Promise<Account> {
	return apiFetch<Account>('/accounts/me', {
		method: 'PATCH',
		body: JSON.stringify(patch)
	});
}

export function listMyTables(): Promise<{ tables: MyTable[] }> {
	return apiFetch('/accounts/me/tables');
}
