import { apiFetch } from './client';
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

export async function logout(): Promise<void> {
	await fetch('/api/sessions', { method: 'DELETE' });
}

export async function getMe(): Promise<Account | null> {
	const res = await fetch('/api/accounts/me');
	if (res.status === 401) return null;
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	return (await res.json()) as Account;
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
