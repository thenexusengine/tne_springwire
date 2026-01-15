// Package idr provides event recording for the Python IDR service
package idr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// flushWorkerCount is the number of concurrent flush workers
	flushWorkerCount = 2
	// flushQueueSize is the max pending flush batches before blocking
	flushQueueSize = 10
	// flushTimeout is the max time to wait for a flush operation
	flushTimeout = 2 * time.Second
)

// EventRecorder sends auction events to the IDR service
// Uses a bounded worker pool to prevent goroutine leaks
type EventRecorder struct {
	baseURL    string
	httpClient *http.Client
	buffer     []BidEvent
	bufferSize int
	mu         sync.Mutex

	// Worker pool for flush operations
	flushQueue chan []BidEvent
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// Metrics for monitoring (atomic for lock-free access)
	droppedEvents  atomic.Int64 // Count of events dropped due to full queue
	droppedBatches atomic.Int64 // Count of batches dropped
	totalEvents    atomic.Int64 // Total events recorded
	flushedEvents  atomic.Int64 // Total events successfully queued for flush
}

// BidEvent represents a bid event to record
type BidEvent struct {
	AuctionID   string   `json:"auction_id"`
	BidderCode  string   `json:"bidder_code"`
	EventType   string   `json:"event_type"` // "bid_response" or "win"
	LatencyMs   float64  `json:"latency_ms,omitempty"`
	HadBid      bool     `json:"had_bid,omitempty"`
	BidCPM      *float64 `json:"bid_cpm,omitempty"`
	WinCPM      *float64 `json:"win_cpm,omitempty"`
	FloorPrice  *float64 `json:"floor_price,omitempty"`
	Country     string   `json:"country,omitempty"`
	DeviceType  string   `json:"device_type,omitempty"`
	MediaType   string   `json:"media_type,omitempty"`
	AdSize      string   `json:"ad_size,omitempty"`
	PublisherID string   `json:"publisher_id,omitempty"`
	TimedOut    bool     `json:"timed_out,omitempty"`
	HadError    bool     `json:"had_error,omitempty"`
	ErrorMsg    string   `json:"error_message,omitempty"`
}

// NewEventRecorder creates a new event recorder with a bounded worker pool
func NewEventRecorder(baseURL string, bufferSize int) *EventRecorder {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	er := &EventRecorder{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		buffer:     make([]BidEvent, 0, bufferSize),
		bufferSize: bufferSize,
		flushQueue: make(chan []BidEvent, flushQueueSize),
		stopCh:     make(chan struct{}),
	}

	// Start worker pool for flush operations
	for i := 0; i < flushWorkerCount; i++ {
		er.wg.Add(1)
		go er.flushWorker()
	}

	return er
}

// flushWorker processes flush requests from the queue
func (r *EventRecorder) flushWorker() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stopCh:
			return
		case events, ok := <-r.flushQueue:
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
			//nolint:errcheck // Best-effort send; errors silently discarded
			_ = r.sendEvents(ctx, events) // Best-effort send; errors silently discarded
			cancel()
		}
	}
}

// sendEvents sends a batch of events to the IDR service
func (r *EventRecorder) sendEvents(ctx context.Context, events []BidEvent) error {
	if len(events) == 0 {
		return nil
	}

	reqBody := map[string]interface{}{
		"events": events,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	url := r.baseURL + "/api/events"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IDR service returned status %d", resp.StatusCode)
	}

	return nil
}

// RecordBidResponse records a bid response event
func (r *EventRecorder) RecordBidResponse(
	auctionID string,
	bidderCode string,
	latencyMs float64,
	hadBid bool,
	bidCPM *float64,
	floorPrice *float64,
	country string,
	deviceType string,
	mediaType string,
	adSize string,
	publisherID string,
	timedOut bool,
	hadError bool,
	errorMsg string,
) {
	event := BidEvent{
		AuctionID:   auctionID,
		BidderCode:  bidderCode,
		EventType:   "bid_response",
		LatencyMs:   latencyMs,
		HadBid:      hadBid,
		BidCPM:      bidCPM,
		FloorPrice:  floorPrice,
		Country:     country,
		DeviceType:  deviceType,
		MediaType:   mediaType,
		AdSize:      adSize,
		PublisherID: publisherID,
		TimedOut:    timedOut,
		HadError:    hadError,
		ErrorMsg:    errorMsg,
	}

	r.totalEvents.Add(1)

	r.mu.Lock()
	r.buffer = append(r.buffer, event)
	shouldFlush := len(r.buffer) >= r.bufferSize
	var eventsToFlush []BidEvent
	if shouldFlush {
		eventsToFlush = r.buffer
		r.buffer = make([]BidEvent, 0, r.bufferSize)
	}
	r.mu.Unlock()

	// Queue flush if buffer was full (non-blocking send)
	if eventsToFlush != nil {
		batchSize := int64(len(eventsToFlush))
		select {
		case r.flushQueue <- eventsToFlush:
			// Queued successfully
			r.flushedEvents.Add(batchSize)
		default:
			// Queue full - drop events rather than block or leak goroutines
			// Track dropped events for monitoring/alerting
			r.droppedEvents.Add(batchSize)
			r.droppedBatches.Add(1)
		}
	}
}

// RecordWin records a win event
func (r *EventRecorder) RecordWin(
	auctionID string,
	bidderCode string,
	winCPM float64,
	country string,
	deviceType string,
	mediaType string,
	adSize string,
	publisherID string,
) {
	event := BidEvent{
		AuctionID:   auctionID,
		BidderCode:  bidderCode,
		EventType:   "win",
		WinCPM:      &winCPM,
		Country:     country,
		DeviceType:  deviceType,
		MediaType:   mediaType,
		AdSize:      adSize,
		PublisherID: publisherID,
	}

	r.totalEvents.Add(1)

	r.mu.Lock()
	r.buffer = append(r.buffer, event)
	shouldFlush := len(r.buffer) >= r.bufferSize
	var eventsToFlush []BidEvent
	if shouldFlush {
		eventsToFlush = r.buffer
		r.buffer = make([]BidEvent, 0, r.bufferSize)
	}
	r.mu.Unlock()

	// Queue flush if buffer was full (non-blocking send)
	if eventsToFlush != nil {
		batchSize := int64(len(eventsToFlush))
		select {
		case r.flushQueue <- eventsToFlush:
			// Queued successfully
			r.flushedEvents.Add(batchSize)
		default:
			// Queue full - drop events rather than block or leak goroutines
			// Track dropped events for monitoring/alerting
			r.droppedEvents.Add(batchSize)
			r.droppedBatches.Add(1)
		}
	}
}

// Flush sends buffered events to the IDR service synchronously
func (r *EventRecorder) Flush(ctx context.Context) error {
	r.mu.Lock()
	if len(r.buffer) == 0 {
		r.mu.Unlock()
		return nil
	}

	// Swap buffer atomically
	events := r.buffer
	r.buffer = make([]BidEvent, 0, r.bufferSize)
	r.mu.Unlock()

	return r.sendEvents(ctx, events)
}

// Close flushes remaining events and shuts down workers gracefully
func (r *EventRecorder) Close() error {
	// Signal workers to stop
	close(r.stopCh)

	// Flush remaining buffer synchronously
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	err := r.Flush(ctx)

	// Close queue and wait for workers
	close(r.flushQueue)
	r.wg.Wait()

	return err
}

// EventRecorderStats contains metrics for monitoring the event recorder
type EventRecorderStats struct {
	TotalEvents    int64 `json:"total_events"`    // Total events recorded
	FlushedEvents  int64 `json:"flushed_events"`  // Events successfully queued for flush
	DroppedEvents  int64 `json:"dropped_events"`  // Events dropped due to full queue
	DroppedBatches int64 `json:"dropped_batches"` // Batches dropped due to full queue
	BufferedEvents int   `json:"buffered_events"` // Events currently in buffer
	QueuedBatches  int   `json:"queued_batches"`  // Batches waiting in flush queue
}

// Stats returns current metrics for the event recorder.
// Use these metrics for monitoring and alerting on event loss.
func (r *EventRecorder) Stats() EventRecorderStats {
	r.mu.Lock()
	buffered := len(r.buffer)
	r.mu.Unlock()

	return EventRecorderStats{
		TotalEvents:    r.totalEvents.Load(),
		FlushedEvents:  r.flushedEvents.Load(),
		DroppedEvents:  r.droppedEvents.Load(),
		DroppedBatches: r.droppedBatches.Load(),
		BufferedEvents: buffered,
		QueuedBatches:  len(r.flushQueue),
	}
}
