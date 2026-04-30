package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/its-me-mayday/cursus/config"
	"github.com/its-me-mayday/cursus/internal/acl"
	"github.com/its-me-mayday/cursus/internal/api"
	"github.com/its-me-mayday/cursus/internal/poller"
	"github.com/its-me-mayday/cursus/internal/scheduled"
	"github.com/its-me-mayday/cursus/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Bootstrap logger (text for debug, JSON for production)
	logLevel := os.Getenv("CURSUS_LOG_LEVEL")
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: parseLevel(logLevel)}
	if logLevel == "debug" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)

	// Load config
	cfg, err := config.Load(logger)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// OpenTelemetry setup (stdout exporter for local dev)
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return fmt.Errorf("otel exporter: %w", err)
	}
	tp := trace.NewTracerProvider(trace.WithBatcher(exp))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	// Wiring
	mem := store.NewMemory()
	translator := acl.NewGTFSTranslator(logger, cfg.MetroRouteIDs)
	scheduledService := scheduled.NewService(cfg.GTFSStaticURL)
	go func() {
		for {
			logger.Info("scheduled gtfs static load started", "url", cfg.GTFSStaticURL)
			if err := scheduledService.Load(); err != nil {
				logger.Warn("scheduled gtfs static load failed", "error", err)
				time.Sleep(30 * time.Second)
				continue
			}
			logger.Info("scheduled gtfs static loaded")
			time.Sleep(24 * time.Hour)
		}
	}()
	httpClient := &http.Client{Timeout: cfg.GTFSFetchTimeout}
	fetcher := acl.NewGTFSFetcher(
		logger, httpClient, translator,
		cfg.GTFSTripUpdatesURL, cfg.GTFSVehiclePositionsURL,
		cfg.GTFSMaxRetries,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Health check at boot
	if err := fetcher.HealthCheck(ctx); err != nil {
		logger.Warn("boot health check failed — will retry on first poll", "error", err)
	}

	// Start poller
	p := poller.New(logger, fetcher, mem, cfg.PollInterval)
	p.Start(ctx)

	// HTTP server
	apiHandler := api.NewWithScheduled(logger, mem, scheduledService)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      apiHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("signal received, shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	}

	cancel()
	p.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	return srv.Shutdown(shutdownCtx)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
