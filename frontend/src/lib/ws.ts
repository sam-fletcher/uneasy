// ws.ts — WebSocket client with automatic reconnection and catch-up.
//
// Usage:
//   const disconnect = createConnection(gameID, (msg) => { ... });
//   // later:
//   disconnect();

export interface WSMessage {
	type: string;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	payload: Record<string, any>;
}

type MessageHandler = (msg: WSMessage) => void;

// createConnection opens a WebSocket to /api/tables/:id/ws and returns a
// cleanup function. Reconnects automatically with exponential backoff if the
// connection drops.
export function createConnection(gameID: string | number, onMessage: MessageHandler): () => void {
	let ws: WebSocket | null = null;
	let stopped = false;
	let retryDelay = 1000; // ms; doubles on each failed attempt, capped at 30s
	let lastPostID: number | null = null; // for catch-up on reconnect

	// Listen for typing events dispatched by the page component.
	function onTyping(e: Event) {
		const { typing } = (e as CustomEvent<{ typing: boolean }>).detail;
		send({ type: typing ? 'typing.start' : 'typing.stop' });
	}
	window.addEventListener('uneasy:typing', onTyping);

	function send(msg: object) {
		if (ws?.readyState === WebSocket.OPEN) {
			ws.send(JSON.stringify(msg));
		}
	}

	function connect() {
		if (stopped) return;

		const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
		const url = `${protocol}//${location.host}/api/tables/${gameID}/ws`;
		ws = new WebSocket(url);

		ws.onopen = () => {
			console.log('[ws] connected');
			retryDelay = 1000; // reset backoff on success
		};

		ws.onmessage = (event) => {
			let msg: WSMessage;
			try {
				msg = JSON.parse(event.data as string) as WSMessage;
			} catch {
				return;
			}

			// Track the latest post ID so we can catch up after a reconnect.
			if (msg.type === 'post.created' && msg.payload.post?.id) {
				lastPostID = msg.payload.post.id as number;
			}

			onMessage(msg);
		};

		ws.onclose = () => {
			if (stopped) return;
			console.log(`[ws] disconnected — retrying in ${retryDelay}ms`);
			setTimeout(() => {
				retryDelay = Math.min(retryDelay * 2, 30_000);
				connect();
			}, retryDelay);
		};

		ws.onerror = () => {
			// onerror is always followed by onclose, which handles reconnect.
			ws?.close();
		};
	}

	connect();

	return () => {
		stopped = true;
		window.removeEventListener('uneasy:typing', onTyping);
		ws?.close();
	};
}
