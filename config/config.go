package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port                    string
	LogLevel                string
	PollInterval            time.Duration
	GTFSTripUpdatesURL      string
	GTFSVehiclePositionsURL string
	GTFSFetchTimeout        time.Duration
	GTFSMaxRetries          int
	MetroRouteIDs           map[string][]string
}

func Load(logger *slog.Logger) (*Config, error) {
	cfg := &Config{
		Port:                    getenv("CURSUS_PORT", "8080"),
		LogLevel:                getenv("CURSUS_LOG_LEVEL", "info"),
		GTFSTripUpdatesURL:      getenv("CURSUS_GTFS_TRIP_UPDATES_URL", "https://romamobilita.it/sites/default/files/rome_rtgtfs_trip_updates_feed.pb"),
		GTFSVehiclePositionsURL: getenv("CURSUS_GTFS_VEHICLE_POSITIONS_URL", "https://romamobilita.it/sites/default/files/rome_rtgtfs_vehicle_positions_feed.pb"),
	}

	var err error

	pollStr := getenv("CURSUS_POLL_INTERVAL", "30s")
	cfg.PollInterval, err = time.ParseDuration(pollStr)
	if err != nil {
		logger.Error("configuration invalid", "field", "CURSUS_POLL_INTERVAL", "error", err)
		return nil, fmt.Errorf("invalid CURSUS_POLL_INTERVAL %q: %w", pollStr, err)
	}

	fetchStr := getenv("CURSUS_GTFS_FETCH_TIMEOUT", "10s")
	cfg.GTFSFetchTimeout, err = time.ParseDuration(fetchStr)
	if err != nil {
		logger.Error("configuration invalid", "field", "CURSUS_GTFS_FETCH_TIMEOUT", "error", err)
		return nil, fmt.Errorf("invalid CURSUS_GTFS_FETCH_TIMEOUT %q: %w", fetchStr, err)
	}

	maxRetriesStr := getenv("CURSUS_GTFS_MAX_RETRIES", "3")
	cfg.GTFSMaxRetries, err = parseInt(maxRetriesStr)
	if err != nil {
		logger.Error("configuration invalid", "field", "CURSUS_GTFS_MAX_RETRIES", "error", err)
		return nil, fmt.Errorf("invalid CURSUS_GTFS_MAX_RETRIES %q: %w", maxRetriesStr, err)
	}

	for _, rawURL := range []struct{ field, val string }{
		{"CURSUS_GTFS_TRIP_UPDATES_URL", cfg.GTFSTripUpdatesURL},
		{"CURSUS_GTFS_VEHICLE_POSITIONS_URL", cfg.GTFSVehiclePositionsURL},
	} {
		if err := validateURL(rawURL.val); err != nil {
			logger.Error("configuration invalid", "field", rawURL.field, "error", err)
			return nil, fmt.Errorf("invalid %s %q: %w", rawURL.field, rawURL.val, err)
		}
	}

	cfg.MetroRouteIDs = map[string][]string{
		"MA":  parseRouteIDs(getenv("CURSUS_METRO_ROUTE_IDS_MA", "MA")),
		"MB":  parseRouteIDs(getenv("CURSUS_METRO_ROUTE_IDS_MB", "MB")),
		"MB1": parseRouteIDs(getenv("CURSUS_METRO_ROUTE_IDS_MB1", "MB1")),
		"MC":  parseRouteIDs(getenv("CURSUS_METRO_ROUTE_IDS_MC", "MC")),
	}

	logger.Info("configuration loaded",
		"port", cfg.Port,
		"poll_interval", cfg.PollInterval,
		"log_level", cfg.LogLevel,
	)

	return cfg, nil
}

func validateURL(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

func parseRouteIDs(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, fmt.Errorf("not an integer: %w", err)
	}
	return n, nil
}
