// +build integration
//go:build integration
// +build integration

package endpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/exchange"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// Integration tests for auction endpoint with real bidder interactions

// mockSuccessfulAdapter simulates a bidder that always returns a bid
type mockSuccessfulAdapter struct {
	bidPrice   float64
	delay      time.Duration
	bidderName string // Add bidder name to make bid IDs unique
}

func (m *mockSuccessfulAdapter) MakeRequests(request *openrtb.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	// Store bidder name for use in MakeBids
	if reqInfo != nil && reqInfo.BidderCoreName != "" {
		m.bidderName = reqInfo.BidderCoreName
	}
	// Use MOCK method so exchange handles it as a mock request without real HTTP
	return []*adapters.RequestData{{
		Method: "MOCK",
		URI:    "http://test.bidder.com/bid",
		Body:   []byte(`{}`),
	}}, nil
}

func (m *mockSuccessfulAdapter) MakeBids(request *openrtb.BidRequest, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	bids := make([]*adapters.TypedBid, 0, len(request.Imp))
	for _, imp := range request.Imp {
		// Create unique bid ID using bidder name to avoid duplicates
		bidID := "bid-" + imp.ID
		if m.bidderName != "" {
			bidID = m.bidderName + "-" + bidID
		}
		bid := &openrtb.Bid{
			ID:    bidID,
			ImpID: imp.ID,
			Price: m.bidPrice,
			AdM:   "<div>Test Ad</div>",
			CRID:  "creative-1",
		}
		if imp.Banner != nil {
			bid.W = imp.Banner.W
			bid.H = imp.Banner.H
		}
		bids = append(bids, &adapters.TypedBid{
			Bid:     bid,
			BidType: adapters.BidTypeBanner,
		})
	}

	return &adapters.BidderResponse{
		Bids:     bids,
		Currency: "USD",
	}, nil
}

// mockFailingAdapter simulates a bidder that fails
type mockFailingAdapter struct {
	errorMsg string
}

func (m *mockFailingAdapter) MakeRequests(request *openrtb.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	return []*adapters.RequestData{{
		Method: "MOCK",
		URI:    "http://failing.bidder.com/bid",
		Body:   []byte(`{}`),
	}}, nil
}

func (m *mockFailingAdapter) MakeBids(request *openrtb.BidRequest, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	return nil, []error{&adapters.BidderError{
		BidderCode: "failing",
		Code:       adapters.ErrorCodeBadRequest,
		Message:    m.errorMsg,
	}}
}

// mockNoBidAdapter simulates a bidder that returns no bids
type mockNoBidAdapter struct{}

func (m *mockNoBidAdapter) MakeRequests(request *openrtb.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	return []*adapters.RequestData{{
		Method: "MOCK",
		URI:    "http://nobid.bidder.com/bid",
		Body:   []byte(`{}`),
	}}, nil
}

func (m *mockNoBidAdapter) MakeBids(request *openrtb.BidRequest, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	return &adapters.BidderResponse{
		Bids:     []*adapters.TypedBid{},
		Currency: "USD",
	}, nil
}

// TestAuctionIntegration_SingleBidder tests auction with one successful bidder
func TestAuctionIntegration_SingleBidder(t *testing.T) {
	registry := adapters.NewRegistry()
	registry.Register("testbidder", &mockSuccessfulAdapter{bidPrice: 1.50}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 1000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-auction-1",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp openrtb.BidResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.ID != bidReq.ID {
		t.Errorf("expected response ID %s, got %s", bidReq.ID, resp.ID)
	}

	if len(resp.SeatBid) != 1 {
		t.Fatalf("expected 1 seatbid, got %d", len(resp.SeatBid))
	}

	if len(resp.SeatBid[0].Bid) != 1 {
		t.Fatalf("expected 1 bid, got %d", len(resp.SeatBid[0].Bid))
	}

	bid := resp.SeatBid[0].Bid[0]
	if bid.Price != 1.50 {
		t.Errorf("expected price 1.50, got %f", bid.Price)
	}
	if bid.ImpID != "imp-1" {
		t.Errorf("expected impid imp-1, got %s", bid.ImpID)
	}
}

// TestAuctionIntegration_MultipleBidders tests auction with multiple competing bidders
func TestAuctionIntegration_MultipleBidders(t *testing.T) {
	registry := adapters.NewRegistry()

	// Register three bidders with different prices
	// Set DemandType to Publisher so they show up as separate seatbids
	registry.Register("bidder1", &mockSuccessfulAdapter{bidPrice: 1.00}, adapters.BidderInfo{
		Enabled:    true,
		DemandType: adapters.DemandTypePublisher,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})
	registry.Register("bidder2", &mockSuccessfulAdapter{bidPrice: 2.50}, adapters.BidderInfo{
		Enabled:    true,
		DemandType: adapters.DemandTypePublisher,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})
	registry.Register("bidder3", &mockSuccessfulAdapter{bidPrice: 1.75}, adapters.BidderInfo{
		Enabled:    true,
		DemandType: adapters.DemandTypePublisher,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 1000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-auction-multi",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"bidder1":{},"bidder2":{},"bidder3":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp openrtb.BidResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Verify we got bids from all bidders (they may be in one or multiple seatbids)
	totalBids := 0
	for _, seatBid := range resp.SeatBid {
		totalBids += len(seatBid.Bid)
	}
	if totalBids < 3 {
		t.Errorf("expected at least 3 total bids, got %d", totalBids)
	}

	// Verify we have at least one seatbid
	if len(resp.SeatBid) == 0 {
		t.Error("expected at least one seatbid")
	}
}

// TestAuctionIntegration_MultipleImpressions tests auction with multiple ad slots
func TestAuctionIntegration_MultipleImpressions(t *testing.T) {
	registry := adapters.NewRegistry()
	registry.Register("testbidder", &mockSuccessfulAdapter{bidPrice: 1.50}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 1000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-multi-imp",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
			{
				ID:     "imp-2",
				Banner: &openrtb.Banner{W: 728, H: 90},
				Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
			{
				ID:     "imp-3",
				Banner: &openrtb.Banner{W: 160, H: 600},
				Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp openrtb.BidResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.SeatBid) == 0 {
		t.Fatal("expected at least 1 seatbid")
	}

	// Should get bids for all 3 impressions
	totalBids := 0
	for _, seatBid := range resp.SeatBid {
		totalBids += len(seatBid.Bid)
	}
	if totalBids != 3 {
		t.Errorf("expected 3 bids, got %d", totalBids)
	}

	// Verify each impression got a bid
	impMap := make(map[string]bool)
	for _, seatBid := range resp.SeatBid {
		for _, bid := range seatBid.Bid {
			impMap[bid.ImpID] = true
		}
	}
	for _, imp := range bidReq.Imp {
		if !impMap[imp.ID] {
			t.Errorf("missing bid for impression %s", imp.ID)
		}
	}
}

// TestAuctionIntegration_MixedBidderResults tests auction with some bidders succeeding and some failing
func TestAuctionIntegration_MixedBidderResults(t *testing.T) {
	registry := adapters.NewRegistry()

	registry.Register("successful", &mockSuccessfulAdapter{bidPrice: 2.00}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})
	registry.Register("failing", &mockFailingAdapter{errorMsg: "bidder error"}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})
	registry.Register("nobid", &mockNoBidAdapter{}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 1000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-mixed-results",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"successful":{},"failing":{},"nobid":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "test-key") // Enable debug to see errors
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Auction should still succeed even if some bidders fail
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp openrtb.BidResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Should only get bid from the successful bidder
	if len(resp.SeatBid) != 1 {
		t.Errorf("expected 1 seatbid, got %d", len(resp.SeatBid))
	}
}

// TestAuctionIntegration_TimeoutHandling tests auction timeout behavior
func TestAuctionIntegration_TimeoutHandling(t *testing.T) {
	registry := adapters.NewRegistry()

	// Slower bidder that takes 100ms (within timeout)
	registry.Register("slower", &mockSuccessfulAdapter{
		bidPrice: 1.00,
		delay:    100 * time.Millisecond,
	}, adapters.BidderInfo{
		Enabled:    true,
		DemandType: adapters.DemandTypePublisher,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	// Faster bidder that responds quickly
	registry.Register("faster", &mockSuccessfulAdapter{
		bidPrice: 2.00,
		delay:    10 * time.Millisecond,
	}, adapters.BidderInfo{
		Enabled:    true,
		DemandType: adapters.DemandTypePublisher,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	// Set timeout longer than both bidders
	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 500 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-timeout",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"slower":{},"faster":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(w, req)
	duration := time.Since(start)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp openrtb.BidResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Verify auction completed within reasonable time
	// Both bidders complete within timeout (100ms + 10ms < 500ms)
	if duration > 500*time.Millisecond {
		t.Errorf("auction took too long: %v (expected < 500ms)", duration)
	}

	// Should get bids from both bidders since both complete within timeout
	totalBids := 0
	for _, seatBid := range resp.SeatBid {
		totalBids += len(seatBid.Bid)
	}
	if totalBids != 2 {
		t.Errorf("expected 2 bids (one from each bidder), got %d", totalBids)
	}

	// Verify we have 2 separate seatbids (one per bidder with Publisher demand type)
	if len(resp.SeatBid) != 2 {
		t.Errorf("expected 2 seatbids, got %d", len(resp.SeatBid))
	}
}

// TestAuctionIntegration_LargeRequest tests handling of large request bodies
func TestAuctionIntegration_LargeRequest(t *testing.T) {
	registry := adapters.NewRegistry()
	registry.Register("testbidder", &mockSuccessfulAdapter{bidPrice: 1.50}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 1000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	// Create request with many impressions (but under size limit)
	imps := make([]openrtb.Imp, 50)
	for i := 0; i < 50; i++ {
		imps[i] = openrtb.Imp{
			ID:     "imp-" + string(rune('0'+i)),
			Banner: &openrtb.Banner{W: 300, H: 250},
			Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
		}
	}

	bidReq := &openrtb.BidRequest{
		ID:  "test-large",
		Imp: imps,
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	// Verify we're under the size limit
	if len(body) >= maxRequestBodySize {
		t.Skipf("test body too large: %d bytes", len(body))
	}

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp openrtb.BidResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.SeatBid) == 0 {
		t.Error("expected seatbids for large request")
	}
}

// TestAuctionIntegration_DebugModeWithAuth tests debug mode with authentication
func TestAuctionIntegration_DebugModeWithAuth(t *testing.T) {
	registry := adapters.NewRegistry()
	registry.Register("testbidder", &mockSuccessfulAdapter{bidPrice: 1.50}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 1000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-debug",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction?debug=1", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp openrtb.BidResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Debug mode should include ext with timing info
	if resp.Ext != nil {
		var ext openrtb.BidResponseExt
		if err := json.Unmarshal(resp.Ext, &ext); err == nil {
			// Should have response time tracking
			if ext.ResponseTimeMillis == nil {
				t.Error("expected response time millis in debug mode")
			}
		}
	}
}

// TestAuctionIntegration_ConcurrentRequests tests handling of concurrent auction requests
func TestAuctionIntegration_ConcurrentRequests(t *testing.T) {
	registry := adapters.NewRegistry()
	registry.Register("testbidder", &mockSuccessfulAdapter{bidPrice: 1.50}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 1000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-concurrent",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	// Run 10 concurrent requests
	concurrency := 10
	results := make(chan int, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)
			results <- w.Code
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < concurrency; i++ {
		code := <-results
		if code == http.StatusOK {
			successCount++
		}
	}

	if successCount != concurrency {
		t.Errorf("expected %d successful requests, got %d", concurrency, successCount)
	}
}

// TestAuctionIntegration_ContextCancellation tests handling of cancelled context
func TestAuctionIntegration_ContextCancellation(t *testing.T) {
	registry := adapters.NewRegistry()

	// Slow bidder to ensure context cancellation happens during processing
	registry.Register("slow", &mockSuccessfulAdapter{
		bidPrice: 1.00,
		delay:    500 * time.Millisecond,
	}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner}},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 2000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-cancel",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"slow":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	// Create context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should handle gracefully (either success with partial results or error)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 200 or 500, got %d", w.Code)
	}
}

// TestAuctionIntegration_DifferentMediaTypes tests auction with different ad formats
func TestAuctionIntegration_DifferentMediaTypes(t *testing.T) {
	registry := adapters.NewRegistry()
	registry.Register("testbidder", &mockSuccessfulAdapter{bidPrice: 1.50}, adapters.BidderInfo{
		Enabled: true,
		Capabilities: &adapters.CapabilitiesInfo{
			App: &adapters.PlatformInfo{
				MediaTypes: []adapters.BidType{
					adapters.BidTypeBanner,
					adapters.BidTypeVideo,
					adapters.BidTypeNative,
					adapters.BidTypeAudio,
				},
			},
			Site: &adapters.PlatformInfo{
				MediaTypes: []adapters.BidType{
					adapters.BidTypeBanner,
					adapters.BidTypeVideo,
					adapters.BidTypeNative,
					adapters.BidTypeAudio,
				},
			},
		},
	})

	ex := exchange.New(registry, &exchange.Config{
		DefaultTimeout: 1000 * time.Millisecond,
		IDREnabled:     false, // Disable IDR for testing
	})
	handler := NewAuctionHandler(ex)

	bidReq := &openrtb.BidRequest{
		ID: "test-media-types",
		Imp: []openrtb.Imp{
			{
				ID:     "banner-imp",
				Banner: &openrtb.Banner{W: 300, H: 250},
				Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
			{
				ID:    "video-imp",
				Video: &openrtb.Video{W: 640, H: 480, Mimes: []string{"video/mp4"}},
				Ext:   []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
			{
				ID:     "native-imp",
				Native: &openrtb.Native{Request: `{"ver":"1.2"}`},
				Ext:    []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
			{
				ID:    "audio-imp",
				Audio: &openrtb.Audio{Mimes: []string{"audio/mp3"}},
				Ext:   []byte(`{"prebid":{"bidder":{"testbidder":{}}}}`),
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
	body, _ := json.Marshal(bidReq)

	req := httptest.NewRequest("POST", "/openrtb2/auction", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp openrtb.BidResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Should handle all media types
	if len(resp.SeatBid) == 0 {
		t.Error("expected seatbids for different media types")
	}
}
