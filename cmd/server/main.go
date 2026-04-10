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
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/handler"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	dbURL := mustEnv("DATABASE_URL")
	port := env("PORT", "8080")
	devMode := env("DEV_MODE", "false") == "true"
	viteURL := env("VITE_URL", "http://localhost:5173")

	// ── Database ──────────────────────────────────────────────────────────────

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Verify the connection is live.
	if err := pool.Ping(context.Background()); err != nil {
		slog.Error("ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to database")

	// ── Migrations ────────────────────────────────────────────────────────────

	if err := db.RunMigrations(dbURL); err != nil {
		slog.Error("run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	// ── sqlc queries ──────────────────────────────────────────────────────────

	q := dbgen.New(pool)

	// ── Hub manager ───────────────────────────────────────────────────────────

	manager := hub.NewManager()

	// ── Router ────────────────────────────────────────────────────────────────

	r := chi.NewRouter()

	// Standard middleware: structured request logging, panic recovery,
	// and a request timeout so misbehaving clients can't hold goroutines.
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	// API routes — all behind the cookie-auth middleware.
	r.Route("/api", func(r chi.Router) {
		r.Use(appMiddleware.EnsureToken(q))

		// Identity
		r.Post("/identity", handler.SetIdentity(q))
		r.Get("/identity", handler.GetIdentity(q))

		// Tables (creation, join, info)
		r.Post("/tables", handler.CreateTable(q, manager))
		r.Post("/tables/join", handler.JoinTable(q, manager))
		r.Get("/tables/{id}", handler.GetTable(q))
		r.Get("/tables/{id}/state", handler.GetGameState(q))

		// Phase transitions (facilitator actions)
		r.Post("/tables/{id}/start-tone-setting", handler.StartToneSetting(q, manager))
		r.Post("/tables/{id}/start-prologue", handler.StartPrologue(q, manager))
		r.Post("/tables/{id}/start-main-event", handler.StartMainEvent(q, manager))

		// Tone-setting
		r.Get("/tables/{id}/tone", handler.ListToneTopics(q))
		r.Put("/tables/{id}/tone/{topicId}", handler.UpdateToneTopic(q, manager))
		r.Post("/tables/{id}/tone", handler.AddToneTopic(q, manager))

		// Rankings
		r.Get("/tables/{id}/rankings", handler.GetRankings(q))
		r.Put("/tables/{id}/rankings", handler.SetRankings(q, manager))
		r.Put("/tables/{id}/seats", handler.SetSeats(q))

		// Assets (list + create on the table; per-asset actions by asset ID)
		r.Get("/tables/{id}/assets", handler.ListAssets(q))
		r.Post("/tables/{id}/assets", handler.CreateAsset(q, manager))

		r.Route("/assets/{assetId}", func(r chi.Router) {
			r.Put("/", handler.UpdateAsset(q, manager))
			r.Post("/marginalia", handler.AddMarginalia(q, manager))
			r.Put("/marginalia/{pos}", handler.UpdateMarginalia(q, manager))
			r.Delete("/marginalia/{pos}", handler.TearMarginalia(q, manager))
			r.Post("/leverage", handler.LeverageAsset(q, manager))
			r.Post("/refresh", handler.RefreshAsset(q, manager))
			r.Post("/take", handler.TakeAsset(q, manager))
			r.Post("/secrets", handler.WriteSecret(q))
			r.Get("/secrets", handler.GetSecrets(q))
		})

		// Public record + scene threading
		r.Get("/tables/{id}/record", handler.GetFullRecord(q))
		r.Get("/tables/{id}/rows/{row}/posts", handler.ListScenePosts(q))
		r.Post("/tables/{id}/rows/{row}/posts", handler.CreateScenePost(q, manager))
		r.Post("/tables/{id}/rows/{row}/summary", handler.CreateSceneEntry(q, manager))

		// WebSocket (note: no Timeout middleware for WS connections)
		r.Get("/tables/{id}/ws", handler.WebSocket(manager))
	})

	// Frontend — proxy to Vite in dev, serve embedded static files in prod.
	if devMode {
		target, err := url.Parse(viteURL)
		if err != nil {
			slog.Error("parse vite url", "error", err)
			os.Exit(1)
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		r.Handle("/*", proxy)
		slog.Info("dev mode: proxying frontend to Vite", "url", viteURL)
	} else {
		// TODO Phase 1 final: embed the built frontend with //go:embed.
		// For now, serve a minimal placeholder.
		r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "run in DEV_MODE or build the frontend first", http.StatusNotFound)
		}))
	}

	// ── Start server ──────────────────────────────────────────────────────────

	addr := fmt.Sprintf(":%s", port)
	slog.Info("server starting", "addr", addr, "dev_mode", devMode)

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // 0 = no write timeout (needed for WebSocket and SSE)
		IdleTimeout:  60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
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
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}
