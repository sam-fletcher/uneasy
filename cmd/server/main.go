// Command server is the Uneasy API and WebSocket server.
//
// Configuration is via environment variables:
//
//	DATABASE_URL   Postgres connection string (required)
//	PORT           HTTP listen port (default: 8080)
//	DEV_MODE       If "true", proxy non-API requests to VITE_URL
//	VITE_URL       Vite dev server address (default: http://localhost:5173)
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/db"
	"uneasy/game"
	"uneasy/handler"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

const (
	timeOutRead   = 15 * time.Second
	timeOutReqest = 30 * time.Second
	timeOutWrite  = 0 // 0 = no write timeout (needed for WebSocket and SSE)
	timeOutIdle   = 60 * time.Second
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

	// ── Server ────────────────────────────────────────────────────────────────

	if err := runServer(logger, dbURL, port, devMode, viteURL); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// runServer starts the server and handles all initialization.
func runServer(logger *slog.Logger, dbURL, port string, devMode bool, viteURL string) error {
	dbPool, err := pgxpool.New(context.Background(), dbURL)
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

	manager := hub.NewManager()

	router := setupRouter(logger, store, manager)

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

	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// setupRouter creates and configures the HTTP router with all middleware and routes.
//
//nolint:funlen // route table; one line per endpoint
func setupRouter(logger *slog.Logger, store *db.Store, manager *hub.Manager) *chi.Mux {
	r := chi.NewRouter()

	// Standard middleware: structured request logging, panic recovery,
	// and a request timeout so misbehaving clients can't hold goroutines.
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(handler.LoggerMiddleware(logger))
	r.Use(chimiddleware.Timeout(timeOutReqest))

	// API routes — all behind the cookie-auth middleware.
	r.Route("/api", func(r chi.Router) {
		r.Use(appMiddleware.EnsureSession(store.Q))

		// Accounts & sessions
		r.Post("/accounts", handler.CreateAccount(store))
		r.Get("/accounts/me", handler.GetMe())
		r.Patch("/accounts/me", handler.UpdateMe(store))
		r.Get("/accounts/me/tables", handler.ListMyTables(store))
		r.Post("/sessions", handler.CreateSession(store))
		r.Delete("/sessions", handler.DeleteSession(store))

		// Dev-only routes — gated by UNEASY_DEV=1. Never mount in prod.
		if os.Getenv("UNEASY_DEV") == "1" {
			r.Post("/dev/login", handler.DevLogin(store))
			r.Post("/dev/reset", handler.DevReset(store))
		}

		// Tables (creation, join, info)
		r.Post("/tables", handler.CreateTable(store, manager))
		r.Post("/tables/join", handler.JoinTable(store, manager))
		r.Get("/tables/{id}", handler.GetTable(store))
		r.Get("/tables/{id}/state", handler.GetGameState(store))

		// Phase transitions (facilitator actions)
		r.Post("/tables/{id}/start-prologue", handler.StartPrologue(store, manager))
		r.Post("/tables/{id}/start-main-event", handler.StartMainEvent(store, manager))
		r.Post("/tables/{id}/endgame", handler.SetEndgameMode(store, manager))

		// Shake-Up (Phase 4c)
		r.Get("/tables/{id}/shake-up", handler.GetShakeUp(store))
		r.Post("/tables/{id}/shake-up/roll", handler.ShakeUpRoll(store, manager))
		r.Post("/tables/{id}/shake-up/spend", handler.ShakeUpAnnounce(store, manager))
		r.Post("/tables/{id}/shake-up/adjust", handler.ShakeUpAdjust(store, manager))
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
		r.Post("/tables/{id}/prologue/declare-hearts", handler.DeclareHearts(store, manager))
		r.Post("/tables/{id}/prologue/finalize-ranking", handler.FinalizeTrackRanking(store, manager))
		r.Post("/tables/{id}/prologue/place-set-asides", handler.PlaceSetAsides(store, manager))
		r.Post("/tables/{id}/prologue/extra-peer", handler.CreateExtraPeer(store, manager))
		r.Get("/tables/{id}/prologue/ranking-state", handler.GetPrologueRankingState(store))
		r.Post("/tables/{id}/prologue/committed-hearts", handler.CommitTrackHearts(store, manager))
		r.Post("/tables/{id}/prologue/done", handler.SetPrologueDone(store, manager))

		// Assets (list + create on the table; per-asset actions by asset ID)
		r.Get("/tables/{id}/assets", handler.ListAssets(store))
		r.Get("/tables/{id}/secrets/visible", handler.ListVisibleSecretsForGame(store))
		r.Post("/tables/{id}/assets", handler.CreateAsset(store, manager))

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
		r.Route("/rolls/{rollId}", func(r chi.Router) {
			r.Get("/", handler.GetRoll(store))
			r.Post("/leverage", handler.LeverageRoll(store, manager))
			r.Post("/call-vote", handler.CallVote(store, manager))
			r.Post("/vote", handler.Vote(store, manager))
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
			deps := &game.PlanDeps{Store: store, Manager: manager}
			for _, h := range game.AllHandlers() {
				for route, fn := range h.ExtraRoutes(deps) {
					r.Post("/"+route, fn)
				}
			}
		})

		// WebSocket (note: no Timeout middleware for WS connections)
		r.Get("/tables/{id}/ws", handler.WebSocket(store, manager))
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
