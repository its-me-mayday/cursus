package poller

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/its-me-mayday/cursus/internal/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

// --- mocks ---

type mockFeedProvider struct {
	tripUpdates      []domain.TripUpdate
	vehiclePositions []domain.VehiclePosition
	tripErr          error
	vehicleErr       error
	callCount        int32
}

func (m *mockFeedProvider) FetchTripUpdates(ctx context.Context) ([]domain.TripUpdate, error) {
	atomic.AddInt32(&m.callCount, 1)
	return m.tripUpdates, m.tripErr
}

func (m *mockFeedProvider) FetchVehiclePositions(ctx context.Context) ([]domain.VehiclePosition, error) {
	return m.vehiclePositions, m.vehicleErr
}

type mockStore struct {
	tripUpdates      []domain.TripUpdate
	vehiclePositions []domain.VehiclePosition
	setTripCount     int32
	setVehicleCount  int32
}

func (s *mockStore) SetTripUpdates(updates []domain.TripUpdate) {
	atomic.AddInt32(&s.setTripCount, 1)
	s.tripUpdates = updates
}

func (s *mockStore) SetVehiclePositions(positions []domain.VehiclePosition) {
	atomic.AddInt32(&s.setVehicleCount, 1)
	s.vehiclePositions = positions
}

// --- tests ---

func TestPoller_CallsFeedOnStart(t *testing.T) {
	feed := &mockFeedProvider{
		tripUpdates:      []domain.TripUpdate{{TripID: "T1", RouteID: "MA"}},
		vehiclePositions: []domain.VehiclePosition{{VehicleID: "V1"}},
	}
	store := &mockStore{}

	p := New(testLogger(), feed, store, 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.Start(ctx)

	// Wait for initial poll
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&store.setTripCount) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	p.Stop()

	if atomic.LoadInt32(&store.setTripCount) == 0 {
		t.Error("expected SetTripUpdates to be called")
	}
	if atomic.LoadInt32(&store.setVehicleCount) == 0 {
		t.Error("expected SetVehiclePositions to be called")
	}
}

func TestPoller_Stop_NoGoroutineLeak(t *testing.T) {
	feed := &mockFeedProvider{}
	store := &mockStore{}

	p := New(testLogger(), feed, store, 1*time.Hour)
	ctx := context.Background()
	p.Start(ctx)

	// Wait for initial poll
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Stop() did not return within 2s — potential goroutine leak")
	}
}

func TestPoller_TripUpdateError_ContinuesPolling(t *testing.T) {
	feed := &mockFeedProvider{tripErr: errors.New("feed unavailable")}
	store := &mockStore{}

	p := New(testLogger(), feed, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	p.Stop()

	// SetTripUpdates should never be called (error returned before)
	if atomic.LoadInt32(&store.setTripCount) > 0 {
		t.Error("SetTripUpdates should not be called when FetchTripUpdates errors")
	}
	// FetchTripUpdates called multiple times → poller kept running
	if atomic.LoadInt32(&feed.callCount) < 2 {
		t.Errorf("expected ≥2 feed calls, got %d", feed.callCount)
	}
}

func TestPoller_VehiclePositionError_LogsAndContinues(t *testing.T) {
	feed := &mockFeedProvider{
		vehicleErr: errors.New("vehicle feed down"),
	}
	store := &mockStore{}

	p := New(testLogger(), feed, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	p.Stop()

	// trips were set (no error there), vehicles were not
	if atomic.LoadInt32(&store.setTripCount) == 0 {
		t.Error("expected SetTripUpdates to be called when only vehicle fetch fails")
	}
	if atomic.LoadInt32(&store.setVehicleCount) > 0 {
		t.Error("SetVehiclePositions should not be called on vehicle fetch error")
	}
}

func TestPoller_ContextCancellation_Stops(t *testing.T) {
	feed := &mockFeedProvider{}
	store := &mockStore{}

	p := New(testLogger(), feed, store, 1*time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	cancel() // cancel context instead of calling Stop

	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Stop() did not return after context cancellation")
	}
}

func TestPoller_TickerFires(t *testing.T) {
	feed := &mockFeedProvider{
		tripUpdates:      []domain.TripUpdate{},
		vehiclePositions: []domain.VehiclePosition{},
	}
	store := &mockStore{}

	p := New(testLogger(), feed, store, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.Start(ctx)
	time.Sleep(250 * time.Millisecond)
	p.Stop()

	// Initial poll + at least 2 ticker fires
	count := atomic.LoadInt32(&store.setTripCount)
	if count < 3 {
		t.Errorf("expected ≥3 poll cycles, got %d", count)
	}
}
