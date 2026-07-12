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
