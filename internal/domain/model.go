package domain

import "time"

type Line struct {
	ID string
}

type Station struct {
	ID   string
	Name string
}

type Arrival struct {
	LineID       string
	TripID       string
	StopID       string
	ArrivalTime  time.Time
	DelaySeconds int32
	IsRealtime   bool
}

type Headway struct {
	LineID      string
	AvgSeconds  float64
	MinSeconds  float64
	MaxSeconds  float64
	SampleCount int
}

type Vehicle struct {
	ID             string
	LineID         string
	Latitude       float64
	Longitude      float64
	PositionAvail  bool
	CurrentStopID  string
	CurrentStopSeq uint32
	Bearing        float32
	Speed          float32
	Timestamp      time.Time
}

type NextArrival struct {
	LineID        string
	TripID        string
	ArrivalTime   time.Time
	TimeToArrival time.Duration
	DelaySeconds  int32
	IsRealtime    bool
}

// TripUpdate is the domain representation of a GTFS-RT TripUpdate entity.
type TripUpdate struct {
	TripID      string
	RouteID     string
	ServiceType string
	Source      string
	StopTimes   []StopTimeUpdate
}

type StopTimeUpdate struct {
	StopID       string
	StopSequence uint32
	ArrivalTime  time.Time
	DelaySeconds int32
	IsRealtime   bool
}

// VehiclePosition is the domain representation of a GTFS-RT VehiclePosition entity.
type VehiclePosition struct {
	VehicleID      string
	TripID         string
	RouteID        string
	ServiceType    string
	Source         string
	Latitude       float64
	Longitude      float64
	PositionAvail  bool
	CurrentStopID  string
	CurrentStopSeq uint32
	Bearing        float32
	Speed          float32
	Timestamp      time.Time
}
