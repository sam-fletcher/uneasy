// lib/push.ts — web push permission/subscription orchestration
// (adr/NOTIFICATIONS_PLAN.md Session 4). Wraps the browser Push API +
// service worker registration + the POST/DELETE /api/push-subscriptions
// round trip behind one small state machine so Profile and the lobby
// soft-ask can share it.

import { createPushSubscription, deletePushSubscription } from './api';

export type PushState = 'unsupported' | 'ios-needs-install' | 'denied' | 'off' | 'on';

// Base64url (RFC 4648 §5, no padding) → Uint8Array, for the applicationServerKey
// PushManager.subscribe expects. The server hands us webpush-go's
// GenerateVAPIDKeys output verbatim (already base64url), so this is the only
// conversion needed.
export function urlBase64ToUint8Array(base64String: string): Uint8Array {
	const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
	const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
	const raw = atob(base64);
	const bytes = new Uint8Array(raw.length);
	for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
	return bytes;
}

// iPadOS reports as "MacIntel" but, unlike a real Mac, exposes touch points.
export function isIOSDevice(): boolean {
	if (typeof navigator === 'undefined') return false;
	return (
		/iPad|iPhone|iPod/.test(navigator.userAgent) ||
		(navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
	);
}

export function isStandalonePWA(): boolean {
	if (typeof window === 'undefined') return false;
	return (
		window.matchMedia?.('(display-mode: standalone)').matches === true ||
		(navigator as { standalone?: boolean }).standalone === true
	);
}

export function isPushApiSupported(): boolean {
	return (
		typeof navigator !== 'undefined' &&
		'serviceWorker' in navigator &&
		typeof window !== 'undefined' &&
		'PushManager' in window &&
		'Notification' in window
	);
}

// Pure decision table, kept separate from the feature-detection above so it's
// unit-testable without a real browser.
export function derivePushState(input: {
	isIOS: boolean;
	isStandalone: boolean;
	apiSupported: boolean;
	permission: NotificationPermission;
	subscribed: boolean;
}): PushState {
	// iOS Safari only exposes the Push API to installed home-screen apps —
	// check this before generic feature detection so an un-installed iOS
	// visitor gets "install me" rather than a flat "unsupported".
	if (input.isIOS && !input.isStandalone) return 'ios-needs-install';
	if (!input.apiSupported) return 'unsupported';
	if (input.permission === 'denied') return 'denied';
	if (input.subscribed && input.permission === 'granted') return 'on';
	return 'off';
}

// Recovery steps for the one state we cannot fix in code: once the browser
// has this origin blocked, no API resets it and no second prompt is possible
// (see enablePush). All we can do is tell the player exactly where the switch
// lives, which differs per browser — hence a pure function over the UA string
// so the branches are unit-testable without a real browser.
export type BlockedHelp = {
	steps: string[];
	/** Desktop only: the OS can mute the browser even after the site is allowed. */
	note?: string;
};

export function deriveBlockedHelp(input: { ua: string; isIOS: boolean }): BlockedHelp {
	const { ua, isIOS } = input;
	const retry = 'Reload this page and turn notifications on again.';

	// iOS reaches here only as an installed home-screen app (an un-installed
	// one is 'ios-needs-install'), and there the switch is in iOS Settings
	// rather than anywhere in the browser.
	if (isIOS) {
		return {
			steps: [
				'Open the iOS Settings app and tap Notifications.',
				'Find Uneasy in the list and turn Allow Notifications on.',
				'Reopen Uneasy from your Home Screen and turn notifications on again.',
			],
		};
	}

	// Android before browser family: Chrome, Edge and Firefox all hide this
	// behind the same address-bar icon there.
	if (/Android/.test(ua)) {
		return {
			steps: [
				'Tap the icon just left of the web address, then tap Permissions.',
				'If you don\'t see Notifications in the list, you first need to turn Notifications on for the browser itself.',
				'Set Notifications to Allow.',
				retry,
			],
		};
	}

	const note = /Mac OS X/.test(ua)
		? "If your Mac itself is muting your browser, allowing here won't be enough — check System Settings → Notifications."
		: /Windows/.test(ua)
			? "If Windows itself is muting your browser, allowing here won't be enough — check Settings → System → Notifications."
			: undefined;

	if (/Firefox\//.test(ua)) {
		return {
			steps: [
				'Click the lock icon just left of the web address.',
				'Next to “Send Notifications: Blocked”, click the × to clear the setting.',
				retry,
			],
			note,
		};
	}

	// Edge reports both "Edg/" and "Chrome/"; the steps are identical anyway.
	if (/Chrome\/|Chromium\//.test(ua)) {
		return {
			steps: [
				'Click the icon just left of the web address (a bell, sliders, or a lock).',
				'Set Notifications to Allow.',
				retry,
			],
			note,
		};
	}

	if (/Safari\//.test(ua)) {
		return {
			steps: [
				'Open Safari → Settings → Websites → Notifications.',
				'Find this site in the list and set it to Allow.',
				'Come back here and turn notifications on again.',
			],
			note,
		};
	}

	return {
		steps: ["Open your browser's settings for this site.", 'Set Notifications to Allow.', retry],
		note,
	};
}

// Calls back whenever the notification permission changes — including from
// the browser's own settings UI, in another tab, while this page sits open.
// Without it a player who follows the recovery steps above comes back to a
// screen still insisting they're blocked. Returns an unsubscribe function.
//
// Safari doesn't accept 'notifications' in permissions.query (it rejects), so
// the failure path is silent and callers keep their other refresh triggers.
export function onPushPermissionChange(cb: () => void): () => void {
	if (typeof navigator === 'undefined' || !navigator.permissions?.query) return () => {};
	let status: PermissionStatus | null = null;
	let cancelled = false;
	navigator.permissions
		.query({ name: 'notifications' as PermissionName })
		.then((s) => {
			if (cancelled) return;
			status = s;
			s.addEventListener('change', cb);
		})
		.catch(() => {});
	return () => {
		cancelled = true;
		status?.removeEventListener('change', cb);
	};
}

export async function registerServiceWorker(): Promise<ServiceWorkerRegistration | null> {
	if (!isPushApiSupported()) return null;
	try {
		return await navigator.serviceWorker.register('/service-worker.js', { type: 'module' });
	} catch {
		return null;
	}
}

async function getExistingSubscription(): Promise<PushSubscription | null> {
	if (!isPushApiSupported()) return null;
	const registration = await navigator.serviceWorker.ready.catch(() => null);
	if (!registration) return null;
	return registration.pushManager.getSubscription();
}

export async function getPushState(): Promise<PushState> {
	const apiSupported = isPushApiSupported();
	const sub = apiSupported ? await getExistingSubscription() : null;
	return derivePushState({
		isIOS: isIOSDevice(),
		isStandalone: isStandalonePWA(),
		apiSupported,
		permission: apiSupported ? Notification.permission : 'default',
		subscribed: sub !== null,
	});
}

// Requests permission and subscribes. Must be called from a user gesture
// (a click handler) — browsers ignore or auto-deny permission requests that
// aren't. Returns the resulting state either way.
export async function enablePush(vapidPublicKey: string): Promise<PushState> {
	if (!isPushApiSupported() || !vapidPublicKey) return getPushState();

	// Asking again once we're blocked cannot produce a prompt: the promise
	// resolves 'denied' immediately and Chrome pops its own "Notifications
	// blocked" bubble by the address bar, which reads to the player as the app
	// misbehaving. Bail out so the caller shows deriveBlockedHelp instead.
	if (Notification.permission === 'denied') return getPushState();

	const permission = await Notification.requestPermission();
	if (permission !== 'granted') return getPushState();

	const registration = await navigator.serviceWorker.ready;
	let sub = await registration.pushManager.getSubscription();
	if (!sub) {
		sub = await registration.pushManager.subscribe({
			userVisibleOnly: true,
			applicationServerKey: urlBase64ToUint8Array(vapidPublicKey) as BufferSource,
		});
	}
	await createPushSubscription(sub.toJSON());
	return getPushState();
}

export async function disablePush(): Promise<PushState> {
	const sub = await getExistingSubscription();
	if (sub) {
		const endpoint = sub.endpoint;
		await sub.unsubscribe();
		await deletePushSubscription(endpoint);
	}
	return getPushState();
}
