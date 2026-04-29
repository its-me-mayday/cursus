package acl

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	gtfsrt "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"

	"github.com/its-me-mayday/cursus/internal/domain"
)

// GTFSFetcher fetches GTFS-RT protobuf feeds and translates them to domain types.
// It implements domain.FeedProvider.
type GTFSFetcher struct {
	logger      *slog.Logger
	client      *http.Client
	translator  *GTFSTranslator
	tripURL     string
	vehicleURL  string
	maxRetries  int
}

// NewGTFSFetcher constructs a GTFSFetcher.
func NewGTFSFetcher(
	logger *slog.Logger,
	client *http.Client,
	translator *GTFSTranslator,
	tripURL, vehicleURL string,
	maxRetries int,
) *GTFSFetcher {
	return &GTFSFetcher{
		logger:     logger.With("component", "acl.fetcher"),
		client:     client,
		translator: translator,
		tripURL:    tripURL,
		vehicleURL: vehicleURL,
		maxRetries: maxRetries,
	}
}

// FetchTripUpdates fetches and translates the GTFS-RT TripUpdate feed.
func (f *GTFSFetcher) FetchTripUpdates(ctx context.Context) ([]domain.TripUpdate, error) {
	data, err := f.fetchWithRetry(ctx, f.tripURL)
	if err != nil {
		return nil, err
	}
	msg := &gtfsrt.FeedMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("unmarshal trip updates: %w", err)
	}
	return f.translator.TranslateTripUpdates(msg), nil
}

// FetchVehiclePositions fetches and translates the GTFS-RT VehiclePosition feed.
func (f *GTFSFetcher) FetchVehiclePositions(ctx context.Context) ([]domain.VehiclePosition, error) {
	data, err := f.fetchWithRetry(ctx, f.vehicleURL)
	if err != nil {
		return nil, err
	}
	msg := &gtfsrt.FeedMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("unmarshal vehicle positions: %w", err)
	}
	return f.translator.TranslateVehiclePositions(msg), nil
}

// HealthCheck verifies that both feed endpoints are reachable.
func (f *GTFSFetcher) HealthCheck(ctx context.Context) error {
	for _, u := range []string{f.tripURL, f.vehicleURL} {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		if err != nil {
			f.logger.Error("healthcheck failed", "url", u, "error", err)
			return fmt.Errorf("health check %s: %w", u, err)
		}
		resp, err := f.client.Do(req)
		if err != nil {
			f.logger.Error("healthcheck failed", "url", u, "error", err)
			return fmt.Errorf("health check %s: %w", u, err)
		}
		resp.Body.Close()
		f.logger.Info("healthcheck succeeded", "url", u)
	}
	return nil
}

func (f *GTFSFetcher) fetchWithRetry(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= f.maxRetries; attempt++ {
		f.logger.Info("gtfs feed fetch started", "url", url, "attempt", attempt)
		start := time.Now()

		data, err := f.doFetch(ctx, url)
		if err == nil {
			f.logger.Info("gtfs feed fetch succeeded",
				"url", url,
				"bytes", len(data),
				"duration_ms", time.Since(start).Milliseconds(),
			)
			return data, nil
		}

		lastErr = err
		if attempt < f.maxRetries {
			backoff := backoffDuration(attempt)
			f.logger.Warn("gtfs feed fetch failed, retry",
				"url", url,
				"attempt", attempt,
				"error", err,
				"backoff_ms", backoff.Milliseconds(),
			)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	f.logger.Error("gtfs feed fetch failed", "url", url, "attempts", f.maxRetries, "error", lastErr)
	return nil, lastErr
}

func (f *GTFSFetcher) doFetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func backoffDuration(attempt int) time.Duration {
	ms := math.Pow(2, float64(attempt)) * 100
	return time.Duration(ms) * time.Millisecond
}
