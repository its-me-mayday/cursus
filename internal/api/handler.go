package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/its-me-mayday/cursus/internal/domain"
	"github.com/its-me-mayday/cursus/internal/metrics"
)

var validLines = map[string]bool{"MA": true, "MB": true, "MB1": true, "MC": true}

// Store is the read-only subset of the store used by handlers.
type Store interface {
	GetTripUpdates() []domain.TripUpdate
	GetVehiclePositions() []domain.VehiclePosition
	LastFetchTime() time.Time
}

// Handler bundles all HTTP handlers for the Cursus API.
type Handler struct {
	logger *slog.Logger
	store  Store
	mux    *http.ServeMux
}

// New creates a Handler and registers routes.
func New(logger *slog.Logger, store Store) *Handler {
	h := &Handler{
		logger: logger.With("component", "api"),
		store:  store,
		mux:    http.NewServeMux(),
	}
	h.mux.HandleFunc("GET /health", h.health)
	h.mux.HandleFunc("GET /api/v1/lines", h.lines)
	h.mux.HandleFunc("GET /api/v1/lines/{line_id}", h.lineDetail)
	h.mux.HandleFunc("GET /api/v1/lines/{line_id}/vehicles", h.lineVehicles)
	h.mux.HandleFunc("GET /api/v1/stations/{stop_id}", h.stationArrivals)
	h.mux.HandleFunc("GET /api/v1/stations/{stop_id}/next-arrival", h.stationNextArrival)
	h.mux.HandleFunc("GET /api/v1/metrics", h.globalMetrics)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	h.logger.Info("request received", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
	h.mux.ServeHTTP(rw, r)
	h.logger.Info("request completed",
		"method", r.Method, "path", r.URL.Path,
		"status", rw.status, "duration_ms", time.Since(start).Milliseconds(),
	)
}

// --- handlers ---

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	ft := h.store.LastFetchTime()
	var feedAge float64
	if !ft.IsZero() {
		feedAge = time.Since(ft).Seconds()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"last_fetch":       ft,
		"feed_age_seconds": feedAge,
	})
}

type lineResponse struct {
	ID             string  `json:"id"`
	HeadwayAvgSec  float64 `json:"headway_avg_seconds"`
	ActiveVehicles int     `json:"active_vehicles"`
}

func (h *Handler) lines(w http.ResponseWriter, r *http.Request) {
	updates := h.store.GetTripUpdates()
	positions := h.store.GetVehiclePositions()

	var resp []lineResponse
	for _, line := range []string{"MA", "MB", "MB1", "MC"} {
		hw := metrics.HeadwayForLine(line, updates)
		veh := metrics.VehiclesForLine(line, positions)
		resp = append(resp, lineResponse{
			ID:             line,
			HeadwayAvgSec:  hw.AvgSeconds,
			ActiveVehicles: len(veh),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

type lineDetailResponse struct {
	ID             string  `json:"id"`
	HeadwayAvgSec  float64 `json:"headway_avg_seconds"`
	HeadwayMinSec  float64 `json:"headway_min_seconds"`
	HeadwayMaxSec  float64 `json:"headway_max_seconds"`
	ActiveVehicles int     `json:"active_vehicles"`
	LastUpdated    time.Time `json:"last_updated"`
}

func (h *Handler) lineDetail(w http.ResponseWriter, r *http.Request) {
	lineID := r.PathValue("line_id")
	if !validLines[lineID] {
		h.logger.Warn("resource not found", "path", r.URL.Path, "param", lineID)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown line_id"})
		return
	}
	updates := h.store.GetTripUpdates()
	positions := h.store.GetVehiclePositions()
	hw := metrics.HeadwayForLine(lineID, updates)
	veh := metrics.VehiclesForLine(lineID, positions)
	writeJSON(w, http.StatusOK, lineDetailResponse{
		ID:             lineID,
		HeadwayAvgSec:  hw.AvgSeconds,
		HeadwayMinSec:  hw.MinSeconds,
		HeadwayMaxSec:  hw.MaxSeconds,
		ActiveVehicles: len(veh),
		LastUpdated:    h.store.LastFetchTime(),
	})
}

type vehicleJSON struct {
	ID              string    `json:"id"`
	Latitude        *float64  `json:"latitude"`
	Longitude       *float64  `json:"longitude"`
	CurrentStopID   string    `json:"current_stop_id"`
	Bearing         float32   `json:"bearing"`
	SpeedKmh        float32   `json:"speed_kmh"`
	LastUpdated     time.Time `json:"last_updated"`
	PositionAvail   bool      `json:"position_available"`
}

type vehiclesResponse struct {
	LineID       string        `json:"line_id"`
	VehicleCount int           `json:"vehicle_count"`
	Vehicles     []vehicleJSON `json:"vehicles"`
	DataAgeSec   float64       `json:"data_age_seconds"`
}

func (h *Handler) lineVehicles(w http.ResponseWriter, r *http.Request) {
	lineID := r.PathValue("line_id")
	if !validLines[lineID] {
		h.logger.Warn("resource not found", "path", r.URL.Path, "param", lineID)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown line_id"})
		return
	}
	positions := h.store.GetVehiclePositions()
	vehicles := metrics.VehiclesForLine(lineID, positions)

	var vjs []vehicleJSON
	for _, v := range vehicles {
		vj := vehicleJSON{
			ID:            v.ID,
			CurrentStopID: v.CurrentStopID,
			Bearing:       v.Bearing,
			SpeedKmh:      v.Speed,
			LastUpdated:   v.Timestamp,
			PositionAvail: v.PositionAvail,
		}
		if v.PositionAvail {
			lat := v.Latitude
			lon := v.Longitude
			vj.Latitude = &lat
			vj.Longitude = &lon
		}
		vjs = append(vjs, vj)
	}
	if vjs == nil {
		vjs = []vehicleJSON{}
	}

	ft := h.store.LastFetchTime()
	var age float64
	if !ft.IsZero() {
		age = time.Since(ft).Seconds()
	}
	writeJSON(w, http.StatusOK, vehiclesResponse{
		LineID:       lineID,
		VehicleCount: len(vjs),
		Vehicles:     vjs,
		DataAgeSec:   age,
	})
}

type arrivalJSON struct {
	LineID       string    `json:"line_id"`
	TripID       string    `json:"trip_id"`
	ArrivalTime  time.Time `json:"arrival_time"`
	DelaySeconds int32     `json:"delay_seconds"`
	IsRealtime   bool      `json:"is_realtime"`
}

func (h *Handler) stationArrivals(w http.ResponseWriter, r *http.Request) {
	stopID := r.PathValue("stop_id")
	updates := h.store.GetTripUpdates()
	now := time.Now()
	arrivals := metrics.ArrivalsForStop(stopID, updates, now)
	var resp []arrivalJSON
	for _, a := range arrivals {
		resp = append(resp, arrivalJSON{
			LineID:       a.LineID,
			TripID:       a.TripID,
			ArrivalTime:  a.ArrivalTime,
			DelaySeconds: a.DelaySeconds,
			IsRealtime:   a.IsRealtime,
		})
	}
	if resp == nil {
		resp = []arrivalJSON{}
	}
	writeJSON(w, http.StatusOK, resp)
}

type nextArrivalResponse struct {
	StopID              string    `json:"stop_id"`
	LineID              string    `json:"line_id"`
	TripID              string    `json:"trip_id"`
	ArrivalTime         time.Time `json:"arrival_time"`
	TimeToArrivalSec    int64     `json:"time_to_arrival_seconds"`
	TimeToArrivalHuman  string    `json:"time_to_arrival_human"`
	DelaySeconds        int32     `json:"delay_seconds"`
	IsRealtime          bool      `json:"is_realtime"`
	DataAgeSec          float64   `json:"data_age_seconds"`
}

func (h *Handler) stationNextArrival(w http.ResponseWriter, r *http.Request) {
	stopID := r.PathValue("stop_id")
	updates := h.store.GetTripUpdates()
	now := time.Now()
	na := metrics.NextArrivalForStop(stopID, updates, now)
	if na == nil {
		h.logger.Warn("resource not found", "path", r.URL.Path, "param", stopID)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no upcoming arrival for stop"})
		return
	}

	tta := na.ArrivalTime.Sub(now)
	ft := h.store.LastFetchTime()
	var age float64
	if !ft.IsZero() {
		age = time.Since(ft).Seconds()
	}

	writeJSON(w, http.StatusOK, nextArrivalResponse{
		StopID:             stopID,
		LineID:             na.LineID,
		TripID:             na.TripID,
		ArrivalTime:        na.ArrivalTime,
		TimeToArrivalSec:   int64(tta.Seconds()),
		TimeToArrivalHuman: formatDuration(tta),
		DelaySeconds:       na.DelaySeconds,
		IsRealtime:         na.IsRealtime,
		DataAgeSec:         age,
	})
}

type globalMetricsResponse struct {
	TotalActiveVehicles int                    `json:"total_active_vehicles"`
	HeadwayByLine       map[string]float64     `json:"headway_avg_by_line"`
	LastPollTime        time.Time              `json:"last_poll_time"`
	FeedAgeSec          float64                `json:"feed_age_seconds"`
}

func (h *Handler) globalMetrics(w http.ResponseWriter, r *http.Request) {
	updates := h.store.GetTripUpdates()
	positions := h.store.GetVehiclePositions()
	ft := h.store.LastFetchTime()

	var age float64
	if !ft.IsZero() {
		age = time.Since(ft).Seconds()
	}

	total := 0
	headways := map[string]float64{}
	for _, line := range []string{"MA", "MB", "MB1", "MC"} {
		veh := metrics.VehiclesForLine(line, positions)
		total += len(veh)
		hw := metrics.HeadwayForLine(line, updates)
		headways[line] = hw.AvgSeconds
	}

	writeJSON(w, http.StatusOK, globalMetricsResponse{
		TotalActiveVehicles: total,
		HeadwayByLine:       headways,
		LastPollTime:        ft,
		FeedAgeSec:          age,
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%d min %d sec", m, s)
	}
	return fmt.Sprintf("%d sec", s)
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

