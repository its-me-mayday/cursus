package scheduled

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service struct {
	mu        sync.RWMutex
	staticURL string
	now       func() time.Time

	stops         []stop
	tripsByID     map[string]trip
	stopTimes     []stopTime
	calendar      []calendarRow
	calendarDates []calendarDateRow
	loaded        bool
}

type stop struct {
	ID   string
	Name string
}

type trip struct {
	ID        string
	RouteID   string
	ServiceID string
	Headsign  string
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
	ServiceID     string
	Date          string
	ExceptionType string
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
	zipPath, err := downloadToTempFile(s.staticURL)
	if err != nil {
		return err
	}
	defer os.Remove(zipPath)

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open static gtfs zip: %w", err)
	}
	defer zr.Close()

	var stops []stop
	if err := readZipCSV(&zr.Reader, "stops.txt", func(r map[string]string) error {
		stops = append(stops, stop{
			ID:   r["stop_id"],
			Name: r["stop_name"],
		})
		return nil
	}); err != nil {
		return err
	}

	metroRouteIDs := map[string]bool{}
	for _, routeID := range lineToRouteID {
		metroRouteIDs[routeID] = true
	}
	tripsByID := map[string]trip{}
	metroServiceIDs := map[string]bool{}
	if err := readZipCSV(&zr.Reader, "trips.txt", func(r map[string]string) error {
		routeID := r["route_id"]
		if !metroRouteIDs[routeID] {
			return nil
		}
		t := trip{
			ID:        r["trip_id"],
			RouteID:   routeID,
			ServiceID: r["service_id"],
			Headsign:  r["trip_headsign"],
		}
		tripsByID[t.ID] = t
		metroServiceIDs[t.ServiceID] = true
		return nil
	}); err != nil {
		return err
	}

	var stopTimes []stopTime
	if err := readZipCSV(&zr.Reader, "stop_times.txt", func(r map[string]string) error {
		tripID := r["trip_id"]
		if _, ok := tripsByID[tripID]; !ok {
			return nil
		}
		stopTimes = append(stopTimes, stopTime{
			TripID:      tripID,
			StopID:      r["stop_id"],
			ArrivalTime: r["arrival_time"],
		})
		return nil
	}); err != nil {
		return err
	}

	var calendar []calendarRow
	if err := readOptionalZipCSV(&zr.Reader, "calendar.txt", func(r map[string]string) error {
		if !metroServiceIDs[r["service_id"]] {
			return nil
		}
		calendar = append(calendar, calendarRow{
			ServiceID: r["service_id"],
			Monday:    r["monday"],
			Tuesday:   r["tuesday"],
			Wednesday: r["wednesday"],
			Thursday:  r["thursday"],
			Friday:    r["friday"],
			Saturday:  r["saturday"],
			Sunday:    r["sunday"],
			StartDate: r["start_date"],
			EndDate:   r["end_date"],
		})
		return nil
	}); err != nil {
		return err
	}

	var calendarDates []calendarDateRow
	if err := readOptionalZipCSV(&zr.Reader, "calendar_dates.txt", func(r map[string]string) error {
		if !metroServiceIDs[r["service_id"]] {
			return nil
		}
		calendarDates = append(calendarDates, calendarDateRow{
			ServiceID:     r["service_id"],
			Date:          r["date"],
			ExceptionType: r["exception_type"],
		})
		return nil
	}); err != nil {
		return err
	}

	s.mu.Lock()
	s.stops = stops
	s.tripsByID = tripsByID
	s.stopTimes = stopTimes
	s.calendar = calendar
	s.calendarDates = calendarDates
	s.loaded = true
	s.mu.Unlock()

	return nil
}

func downloadToTempFile(url string) (string, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download static gtfs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download static gtfs: unexpected status %d", resp.StatusCode)
	}

	f, err := os.CreateTemp("", "cursus-static-gtfs-*.zip")
	if err != nil {
		return "", fmt.Errorf("create static gtfs temp file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write static gtfs temp file: %w", err)
	}

	return f.Name(), nil
}

// StopIDsByName returns all stop IDs whose name contains stationName (case-insensitive).
func (s *Service) StopIDsByName(stationName string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(stationName))
	var ids []string
	for _, st := range s.stops {
		if strings.Contains(strings.ToLower(st.Name), q) {
			ids = append(ids, st.ID)
		}
	}
	return ids
}

func (s *Service) ArrivalsByStationName(stationName string, line string, limit int) (ArrivalsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		return ArrivalsResponse{}, fmt.Errorf("scheduled gtfs static is still loading")
	}

	line = strings.ToUpper(strings.TrimSpace(line))
	routeID, ok := lineToRouteID[line]
	if !ok {
		return ArrivalsResponse{}, fmt.Errorf("unsupported line %q", line)
	}

	now := s.now()
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
			Station:              stationLabel,
			StopID:               st.StopID,
			Line:                 line,
			RouteID:              routeID,
			TripID:               st.TripID,
			Direction:            t.Headsign,
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

func readZipCSV(zr *zip.Reader, name string, handle func(map[string]string) error) error {
	found, err := readZipCSVFile(zr, name, handle)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("file not found in gtfs zip: %s", name)
	}
	return nil
}

func readOptionalZipCSV(zr *zip.Reader, name string, handle func(map[string]string) error) error {
	_, err := readZipCSVFile(zr, name, handle)
	return err
}

func readZipCSVFile(zr *zip.Reader, name string, handle func(map[string]string) error) (bool, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return true, err
		}
		defer rc.Close()

		reader := csv.NewReader(rc)
		headers, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				return true, nil
			}
			return true, err
		}

		for {
			row, err := reader.Read()
			if err == io.EOF {
				return true, nil
			}
			if err != nil {
				return true, err
			}
			item := map[string]string{}
			for i, h := range headers {
				if i < len(row) {
					item[h] = row[i]
				}
			}
			if err := handle(item); err != nil {
				return true, err
			}
		}
	}

	return false, nil
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
