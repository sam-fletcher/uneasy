package middleware

import "net/http"

// baseCSP allows same-origin scripts/styles/images. 'unsafe-inline' is
// needed on both style-src (Svelte injects component styles as inline
// <style> tags) and script-src (SvelteKit's adapter-static build emits a
// small inline bootstrap <script> on every page to kick off hydration —
// confirmed via a devtools CSP violation during Session 1 testing).
// Nonce/hash-based CSP would avoid this, but needs a real per-request
// server to mint a nonce; adapter-static's fully static export has none.
// Tighten later only if that changes.
const baseCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data:"

// devViteWS is Vite's HMR WebSocket in the docker-compose dev stack. Pages
// are served through the Go server's proxy on :8080, but Vite's client-side
// HMR code still opens its live-reload socket directly against Vite's own
// port — update this if VITE_URL's default port ever changes.
const devViteWS = "ws://localhost:5173"

// HeadersConfig configures SecurityHeaders. Both fields come from main.go's
// env parsing (DEV_MODE and PUBLIC_ORIGIN respectively) and are fixed for
// the process lifetime.
type HeadersConfig struct {
	// DevMode additionally allows devViteWS in connect-src, for the
	// docker-compose dev stack's Vite HMR socket. Production never needs it.
	DevMode bool
	// SecureMode is true when PUBLIC_ORIGIN starts with https:// (i.e. the
	// server is actually reachable over TLS). Enables HSTS — sending it over
	// plain http would be a lie the browser would remember for a year.
	SecureMode bool
}

// SecurityHeaders sets a baseline set of response headers appropriate for a
// public site. Applied router-wide (API and static frontend alike).
func SecurityHeaders(cfg HeadersConfig) func(http.Handler) http.Handler {
	connectSrc := "connect-src 'self' wss:"
	if cfg.DevMode {
		connectSrc += " " + devViteWS
	}
	csp := baseCSP + "; " + connectSrc

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("Content-Security-Policy", csp)
			if cfg.SecureMode {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}
