package domain

import (
	"context"
	"time"
)

// FeedProvider is the port that the ACL must implement to supply GTFS-RT data
// to the domain. The domain never knows about protobuf or HTTP.
type FeedProvider interface {
	FetchTripUpdates(ctx context.Context) ([]TripUpdate, error)
	FetchVehiclePositions(ctx context.Context) ([]VehiclePosition, error)
}

// MetricsStore is the port for reading and writing computed domain data.
type MetricsStore interface {
	SetTripUpdates(updates []TripUpdate)
	GetTripUpdates() []TripUpdate
	SetVehiclePositions(positions []VehiclePosition)
	GetVehiclePositions() []VehiclePosition
	LastFetchTime() time.Time
}
