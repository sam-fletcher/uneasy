import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		// adapter-static outputs a directory of HTML/JS/CSS that the Go binary
		// can serve as static files (or embed). The fallback index.html enables
		// SPA-style client-side routing — all unknown paths return index.html
		// and the client-side router takes over.
		adapter: adapter({
			fallback: 'index.html'
		})
	}
};

export default config;
