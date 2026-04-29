package acl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	gtfsrt "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"
)

func validFeedBytes(t *testing.T) []byte {
	t.Helper()
	msg := &gtfsrt.FeedMessage{
		Header: &gtfsrt.FeedHeader{
			GtfsRealtimeVersion: proto.String("2.0"),
			Timestamp:           proto.Uint64(1000),
		},
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func newTestFetcher(tripURL, vehicleURL string, maxRetries int) *GTFSFetcher {
	client := &http.Client{Timeout: 5 * time.Second}
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	return NewGTFSFetcher(testLogger(), client, tr, tripURL, vehicleURL, maxRetries)
}

func TestFetchTripUpdates_Success(t *testing.T) {
	data := validFeedBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer srv.Close()

	f := newTestFetcher(srv.URL, srv.URL, 1)
	updates, err := f.FetchTripUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = updates
}

func TestFetchVehiclePositions_Success(t *testing.T) {
	data := validFeedBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	f := newTestFetcher(srv.URL, srv.URL, 1)
	positions, err := f.FetchVehiclePositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = positions
}

func TestFetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := newTestFetcher(srv.URL, srv.URL, 1)
	_, err := f.FetchTripUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

func TestFetch_MalformedProtobuf(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-valid-protobuf-data!!!"))
	}))
	defer srv.Close()

	f := newTestFetcher(srv.URL, srv.URL, 1)
	_, err := f.FetchTripUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed protobuf")
	}
}

func TestFetch_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 10 * time.Millisecond}
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	f := NewGTFSFetcher(testLogger(), client, tr, srv.URL, srv.URL, 1)

	ctx := context.Background()
	_, err := f.FetchTripUpdates(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestFetch_RetryThenSuccess(t *testing.T) {
	var callCount int32
	data := validFeedBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write(data)
	}))
	defer srv.Close()

	// Use very short backoff: override via small http.Client and maxRetries=3
	client := &http.Client{Timeout: 5 * time.Second}
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	f := NewGTFSFetcher(testLogger(), client, tr, srv.URL, srv.URL, 3)

	// Patch backoff to be instant for test speed – not feasible without injection,
	// so accept the ~400ms test cost from real backoff (2^1*100=200ms, 2^2*100=400ms).
	// We set maxRetries=3, fail on attempt 1 and 2, succeed on 3.
	_, err := f.FetchTripUpdates(context.Background())
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestFetch_AllRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	f := newTestFetcher(srv.URL, srv.URL, 2)
	_, err := f.FetchTripUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error when all retries exhausted")
	}
}

func TestFetch_ContextCancelled(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	f := NewGTFSFetcher(testLogger(), client, tr, srv.URL, srv.URL, 5)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first failure triggers backoff
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := f.FetchTripUpdates(ctx)
	if err == nil {
		t.Fatal("expected error when context cancelled")
	}
}

func TestHealthCheck_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := newTestFetcher(srv.URL, srv.URL, 1)
	if err := f.HealthCheck(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHealthCheck_Failure(t *testing.T) {
	// Server is closed immediately — connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	f := newTestFetcher(url, url, 1)
	if err := f.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error for down server")
	}
}

func TestHealthCheck_InvalidURL(t *testing.T) {
	f := newTestFetcher("http://\x00invalid", "http://example.com", 1)
	if err := f.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestFetchVehiclePositions_MalformedProtobuf(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("garbage"))
	}))
	defer srv.Close()

	f := newTestFetcher(srv.URL, srv.URL, 1)
	_, err := f.FetchVehiclePositions(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed vehicle positions protobuf")
	}
}

func TestFetchVehiclePositions_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := newTestFetcher(srv.URL, srv.URL, 1)
	_, err := f.FetchVehiclePositions(context.Background())
	if err == nil {
		t.Fatal("expected error for vehicle positions HTTP error")
	}
}

func TestFetch_InvalidURLInDoFetch(t *testing.T) {
	// \x00 in URL causes NewRequestWithContext to fail
	client := &http.Client{Timeout: 5 * time.Second}
	tr := NewGTFSTranslator(testLogger(), testRouteIDs)
	f := NewGTFSFetcher(testLogger(), client, tr, "http://\x00bad", "http://example.com", 1)
	_, err := f.FetchTripUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid URL in doFetch")
	}
}

func TestBackoffDuration(t *testing.T) {
	// 2^1 * 100ms = 200ms
	d := backoffDuration(1)
	if d != 200*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 200ms", d)
	}
	// 2^2 * 100ms = 400ms
	d = backoffDuration(2)
	if d != 400*time.Millisecond {
		t.Errorf("backoff(2) = %v, want 400ms", d)
	}
}
