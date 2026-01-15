package idr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNewEventRecorder(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 50)

	if recorder == nil {
		t.Fatal("Expected recorder to be created")
	}

	if recorder.bufferSize != 50 {
		t.Errorf("Expected buffer size 50, got %d", recorder.bufferSize)
	}

	if recorder.baseURL != "http://localhost:8000" {
		t.Errorf("Expected baseURL http://localhost:8000, got %s", recorder.baseURL)
	}

	// Clean up
	recorder.Close()
}

func TestNewEventRecorder_DefaultBufferSize(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 0)

	if recorder.bufferSize != 100 {
		t.Errorf("Expected default buffer size 100, got %d", recorder.bufferSize)
	}

	recorder.Close()
}

func TestRecordBidResponse_SingleEvent(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 100)
	defer recorder.Close()

	bidCPM := 1.50
	floorPrice := 0.50

	recorder.RecordBidResponse(
		"auction-123",
		"appnexus",
		25.5,
		true,
		&bidCPM,
		&floorPrice,
		"US",
		"desktop",
		"banner",
		"300x250",
		"pub-456",
		false,
		false,
		"",
	)

	stats := recorder.Stats()
	if stats.TotalEvents != 1 {
		t.Errorf("Expected 1 total event, got %d", stats.TotalEvents)
	}

	if stats.BufferedEvents != 1 {
		t.Errorf("Expected 1 buffered event, got %d", stats.BufferedEvents)
	}
}

func TestRecordBidResponse_BufferFlush(t *testing.T) {
	// Create mock server
	var receivedEvents []BidEvent
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)

		mu.Lock()
		if events, ok := payload["events"].([]interface{}); ok {
			for _, e := range events {
				eventJSON, _ := json.Marshal(e)
				var bidEvent BidEvent
				json.Unmarshal(eventJSON, &bidEvent)
				receivedEvents = append(receivedEvents, bidEvent)
			}
		}
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Small buffer to trigger flush
	recorder := NewEventRecorder(server.URL, 3)
	defer recorder.Close()

	bidCPM := 1.50

	// Add events to fill buffer
	for i := 0; i < 3; i++ {
		recorder.RecordBidResponse(
			"auction-123",
			"appnexus",
			25.5,
			true,
			&bidCPM,
			nil,
			"US",
			"desktop",
			"banner",
			"300x250",
			"pub-456",
			false,
			false,
			"",
		)
	}

	// Wait for flush to complete
	time.Sleep(100 * time.Millisecond)

	stats := recorder.Stats()
	if stats.TotalEvents != 3 {
		t.Errorf("Expected 3 total events, got %d", stats.TotalEvents)
	}

	if stats.FlushedEvents != 3 {
		t.Errorf("Expected 3 flushed events, got %d", stats.FlushedEvents)
	}

	// Verify events were received by server
	mu.Lock()
	if len(receivedEvents) != 3 {
		t.Errorf("Expected server to receive 3 events, got %d", len(receivedEvents))
	}
	mu.Unlock()
}

func TestRecordWin(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 100)
	defer recorder.Close()

	recorder.RecordWin(
		"auction-123",
		"appnexus",
		1.75,
		"US",
		"mobile",
		"banner",
		"320x50",
		"pub-789",
	)

	stats := recorder.Stats()
	if stats.TotalEvents != 1 {
		t.Errorf("Expected 1 total event, got %d", stats.TotalEvents)
	}

	if stats.BufferedEvents != 1 {
		t.Errorf("Expected 1 buffered event, got %d", stats.BufferedEvents)
	}
}

func TestFlush_EmptyBuffer(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 100)
	defer recorder.Close()

	ctx := context.Background()
	err := recorder.Flush(ctx)

	if err != nil {
		t.Errorf("Expected no error for empty flush, got %v", err)
	}
}

func TestFlush_WithEvents(t *testing.T) {
	// Create mock server
	eventsReceived := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)

		if events, ok := payload["events"].([]interface{}); ok {
			eventsReceived = len(events)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	recorder := NewEventRecorder(server.URL, 100)
	defer recorder.Close()

	bidCPM := 1.50

	// Add some events
	recorder.RecordBidResponse(
		"auction-1",
		"appnexus",
		10.0,
		true,
		&bidCPM,
		nil,
		"US",
		"desktop",
		"banner",
		"300x250",
		"pub-1",
		false,
		false,
		"",
	)

	recorder.RecordBidResponse(
		"auction-2",
		"rubicon",
		15.0,
		true,
		&bidCPM,
		nil,
		"GB",
		"mobile",
		"video",
		"640x480",
		"pub-2",
		false,
		false,
		"",
	)

	// Flush manually
	ctx := context.Background()
	err := recorder.Flush(ctx)

	if err != nil {
		t.Errorf("Expected successful flush, got error: %v", err)
	}

	// Wait for HTTP request
	time.Sleep(50 * time.Millisecond)

	if eventsReceived != 2 {
		t.Errorf("Expected 2 events received, got %d", eventsReceived)
	}

	// Buffer should be empty now
	stats := recorder.Stats()
	if stats.BufferedEvents != 0 {
		t.Errorf("Expected 0 buffered events after flush, got %d", stats.BufferedEvents)
	}
}

func TestFlush_HTTPError(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	recorder := NewEventRecorder(server.URL, 100)
	defer recorder.Close()

	bidCPM := 1.50
	recorder.RecordBidResponse(
		"auction-1",
		"appnexus",
		10.0,
		true,
		&bidCPM,
		nil,
		"US",
		"desktop",
		"banner",
		"300x250",
		"pub-1",
		false,
		false,
		"",
	)

	ctx := context.Background()
	err := recorder.Flush(ctx)

	if err == nil {
		t.Error("Expected error for HTTP 500 response")
	}
}

func TestClose_FlushesRemainingEvents(t *testing.T) {
	eventsReceived := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)

		if events, ok := payload["events"].([]interface{}); ok {
			eventsReceived = len(events)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	recorder := NewEventRecorder(server.URL, 100)

	bidCPM := 1.50
	recorder.RecordBidResponse(
		"auction-1",
		"appnexus",
		10.0,
		true,
		&bidCPM,
		nil,
		"US",
		"desktop",
		"banner",
		"300x250",
		"pub-1",
		false,
		false,
		"",
	)

	// Close should flush remaining events
	err := recorder.Close()
	if err != nil {
		t.Errorf("Expected successful close, got error: %v", err)
	}

	// Wait for HTTP request
	time.Sleep(100 * time.Millisecond)

	if eventsReceived != 1 {
		t.Errorf("Expected 1 event flushed on close, got %d", eventsReceived)
	}
}

func TestStats_Accuracy(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 100)
	defer recorder.Close()

	// Initial stats should be zero
	stats := recorder.Stats()
	if stats.TotalEvents != 0 {
		t.Errorf("Expected 0 total events initially, got %d", stats.TotalEvents)
	}

	bidCPM := 1.50

	// Record some events
	for i := 0; i < 5; i++ {
		recorder.RecordBidResponse(
			"auction-1",
			"appnexus",
			10.0,
			true,
			&bidCPM,
			nil,
			"US",
			"desktop",
			"banner",
			"300x250",
			"pub-1",
			false,
			false,
			"",
		)
	}

	stats = recorder.Stats()
	if stats.TotalEvents != 5 {
		t.Errorf("Expected 5 total events, got %d", stats.TotalEvents)
	}

	if stats.BufferedEvents != 5 {
		t.Errorf("Expected 5 buffered events, got %d", stats.BufferedEvents)
	}
}

func TestRecordBidResponse_TimeoutEvent(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 100)
	defer recorder.Close()

	recorder.RecordBidResponse(
		"auction-123",
		"slow-bidder",
		500.0,
		false,
		nil,
		nil,
		"US",
		"desktop",
		"banner",
		"728x90",
		"pub-456",
		true, // timedOut
		false,
		"",
	)

	stats := recorder.Stats()
	if stats.TotalEvents != 1 {
		t.Errorf("Expected 1 timeout event, got %d", stats.TotalEvents)
	}
}

func TestRecordBidResponse_ErrorEvent(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 100)
	defer recorder.Close()

	recorder.RecordBidResponse(
		"auction-123",
		"error-bidder",
		10.0,
		false,
		nil,
		nil,
		"US",
		"desktop",
		"banner",
		"300x250",
		"pub-456",
		false,
		true, // hadError
		"connection refused",
	)

	stats := recorder.Stats()
	if stats.TotalEvents != 1 {
		t.Errorf("Expected 1 error event, got %d", stats.TotalEvents)
	}
}

func TestConcurrentRecording(t *testing.T) {
	recorder := NewEventRecorder("http://localhost:8000", 1000)
	defer recorder.Close()

	bidCPM := 1.50
	var wg sync.WaitGroup

	// Record events concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorder.RecordBidResponse(
				"auction-1",
				"appnexus",
				10.0,
				true,
				&bidCPM,
				nil,
				"US",
				"desktop",
				"banner",
				"300x250",
				"pub-1",
				false,
				false,
				"",
			)
		}()
	}

	wg.Wait()

	stats := recorder.Stats()
	if stats.TotalEvents != 10 {
		t.Errorf("Expected 10 events from concurrent recording, got %d", stats.TotalEvents)
	}
}
