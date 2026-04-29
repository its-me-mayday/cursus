package store

import (
	"sync"
	"testing"
	"time"

	"github.com/lucamaggio/cursus/internal/domain"
)

func TestSetGetTripUpdates(t *testing.T) {
	m := NewMemory()
	updates := []domain.TripUpdate{
		{TripID: "T1", RouteID: "MA"},
		{TripID: "T2", RouteID: "MB"},
	}
	m.SetTripUpdates(updates)
	got := m.GetTripUpdates()
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
	if got[0].TripID != "T1" {
		t.Errorf("expected T1, got %q", got[0].TripID)
	}
}

func TestSetGetVehiclePositions(t *testing.T) {
	m := NewMemory()
	positions := []domain.VehiclePosition{
		{VehicleID: "V1", RouteID: "MC"},
	}
	m.SetVehiclePositions(positions)
	got := m.GetVehiclePositions()
	if len(got) != 1 {
		t.Errorf("expected 1, got %d", len(got))
	}
	if got[0].VehicleID != "V1" {
		t.Errorf("expected V1, got %q", got[0].VehicleID)
	}
}

func TestGetTripUpdates_Empty(t *testing.T) {
	m := NewMemory()
	got := m.GetTripUpdates()
	if got != nil {
		t.Errorf("expected nil for empty store, got %v", got)
	}
}

func TestGetVehiclePositions_Empty(t *testing.T) {
	m := NewMemory()
	got := m.GetVehiclePositions()
	if got != nil {
		t.Errorf("expected nil for empty store, got %v", got)
	}
}

func TestSetTripUpdates_Overwrite(t *testing.T) {
	m := NewMemory()
	m.SetTripUpdates([]domain.TripUpdate{{TripID: "OLD"}})
	m.SetTripUpdates([]domain.TripUpdate{{TripID: "NEW"}})
	got := m.GetTripUpdates()
	if len(got) != 1 || got[0].TripID != "NEW" {
		t.Errorf("expected [NEW], got %v", got)
	}
}

func TestSetVehiclePositions_Overwrite(t *testing.T) {
	m := NewMemory()
	m.SetVehiclePositions([]domain.VehiclePosition{{VehicleID: "OLD"}})
	m.SetVehiclePositions([]domain.VehiclePosition{{VehicleID: "NEW"}})
	got := m.GetVehiclePositions()
	if len(got) != 1 || got[0].VehicleID != "NEW" {
		t.Errorf("expected [NEW], got %v", got)
	}
}

func TestLastFetchTime_InitiallyZero(t *testing.T) {
	m := NewMemory()
	if !m.LastFetchTime().IsZero() {
		t.Error("expected zero time before any set")
	}
}

func TestLastFetchTime_UpdatedOnSet(t *testing.T) {
	m := NewMemory()
	before := time.Now()
	m.SetTripUpdates(nil)
	after := time.Now()
	ft := m.LastFetchTime()
	if ft.Before(before) || ft.After(after) {
		t.Errorf("last fetch time %v not in [%v, %v]", ft, before, after)
	}
}

func TestLastFetchTime_UpdatedOnVehicleSet(t *testing.T) {
	m := NewMemory()
	before := time.Now()
	m.SetVehiclePositions(nil)
	after := time.Now()
	ft := m.LastFetchTime()
	if ft.Before(before) || ft.After(after) {
		t.Errorf("last fetch time %v not in [%v, %v]", ft, before, after)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	m := NewMemory()
	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.SetTripUpdates([]domain.TripUpdate{{TripID: "T"}})
			_ = m.GetTripUpdates()
			m.SetVehiclePositions([]domain.VehiclePosition{{VehicleID: "V"}})
			_ = m.GetVehiclePositions()
			_ = m.LastFetchTime()
		}(i)
	}
	wg.Wait()
}
