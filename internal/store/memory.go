package store

import (
	"sync"
	"time"

	"github.com/its-me-mayday/cursus/internal/domain"
)

// Memory is a thread-safe in-memory store for GTFS domain data.
type Memory struct {
	mu               sync.RWMutex
	tripUpdates      []domain.TripUpdate
	vehiclePositions []domain.VehiclePosition
	lastFetchTime    time.Time
}

// NewMemory creates an empty Memory store.
func NewMemory() *Memory {
	return &Memory{}
}

func (m *Memory) SetTripUpdates(updates []domain.TripUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tripUpdates = updates
	m.lastFetchTime = time.Now()
}

func (m *Memory) GetTripUpdates() []domain.TripUpdate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tripUpdates
}

func (m *Memory) SetVehiclePositions(positions []domain.VehiclePosition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vehiclePositions = positions
	m.lastFetchTime = time.Now()
}

func (m *Memory) GetVehiclePositions() []domain.VehiclePosition {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.vehiclePositions
}

func (m *Memory) LastFetchTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastFetchTime
}
