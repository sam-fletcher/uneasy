// Command server is the Uneasy API and WebSocket server.
//
// Configuration is via environment variables:
//
//	DATABASE_URL   Postgres connection string (required)
//	PORT           HTTP listen port (default: 8080)
//	DEV_MODE       If "true", proxy non-API requests to VITE_URL
//	VITE_URL       Vite dev server address (default: http://localhost:5173)
//	PUBLIC_ORIGIN  Public URL the server is reachable at, e.g.
//	               "https://uneasy.example". Unset = today's dev behavior
//	               (cookies without Secure, no HSTS, WebSocket accepts any
//	               Origin). When set with an "https://" scheme: session
//	               cookies get the Secure flag, responses get HSTS, and the
//	               WebSocket handshake only accepts the given host as Origin.
//	DISCORD_WEBHOOK_URL  Discord webhook for feedback/reset-request
//	               notifications (adr/FEEDBACK_AND_RESET_PLAN.md). Unset in
//	               dev: notifications are logged to stdout instead of posted.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/db"
	"uneasy/handler"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

const (
	timeOutRead     = 15 * time.Second
	timeOutReqest   = 30 * time.Second
	timeOutWrite    = 0 // 0 = no write timeout (needed for WebSocket and SSE)
	timeOutIdle     = 60 * time.Second
	timeOutShutdown = 10 * time.Second
)

// poolMaxConnIdleTime closes pool connections that sit unused, rather than
// pgx's 30-minute default. A serverless Postgres (e.g. Neon) suspends after
// ~5 idle minutes and severs open connections when it does; closing ours
// first means the pool isn't holding dead connections through a quiet
// stretch. The pool reopens connections on demand.
const poolMaxConnIdleTime = 2 * time.Minute

// credentialRateLimit and credentialRateWindow bound POST /api/sessions
// (login) and POST /api/accounts (signup), sharing one bucket per IP.
// Generous enough that no honest player ever sees it; it exists to end
// offline-speed bcrypt guessing and drive-by account spam.
const (
	credentialRateLimit  = 10
	credentialRateWindow = time.Minute
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	dbURL := mustEnv("DATABASE_URL", logger)
	port := env("PORT", "8080")
	devMode := env("DEV_MODE", "false") == "true"
	viteURL := env("VITE_URL", "http://localhost:5173")
	publicOrigin := env("PUBLIC_ORIGIN", "")
	secureMode := strings.HasPrefix(publicOrigin, "https://")
	publicHost := parsePublicHost(publicOrigin)
	discordWebhookURL := env("DISCORD_WEBHOOK_URL", "")

	// ── Server ────────────────────────────────────────────────────────────────

	if err := runServer(logger, dbURL, port, devMode, viteURL, secureMode, publicHost, discordWebhookURL); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// parsePublicHost extracts the host (e.g. "uneasy.example") from a
// PUBLIC_ORIGIN value like "https://uneasy.example". Returns "" if origin is
// unset or unparseable, in which case callers fall back to dev behavior.
func parsePublicHost(origin string) string {
	if origin == "" {
		return ""
	}
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return u.Host
}

// runServer starts the server and handles all initialization.
func runServer(
	logger *slog.Logger,
	dbURL, port string,
	devMode bool,
	viteURL string,
	secureMode bool,
	publicHost string,
	discordWebhookURL string,
) error {
	poolCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	poolCfg.MaxConnIdleTime = poolMaxConnIdleTime
	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer dbPool.Close()

	// Verify the connection is live.
	if err = dbPool.Ping(context.Background()); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	logger.Info("connected to database")

	if err = db.RunMigrations(dbURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	logger.Info("migrations applied")

	store := db.NewStore(dbPool)

	if err := store.Q.DeleteExpiredSessions(context.Background()); err != nil {
		logger.Warn("startup expired-session cleanup failed", "error", err)
	} else {
		logger.Info("expired sessions cleaned up")
	}
	go expireSessionsDaily(logger, store)

	manager := hub.NewManager()

	router := setupRouter(logger, store, manager, devMode, secureMode, publicHost, discordWebhookURL)

	if err := setupFrontend(router, devMode, viteURL); err != nil {
		return err
	}

	// Start server

	addr := fmt.Sprintf(":%s", port)
	logger.Info("server starting", "addr", addr, "dev_mode", devMode)

	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  timeOutRead,
		WriteTimeout: timeOutWrite,
		IdleTimeout:  timeOutIdle,
	}

	// Serve until the platform asks us to stop (SIGTERM on deploys and
	// machine stops; SIGINT for ctrl-C locally), then drain in-flight
	// requests. Shutdown doesn't wait for hijacked connections, so open
	// WebSockets don't block it — they die with the process, and clients
	// auto-reconnect (see frontend/src/lib/ws.ts).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutdown signal received, draining requests")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeOutShutdown)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

// setupRouter creates and configures the HTTP router with all middleware and routes.
//
//nolint:funlen // route table; one line per endpoint
func setupRouter(
	logger *slog.Logger,
	store *db.Store,
	manager *hub.Manager,
	devMode, secureMode bool,
	publicHost string,
	discordWebhookURL string,
) *chi.Mux {
	r := chi.NewRouter()

	// Configure cookie/WebSocket-origin behavior once, before any routes are
	// mounted — mirrors how UNEASY_DEV is read once at startup below.
	handler.SetSecureCookies(secureMode)
	handler.SetDiscordWebhookURL(discordWebhookURL)
	wsOriginPatterns := []string{"*"}
	if publicHost != "" {
		wsOriginPatterns = []string{publicHost}
	}

	// Standard middleware: structured request logging and panic recovery.
	// The request timeout is applied per-scope below — WebSocket connections
	// are long-lived and must not run under it (their request context would
	// be cancelled mid-session, killing the connection).
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(handler.LoggerMiddleware(logger))
	r.Use(appMiddleware.SecurityHeaders(appMiddleware.HeadersConfig{DevMode: devMode, SecureMode: secureMode}))

	// Shared by the two credential endpoints below: one bucket per IP across
	// both login and signup. KeyByIP reads r.RemoteAddr, which RealIP (above)
	// has already rewritten from X-Forwarded-For/X-Real-IP — Fly sets XFF, so
	// this resolves the real client IP behind the proxy. chi v5.2.5 predates
	// the non-deprecated ClientIPFrom*/GetClientIP replacement (needs v5.3+);
	// revisit if chi is ever bumped.
	credentialLimiter := httprate.LimitBy(credentialRateLimit, credentialRateWindow,
		httprate.KeyByIP, //nolint:staticcheck // deprecated; see comment above — RealIP already normalizes RemoteAddr
		httprate.WithLimitHandler(tooManyAttempts))

	// Liveness probe for the hosting platform. Deliberately DB-free: a
	// health check that queried Postgres would count as activity and keep a
	// scale-to-zero database (e.g. Neon) awake around the clock.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Profiling endpoints (/debug/pprof/*) — dev-gated like /api/dev/*,
	// never mounted in production.
	if os.Getenv("UNEASY_DEV") == "1" {
		r.Mount("/debug", chimiddleware.Profiler())
	}

	// API routes — all behind the cookie-auth middleware.
	r.Route("/api", func(r chi.Router) {
		r.Use(appMiddleware.EnsureSession(store.Q))

		// WebSocket — long-lived, must run outside any per-request timeout.
		// Registered before the timeout-bearing group below.
		r.Get("/tables/{id}/ws", handler.WebSocket(store, manager, wsOriginPatterns))

		// Everything else: short-lived HTTP requests under a request timeout
		// so a misbehaving client can't tie up a goroutine indefinitely.
		r.Group(func(r chi.Router) {
			r.Use(chimiddleware.Timeout(timeOutReqest))
			// Body size cap. Scoped to this group (not the outer /api Route)
			// so the WebSocket route above is unaffected.
			r.Use(appMiddleware.BodyLimit)

			// Accounts & sessions
			r.With(credentialLimiter).Post("/accounts", handler.CreateAccount(store))
			r.Get("/accounts/me", handler.GetMe())
			r.Patch("/accounts/me", handler.UpdateMe(store))
			r.Get("/accounts/me/tables", handler.ListMyTables(store))
			r.With(credentialLimiter).Post("/sessions", handler.CreateSession(store))
			r.Delete("/sessions", handler.DeleteSession(store))

			// Feedback & password-reset intake (adr/FEEDBACK_AND_RESET_PLAN.md).
			// CreateFeedback is session-authed (checks AccountFromContext itself,
			// like GetMe/UpdateMe above); the reset-request endpoint is logged-out
			// by design — the requester is precisely the person who can't log in —
			// so it shares the credential endpoints' IP rate limit instead.
			r.Post("/feedback", handler.CreateFeedback(store))
			r.With(credentialLimiter).Post("/reset-requests", handler.CreateResetRequest(store))

			// Dev-only routes — gated by UNEASY_DEV=1. Never mount in prod.
			if os.Getenv("UNEASY_DEV") == "1" {
				logger.Warn("dev routes are MOUNTED — never run this in production")
				r.Post("/dev/login", handler.DevLogin(store))
				r.Post("/dev/seed", handler.DevSeed(store))
				r.Post("/dev/advance-row", handler.DevAdvanceRow(store, manager))
				r.Post("/dev/delete-game", handler.DevDeleteGame(store))
			} else {
				logger.Info("dev routes are OFF")
			}

			// Tables (creation, join, info)
			r.Post("/tables", handler.CreateTable(store, manager))
			r.Post("/tables/join", handler.JoinTable(store, manager))
			r.Get("/tables/{id}", handler.GetTable(store))
			r.Get("/tables/{id}/state", handler.GetGameState(store))

			// Phase transitions (facilitator actions)
			r.Post("/tables/{id}/start-prologue", handler.StartPrologue(store, manager))
			r.Post("/tables/{id}/endgame", handler.SetEndgameMode(store, manager))

			// Shake-Up (Phase 4c)
			r.Get("/tables/{id}/shake-up", handler.GetShakeUp(store))
			r.Post("/tables/{id}/shake-up/spend", handler.ShakeUpAnnounce(store, manager))
			r.Post("/tables/{id}/shake-up/adjust", handler.ShakeUpAdjust(store, manager))
			r.Post("/tables/{id}/shake-up/pass", handler.ShakeUpPass(store, manager))
			r.Post("/tables/{id}/shake-up/commit", handler.ShakeUpCommit(store, manager))

			// Tone-setting
			r.Get("/tables/{id}/tone", handler.ListToneTopics(store))
			r.Put("/tables/{id}/tone/{topicId}", handler.UpdateToneTopic(store, manager))
			r.Post("/tables/{id}/tone", handler.AddToneTopic(store, manager))

			// Rankings (read-only; ranking flow lives under /prologue/*)
			r.Get("/tables/{id}/rankings", handler.GetRankings(store))

			// Structured prologue (Phase 4b)
			r.Get("/tables/{id}/prologue/sheets", handler.GetPrologueSheets(store))
			r.Get("/tables/{id}/prologue/cards", handler.GetPrologueCards(store))
			r.Get("/tables/{id}/prologue/card-suggestions", handler.GetPrologueCardSuggestions(store))
			r.Post("/tables/{id}/prologue/choose", handler.ChoosePrologue(store, manager))
			r.Post("/tables/{id}/prologue/place-set-asides", handler.PlaceSetAsides(store, manager))
			r.Post("/tables/{id}/prologue/extra-peer", handler.CreateExtraPeer(store, manager))
			r.Get("/tables/{id}/prologue/ranking-state", handler.GetPrologueRankingState(store))
			r.Post("/tables/{id}/prologue/committed-hearts", handler.CommitTrackHearts(store, manager))
			r.Post("/tables/{id}/prologue/done", handler.SetPrologueDone(store, manager))

			// Assets (list + create on the table; per-asset actions by asset ID)
			r.Get("/tables/{id}/assets", handler.ListAssets(store))
			r.Get("/tables/{id}/asset-suggestions", handler.GetAssetSuggestions(store))
			r.Get("/tables/{id}/secrets/visible", handler.ListVisibleSecretsForGame(store))
			r.Post("/tables/{id}/assets", handler.CreateAsset(store, manager))
			r.Post("/tables/{id}/replace-main-character", handler.ReplaceMainCharacterWithNewPeer(store, manager))

			r.Route("/assets/{assetId}", func(r chi.Router) {
				r.Put("/", handler.UpdateAsset(store, manager))
				r.Post("/marginalia", handler.AddMarginalia(store, manager))
				r.Put("/marginalia/{pos}", handler.UpdateMarginalia(store, manager))
				r.Delete("/marginalia/{pos}", handler.TearMarginalia(store, manager))
				r.Post("/leverage", handler.LeverageAsset(store, manager))
				r.Post("/refresh", handler.RefreshAsset(store, manager))
				r.Post("/take", handler.TakeAsset(store, manager))
				r.Post("/secrets", handler.WriteSecret(store, manager))
				r.Get("/secrets", handler.GetSecrets(store))
			})

			// Laws & rumors (long-form narrative records written by players)
			r.Get("/tables/{id}/laws", handler.ListLaws(store))
			r.Patch("/laws/{lawId}", handler.UpdateLaw(store, manager))
			r.Get("/tables/{id}/rumors", handler.ListRumors(store))
			r.Patch("/rumors/{rumorId}", handler.UpdateRumor(store, manager))

			// Public record + scene threading
			r.Get("/tables/{id}/record", handler.GetFullRecord(store))
			r.Get("/tables/{id}/posts", handler.ListGamePosts(store))
			r.Post("/tables/{id}/posts", handler.CreatePlayerPost(store, manager))
			r.Get("/tables/{id}/posts/anchor", handler.GetPostAnchor(store))
			r.Put("/tables/{id}/read-marker", handler.UpdateReadMarker(store))
			r.Post("/tables/{id}/rows/{row}/summary", handler.CreateSceneEntry(store, manager))

			// Scenes (SCENES_PLAN.md)
			r.Post("/tables/{id}/scenes", handler.CreateScene(store, manager))
			r.Get("/tables/{id}/scenes/active", handler.GetActiveSceneHandler(store))
			r.Post("/tables/{id}/scenes/{sid}/claim-peer", handler.ClaimScenePeer(store, manager))

			// Turn structure (Phase 2d)
			r.Post("/tables/{id}/end-scene", handler.EndScene(store, manager))
			r.Post("/tables/{id}/refresh-assets", handler.RefreshAssets(store, manager))
			r.Post("/tables/{id}/advance-row", handler.AdvanceRow(store, manager))
			r.Post("/tables/{id}/pass-focus", handler.PassFocus(store, manager))

			// Dice rolls (Phase 2e)
			r.Get("/tables/{id}/rolls/active", handler.GetActiveRollForGame(store))
			r.Post("/tables/{id}/rolls", handler.CreateRoll(store, manager))
			r.Get("/tables/{id}/banked-dice", handler.ListBankedDice(store))
			r.Route("/rolls/{rollId}", func(r chi.Router) {
				r.Get("/", handler.GetRoll(store))
				r.Post("/leverage", handler.LeverageRoll(store, manager))
				r.Post("/call-vote", handler.CallVote(store, manager))
				r.Post("/skip-vote", handler.SkipVote(store, manager))
				r.Post("/vote", handler.Vote(store, manager))
				r.Post("/intent", handler.SetIntent(store, manager))
				r.Post("/ready", handler.SetReady(store, manager))
				r.Post("/close-leverage", handler.CloseLeverage(store, manager))
				r.Post("/use-banked-die", handler.UseBankedDie(store, manager))
			})

			// Simultaneous reveals (Phase 3c)
			r.Route("/reveals/{revealId}", func(r chi.Router) {
				r.Get("/", handler.GetReveal(store))
				r.Post("/submit", handler.SubmitReveal(store, manager))
			})

			// Make War — read-side war state
			r.Get("/tables/{id}/wars", handler.ListWars(store))

			// Plans (Phase 3a+)
			r.Get("/tables/{id}/plans", handler.ListPlans(store))
			r.Get("/tables/{id}/plan-tokens", handler.ListPlanTokens(store))
			r.Get("/tables/{id}/plan-eligibility", handler.PlanEligibility(store))
			r.Post("/tables/{id}/prepare-plan", handler.PreparePlan(store, manager))
			r.Route("/plans/{planId}", func(r chi.Router) {
				r.Get("/", handler.GetPlan(store))
				r.Get("/duel-state", handler.GetDuelState(store))
				r.Get("/war-state", handler.GetWarState(store))
				r.Post("/resolve", handler.ResolvePlan(store, manager))
				r.Post("/make-choice", handler.MakeChoice(store, manager))
				r.Post("/complete", handler.CompletePlan(store, manager))
				// Mount plan-type-specific routes from the registry (e.g. fair-trade,
				// messy-break for Exchange Courtiers; future plans add their own).
				// Every handler's extra routes share this one flat path namespace,
				// so two plans registering the same key would silently shadow each
				// other (the symptom: one plan's request hitting another's handler).
				// Guard against it at startup: seed with the core per-plan routes and
				// panic on any collision so the conflict surfaces at boot, not in a
				// confusing 4xx mid-game.
				deps := &handler.PlanDeps{Store: store, Manager: manager}
				routeOwner := map[string]string{
					"":            "core", // POST /plans/{planId}
					"complete":    "core",
					"make-choice": "core",
					"resolve":     "core",
				}
				for pt, h := range handler.AllHandlers() {
					for route, fn := range h.ExtraRoutes(deps) {
						if owner, dup := routeOwner[route]; dup {
							panic(fmt.Sprintf(
								"duplicate plan extra-route %q: registered by both %s and %s",
								route, owner, pt))
						}
						routeOwner[route] = string(pt)
						r.Post("/"+route, fn)
					}
				}
			})
		}) // end timeout-bearing group
	})
	return r
}

//go:embed all:frontend_dist
var frontendFS embed.FS

// setupFrontend configures frontend routing (Vite proxy in dev, static in prod).
func setupFrontend(r *chi.Mux, devMode bool, viteURL string) error {
	if devMode {
		target, err := url.Parse(viteURL)
		if err != nil {
			return fmt.Errorf("parse vite url: %w", err)
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		r.Handle("/*", proxy)
		return nil
	}

	sub, err := fs.Sub(frontendFS, "frontend_dist")
	if err != nil {
		return fmt.Errorf("sub frontend_dist: %w", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// SPA fallback: if the requested path doesn't exist in the embed,
		// serve index.html so client-side routing can take over.
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			req = req.Clone(req.Context())
			req.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, req)
	}))
	return nil
}

// tooManyAttempts is the httprate limit-exceeded handler for the credential
// endpoints, shaped like handler package's {"error": "..."} responses.
func tooManyAttempts(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "too many attempts — wait a minute",
	})
}

// sessionCleanupInterval is how often expireSessionsDaily sweeps expired
// sessions after the one-off startup cleanup in runServer.
const sessionCleanupInterval = 24 * time.Hour

// expireSessionsDaily deletes year-stale sessions once a day for the life of
// the process. TouchSession bumps last_seen on every authenticated request,
// so an active player's session is never touched by this — only sessions
// abandoned for a full year are removed.
func expireSessionsDaily(logger *slog.Logger, store *db.Store) {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := store.Q.DeleteExpiredSessions(context.Background()); err != nil {
			logger.Warn("expired session cleanup failed", "error", err)
		}
	}
}

// env returns the value of key, or fallback if unset/empty.
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mustEnv returns the value of key, or exits if it's unset/empty.
func mustEnv(key string, logger *slog.Logger) string {
	v := os.Getenv(key)
	if v == "" {
		logger.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}
