import { apiFetch } from './client';
import type { Account, MyTable } from './types';

export function createAccount(body: {
	username: string;
	code: string;
	email?: string | null;
}): Promise<Account> {
	return apiFetch<Account>('/accounts', {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

export function login(username: string, code: string): Promise<Account> {
	return apiFetch<Account>('/sessions', {
		method: 'POST',
		body: JSON.stringify({ username, code })
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
	code?: string;
}): Promise<Account> {
	return apiFetch<Account>('/accounts/me', {
		method: 'PATCH',
		body: JSON.stringify(patch)
	});
}

export function listMyTables(): Promise<{ tables: MyTable[] }> {
	return apiFetch('/accounts/me/tables');
}
