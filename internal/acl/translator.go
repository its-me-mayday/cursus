package acl

import (
	"log/slog"
	"sort"
	"time"

	gtfsrt "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"

	"github.com/its-me-mayday/cursus/internal/domain"
)

// GTFSTranslator converts GTFS-RT protobuf types into domain types.
// No protobuf types escape this package boundary.
type GTFSTranslator struct {
	logger       *slog.Logger
	routeLineMap map[string]string // route_id → line ID (MA/MB/MB1/MC)
}

// NewGTFSTranslator builds a translator from a map of line → []route_ids.
func NewGTFSTranslator(logger *slog.Logger, metroRouteIDs map[string][]string) *GTFSTranslator {
	m := make(map[string]string)
	for line, ids := range metroRouteIDs {
		for _, id := range ids {
			m[id] = line
		}
	}
	return &GTFSTranslator{
		logger:       logger.With("component", "acl.translator"),
		routeLineMap: m,
	}
}

// TranslateTripUpdates converts a GTFS-RT FeedMessage to domain TripUpdates.
func (t *GTFSTranslator) TranslateTripUpdates(msg *gtfsrt.FeedMessage) []domain.TripUpdate {
	if msg == nil {
		return nil
	}

	entities := len(msg.Entity)
	var ts int64
	if msg.Header != nil && msg.Header.Timestamp != nil {
		ts = int64(*msg.Header.Timestamp)
	}

	t.logger.Debug("translating feed message", "entities", entities, "timestamp", ts)

	var out []domain.TripUpdate
	unknownRouteCounts := map[string]int{}
	for _, e := range msg.Entity {
		if e == nil || e.TripUpdate == nil {
			t.logger.Warn("skipping entity", "reason", "no trip_update", "entity_id", entityID(e))
			continue
		}
		tu := e.TripUpdate
		if tu.Trip == nil {
			t.logger.Warn("skipping entity", "reason", "no trip descriptor", "entity_id", entityID(e))
			continue
		}

		routeID := stringVal(tu.Trip.RouteId)
		lineID, ok := t.routeLineMap[routeID]
		if !ok {
			unknownRouteCounts[routeID]++
			continue
		}

		var stus []domain.StopTimeUpdate
		for _, stu := range tu.StopTimeUpdate {
			if stu == nil {
				continue
			}
			var arrivalTime time.Time
			var delaySeconds int32
			isRealtime := false
			if stu.Arrival != nil {
				isRealtime = true
				if stu.Arrival.Time != nil {
					arrivalTime = time.Unix(*stu.Arrival.Time, 0)
				}
				if stu.Arrival.Delay != nil {
					delaySeconds = *stu.Arrival.Delay
				}
			}
			stus = append(stus, domain.StopTimeUpdate{
				StopID:       stringVal(stu.StopId),
				StopSequence: uint32Val(stu.StopSequence),
				ArrivalTime:  arrivalTime,
				DelaySeconds: delaySeconds,
				IsRealtime:   isRealtime,
			})
		}

		_ = lineID
		out = append(out, domain.TripUpdate{
			TripID:    stringVal(tu.Trip.TripId),
			RouteID:   lineID,
			StopTimes: stus,
		})
	}

	t.logUnknownRoutes("trip_updates", unknownRouteCounts)
	t.logger.Info("translation completed", "trip_updates", len(out), "vehicle_positions", 0)
	return out
}

// TranslateVehiclePositions converts a GTFS-RT FeedMessage to domain VehiclePositions.
func (t *GTFSTranslator) TranslateVehiclePositions(msg *gtfsrt.FeedMessage) []domain.VehiclePosition {
	if msg == nil {
		return nil
	}

	entities := len(msg.Entity)
	var ts int64
	if msg.Header != nil && msg.Header.Timestamp != nil {
		ts = int64(*msg.Header.Timestamp)
	}

	t.logger.Debug("translating feed message", "entities", entities, "timestamp", ts)

	var out []domain.VehiclePosition
	unknownRouteCounts := map[string]int{}
	for _, e := range msg.Entity {
		if e == nil || e.Vehicle == nil {
			t.logger.Warn("skipping entity", "reason", "no vehicle", "entity_id", entityID(e))
			continue
		}
		v := e.Vehicle
		if v.Trip == nil {
			t.logger.Warn("skipping entity", "reason", "no trip descriptor", "entity_id", entityID(e))
			continue
		}

		routeID := stringVal(v.Trip.RouteId)
		lineID, ok := t.routeLineMap[routeID]
		if !ok {
			unknownRouteCounts[routeID]++
			continue
		}

		vp := domain.VehiclePosition{
			RouteID:       lineID,
			TripID:        stringVal(v.Trip.TripId),
			CurrentStopID: stringVal(v.StopId),
		}

		if v.Vehicle != nil {
			vp.VehicleID = stringVal(v.Vehicle.Id)
		}

		if v.CurrentStopSequence != nil {
			vp.CurrentStopSeq = *v.CurrentStopSequence
		}

		if v.Position != nil {
			vp.PositionAvail = true
			vp.Latitude = float64(*v.Position.Latitude)
			vp.Longitude = float64(*v.Position.Longitude)
			if v.Position.Bearing != nil {
				vp.Bearing = *v.Position.Bearing
			}
			if v.Position.Speed != nil {
				vp.Speed = *v.Position.Speed * 3.6 // m/s → km/h
			}
		}

		if v.Timestamp != nil {
			vp.Timestamp = time.Unix(int64(*v.Timestamp), 0)
		}

		out = append(out, vp)
	}

	t.logUnknownRoutes("vehicle_positions", unknownRouteCounts)
	t.logger.Info("translation completed", "trip_updates", 0, "vehicle_positions", len(out))
	return out
}

func (t *GTFSTranslator) logUnknownRoutes(feed string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}

	ids := make([]string, 0, len(counts))
	total := 0
	for id, count := range counts {
		ids = append(ids, id)
		total += count
	}
	sort.Strings(ids)

	const maxSample = 20
	sample := ids
	if len(sample) > maxSample {
		sample = sample[:maxSample]
	}

	t.logger.Warn("unknown route_ids skipped",
		"feed", feed,
		"unique_route_ids", len(ids),
		"entity_count", total,
		"sample_route_ids", sample,
	)
	t.logger.Debug("unknown route_ids skipped detail", "feed", feed, "route_ids", ids)
}

func entityID(e *gtfsrt.FeedEntity) string {
	if e == nil || e.Id == nil {
		return "<nil>"
	}
	return *e.Id
}

func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func uint32Val(v *uint32) uint32 {
	if v == nil {
		return 0
	}
	return *v
}
