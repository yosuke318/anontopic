// Command server is the entrypoint of the anontopic modular monolith.
//
// It wires every internal module together and exposes a single HTTP server.
// Modules must be wired here (and only here) so that the dependency graph
// between them stays explicit; see CONTRIBUTING.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/yosuke318/anontopic/internal/chat"
)

const (
	defaultAddr     = ":8080"
	shutdownTimeout = 10 * time.Second
	readyTimeout    = 2 * time.Second

	// Defaults point at the ports compose.yaml publishes on the host, so that
	// `make run` works against the datastores started by `docker compose up`.
	defaultDatabaseURL = "postgres://anontopic:anontopic@localhost:5432/anontopic?sslmode=disable"
	defaultRedisURL    = "redis://localhost:6379/0"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		logger.Error("server exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Both clients connect lazily, so the server starts even while PostgreSQL
	// or Redis is still booting. /readyz reports whether they are reachable.
	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", defaultDatabaseURL))
	if err != nil {
		return err
	}
	defer pool.Close()

	redisOpts, err := redis.ParseURL(envOr("REDIS_URL", defaultRedisURL))
	if err != nil {
		return err
	}
	rdb := redis.NewClient(redisOpts)
	defer func() { _ = rdb.Close() }()

	addr := envOr("APP_ADDR", defaultAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /readyz", handleReady(pool, rdb))
	chat.NewHandler().Register(mux)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}

// handleHealth reports that the process itself is running. It must not touch
// PostgreSQL or Redis: container health checks and load balancers use it to
// decide whether the process needs a restart.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady reports whether the datastores the server depends on answer.
func handleReady(pool *pgxpool.Pool, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()

		status := http.StatusOK
		body := map[string]string{"status": "ok", "postgres": "ok", "redis": "ok"}

		if err := pool.Ping(ctx); err != nil {
			status = http.StatusServiceUnavailable
			body["postgres"] = err.Error()
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			status = http.StatusServiceUnavailable
			body["redis"] = err.Error()
		}
		if status != http.StatusOK {
			body["status"] = "unavailable"
		}

		writeJSON(w, status, body)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
