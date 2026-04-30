package scheduled

type Arrival struct {
	Station              string `json:"station"`
	StopID               string `json:"stop_id"`
	Line                 string `json:"line"`
	RouteID              string `json:"route_id"`
	TripID               string `json:"trip_id"`
	Direction            string `json:"direction"`
	ScheduledTime        string `json:"scheduled_time"`
	TimeToArrivalSeconds int    `json:"time_to_arrival_seconds"`
	TimeToArrivalHuman   string `json:"time_to_arrival_human"`
}

type ArrivalsResponse struct {
	Station  string    `json:"station"`
	Line     string    `json:"line"`
	Source   string    `json:"source"`
	Realtime bool      `json:"realtime"`
	Arrivals []Arrival `json:"arrivals"`
}

type SurfaceArrival struct {
	StopID               string `json:"stop_id"`
	RouteID              string `json:"route_id"`
	TripID               string `json:"trip_id"`
	Direction            string `json:"direction"`
	ScheduledTime        string `json:"scheduled_time"`
	TimeToArrivalSeconds int    `json:"time_to_arrival_seconds"`
	TimeToArrivalHuman   string `json:"time_to_arrival_human"`
}

type SurfaceArrivalsResponse struct {
	Station  string           `json:"station"`
	Source   string           `json:"source"`
	Arrivals []SurfaceArrival `json:"arrivals"`
}

type StationsResponse struct {
	Line     string   `json:"line"`
	Stations []string `json:"stations"`
}
