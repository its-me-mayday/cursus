package metrics

import (
	"sort"
	"time"

	"github.com/its-me-mayday/cursus/internal/domain"
)

// ComputeHeadway calculates headway statistics for a set of arrivals at a station for a given line.
// It expects unsorted arrivals; it will sort them internally.
func ComputeHeadway(lineID string, arrivals []domain.Arrival) domain.Headway {
	hw := domain.Headway{LineID: lineID}
	if len(arrivals) < 2 {
		hw.SampleCount = len(arrivals)
		return hw
	}

	sorted := make([]domain.Arrival, len(arrivals))
	copy(sorted, arrivals)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ArrivalTime.Before(sorted[j].ArrivalTime)
	})

	intervals := make([]float64, 0, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		diff := sorted[i].ArrivalTime.Sub(sorted[i-1].ArrivalTime).Seconds()
		intervals = append(intervals, diff)
	}

	var sum, min, max float64
	min = intervals[0]
	max = intervals[0]
	for _, v := range intervals {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	hw.AvgSeconds = sum / float64(len(intervals))
	hw.MinSeconds = min
	hw.MaxSeconds = max
	hw.SampleCount = len(arrivals)
	return hw
}

// NextArrivalForStop returns the next future arrival at a stop, or nil if none.
// The TimeToArrival field is computed relative to now.
func NextArrivalForStop(stopID string, updates []domain.TripUpdate, now time.Time) *domain.NextArrival {
	var best *domain.NextArrival

	for _, tu := range updates {
		for _, stu := range tu.StopTimes {
			if stu.StopID != stopID {
				continue
			}
			if stu.ArrivalTime.IsZero() || !stu.ArrivalTime.After(now) {
				continue
			}
			candidate := &domain.NextArrival{
				LineID:        tu.RouteID,
				TripID:        tu.TripID,
				ArrivalTime:   stu.ArrivalTime,
				TimeToArrival: stu.ArrivalTime.Sub(now),
				DelaySeconds:  stu.DelaySeconds,
				IsRealtime:    stu.IsRealtime,
			}
			if best == nil || candidate.ArrivalTime.Before(best.ArrivalTime) {
				best = candidate
			}
		}
	}
	return best
}

// ArrivalsForStop returns all future arrivals at a stop, sorted by ArrivalTime.
func ArrivalsForStop(stopID string, updates []domain.TripUpdate, now time.Time) []domain.Arrival {
	var arrivals []domain.Arrival
	for _, tu := range updates {
		for _, stu := range tu.StopTimes {
			if stu.StopID != stopID {
				continue
			}
			if stu.ArrivalTime.IsZero() || !stu.ArrivalTime.After(now) {
				continue
			}
			arrivals = append(arrivals, domain.Arrival{
				LineID:       tu.RouteID,
				TripID:       tu.TripID,
				StopID:       stopID,
				ArrivalTime:  stu.ArrivalTime,
				DelaySeconds: stu.DelaySeconds,
				IsRealtime:   stu.IsRealtime,
			})
		}
	}
	sort.Slice(arrivals, func(i, j int) bool {
		return arrivals[i].ArrivalTime.Before(arrivals[j].ArrivalTime)
	})
	return arrivals
}

// VehiclesForLine returns all vehicles active on a line.
func VehiclesForLine(lineID string, positions []domain.VehiclePosition) []domain.Vehicle {
	var vehicles []domain.Vehicle
	for _, vp := range positions {
		if vp.RouteID != lineID {
			continue
		}
		vehicles = append(vehicles, domain.Vehicle{
			ID:             vp.VehicleID,
			LineID:         lineID,
			Latitude:       vp.Latitude,
			Longitude:      vp.Longitude,
			PositionAvail:  vp.PositionAvail,
			CurrentStopID:  vp.CurrentStopID,
			CurrentStopSeq: vp.CurrentStopSeq,
			Bearing:        vp.Bearing,
			Speed:          vp.Speed,
			Timestamp:      vp.Timestamp,
		})
	}
	return vehicles
}

// HeadwayForLine computes headway across all stops for all trips of a given line.
func HeadwayForLine(lineID string, updates []domain.TripUpdate) domain.Headway {
	// Collect all unique stops for this line
	stopMap := map[string][]domain.Arrival{}
	for _, tu := range updates {
		if tu.RouteID != lineID {
			continue
		}
		for _, stu := range tu.StopTimes {
			stopMap[stu.StopID] = append(stopMap[stu.StopID], domain.Arrival{
				LineID:      tu.RouteID,
				TripID:      tu.TripID,
				StopID:      stu.StopID,
				ArrivalTime: stu.ArrivalTime,
			})
		}
	}

	var allIntervals []float64
	for _, arrivals := range stopMap {
		if len(arrivals) < 2 {
			continue
		}
		sorted := make([]domain.Arrival, len(arrivals))
		copy(sorted, arrivals)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].ArrivalTime.Before(sorted[j].ArrivalTime)
		})
		for i := 1; i < len(sorted); i++ {
			diff := sorted[i].ArrivalTime.Sub(sorted[i-1].ArrivalTime).Seconds()
			allIntervals = append(allIntervals, diff)
		}
	}

	hw := domain.Headway{LineID: lineID, SampleCount: len(allIntervals)}
	if len(allIntervals) == 0 {
		return hw
	}

	var sum, min, max float64
	min = allIntervals[0]
	max = allIntervals[0]
	for _, v := range allIntervals {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	hw.AvgSeconds = sum / float64(len(allIntervals))
	hw.MinSeconds = min
	hw.MaxSeconds = max
	return hw
}
