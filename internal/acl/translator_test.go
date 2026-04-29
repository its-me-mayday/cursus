package acl

import (
	"log/slog"
	"os"
	"testing"
	"time"

	gtfsrt "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"
)

var testRouteIDs = map[string][]string{
	"MA":  {"MA"},
	"MB":  {"MB"},
	"MB1": {"MB1"},
	"MC":  {"MC"},
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func ptr[T any](v T) *T { return &v }

// --- TripUpdate tests ---

func TestTranslateTripUpdates_Nil(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	result := tr.TranslateTripUpdates(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestTranslateTripUpdates_EmptyMessage(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := &gtfsrt.FeedMessage{
		Header: &gtfsrt.FeedHeader{
			GtfsRealtimeVersion: proto.String("2.0"),
			Timestamp:           proto.Uint64(1000),
		},
	}
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestTranslateTripUpdates_EntityWithoutTripUpdate(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := feedMsg(entity("e1", nil, nil))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestTranslateTripUpdates_EntityWithNoTrip(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	tu := &gtfsrt.TripUpdate{}
	msg := feedMsg(entity("e1", tu, nil))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestTranslateTripUpdates_UnknownRouteID(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := feedMsg(tripUpdateEntity("e1", "UNKNOWN", "trip1", nil))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 0 {
		t.Errorf("expected 0 results for unknown route, got %d", len(result))
	}
}

func TestTranslateTripUpdates_ValidMA(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	arrivalUnix := int64(1700000000)
	stus := []*gtfsrt.TripUpdate_StopTimeUpdate{
		{
			StopId:       ptr("STOP1"),
			StopSequence: ptr(uint32(1)),
			Arrival: &gtfsrt.TripUpdate_StopTimeEvent{
				Time:  ptr(arrivalUnix),
				Delay: ptr(int32(60)),
			},
		},
	}
	msg := feedMsg(tripUpdateEntity("e1", "MA", "trip-123", stus))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	tu := result[0]
	if tu.RouteID != "MA" {
		t.Errorf("route = %q, want MA", tu.RouteID)
	}
	if tu.TripID != "trip-123" {
		t.Errorf("trip_id = %q, want trip-123", tu.TripID)
	}
	if len(tu.StopTimes) != 1 {
		t.Fatalf("stop times = %d, want 1", len(tu.StopTimes))
	}
	stu := tu.StopTimes[0]
	if stu.StopID != "STOP1" {
		t.Errorf("stop_id = %q, want STOP1", stu.StopID)
	}
	if stu.DelaySeconds != 60 {
		t.Errorf("delay = %d, want 60", stu.DelaySeconds)
	}
	if !stu.IsRealtime {
		t.Error("expected is_realtime = true")
	}
	if !stu.ArrivalTime.Equal(time.Unix(arrivalUnix, 0)) {
		t.Errorf("arrival_time = %v, want %v", stu.ArrivalTime, time.Unix(arrivalUnix, 0))
	}
}

func TestTranslateTripUpdates_StopTimeUpdateNilArrival(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	stus := []*gtfsrt.TripUpdate_StopTimeUpdate{
		{StopId: ptr("STOP1"), StopSequence: ptr(uint32(1))},
	}
	msg := feedMsg(tripUpdateEntity("e1", "MB", "trip-x", stus))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].StopTimes[0].IsRealtime {
		t.Error("expected is_realtime = false when arrival is nil")
	}
}

func TestTranslateTripUpdates_NilStopTimeUpdate(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	stus := []*gtfsrt.TripUpdate_StopTimeUpdate{nil}
	msg := feedMsg(tripUpdateEntity("e1", "MC", "trip-y", stus))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1 (trip still valid), got %d", len(result))
	}
	if len(result[0].StopTimes) != 0 {
		t.Errorf("expected 0 stop_times after nil is skipped, got %d", len(result[0].StopTimes))
	}
}

func TestTranslateTripUpdates_MB1AsDistinctRouteID(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := feedMsg(tripUpdateEntity("e1", "MB1", "trip-mb1", nil))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].RouteID != "MB1" {
		t.Errorf("expected MB1, got %q", result[0].RouteID)
	}
}

func TestTranslateTripUpdates_MB1AsAlias(t *testing.T) {
	routeIDs := map[string][]string{
		"MA":  {"MA"},
		"MB":  {"MB"},
		"MB1": {"MB1", "Jonio"},
		"MC":  {"MC"},
	}
	tr := NewGTFSTranslator(testLogger(), routeIDs)
	msg := feedMsg(tripUpdateEntity("e1", "Jonio", "trip-j", nil))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].RouteID != "MB1" {
		t.Errorf("expected MB1 via alias, got %q", result[0].RouteID)
	}
}

func TestTranslateTripUpdates_NilEntityInList(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := &gtfsrt.FeedMessage{
		Header: feedHeader(),
		Entity: []*gtfsrt.FeedEntity{nil},
	}
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestTranslateTripUpdates_NoHeader(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := &gtfsrt.FeedMessage{
		Entity: []*gtfsrt.FeedEntity{tripUpdateEntity("e1", "MA", "t1", nil)},
	}
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

// --- VehiclePosition tests ---

func TestTranslateVehiclePositions_Nil(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	result := tr.TranslateVehiclePositions(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestTranslateVehiclePositions_EmptyMessage(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	result := tr.TranslateVehiclePositions(feedMsg())
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestTranslateVehiclePositions_EntityWithoutVehicle(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := feedMsg(entity("e1", nil, nil))
	result := tr.TranslateVehiclePositions(msg)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestTranslateVehiclePositions_EntityVehicleNoTrip(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	e := &gtfsrt.FeedEntity{
		Id:      ptr("e1"),
		Vehicle: &gtfsrt.VehiclePosition{},
	}
	msg := feedMsg(e)
	result := tr.TranslateVehiclePositions(msg)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestTranslateVehiclePositions_UnknownRouteID(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := feedMsg(vehicleEntity("e1", "UNKNOWN", "v1", "trip1", true))
	result := tr.TranslateVehiclePositions(msg)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestTranslateVehiclePositions_WithGPS(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := feedMsg(vehicleEntity("e1", "MA", "v42", "trip-99", true))
	result := tr.TranslateVehiclePositions(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	vp := result[0]
	if !vp.PositionAvail {
		t.Error("expected position_available = true")
	}
	if vp.Latitude == 0 || vp.Longitude == 0 {
		t.Error("expected non-zero lat/lon")
	}
	if vp.RouteID != "MA" {
		t.Errorf("route = %q, want MA", vp.RouteID)
	}
	if vp.VehicleID != "v42" {
		t.Errorf("vehicle_id = %q, want v42", vp.VehicleID)
	}
}

func TestTranslateVehiclePositions_WithoutGPS(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := feedMsg(vehicleEntity("e1", "MB", "v10", "trip-10", false))
	result := tr.TranslateVehiclePositions(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].PositionAvail {
		t.Error("expected position_available = false")
	}
	if result[0].Latitude != 0 || result[0].Longitude != 0 {
		t.Error("expected zero lat/lon when GPS missing")
	}
}

func TestTranslateVehiclePositions_NoVehicleDescriptor(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	e := &gtfsrt.FeedEntity{
		Id: ptr("e1"),
		Vehicle: &gtfsrt.VehiclePosition{
			Trip: &gtfsrt.TripDescriptor{
				RouteId: ptr("MA"),
				TripId:  ptr("trip1"),
			},
			Position: &gtfsrt.Position{Latitude: ptr(float32(41.9)), Longitude: ptr(float32(12.5))},
		},
	}
	msg := feedMsg(e)
	result := tr.TranslateVehiclePositions(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].VehicleID != "" {
		t.Errorf("expected empty vehicle_id, got %q", result[0].VehicleID)
	}
}

func TestTranslateVehiclePositions_SpeedConversion(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	e := &gtfsrt.FeedEntity{
		Id: ptr("e1"),
		Vehicle: &gtfsrt.VehiclePosition{
			Trip:     &gtfsrt.TripDescriptor{RouteId: ptr("MA"), TripId: ptr("t1")},
			Vehicle:  &gtfsrt.VehicleDescriptor{Id: ptr("v1")},
			Position: &gtfsrt.Position{Latitude: ptr(float32(41.9)), Longitude: ptr(float32(12.5)), Speed: ptr(float32(10.0))},
		},
	}
	msg := feedMsg(e)
	result := tr.TranslateVehiclePositions(msg)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	// 10 m/s * 3.6 = 36 km/h
	if result[0].Speed < 35.9 || result[0].Speed > 36.1 {
		t.Errorf("speed = %v, want ~36", result[0].Speed)
	}
}

func TestTranslateVehiclePositions_TimestampSet(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	ts := uint64(1700000000)
	e := &gtfsrt.FeedEntity{
		Id: ptr("e1"),
		Vehicle: &gtfsrt.VehiclePosition{
			Trip:      &gtfsrt.TripDescriptor{RouteId: ptr("MA"), TripId: ptr("t1")},
			Vehicle:   &gtfsrt.VehicleDescriptor{Id: ptr("v1")},
			Timestamp: &ts,
		},
	}
	msg := feedMsg(e)
	result := tr.TranslateVehiclePositions(msg)
	want := time.Unix(int64(ts), 0)
	if !result[0].Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", result[0].Timestamp, want)
	}
}

func TestTranslateVehiclePositions_CurrentStopSequence(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	e := &gtfsrt.FeedEntity{
		Id: ptr("e1"),
		Vehicle: &gtfsrt.VehiclePosition{
			Trip:                &gtfsrt.TripDescriptor{RouteId: ptr("MA"), TripId: ptr("t1")},
			Vehicle:             &gtfsrt.VehicleDescriptor{Id: ptr("v1")},
			CurrentStopSequence: ptr(uint32(5)),
			StopId:              ptr("STOP5"),
		},
	}
	msg := feedMsg(e)
	result := tr.TranslateVehiclePositions(msg)
	if result[0].CurrentStopSeq != 5 {
		t.Errorf("stop_seq = %d, want 5", result[0].CurrentStopSeq)
	}
	if result[0].CurrentStopID != "STOP5" {
		t.Errorf("stop_id = %q, want STOP5", result[0].CurrentStopID)
	}
}

func TestTranslateVehiclePositions_NilEntityInList(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	msg := &gtfsrt.FeedMessage{
		Header: feedHeader(),
		Entity: []*gtfsrt.FeedEntity{nil},
	}
	result := tr.TranslateVehiclePositions(msg)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestTranslateVehiclePositions_BearingSet(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	e := &gtfsrt.FeedEntity{
		Id: ptr("e1"),
		Vehicle: &gtfsrt.VehiclePosition{
			Trip:     &gtfsrt.TripDescriptor{RouteId: ptr("MA"), TripId: ptr("t1")},
			Vehicle:  &gtfsrt.VehicleDescriptor{Id: ptr("v1")},
			Position: &gtfsrt.Position{Latitude: ptr(float32(41.9)), Longitude: ptr(float32(12.5)), Bearing: ptr(float32(270))},
		},
	}
	result := tr.TranslateVehiclePositions(feedMsg(e))
	if result[0].Bearing != 270 {
		t.Errorf("bearing = %v, want 270", result[0].Bearing)
	}
}

// --- helpers ---

func feedHeader() *gtfsrt.FeedHeader {
	return &gtfsrt.FeedHeader{
		GtfsRealtimeVersion: proto.String("2.0"),
		Timestamp:           proto.Uint64(1000),
	}
}

func feedMsg(entities ...*gtfsrt.FeedEntity) *gtfsrt.FeedMessage {
	return &gtfsrt.FeedMessage{
		Header: feedHeader(),
		Entity: entities,
	}
}

func entity(id string, tu *gtfsrt.TripUpdate, vp *gtfsrt.VehiclePosition) *gtfsrt.FeedEntity {
	e := &gtfsrt.FeedEntity{Id: ptr(id)}
	if tu != nil {
		e.TripUpdate = tu
	}
	if vp != nil {
		e.Vehicle = vp
	}
	return e
}

func tripUpdateEntity(id, routeID, tripID string, stus []*gtfsrt.TripUpdate_StopTimeUpdate) *gtfsrt.FeedEntity {
	return entity(id, &gtfsrt.TripUpdate{
		Trip: &gtfsrt.TripDescriptor{
			TripId:  ptr(tripID),
			RouteId: ptr(routeID),
		},
		StopTimeUpdate: stus,
	}, nil)
}

func vehicleEntity(id, routeID, vehicleID, tripID string, withGPS bool) *gtfsrt.FeedEntity {
	vp := &gtfsrt.VehiclePosition{
		Trip:    &gtfsrt.TripDescriptor{RouteId: ptr(routeID), TripId: ptr(tripID)},
		Vehicle: &gtfsrt.VehicleDescriptor{Id: ptr(vehicleID)},
	}
	if withGPS {
		vp.Position = &gtfsrt.Position{
			Latitude:  ptr(float32(41.9)),
			Longitude: ptr(float32(12.5)),
			Bearing:   ptr(float32(180)),
		}
	}
	return entity(id, nil, vp)
}

func TestEntityID_NilEntity(t *testing.T) {
	result := entityID(nil)
	if result != "<nil>" {
		t.Errorf("expected <nil>, got %q", result)
	}
}

func TestEntityID_NilID(t *testing.T) {
	result := entityID(&gtfsrt.FeedEntity{})
	if result != "<nil>" {
		t.Errorf("expected <nil>, got %q", result)
	}
}

func TestStringVal_Nil(t *testing.T) {
	if stringVal(nil) != "" {
		t.Error("expected empty string for nil")
	}
}

func TestUint32Val_Nil(t *testing.T) {
	if uint32Val(nil) != 0 {
		t.Error("expected 0 for nil")
	}
}

func TestTranslateTripUpdates_ArrivalTimeNilTime(t *testing.T) {
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	stus := []*gtfsrt.TripUpdate_StopTimeUpdate{
		{
			StopId:   ptr("STOP1"),
			Arrival: &gtfsrt.TripUpdate_StopTimeEvent{
				// Time is nil but Delay is set
				Delay: ptr(int32(30)),
			},
		},
	}
	msg := feedMsg(tripUpdateEntity("e1", "MA", "trip-x", stus))
	result := tr.TranslateTripUpdates(msg)
	if len(result) != 1 || len(result[0].StopTimes) != 1 {
		t.Fatalf("unexpected result: %v", result)
	}
	stu := result[0].StopTimes[0]
	if !stu.IsRealtime {
		t.Error("expected is_realtime = true when Arrival is set")
	}
	if stu.ArrivalTime != (time.Time{}) {
		t.Errorf("expected zero time, got %v", stu.ArrivalTime)
	}
	if stu.DelaySeconds != 30 {
		t.Errorf("expected delay 30, got %d", stu.DelaySeconds)
	}
}
