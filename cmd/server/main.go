// Command server is the entrypoint of the anontopic modular monolith.
//
// It wires every internal module together and exposes a single HTTP server.
// Modules must be wired here (and only here) so that the dependency graph
// between them stays explicit; see CONTRIBUTING.md.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/yosuke318/anontopic/internal/chat"
	"github.com/yosuke318/anontopic/internal/session"
)

const (
	defaultAddr     = ":8080"
	shutdownTimeout = 10 * time.Second
	readyTimeout    = 2 * time.Second

	// Defaults point at the ports compose.yaml publishes on the host, so that
	// `make run` works against the datastores started by `docker compose up`.
	defaultDatabaseURL = "postgres://anontopic:anontopic@localhost:5432/anontopic?sslmode=disable"
	defaultRedisURL    = "redis://localhost:6379/0"

	// The web container publishes the frontend here, and the browser talks to
	// the API on another port, which makes every call cross-origin.
	defaultAllowedOrigins = "http://localhost:3000"

	// ipHashKeyBytes sizes the generated fallback key of SESSION_IP_HASH_SECRET.
	ipHashKeyBytes = 32
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

	sessions, err := newSessionService(rdb)
	if err != nil {
		return err
	}

	addr := envOr("APP_ADDR", defaultAddr)
	allowedOrigins := splitList(envOr("APP_ALLOWED_ORIGINS", defaultAllowedOrigins))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /readyz", handleReady(pool, rdb))
	session.NewHandler(sessions).Register(mux)
	chat.NewHandler(sessions, allowedOrigins).Register(mux)

	srv := &http.Server{
		Addr:              addr,
		Handler:           withCORS(allowedOrigins, mux),
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

// newSessionService builds the session module from the environment.
func newSessionService(rdb *redis.Client) (*session.Service, error) {
	sameSite, err := parseSameSite(envOr("SESSION_COOKIE_SAMESITE", "lax"))
	if err != nil {
		return nil, err
	}

	secure := envBool("SESSION_COOKIE_SECURE", true)
	if sameSite == http.SameSiteNoneMode && !secure {
		return nil, errors.New("SESSION_COOKIE_SAMESITE=none requires SESSION_COOKIE_SECURE=true")
	}

	ipHashKey, err := ipHashKey()
	if err != nil {
		return nil, err
	}

	return session.NewService(session.NewRedisStore(rdb), ipHashKey, session.Options{
		TrustForwardedFor: envBool("APP_TRUST_FORWARDED_FOR", false),
		CookieSecure:      secure,
		CookieSameSite:    sameSite,
	}), nil
}

// ipHashKey is the secret client addresses are hashed with. A generated key
// lives as long as the process, so hashes taken before a restart stop matching
// and bans tied to them stop applying: deployments have to set the variable.
func ipHashKey() ([]byte, error) {
	if v := os.Getenv("SESSION_IP_HASH_SECRET"); v != "" {
		return []byte(v), nil
	}

	key := make([]byte, ipHashKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate ip hash key: %w", err)
	}
	slog.Warn("SESSION_IP_HASH_SECRET is unset, hashing client addresses with a key that lasts only as long as this process")

	return key, nil
}

// withCORS lets the configured origins send credentialed requests, which the
// session cookie needs as long as the frontend is served from another origin.
func withCORS(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.ToLower(origin)] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		// The answer differs per origin whether or not this one is allowed, so
		// caches must not hand it to another origin.
		header.Add("Vary", "Origin")

		origin := r.Header.Get("Origin")
		if _, ok := allowed[strings.ToLower(origin)]; ok && origin != "" {
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions {
				header.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				header.Set("Access-Control-Allow-Headers", "Content-Type")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
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

func envBool(key string, fallback bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}

// splitList reads a comma separated environment variable.
func splitList(v string) []string {
	parts := strings.Split(v, ",")
	list := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			list = append(list, trimmed)
		}
	}
	return list
}

func parseSameSite(v string) (http.SameSite, error) {
	switch strings.ToLower(v) {
	case "lax":
		return http.SameSiteLaxMode, nil
	case "strict":
		return http.SameSiteStrictMode, nil
	case "none":
		return http.SameSiteNoneMode, nil
	default:
		return 0, fmt.Errorf("unknown SESSION_COOKIE_SAMESITE %q", v)
	}
}
