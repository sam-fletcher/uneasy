// api/index.ts — barrel for the typed API client. Re-exports every domain
// module so existing `import { ... } from '$lib/api'` call sites keep working.
// (client.ts is intentionally not re-exported; apiFetch stays internal to api/.)

export * from './types';
export * from './accounts';
export * from './tables';
export * from './shakeup';
export * from './endgame';
export * from './prologue';
export * from './record';
export * from './chat';
export * from './scenes';
export * from './assets';
export * from './turn';
export * from './rolls';
export * from './plans';
export * from './laws';
export * from './support';
