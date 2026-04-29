package scheduled

import (
	"bytes"
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	staticURL string
	now       func() time.Time

	stops         []stop
	tripsByID     map[string]trip
	stopTimes     []stopTime
	calendar      []calendarRow
	calendarDates []calendarDateRow
}

type stop struct {
	ID   string
	Name string
}

type trip struct {
	ID        string
	RouteID   string
	ServiceID string
	Headsign string
}

type stopTime struct {
	TripID      string
	StopID      string
	ArrivalTime string
}

type calendarRow struct {
	ServiceID string
	Monday    string
	Tuesday   string
	Wednesday string
	Thursday  string
	Friday    string
	Saturday  string
	Sunday    string
	StartDate string
	EndDate   string
}

type calendarDateRow struct {
	ServiceID      string
	Date           string
	ExceptionType  string
}

var lineToRouteID = map[string]string{
	"MA":  "MEA",
	"MB":  "MEB",
	"MB1": "MEB1",
	"MC":  "MEC",
}

func NewService(staticURL string) *Service {
	return &Service{
		staticURL: staticURL,
		now:       func() time.Time { return time.Now().In(time.FixedZone("Europe/Rome", 3600)) },
		tripsByID: map[string]trip{},
	}
}

func (s *Service) Load() error {
	resp, err := http.Get(s.staticURL)
	if err != nil {
		return fmt.Errorf("download static gtfs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download static gtfs: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read static gtfs: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("open static gtfs zip: %w", err)
	}

	stopsRows, err := readZipCSV(zr, "stops.txt")
	if err != nil {
		return err
	}

	for _, r := range stopsRows {
		s.stops = append(s.stops, stop{
			ID:   r["stop_id"],
			Name: r["stop_name"],
		})
	}

	tripsRows, err := readZipCSV(zr, "trips.txt")
	if err != nil {
		return err
	}

	for _, r := range tripsRows {
		t := trip{
			ID:        r["trip_id"],
			RouteID:   r["route_id"],
			ServiceID: r["service_id"],
			Headsign: r["trip_headsign"],
		}
		s.tripsByID[t.ID] = t
	}

	stopTimeRows, err := readZipCSV(zr, "stop_times.txt")
	if err != nil {
		return err
	}

	for _, r := range stopTimeRows {
		s.stopTimes = append(s.stopTimes, stopTime{
			TripID:      r["trip_id"],
			StopID:      r["stop_id"],
			ArrivalTime: r["arrival_time"],
		})
	}

	calendarRows, err := readZipCSV(zr, "calendar.txt")
	if err != nil {
		return err
	}

	for _, r := range calendarRows {
		s.calendar = append(s.calendar, calendarRow{
			ServiceID: r["service_id"],
			Monday: r["monday"],
			Tuesday: r["tuesday"],
			Wednesday: r["wednesday"],
			Thursday: r["thursday"],
			Friday: r["friday"],
			Saturday: r["saturday"],
			Sunday: r["sunday"],
			StartDate: r["start_date"],
			EndDate: r["end_date"],
		})
	}

	calendarDateRows, err := readZipCSV(zr, "calendar_dates.txt")
	if err != nil {
		return err
	}

	for _, r := range calendarDateRows {
		s.calendarDates = append(s.calendarDates, calendarDateRow{
			ServiceID:     r["service_id"],
			Date:          r["date"],
			ExceptionType: r["exception_type"],
		})
	}

	return nil
}

func (s *Service) ArrivalsByStationName(stationName string, line string, limit int) (ArrivalsResponse, error) {
	line = strings.ToUpper(strings.TrimSpace(line))
	routeID, ok := lineToRouteID[line]
	if !ok {
		return ArrivalsResponse{}, fmt.Errorf("unsupported line %q", line)
	}

	now := time.Now()
	today := now.Format("20060102")
	nowSeconds := now.Hour()*3600 + now.Minute()*60 + now.Second()

	stopIDs := map[string]string{}

	for _, st := range s.stops {
		if strings.Contains(strings.ToLower(st.Name), strings.ToLower(stationName)) {
			stopIDs[st.ID] = st.Name
		}
	}

	if len(stopIDs) == 0 {
		return ArrivalsResponse{}, fmt.Errorf("station not found: %s", stationName)
	}

	activeServices := s.activeServiceIDs(today, now.Weekday())

	var arrivals []Arrival

	for _, st := range s.stopTimes {
		stationLabel, isWantedStop := stopIDs[st.StopID]
		if !isWantedStop {
			continue
		}

		t, ok := s.tripsByID[st.TripID]
		if !ok {
			continue
		}

		if t.RouteID != routeID {
			continue
		}

		if !activeServices[t.ServiceID] {
			continue
		}

		arrivalSeconds, err := parseGTFSTime(st.ArrivalTime)
		if err != nil {
			continue
		}

		if arrivalSeconds < nowSeconds {
			continue
		}

		diff := arrivalSeconds - nowSeconds

		arrivals = append(arrivals, Arrival{
			Station:             stationLabel,
			StopID:              st.StopID,
			Line:                line,
			RouteID:             routeID,
			TripID:              st.TripID,
			Direction:           t.Headsign,
			ScheduledTime:        formatSecondsAsClock(arrivalSeconds),
			TimeToArrivalSeconds: diff,
			TimeToArrivalHuman:   humanMinutes(diff),
		})
	}

	sort.Slice(arrivals, func(i, j int) bool {
		return arrivals[i].TimeToArrivalSeconds < arrivals[j].TimeToArrivalSeconds
	})

	if limit <= 0 {
		limit = 10
	}

	if len(arrivals) > limit {
		arrivals = arrivals[:limit]
	}

	return ArrivalsResponse{
		Station:  stationName,
		Line:     line,
		Source:   "gtfs_static",
		Realtime: false,
		Arrivals: arrivals,
	}, nil
}

func (s *Service) activeServiceIDs(today string, weekday time.Weekday) map[string]bool {
	active := map[string]bool{}
	weekdayName := strings.ToLower(weekday.String())

	for _, row := range s.calendar {
		if today < row.StartDate || today > row.EndDate {
			continue
		}

		if row.isActiveOn(weekdayName) {
			active[row.ServiceID] = true
		}
	}

	for _, row := range s.calendarDates {
		if row.Date != today {
			continue
		}

		switch row.ExceptionType {
		case "1":
			active[row.ServiceID] = true
		case "2":
			delete(active, row.ServiceID)
		}
	}

	return active
}

func (r calendarRow) isActiveOn(day string) bool {
	switch day {
	case "monday":
		return r.Monday == "1"
	case "tuesday":
		return r.Tuesday == "1"
	case "wednesday":
		return r.Wednesday == "1"
	case "thursday":
		return r.Thursday == "1"
	case "friday":
		return r.Friday == "1"
	case "saturday":
		return r.Saturday == "1"
	case "sunday":
		return r.Sunday == "1"
	default:
		return false
	}
}

func readZipCSV(zr *zip.Reader, name string) ([]map[string]string, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		reader := csv.NewReader(rc)
		rows, err := reader.ReadAll()
		if err != nil {
			return nil, err
		}

		if len(rows) == 0 {
			return nil, nil
		}

		headers := rows[0]
		var out []map[string]string

		for _, row := range rows[1:] {
			item := map[string]string{}
			for i, h := range headers {
				if i < len(row) {
					item[h] = row[i]
				}
			}
			out = append(out, item)
		}

		return out, nil
	}

	return nil, fmt.Errorf("file not found in gtfs zip: %s", name)
}

func parseGTFSTime(value string) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid gtfs time: %s", value)
	}

	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}

	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}

	sec, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, err
	}

	return h*3600 + m*60 + sec, nil
}

func formatSecondsAsClock(seconds int) string {
	seconds = seconds % 86400
	h := seconds / 3600
	m := (seconds % 3600) / 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func humanMinutes(seconds int) string {
	minutes := seconds / 60
	if minutes <= 0 {
		return "now"
	}
	if minutes == 1 {
		return "1 min"
	}
	return fmt.Sprintf("%d min", minutes)
}
