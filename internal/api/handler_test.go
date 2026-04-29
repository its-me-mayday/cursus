package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"
	"os"

	"github.com/its-me-mayday/cursus/internal/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

// --- mock store ---

type mockStore struct {
	tripUpdates      []domain.TripUpdate
	vehiclePositions []domain.VehiclePosition
	lastFetchTime    time.Time
}

func (s *mockStore) GetTripUpdates() []domain.TripUpdate           { return s.tripUpdates }
func (s *mockStore) GetVehiclePositions() []domain.VehiclePosition { return s.vehiclePositions }
func (s *mockStore) LastFetchTime() time.Time                      { return s.lastFetchTime }

func newHandler(store Store) *Handler {
	return New(testLogger(), store)
}

func get(h *Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, body string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("decode JSON: %v\nbody: %s", err, body)
	}
}

var baseTime = time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)

// --- /health ---

func TestHealth_Status200(t *testing.T) {
	store := &mockStore{lastFetchTime: time.Now().Add(-5 * time.Second)}
	h := newHandler(store)
	w := get(h, "/health")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	decodeJSON(t, w.Body.String(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status field = %v, want ok", resp["status"])
	}
	age, ok := resp["feed_age_seconds"].(float64)
	if !ok || age < 0 {
		t.Errorf("feed_age_seconds = %v, want positive float", resp["feed_age_seconds"])
	}
}

func TestHealth_ZeroFetchTime(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/health")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	decodeJSON(t, w.Body.String(), &resp)
	if resp["feed_age_seconds"].(float64) != 0 {
		t.Errorf("expected 0 age when no fetch done")
	}
}

// --- /api/v1/lines ---

func TestLines_AllPresent(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/api/v1/lines")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp []map[string]any
	decodeJSON(t, w.Body.String(), &resp)
	if len(resp) != 4 {
		t.Errorf("expected 4 lines, got %d", len(resp))
	}
	ids := map[string]bool{}
	for _, l := range resp {
		ids[l["id"].(string)] = true
	}
	for _, line := range []string{"MA", "MB", "MB1", "MC"} {
		if !ids[line] {
			t.Errorf("line %q missing from response", line)
		}
	}
}

// --- /api/v1/lines/{line_id} ---

func TestLineDetail_ValidLine(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	for _, line := range []string{"MA", "MB", "MB1", "MC"} {
		w := get(h, "/api/v1/lines/"+line)
		if w.Code != http.StatusOK {
			t.Errorf("[%s] status = %d, want 200", line, w.Code)
		}
		var resp map[string]any
		decodeJSON(t, w.Body.String(), &resp)
		if resp["id"] != line {
			t.Errorf("[%s] id = %v, want %s", line, resp["id"], line)
		}
	}
}

func TestLineDetail_UnknownLine(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/api/v1/lines/XX")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- /api/v1/lines/{line_id}/vehicles ---

func TestLineVehicles_Valid_WithGPS(t *testing.T) {
	store := &mockStore{
		vehiclePositions: []domain.VehiclePosition{
			{VehicleID: "V1", RouteID: "MA", Latitude: 41.9, Longitude: 12.5, PositionAvail: true},
		},
		lastFetchTime: time.Now().Add(-3 * time.Second),
	}
	h := newHandler(store)
	w := get(h, "/api/v1/lines/MA/vehicles")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp vehiclesResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.VehicleCount != 1 {
		t.Errorf("vehicle_count = %d, want 1", resp.VehicleCount)
	}
	if !resp.Vehicles[0].PositionAvail {
		t.Error("expected position_available = true")
	}
	if resp.Vehicles[0].Latitude == nil {
		t.Error("expected non-nil latitude")
	}
}

func TestLineVehicles_Valid_WithoutGPS(t *testing.T) {
	store := &mockStore{
		vehiclePositions: []domain.VehiclePosition{
			{VehicleID: "V2", RouteID: "MB", PositionAvail: false},
		},
	}
	h := newHandler(store)
	w := get(h, "/api/v1/lines/MB/vehicles")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp vehiclesResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.Vehicles[0].PositionAvail {
		t.Error("expected position_available = false")
	}
	if resp.Vehicles[0].Latitude != nil {
		t.Error("expected nil latitude when GPS unavailable")
	}
}

func TestLineVehicles_UnknownLine(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/api/v1/lines/ZZ/vehicles")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestLineVehicles_EmptyList(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/api/v1/lines/MC/vehicles")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp vehiclesResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.VehicleCount != 0 {
		t.Errorf("expected 0 vehicles, got %d", resp.VehicleCount)
	}
	if resp.Vehicles == nil {
		t.Error("vehicles array should not be null")
	}
}

func TestLineVehicles_ZeroFetchTime(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/api/v1/lines/MA/vehicles")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp vehiclesResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.DataAgeSec != 0 {
		t.Errorf("expected 0 age when no fetch, got %v", resp.DataAgeSec)
	}
}

func TestRealtimeRoutes_ReturnsSurfaceRoutes(t *testing.T) {
	store := &mockStore{
		tripUpdates: []domain.TripUpdate{
			{RouteID: "64", ServiceType: "surface", Source: "realtime"},
			{RouteID: "MA", ServiceType: "metro", Source: "realtime"},
		},
		vehiclePositions: []domain.VehiclePosition{
			{VehicleID: "V1", RouteID: "64", ServiceType: "surface", Source: "realtime"},
		},
	}
	h := newHandler(store)
	w := get(h, "/api/v1/realtime/routes")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp []realtimeRouteResponse
	decodeJSON(t, w.Body.String(), &resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 realtime route, got %d", len(resp))
	}
	if resp[0].ID != "64" || resp[0].Source != "realtime" || resp[0].ServiceType != "surface" {
		t.Errorf("unexpected route response: %+v", resp[0])
	}
	if resp[0].ActiveVehicles != 1 {
		t.Errorf("active vehicles = %d, want 1", resp[0].ActiveVehicles)
	}
}

func TestRealtimeRouteVehicles_ReturnsVehicles(t *testing.T) {
	store := &mockStore{
		vehiclePositions: []domain.VehiclePosition{
			{VehicleID: "V1", RouteID: "8", ServiceType: "surface", Source: "realtime", Latitude: 41.9, Longitude: 12.5, PositionAvail: true},
			{VehicleID: "V2", RouteID: "MA", ServiceType: "metro", Source: "realtime", PositionAvail: true},
		},
		lastFetchTime: time.Now().Add(-2 * time.Second),
	}
	h := newHandler(store)
	w := get(h, "/api/v1/realtime/routes/8/vehicles")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp vehiclesResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.RouteID != "8" || resp.Source != "realtime" || resp.ServiceType != "surface" {
		t.Errorf("unexpected vehicles response: %+v", resp)
	}
	if resp.VehicleCount != 1 || resp.Vehicles[0].ID != "V1" {
		t.Errorf("vehicles = %+v, want V1 only", resp.Vehicles)
	}
}

// --- /api/v1/stations/{stop_id} ---

func TestStationArrivals_Sorted(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		tripUpdates: []domain.TripUpdate{
			{RouteID: "MA", TripID: "T2", StopTimes: []domain.StopTimeUpdate{
				{StopID: "S1", ArrivalTime: now.Add(15 * time.Minute)},
			}},
			{RouteID: "MA", TripID: "T1", StopTimes: []domain.StopTimeUpdate{
				{StopID: "S1", ArrivalTime: now.Add(5 * time.Minute)},
			}},
		},
	}
	h := newHandler(store)
	w := get(h, "/api/v1/stations/S1")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp []arrivalJSON
	decodeJSON(t, w.Body.String(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2, got %d", len(resp))
	}
	if resp[0].TripID != "T1" {
		t.Errorf("first should be T1 (earliest), got %q", resp[0].TripID)
	}
}

func TestStationArrivals_EmptyStop(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/api/v1/stations/UNKNOWN")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp []arrivalJSON
	decodeJSON(t, w.Body.String(), &resp)
	if len(resp) != 0 {
		t.Errorf("expected empty array, got %v", resp)
	}
}

// --- /api/v1/stations/{stop_id}/next-arrival ---

func TestStationNextArrival_Success(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		tripUpdates: []domain.TripUpdate{
			{RouteID: "MA", TripID: "T1", StopTimes: []domain.StopTimeUpdate{
				{StopID: "S1", ArrivalTime: now.Add(3*time.Minute + 7*time.Second), IsRealtime: true, DelaySeconds: 60},
			}},
		},
		lastFetchTime: now.Add(-12 * time.Second),
	}
	h := newHandler(store)
	w := get(h, "/api/v1/stations/S1/next-arrival")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
	}
	var resp nextArrivalResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.StopID != "S1" {
		t.Errorf("stop_id = %q, want S1", resp.StopID)
	}
	if resp.LineID != "MA" {
		t.Errorf("line_id = %q, want MA", resp.LineID)
	}
	if resp.TimeToArrivalSec <= 0 {
		t.Errorf("time_to_arrival_seconds = %d, want positive", resp.TimeToArrivalSec)
	}
	if !strings.Contains(resp.TimeToArrivalHuman, "min") {
		t.Errorf("human format = %q, expected minutes", resp.TimeToArrivalHuman)
	}
	if resp.DataAgeSec <= 0 {
		t.Errorf("data_age_seconds = %v, want positive", resp.DataAgeSec)
	}
}

func TestStationNextArrival_NotFound(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/api/v1/stations/UNKNOWN/next-arrival")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStationNextArrival_ZeroFetchTime(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		tripUpdates: []domain.TripUpdate{
			{RouteID: "MA", TripID: "T1", StopTimes: []domain.StopTimeUpdate{
				{StopID: "S1", ArrivalTime: now.Add(5 * time.Minute)},
			}},
		},
		// lastFetchTime is zero
	}
	h := newHandler(store)
	w := get(h, "/api/v1/stations/S1/next-arrival")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp nextArrivalResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.DataAgeSec != 0 {
		t.Errorf("expected 0 data age, got %v", resp.DataAgeSec)
	}
}

// --- /api/v1/metrics ---

func TestGlobalMetrics(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		vehiclePositions: []domain.VehiclePosition{
			{VehicleID: "V1", RouteID: "MA"},
			{VehicleID: "V2", RouteID: "MB"},
		},
		lastFetchTime: now.Add(-8 * time.Second),
	}
	h := newHandler(store)
	w := get(h, "/api/v1/metrics")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp globalMetricsResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.TotalActiveVehicles != 2 {
		t.Errorf("total_active_vehicles = %d, want 2", resp.TotalActiveVehicles)
	}
	if len(resp.HeadwayByLine) != 4 {
		t.Errorf("headway_by_line should have 4 entries, got %d", len(resp.HeadwayByLine))
	}
	if resp.FeedAgeSec <= 0 {
		t.Errorf("feed_age_seconds = %v, want positive", resp.FeedAgeSec)
	}
}

func TestGlobalMetrics_ZeroFetchTime(t *testing.T) {
	store := &mockStore{}
	h := newHandler(store)
	w := get(h, "/api/v1/metrics")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp globalMetricsResponse
	decodeJSON(t, w.Body.String(), &resp)
	if resp.FeedAgeSec != 0 {
		t.Errorf("expected 0 feed age, got %v", resp.FeedAgeSec)
	}
}

// --- helper coverage ---

func TestFormatDuration_Seconds(t *testing.T) {
	result := formatDuration(45 * time.Second)
	if result != "45 sec" {
		t.Errorf("expected '45 sec', got %q", result)
	}
}

func TestFormatDuration_Minutes(t *testing.T) {
	result := formatDuration(3*time.Minute + 7*time.Second)
	if result != "3 min 7 sec" {
		t.Errorf("expected '3 min 7 sec', got %q", result)
	}
}

func TestResponseWriter_DefaultStatus(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	if rw.status != http.StatusOK {
		t.Errorf("default status = %d, want 200", rw.status)
	}
}

func TestResponseWriter_CapturesStatus(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	rw.WriteHeader(http.StatusNotFound)
	if rw.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rw.status)
	}
}
