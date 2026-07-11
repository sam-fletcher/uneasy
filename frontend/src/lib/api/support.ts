// api/support.ts — client for the feedback/reset-request/password-reset
// intakes (adr/FEEDBACK_AND_RESET_PLAN.md). Named "support" rather than
// "feedback" to avoid confusion with the retired lib/feedback.ts (the old
// mailto: link builder).

import { apiFetch } from './client';

export interface FeedbackSubmission {
	id: number;
	kind: 'feedback' | 'reset_request';
	account_id: number | null;
	username: string | null;
	game_id: number | null;
	body: string;
	contact: string | null;
	created_at: string;
}

// POST /api/feedback — session-authed. route/phase/game_id are client-side
// context (server can't see them); the account and User-Agent are stamped
// server-side.
export function submitFeedback(body: {
	body: string;
	contact?: string;
	game_id?: number;
	route?: string;
	phase?: string;
}): Promise<{ submission: FeedbackSubmission }> {
	return apiFetch('/feedback', {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

// POST /api/reset-requests — logged-out. `website` is a hidden honeypot;
// real users never see or fill it in.
export function submitResetRequest(body: {
	username: string;
	contact: string;
	body?: string;
	website?: string;
}): Promise<{ ok: boolean }> {
	return apiFetch('/reset-requests', {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

// POST /api/password-resets — logged-out. On success the caller sends the
// user to the login page; the server deliberately doesn't set a session.
export function submitPasswordReset(body: {
	token: string;
	new_password: string;
}): Promise<{ ok: boolean }> {
	return apiFetch('/password-resets', {
		method: 'POST',
		body: JSON.stringify(body),
	});
}
