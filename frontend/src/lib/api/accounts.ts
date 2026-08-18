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
// Concurrent callers share one request. The root layout and the table page
// both need the account on mount and neither knows about the other, so they
// each used to fire their own /accounts/me — two identical requests in
// parallel, each paying the server's session + account query floor.
//
// Deliberately in-flight only: the slot is cleared the moment the request
// settles, so this coalesces the simultaneous-mount case and nothing else.
// Caching a *resolved* account would make login, logout and profile edits
// serve a stale identity, which is a far worse bug than a duplicate GET.
let inFlightMe: Promise<Account | null> | null = null;

export function getMe(): Promise<Account | null> {
	if (inFlightMe) return inFlightMe;

	const pending = (async () => {
		try {
			return await apiFetch<Account>('/accounts/me');
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) return null;
			throw e;
		}
	})();
	inFlightMe = pending;

	// `finally`, not `then`: a rejected fetch has to free the slot too, or every
	// later caller would re-await the same stale failure forever. The identity
	// check keeps a slow failure from clearing a newer request's slot, and the
	// trailing catch is for this derived promise only — `pending` itself is
	// returned to the caller, who handles it.
	pending
		.finally(() => { if (inFlightMe === pending) inFlightMe = null; })
		.catch(() => {});

	return pending;
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
