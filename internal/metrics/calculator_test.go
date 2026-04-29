package metrics

import (
	"testing"
	"time"

	"github.com/lucamaggio/cursus/internal/domain"
)

var baseTime = time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)

func arrAt(offset time.Duration) domain.Arrival {
	return domain.Arrival{
		LineID:      "MA",
		ArrivalTime: baseTime.Add(offset),
	}
}

// --- ComputeHeadway ---

func TestComputeHeadway_NoArrivals(t *testing.T) {
	hw := ComputeHeadway("MA", nil)
	if hw.SampleCount != 0 || hw.AvgSeconds != 0 {
		t.Errorf("unexpected headway for zero arrivals: %+v", hw)
	}
}

func TestComputeHeadway_OneArrival(t *testing.T) {
	hw := ComputeHeadway("MA", []domain.Arrival{arrAt(0)})
	if hw.SampleCount != 1 {
		t.Errorf("sample count = %d, want 1", hw.SampleCount)
	}
	if hw.AvgSeconds != 0 {
		t.Errorf("avg = %v, want 0", hw.AvgSeconds)
	}
}

func TestComputeHeadway_TwoArrivals(t *testing.T) {
	hw := ComputeHeadway("MA", []domain.Arrival{arrAt(0), arrAt(5 * time.Minute)})
	if hw.AvgSeconds != 300 {
		t.Errorf("avg = %v, want 300", hw.AvgSeconds)
	}
	if hw.MinSeconds != 300 || hw.MaxSeconds != 300 {
		t.Errorf("min/max = %v/%v, want 300/300", hw.MinSeconds, hw.MaxSeconds)
	}
	if hw.SampleCount != 2 {
		t.Errorf("sample count = %d, want 2", hw.SampleCount)
	}
}

func TestComputeHeadway_MultipleArrivals(t *testing.T) {
	arrivals := []domain.Arrival{
		// Intentionally unsorted
		arrAt(10 * time.Minute),
		arrAt(0),
		arrAt(4 * time.Minute),
	}
	hw := ComputeHeadway("MA", arrivals)
	// Sorted: 0, 4m, 10m → intervals: 240s, 360s
	// avg = 300, min = 240, max = 360
	if hw.AvgSeconds != 300 {
		t.Errorf("avg = %v, want 300", hw.AvgSeconds)
	}
	if hw.MinSeconds != 240 {
		t.Errorf("min = %v, want 240", hw.MinSeconds)
	}
	if hw.MaxSeconds != 360 {
		t.Errorf("max = %v, want 360", hw.MaxSeconds)
	}
}

// --- NextArrivalForStop ---

func tripWith(routeID, tripID, stopID string, offset time.Duration, isRealtime bool) domain.TripUpdate {
	return domain.TripUpdate{
		RouteID: routeID,
		TripID:  tripID,
		StopTimes: []domain.StopTimeUpdate{
			{StopID: stopID, ArrivalTime: baseTime.Add(offset), IsRealtime: isRealtime},
		},
	}
}

func TestNextArrivalForStop_FutureArrival(t *testing.T) {
	now := baseTime.Add(-1 * time.Minute)
	updates := []domain.TripUpdate{tripWith("MA", "T1", "S1", 5*time.Minute, true)}
	na := NextArrivalForStop("S1", updates, now)
	if na == nil {
		t.Fatal("expected a next arrival")
	}
	if na.LineID != "MA" {
		t.Errorf("line = %q, want MA", na.LineID)
	}
	if na.TimeToArrival <= 0 {
		t.Errorf("time_to_arrival should be positive, got %v", na.TimeToArrival)
	}
}

func TestNextArrivalForStop_PastArrivalSkipped(t *testing.T) {
	now := baseTime.Add(10 * time.Minute) // arrival is in the past
	updates := []domain.TripUpdate{tripWith("MA", "T1", "S1", 5*time.Minute, true)}
	na := NextArrivalForStop("S1", updates, now)
	if na != nil {
		t.Error("expected nil when all arrivals are in the past")
	}
}

func TestNextArrivalForStop_AllPassed(t *testing.T) {
	now := baseTime.Add(1 * time.Hour)
	updates := []domain.TripUpdate{
		tripWith("MA", "T1", "S1", 5*time.Minute, true),
		tripWith("MA", "T2", "S1", 20*time.Minute, true),
	}
	na := NextArrivalForStop("S1", updates, now)
	if na != nil {
		t.Error("expected nil when all trips have passed")
	}
}

func TestNextArrivalForStop_PicksEarliest(t *testing.T) {
	now := baseTime.Add(-1 * time.Minute)
	updates := []domain.TripUpdate{
		tripWith("MA", "T1", "S1", 15*time.Minute, true),
		tripWith("MA", "T2", "S1", 5*time.Minute, true),
	}
	na := NextArrivalForStop("S1", updates, now)
	if na == nil {
		t.Fatal("expected a next arrival")
	}
	if na.TripID != "T2" {
		t.Errorf("expected T2 (earliest), got %q", na.TripID)
	}
}

func TestNextArrivalForStop_WrongStop(t *testing.T) {
	now := baseTime.Add(-1 * time.Minute)
	updates := []domain.TripUpdate{tripWith("MA", "T1", "S1", 5*time.Minute, true)}
	na := NextArrivalForStop("S99", updates, now)
	if na != nil {
		t.Error("expected nil for unknown stop")
	}
}

func TestNextArrivalForStop_ZeroArrivalTimeSkipped(t *testing.T) {
	now := baseTime.Add(-1 * time.Minute)
	updates := []domain.TripUpdate{
		{
			RouteID: "MA", TripID: "T1",
			StopTimes: []domain.StopTimeUpdate{{StopID: "S1", ArrivalTime: time.Time{}}},
		},
	}
	na := NextArrivalForStop("S1", updates, now)
	if na != nil {
		t.Error("expected nil for zero arrival time")
	}
}

// --- ArrivalsForStop ---

func TestArrivalsForStop_ReturnsSorted(t *testing.T) {
	now := baseTime.Add(-1 * time.Minute)
	updates := []domain.TripUpdate{
		tripWith("MA", "T2", "S1", 15*time.Minute, true),
		tripWith("MA", "T1", "S1", 5*time.Minute, false),
	}
	arrivals := ArrivalsForStop("S1", updates, now)
	if len(arrivals) != 2 {
		t.Fatalf("expected 2, got %d", len(arrivals))
	}
	if !arrivals[0].ArrivalTime.Before(arrivals[1].ArrivalTime) {
		t.Error("arrivals not sorted by arrival_time")
	}
	if arrivals[0].TripID != "T1" {
		t.Errorf("first arrival should be T1, got %q", arrivals[0].TripID)
	}
}

func TestArrivalsForStop_EmptyWhenAllPast(t *testing.T) {
	now := baseTime.Add(1 * time.Hour)
	updates := []domain.TripUpdate{tripWith("MA", "T1", "S1", 5*time.Minute, true)}
	arrivals := ArrivalsForStop("S1", updates, now)
	if len(arrivals) != 0 {
		t.Errorf("expected 0, got %d", len(arrivals))
	}
}

func TestArrivalsForStop_WrongStop(t *testing.T) {
	now := baseTime.Add(-1 * time.Minute)
	updates := []domain.TripUpdate{tripWith("MA", "T1", "S1", 5*time.Minute, true)}
	arrivals := ArrivalsForStop("X", updates, now)
	if len(arrivals) != 0 {
		t.Errorf("expected 0, got %d", len(arrivals))
	}
}

func TestArrivalsForStop_ZeroTimeSkipped(t *testing.T) {
	now := baseTime.Add(-1 * time.Minute)
	updates := []domain.TripUpdate{
		{
			RouteID: "MA", TripID: "T1",
			StopTimes: []domain.StopTimeUpdate{{StopID: "S1", ArrivalTime: time.Time{}}},
		},
	}
	arrivals := ArrivalsForStop("S1", updates, now)
	if len(arrivals) != 0 {
		t.Errorf("expected 0, got %d", len(arrivals))
	}
}

// --- VehiclesForLine ---

func TestVehiclesForLine_FiltersCorrectly(t *testing.T) {
	positions := []domain.VehiclePosition{
		{VehicleID: "V1", RouteID: "MA"},
		{VehicleID: "V2", RouteID: "MB"},
		{VehicleID: "V3", RouteID: "MA"},
	}
	vehicles := VehiclesForLine("MA", positions)
	if len(vehicles) != 2 {
		t.Errorf("expected 2, got %d", len(vehicles))
	}
	for _, v := range vehicles {
		if v.LineID != "MA" {
			t.Errorf("unexpected line_id %q", v.LineID)
		}
	}
}

func TestVehiclesForLine_NoMatch(t *testing.T) {
	positions := []domain.VehiclePosition{{VehicleID: "V1", RouteID: "MB"}}
	vehicles := VehiclesForLine("MA", positions)
	if len(vehicles) != 0 {
		t.Errorf("expected 0, got %d", len(vehicles))
	}
}

func TestVehiclesForLine_Empty(t *testing.T) {
	vehicles := VehiclesForLine("MA", nil)
	if len(vehicles) != 0 {
		t.Errorf("expected 0, got %d", len(vehicles))
	}
}

func TestVehiclesForLine_PositionFields(t *testing.T) {
	positions := []domain.VehiclePosition{
		{
			VehicleID: "V1", RouteID: "MA",
			Latitude: 41.9, Longitude: 12.5,
			PositionAvail: true, Bearing: 90, Speed: 36,
		},
	}
	vehicles := VehiclesForLine("MA", positions)
	v := vehicles[0]
	if v.Latitude != 41.9 || v.Longitude != 12.5 {
		t.Errorf("lat/lon = %v/%v, want 41.9/12.5", v.Latitude, v.Longitude)
	}
	if !v.PositionAvail {
		t.Error("position_avail should be true")
	}
}

// --- HeadwayForLine ---

func TestHeadwayForLine_NoUpdates(t *testing.T) {
	hw := HeadwayForLine("MA", nil)
	if hw.LineID != "MA" {
		t.Errorf("line_id = %q, want MA", hw.LineID)
	}
	if hw.SampleCount != 0 {
		t.Errorf("sample_count = %d, want 0", hw.SampleCount)
	}
}

func TestHeadwayForLine_NoMatchingLine(t *testing.T) {
	updates := []domain.TripUpdate{
		{RouteID: "MB", TripID: "T1", StopTimes: []domain.StopTimeUpdate{{StopID: "S1", ArrivalTime: baseTime}}},
	}
	hw := HeadwayForLine("MA", updates)
	if hw.SampleCount != 0 {
		t.Errorf("sample_count = %d, want 0", hw.SampleCount)
	}
}

func TestHeadwayForLine_SingleStopMultipleTrips(t *testing.T) {
	updates := []domain.TripUpdate{
		{RouteID: "MA", TripID: "T1", StopTimes: []domain.StopTimeUpdate{{StopID: "S1", ArrivalTime: baseTime}}},
		{RouteID: "MA", TripID: "T2", StopTimes: []domain.StopTimeUpdate{{StopID: "S1", ArrivalTime: baseTime.Add(5 * time.Minute)}}},
	}
	hw := HeadwayForLine("MA", updates)
	if hw.AvgSeconds != 300 {
		t.Errorf("avg = %v, want 300", hw.AvgSeconds)
	}
}

func TestComputeHeadway_MinMaxDiffer(t *testing.T) {
	// 4 arrivals → 3 intervals: 300s, 60s, 180s → avg=180, min=60, max=300
	// First interval is 300s so initial min=max=300, then 60s triggers min update, 180s is neither
	arrivals := []domain.Arrival{
		{LineID: "MA", ArrivalTime: baseTime},
		{LineID: "MA", ArrivalTime: baseTime.Add(5 * time.Minute)},
		{LineID: "MA", ArrivalTime: baseTime.Add(6 * time.Minute)},
		{LineID: "MA", ArrivalTime: baseTime.Add(9 * time.Minute)},
	}
	hw := ComputeHeadway("MA", arrivals)
	if hw.MinSeconds != 60 {
		t.Errorf("min = %v, want 60", hw.MinSeconds)
	}
	if hw.MaxSeconds != 300 {
		t.Errorf("max = %v, want 300", hw.MaxSeconds)
	}
}

func TestHeadwayForLine_MultipleStops(t *testing.T) {
	updates := []domain.TripUpdate{
		{RouteID: "MA", TripID: "T1", StopTimes: []domain.StopTimeUpdate{
			{StopID: "S1", ArrivalTime: baseTime},
			{StopID: "S2", ArrivalTime: baseTime.Add(2 * time.Minute)},
		}},
		{RouteID: "MA", TripID: "T2", StopTimes: []domain.StopTimeUpdate{
			{StopID: "S1", ArrivalTime: baseTime.Add(5 * time.Minute)},
			{StopID: "S2", ArrivalTime: baseTime.Add(10 * time.Minute)},
		}},
		{RouteID: "MA", TripID: "T3", StopTimes: []domain.StopTimeUpdate{
			{StopID: "S1", ArrivalTime: baseTime.Add(8 * time.Minute)},
			{StopID: "S2", ArrivalTime: baseTime.Add(15 * time.Minute)},
		}},
	}
	hw := HeadwayForLine("MA", updates)
	// S1: intervals 300s, 180s → min=180, max=300
	// S2: intervals 480s, 300s → min=300, max=480
	// Combined 4 intervals, min=180, max=480
	if hw.SampleCount != 4 {
		t.Errorf("sample_count = %d, want 4", hw.SampleCount)
	}
	if hw.MinSeconds != 180 {
		t.Errorf("min = %v, want 180", hw.MinSeconds)
	}
	if hw.MaxSeconds != 480 {
		t.Errorf("max = %v, want 480", hw.MaxSeconds)
	}
}

func TestHeadwayForLine_StopWithOneTrip_NoInterval(t *testing.T) {
	updates := []domain.TripUpdate{
		{RouteID: "MA", TripID: "T1", StopTimes: []domain.StopTimeUpdate{{StopID: "S1", ArrivalTime: baseTime}}},
	}
	hw := HeadwayForLine("MA", updates)
	if hw.SampleCount != 0 {
		t.Errorf("expected 0 intervals for single trip, got %d", hw.SampleCount)
	}
}
