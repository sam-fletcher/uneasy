// Disable SSR globally — we're running as a pure SPA backed by the Go API.
// SvelteKit still gives us file-based routing; it just renders everything
// client-side rather than on a Node server.
export const ssr = false;
export const prerender = false;
