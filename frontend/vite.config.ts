import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [sveltekit()],
	test: {
		// Pure-TS unit tests co-located under src/ ($lib plus route folders).
		// The Playwright suite lives under tests/e2e/ (outside src/) and is
		// excluded here.
		include: ['src/lib/**/*.{test,spec}.ts', 'src/routes/**/*.{test,spec}.ts'],
	},
	server: {
		// In docker-compose, Vite needs to accept connections from outside the
		// container (the Go proxy running in a different container).
		host: '0.0.0.0',
		port: 5173,
		// HMR websocket: tell the browser where to connect for hot reload.
		// When running behind the Go proxy on port 8080, HMR still connects
		// directly to Vite on 5173.
		hmr: {
			host: 'localhost',
			port: 5173
		}
	}
});
