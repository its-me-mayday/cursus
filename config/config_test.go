package config

import (
	"log/slog"
	"os"
	"testing"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	cfg, err := Load(noopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("port = %q, want 8080", cfg.Port)
	}
	if cfg.PollInterval.String() != "30s" {
		t.Errorf("poll interval = %v, want 30s", cfg.PollInterval)
	}
	if cfg.GTFSMaxRetries != 3 {
		t.Errorf("max retries = %d, want 3", cfg.GTFSMaxRetries)
	}
	if len(cfg.MetroRouteIDs["MA"]) == 0 {
		t.Error("MA route IDs should not be empty")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_PORT", "9090")
	t.Setenv("CURSUS_POLL_INTERVAL", "1m")
	t.Setenv("CURSUS_GTFS_FETCH_TIMEOUT", "5s")
	t.Setenv("CURSUS_GTFS_MAX_RETRIES", "5")

	cfg, err := Load(noopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("port = %q, want 9090", cfg.Port)
	}
	if cfg.PollInterval.String() != "1m0s" {
		t.Errorf("poll interval = %v, want 1m0s", cfg.PollInterval)
	}
	if cfg.GTFSFetchTimeout.String() != "5s" {
		t.Errorf("fetch timeout = %v, want 5s", cfg.GTFSFetchTimeout)
	}
	if cfg.GTFSMaxRetries != 5 {
		t.Errorf("max retries = %d, want 5", cfg.GTFSMaxRetries)
	}
}

func TestLoad_InvalidPollInterval(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_POLL_INTERVAL", "not-a-duration")
	_, err := Load(noopLogger())
	if err == nil {
		t.Fatal("expected error for invalid poll interval")
	}
}

func TestLoad_InvalidFetchTimeout(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_GTFS_FETCH_TIMEOUT", "bad")
	_, err := Load(noopLogger())
	if err == nil {
		t.Fatal("expected error for invalid fetch timeout")
	}
}

func TestLoad_InvalidMaxRetries(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_GTFS_MAX_RETRIES", "abc")
	_, err := Load(noopLogger())
	if err == nil {
		t.Fatal("expected error for invalid max retries")
	}
}

func TestLoad_InvalidTripUpdatesURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_GTFS_TRIP_UPDATES_URL", "not-a-url")
	_, err := Load(noopLogger())
	if err == nil {
		t.Fatal("expected error for malformed trip updates URL")
	}
}

func TestLoad_InvalidVehiclePositionsURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_GTFS_VEHICLE_POSITIONS_URL", "not-a-url")
	_, err := Load(noopLogger())
	if err == nil {
		t.Fatal("expected error for malformed vehicle positions URL")
	}
}

func TestLoad_URLSchemeMustBeHTTP(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_GTFS_TRIP_UPDATES_URL", "ftp://example.com/feed.pb")
	_, err := Load(noopLogger())
	if err == nil {
		t.Fatal("expected error for non-http scheme")
	}
}

func TestLoad_URLMissingHost(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_GTFS_TRIP_UPDATES_URL", "http:///path")
	_, err := Load(noopLogger())
	if err == nil {
		t.Fatal("expected error for URL with missing host")
	}
}

func TestLoad_MB1MultipleRouteIDs(t *testing.T) {
	clearEnv(t)
	t.Setenv("CURSUS_METRO_ROUTE_IDS_MB1", "MB1, MB-branch , Jonio")
	cfg, err := Load(noopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids := cfg.MetroRouteIDs["MB1"]
	if len(ids) != 3 {
		t.Errorf("MB1 route IDs = %v, want 3 entries", ids)
	}
	if ids[0] != "MB1" || ids[1] != "MB-branch" || ids[2] != "Jonio" {
		t.Errorf("unexpected MB1 route IDs: %v", ids)
	}
}

func TestLoad_LogLevelDefault(t *testing.T) {
	clearEnv(t)
	cfg, err := Load(noopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("log level = %q, want info", cfg.LogLevel)
	}
}

func TestParseRouteIDs_Empty(t *testing.T) {
	ids := parseRouteIDs("")
	if len(ids) != 0 {
		t.Errorf("expected empty, got %v", ids)
	}
}

func TestValidateURL_Valid(t *testing.T) {
	if err := validateURL("https://example.com/feed"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"CURSUS_PORT", "CURSUS_LOG_LEVEL", "CURSUS_POLL_INTERVAL",
		"CURSUS_GTFS_TRIP_UPDATES_URL", "CURSUS_GTFS_VEHICLE_POSITIONS_URL",
		"CURSUS_GTFS_FETCH_TIMEOUT", "CURSUS_GTFS_MAX_RETRIES",
		"CURSUS_METRO_ROUTE_IDS_MA", "CURSUS_METRO_ROUTE_IDS_MB",
		"CURSUS_METRO_ROUTE_IDS_MB1", "CURSUS_METRO_ROUTE_IDS_MC",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}
