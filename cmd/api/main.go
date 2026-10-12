// Command api is the Go API under test: login, feed, posts and likes over
// Postgres (SPEC.md, "The application").
//
// Config comes from -config (default: step-0 defaults); secrets come from the
// environment: DATABASE_URL and STRESSLAB_TOKEN_KEY.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adssib/StressLab/internal/api"
	"github.com/adssib/StressLab/internal/config"
	"github.com/adssib/StressLab/internal/store"
	"github.com/adssib/StressLab/internal/telemetry"
)

func main() {
	log := telemetry.NewLogger(os.Stdout)
	if err := run(log); err != nil {
		log.Error("api stopped", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	configPath := flag.String("config", "", "path to config.yaml (empty: step-0 defaults)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	dbURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	key, err := requireEnv("STRESSLAB_TOKEN_KEY")
	if err != nil {
		return err
	}
	tokens, err := api.NewTokens([]byte(key), cfg.Auth.TokenTTL)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pg, err := store.NewPostgres(ctx, dbURL, cfg.DB.PoolSize)
	if err != nil {
		return err
	}
	defer pg.Close()

	reg := telemetry.NewRegistry()
	reg.MustRegister(telemetry.NewPoolCollector(pg.Pool()))

	srv, err := api.NewServer(pg, tokens, log, reg)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second, // pprof's 30 s CPU profile must fit
		IdleTimeout:       120 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("api listening", slog.String("addr", cfg.Listen), slog.Int("pool_size", cfg.DB.PoolSize))
		errc <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

func requireEnv(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("environment variable %s must be set", name)
	}
	return v, nil
}
