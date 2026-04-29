package poller

import (
	"context"
	"log/slog"
	"time"

	"github.com/lucamaggio/cursus/internal/domain"
)

// Store is the subset of domain.MetricsStore used by the poller.
type Store interface {
	SetTripUpdates(updates []domain.TripUpdate)
	SetVehiclePositions(positions []domain.VehiclePosition)
}

// Poller periodically fetches GTFS-RT data and stores it.
type Poller struct {
	logger   *slog.Logger
	feed     domain.FeedProvider
	store    Store
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

// New creates a Poller.
func New(logger *slog.Logger, feed domain.FeedProvider, store Store, interval time.Duration) *Poller {
	return &Poller{
		logger:   logger.With("component", "poller"),
		feed:     feed,
		store:    store,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start begins polling in the background. Call Stop to terminate.
func (p *Poller) Start(ctx context.Context) {
	p.logger.Info("poller started", "interval", p.interval)
	go p.run(ctx)
}

// Stop signals the poller to stop and waits for it to finish.
func (p *Poller) Stop() {
	close(p.stop)
	<-p.done
	p.logger.Info("poller stopped")
}

func (p *Poller) run(ctx context.Context) {
	defer close(p.done)

	p.poll(ctx, 0)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	var tick int
	for {
		select {
		case <-p.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick++
			p.poll(ctx, tick)
		}
	}
}

func (p *Poller) poll(ctx context.Context, tick int) {
	p.logger.Info("poll cycle started", "tick", tick)
	start := time.Now()

	tripUpdates, err := p.feed.FetchTripUpdates(ctx)
	if err != nil {
		p.logger.Warn("poll cycle failed", "error", err)
		return
	}
	p.store.SetTripUpdates(tripUpdates)

	vehiclePositions, err := p.feed.FetchVehiclePositions(ctx)
	if err != nil {
		p.logger.Warn("poll cycle failed", "error", err)
		return
	}
	p.store.SetVehiclePositions(vehiclePositions)

	p.logger.Info("poll cycle completed",
		"duration_ms", time.Since(start).Milliseconds(),
		"trip_updates", len(tripUpdates),
		"vehicle_positions", len(vehiclePositions),
	)
}
