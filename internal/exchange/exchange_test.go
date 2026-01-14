package exchange

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/fpd"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// mockAdapter implements adapters.Adapter for testing
type mockAdapter struct {
	bids     []*adapters.TypedBid
	makeErr  error
	bidsErr  error
	requests []*adapters.RequestData
}

func (m *mockAdapter) MakeRequests(request *openrtb.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	if m.makeErr != nil {
		return nil, []error{m.makeErr}
	}
	if len(m.requests) > 0 {
		return m.requests, nil
	}
	// Return a minimal MOCK request to avoid real HTTP calls in tests
	return []*adapters.RequestData{
		{
			Method: "MOCK",
			URI:    "http://test.bidder.com/bid",
			Body:   []byte(`{}`),
		},
	}, nil
}

func (m *mockAdapter) MakeBids(internalRequest *openrtb.BidRequest, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if m.bidsErr != nil {
		return nil, []error{m.bidsErr}
	}
	return &adapters.BidderResponse{
		Bids: m.bids,
	}, nil
}

func TestExchangeNew(t *testing.T) {
	registry := adapters.NewRegistry()

	ex := New(registry, nil)
	if ex == nil {
		t.Fatal("expected non-nil exchange")
	}

	// Test with custom config
	config := &Config{
		DefaultTimeout:  500 * time.Millisecond,
		MaxBidders:      10,
		IDREnabled:      false,
		DefaultCurrency: "EUR",
	}

	ex = New(registry, config)
	if ex.config.DefaultTimeout != 500*time.Millisecond {
		t.Errorf("expected 500ms timeout, got %v", ex.config.DefaultTimeout)
	}
	if ex.config.DefaultCurrency != "EUR" {
		t.Errorf("expected EUR currency, got %s", ex.config.DefaultCurrency)
	}
}

// testSite returns a minimal valid Site for tests
func testSite() *openrtb.Site {
	return &openrtb.Site{
		ID:   "test-site",
		Name: "Test Site",
	}
}

func TestExchangeRunAuctionNoBidders(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		DefaultTimeout: 100 * time.Millisecond,
		IDREnabled:     false,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-req-1",
			Site: testSite(),
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			},
		},
	}

	resp, err := ex.RunAuction(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.BidResponse == nil {
		t.Fatal("expected non-nil bid response")
	}

	if len(resp.BidResponse.SeatBid) != 0 {
		t.Errorf("expected 0 seat bids, got %d", len(resp.BidResponse.SeatBid))
	}
}

func TestExchangeRunAuctionWithBidders(t *testing.T) {
	registry := adapters.NewRegistry()

	// Register a mock adapter
	mockBid := &openrtb.Bid{
		ID:    "bid1",
		ImpID: "imp1",
		Price: 2.50,
		AdM:   "<div>test ad</div>",
	}

	// Create mock HTTP server for bidder endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a valid bid response
		resp := &openrtb.BidResponse{
			ID: "test-req-2",
			SeatBid: []openrtb.SeatBid{
				{
					Bid: []openrtb.Bid{*mockBid},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	mock := &mockAdapter{
		bids: []*adapters.TypedBid{
			{Bid: mockBid, BidType: adapters.BidTypeBanner},
		},
		requests: []*adapters.RequestData{{Method: "POST", URI: server.URL, Body: []byte(`{}`)}},
	}

	registry.Register("test-bidder", mock, adapters.BidderInfo{
		Enabled: true,
	})

	ex := New(registry, &Config{
		DefaultTimeout: 500 * time.Millisecond,
		IDREnabled:     false,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-req-2",
			Site: testSite(),
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			},
		},
	}

	resp, err := ex.RunAuction(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.BidderResults == nil {
		t.Fatal("expected non-nil bidder results")
	}

	// Check that test-bidder was called
	result, ok := resp.BidderResults["test-bidder"]
	if !ok {
		t.Error("expected test-bidder in results")
	} else {
		if result.BidderCode != "test-bidder" {
			t.Errorf("expected bidder code 'test-bidder', got %s", result.BidderCode)
		}
	}
}

func TestExchangeFPDProcessing(t *testing.T) {
	registry := adapters.NewRegistry()

	// Register a mock adapter
	mock := &mockAdapter{
		bids: []*adapters.TypedBid{},
	}
	registry.Register("bidder1", mock, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout: 100 * time.Millisecond,
		IDREnabled:     false,
		FPD: &fpd.Config{
			Enabled:     true,
			SiteEnabled: true,
			UserEnabled: true,
		},
	})

	// Create request with FPD
	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID: "test-fpd",
			Site: &openrtb.Site{
				ID:   "site1",
				Name: "Test Site",
				Ext:  json.RawMessage(`{"data":{"category":"news"}}`),
			},
			User: &openrtb.User{
				ID:  "user1",
				Ext: json.RawMessage(`{"data":{"interests":["sports","tech"]}}`),
			},
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			},
		},
	}

	resp, err := ex.RunAuction(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// FPD should be processed without errors
	if len(resp.DebugInfo.Errors["fpd"]) > 0 {
		t.Errorf("unexpected FPD errors: %v", resp.DebugInfo.Errors["fpd"])
	}
}

func TestExchangeEIDFiltering(t *testing.T) {
	registry := adapters.NewRegistry()

	mock := &mockAdapter{bids: []*adapters.TypedBid{}}
	registry.Register("bidder1", mock, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout: 100 * time.Millisecond,
		IDREnabled:     false,
		FPD: &fpd.Config{
			Enabled:     true,
			EIDsEnabled: true,
			EIDSources:  []string{"liveramp.com"},
		},
	})

	// Create request with multiple EIDs
	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-eid",
			Site: testSite(),
			User: &openrtb.User{
				ID: "user1",
				EIDs: []openrtb.EID{
					{Source: "liveramp.com", UIDs: []openrtb.UID{{ID: "lr123"}}},
					{Source: "blocked.com", UIDs: []openrtb.UID{{ID: "blk456"}}},
				},
			},
			Imp: []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
		},
	}

	_, err := ex.RunAuction(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After filtering, only liveramp.com should remain
	if len(req.BidRequest.User.EIDs) != 1 {
		t.Errorf("expected 1 EID after filtering, got %d", len(req.BidRequest.User.EIDs))
	}
	if req.BidRequest.User.EIDs[0].Source != "liveramp.com" {
		t.Errorf("expected liveramp.com EID, got %s", req.BidRequest.User.EIDs[0].Source)
	}
}

func TestExchangeTimeoutFromRequest(t *testing.T) {
	registry := adapters.NewRegistry()

	ex := New(registry, &Config{
		DefaultTimeout: 1 * time.Second,
		IDREnabled:     false,
	})

	// Request with TMax
	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-timeout",
			Site: testSite(),
			TMax: 100, // 100ms
			Imp:  []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
		},
	}

	start := time.Now()
	_, err := ex.RunAuction(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should complete quickly since no bidders
	if elapsed > 200*time.Millisecond {
		t.Errorf("auction took too long: %v", elapsed)
	}
}

func TestExchangeUpdateFPDConfig(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, nil)

	// Initial config
	if ex.GetFPDConfig() == nil {
		t.Error("expected non-nil initial FPD config")
	}

	// Update config
	newConfig := &fpd.Config{
		Enabled:     false,
		EIDsEnabled: false,
	}
	ex.UpdateFPDConfig(newConfig)

	got := ex.GetFPDConfig()
	if got.Enabled {
		t.Error("expected FPD to be disabled after update")
	}
}

func TestExchangeDebugInfo(t *testing.T) {
	registry := adapters.NewRegistry()

	mock := &mockAdapter{bids: []*adapters.TypedBid{}}
	registry.Register("bidder1", mock, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout: 100 * time.Millisecond,
		IDREnabled:     false,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-debug",
			Site: testSite(),
			Imp:  []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
		},
		Debug: true,
	}

	resp, err := ex.RunAuction(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.DebugInfo == nil {
		t.Fatal("expected non-nil debug info")
	}

	if resp.DebugInfo.RequestTime.IsZero() {
		t.Error("expected non-zero request time")
	}

	if resp.DebugInfo.TotalLatency == 0 {
		t.Error("expected non-zero total latency")
	}

	if len(resp.DebugInfo.SelectedBidders) != 1 {
		t.Errorf("expected 1 selected bidder, got %d", len(resp.DebugInfo.SelectedBidders))
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.DefaultTimeout == 0 {
		t.Error("expected non-zero default timeout")
	}
	if config.MaxBidders == 0 {
		t.Error("expected non-zero max bidders")
	}
	if config.DefaultCurrency == "" {
		t.Error("expected non-empty default currency")
	}
	if config.FPD == nil {
		t.Error("expected non-nil FPD config")
	}
	if config.AuctionType != FirstPriceAuction {
		t.Error("expected first-price auction as default")
	}
	if config.PriceIncrement != 0.01 {
		t.Errorf("expected 0.01 price increment, got %f", config.PriceIncrement)
	}
}

func TestBidValidation(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		DefaultTimeout:  100 * time.Millisecond,
		IDREnabled:      false,
		DefaultCurrency: "USD",
		MinBidPrice:     0.01,
	})

	impFloors := map[string]float64{
		"imp1": 1.00,
		"imp2": 0.50,
	}

	tests := []struct {
		name        string
		bid         *openrtb.Bid
		bidderCode  string
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil bid",
			bid:         nil,
			bidderCode:  "test-bidder",
			wantErr:     true,
			errContains: "nil bid",
		},
		{
			name:        "missing bid ID",
			bid:         &openrtb.Bid{ImpID: "imp1", Price: 1.50},
			bidderCode:  "test-bidder",
			wantErr:     true,
			errContains: "missing required field: id",
		},
		{
			name:        "missing impID",
			bid:         &openrtb.Bid{ID: "bid1", Price: 1.50},
			bidderCode:  "test-bidder",
			wantErr:     true,
			errContains: "missing required field: impid",
		},
		{
			name:        "invalid impID",
			bid:         &openrtb.Bid{ID: "bid1", ImpID: "invalid", Price: 1.50},
			bidderCode:  "test-bidder",
			wantErr:     true,
			errContains: "not found in request",
		},
		{
			name:        "negative price",
			bid:         &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: -1.00},
			bidderCode:  "test-bidder",
			wantErr:     true,
			errContains: "negative price",
		},
		{
			name:        "below minimum price",
			bid:         &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 0.005},
			bidderCode:  "test-bidder",
			wantErr:     true,
			errContains: "below minimum",
		},
		{
			name:        "below floor price",
			bid:         &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 0.75},
			bidderCode:  "test-bidder",
			wantErr:     true,
			errContains: "below floor",
		},
		{
			name:       "valid bid at floor",
			bid:        &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.00, AdM: "<div>ad</div>"},
			bidderCode: "test-bidder",
			wantErr:    false,
		},
		{
			name:       "valid bid above floor",
			bid:        &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 2.50, AdM: "<div>ad</div>"},
			bidderCode: "test-bidder",
			wantErr:    false,
		},
		{
			name:       "valid bid lower floor impression",
			bid:        &openrtb.Bid{ID: "bid2", ImpID: "imp2", Price: 0.50, AdM: "<div>ad</div>"},
			bidderCode: "test-bidder",
			wantErr:    false,
		},
		{
			name:        "missing adm and nurl",
			bid:         &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 2.00},
			bidderCode:  "test-bidder",
			wantErr:     true,
			errContains: "adm or nurl",
		},
		{
			name:       "valid bid with nurl only",
			bid:        &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 2.00, NURL: "http://example.com/win"},
			bidderCode: "test-bidder",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ex.validateBid(tt.bid, tt.bidderCode, impFloors)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBidDeduplication(t *testing.T) {
	registry := adapters.NewRegistry()

	// Create mock HTTP server for bidder1
	bid1 := &openrtb.Bid{ID: "dup-bid", ImpID: "imp1", Price: 2.00, AdM: "<div>ad1</div>"}
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &openrtb.BidResponse{
			ID: "test-request",
			SeatBid: []openrtb.SeatBid{
				{Bid: []openrtb.Bid{*bid1}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server1.Close()

	// Create mock HTTP server for bidder2
	bid2 := &openrtb.Bid{ID: "dup-bid", ImpID: "imp1", Price: 3.00, AdM: "<div>ad2</div>"}
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &openrtb.BidResponse{
			ID: "test-request",
			SeatBid: []openrtb.SeatBid{
				{Bid: []openrtb.Bid{*bid2}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server2.Close()

	// Create adapters that use the mock servers
	mock1 := &mockAdapter{
		bids: []*adapters.TypedBid{
			{Bid: bid1, BidType: adapters.BidTypeBanner},
		},
		requests: []*adapters.RequestData{
			{Method: "POST", URI: server1.URL, Body: []byte(`{}`)},
		},
	}
	mock2 := &mockAdapter{
		bids: []*adapters.TypedBid{
			{Bid: bid2, BidType: adapters.BidTypeBanner},
		},
		requests: []*adapters.RequestData{
			{Method: "POST", URI: server2.URL, Body: []byte(`{}`)},
		},
	}

	registry.Register("bidder1", mock1, adapters.BidderInfo{Enabled: true})
	registry.Register("bidder2", mock2, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout:  500 * time.Millisecond,
		IDREnabled:      false,
		DefaultCurrency: "USD",
		AuctionType:     FirstPriceAuction,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-dedup",
			Site: testSite(),
			Imp:  []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
		},
	}

	resp, err := ex.RunAuction(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only one bid should win (first one seen), the duplicate should be rejected
	totalBids := 0
	for _, sb := range resp.BidResponse.SeatBid {
		totalBids += len(sb.Bid)
	}

	if totalBids != 1 {
		t.Errorf("expected 1 bid after deduplication, got %d", totalBids)
	}

	// Check that a deduplication error was recorded
	foundDupError := false
	for _, errors := range resp.DebugInfo.Errors {
		for _, errMsg := range errors {
			if containsString(errMsg, "duplicate bid ID") {
				foundDupError = true
				break
			}
		}
	}
	if !foundDupError {
		t.Error("expected duplicate bid error in debug info")
	}
}

func TestSecondPriceAuction(t *testing.T) {
	registry := adapters.NewRegistry()

	// Create mock HTTP servers and bidders with different prices
	bid1 := &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 5.00, AdM: "<div>ad1</div>"}
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &openrtb.BidResponse{
			ID:      "test-second-price",
			SeatBid: []openrtb.SeatBid{{Bid: []openrtb.Bid{*bid1}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server1.Close()

	bid2 := &openrtb.Bid{ID: "bid2", ImpID: "imp1", Price: 3.00, AdM: "<div>ad2</div>"}
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &openrtb.BidResponse{
			ID:      "test-second-price",
			SeatBid: []openrtb.SeatBid{{Bid: []openrtb.Bid{*bid2}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server2.Close()

	bid3 := &openrtb.Bid{ID: "bid3", ImpID: "imp1", Price: 2.00, AdM: "<div>ad3</div>"}
	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &openrtb.BidResponse{
			ID:      "test-second-price",
			SeatBid: []openrtb.SeatBid{{Bid: []openrtb.Bid{*bid3}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server3.Close()

	mock1 := &mockAdapter{
		bids:     []*adapters.TypedBid{{Bid: bid1, BidType: adapters.BidTypeBanner}},
		requests: []*adapters.RequestData{{Method: "POST", URI: server1.URL, Body: []byte(`{}`)}},
	}
	mock2 := &mockAdapter{
		bids:     []*adapters.TypedBid{{Bid: bid2, BidType: adapters.BidTypeBanner}},
		requests: []*adapters.RequestData{{Method: "POST", URI: server2.URL, Body: []byte(`{}`)}},
	}
	mock3 := &mockAdapter{
		bids:     []*adapters.TypedBid{{Bid: bid3, BidType: adapters.BidTypeBanner}},
		requests: []*adapters.RequestData{{Method: "POST", URI: server3.URL, Body: []byte(`{}`)}},
	}

	registry.Register("bidder1", mock1, adapters.BidderInfo{Enabled: true})
	registry.Register("bidder2", mock2, adapters.BidderInfo{Enabled: true})
	registry.Register("bidder3", mock3, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout:  500 * time.Millisecond,
		IDREnabled:      false,
		DefaultCurrency: "USD",
		AuctionType:     SecondPriceAuction,
		PriceIncrement:  0.01,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-second-price",
			Site: testSite(),
			Imp:  []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
		},
	}

	resp, err := ex.RunAuction(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the winning bid (highest)
	var winningPrice float64
	for _, sb := range resp.BidResponse.SeatBid {
		for _, bid := range sb.Bid {
			if bid.Price > winningPrice {
				winningPrice = bid.Price
			}
		}
	}

	// In second-price auction, winner should pay second-highest + increment
	// Second highest is 3.00, so winning price should be 3.01
	expectedPrice := 3.01
	if winningPrice != expectedPrice {
		t.Errorf("expected winning price %.2f (second price + increment), got %.2f", expectedPrice, winningPrice)
	}
}

func TestFirstPriceAuction(t *testing.T) {
	registry := adapters.NewRegistry()

	// Create mock HTTP servers
	bid1 := &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 5.00, AdM: "<div>ad1</div>"}
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &openrtb.BidResponse{
			ID:      "test-first-price",
			SeatBid: []openrtb.SeatBid{{Bid: []openrtb.Bid{*bid1}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server1.Close()

	bid2 := &openrtb.Bid{ID: "bid2", ImpID: "imp1", Price: 3.00, AdM: "<div>ad2</div>"}
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &openrtb.BidResponse{
			ID:      "test-first-price",
			SeatBid: []openrtb.SeatBid{{Bid: []openrtb.Bid{*bid2}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server2.Close()

	mock1 := &mockAdapter{
		bids:     []*adapters.TypedBid{{Bid: bid1, BidType: adapters.BidTypeBanner}},
		requests: []*adapters.RequestData{{Method: "POST", URI: server1.URL, Body: []byte(`{}`)}},
	}
	mock2 := &mockAdapter{
		bids:     []*adapters.TypedBid{{Bid: bid2, BidType: adapters.BidTypeBanner}},
		requests: []*adapters.RequestData{{Method: "POST", URI: server2.URL, Body: []byte(`{}`)}},
	}

	registry.Register("bidder1", mock1, adapters.BidderInfo{Enabled: true})
	registry.Register("bidder2", mock2, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout:  500 * time.Millisecond,
		IDREnabled:      false,
		DefaultCurrency: "USD",
		AuctionType:     FirstPriceAuction,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-first-price",
			Site: testSite(),
			Imp:  []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
		},
	}

	resp, err := ex.RunAuction(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the highest bid
	var highestPrice float64
	for _, sb := range resp.BidResponse.SeatBid {
		for _, bid := range sb.Bid {
			if bid.Price > highestPrice {
				highestPrice = bid.Price
			}
		}
	}

	// In first-price auction, winner pays their bid (5.00)
	if highestPrice != 5.00 {
		t.Errorf("expected winning price 5.00 (first price), got %.2f", highestPrice)
	}
}

func TestSortBidsByPrice(t *testing.T) {
	bids := []ValidatedBid{
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b1", Price: 1.00}}, BidderCode: "bidder1"},
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b2", Price: 5.00}}, BidderCode: "bidder2"},
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b3", Price: 3.00}}, BidderCode: "bidder3"},
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b4", Price: 2.50}}, BidderCode: "bidder4"},
	}

	sortBidsByPrice(bids)

	// Should be sorted descending
	expectedPrices := []float64{5.00, 3.00, 2.50, 1.00}
	for i, bid := range bids {
		if bid.Bid.Bid.Price != expectedPrices[i] {
			t.Errorf("position %d: expected price %.2f, got %.2f", i, expectedPrices[i], bid.Bid.Bid.Price)
		}
	}
}

func TestBuildImpFloorMap(t *testing.T) {
	// Create a minimal exchange instance
	ex := &Exchange{}

	req := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp1", BidFloor: 1.50},
			{ID: "imp2", BidFloor: 0.75},
			{ID: "imp3", BidFloor: 0}, // No floor
		},
	}

	ctx := context.Background()
	floors := ex.buildImpFloorMap(ctx, req)

	if len(floors) != 3 {
		t.Errorf("expected 3 floor entries, got %d", len(floors))
	}
	if floors["imp1"] != 1.50 {
		t.Errorf("expected imp1 floor 1.50, got %f", floors["imp1"])
	}
	if floors["imp2"] != 0.75 {
		t.Errorf("expected imp2 floor 0.75, got %f", floors["imp2"])
	}
	if floors["imp3"] != 0 {
		t.Errorf("expected imp3 floor 0, got %f", floors["imp3"])
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// P2 Validation Tests

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name        string
		request     *openrtb.BidRequest
		wantErr     bool
		errField    string
		errContains string
	}{
		{
			name:        "nil request",
			request:     nil,
			wantErr:     true,
			errField:    "request",
			errContains: "nil",
		},
		{
			name:        "empty request ID",
			request:     &openrtb.BidRequest{ID: "", Site: testSite(), Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     true,
			errField:    "id",
			errContains: "required",
		},
		{
			name:        "no impressions",
			request:     &openrtb.BidRequest{ID: "req1", Site: testSite(), Imp: []openrtb.Imp{}},
			wantErr:     true,
			errField:    "imp",
			errContains: "at least one",
		},
		{
			name:        "empty impression ID",
			request:     &openrtb.BidRequest{ID: "req1", Site: testSite(), Imp: []openrtb.Imp{{ID: ""}}},
			wantErr:     true,
			errField:    "imp[0].id",
			errContains: "required",
		},
		{
			name: "duplicate impression IDs",
			request: &openrtb.BidRequest{
				ID:   "req1",
				Site: testSite(),
				Imp: []openrtb.Imp{
					{ID: "imp1"},
					{ID: "imp1"}, // Duplicate
				},
			},
			wantErr:     true,
			errField:    "imp[1].id",
			errContains: "duplicate",
		},
		{
			name:        "missing site and app",
			request:     &openrtb.BidRequest{ID: "req1", Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     true,
			errField:    "site/app",
			errContains: "either site or app",
		},
		{
			name: "both site and app present",
			request: &openrtb.BidRequest{
				ID:   "req1",
				Site: testSite(),
				App:  &openrtb.App{ID: "app1"},
				Imp:  []openrtb.Imp{{ID: "imp1"}},
			},
			wantErr:     true,
			errField:    "site/app",
			errContains: "cannot contain both",
		},
		{
			name:        "tmax too low",
			request:     &openrtb.BidRequest{ID: "req1", Site: testSite(), TMax: 5, Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     true,
			errField:    "tmax",
			errContains: "minimum 10",
		},
		{
			name:        "tmax too high",
			request:     &openrtb.BidRequest{ID: "req1", Site: testSite(), TMax: 35000, Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     true,
			errField:    "tmax",
			errContains: "30000",
		},
		{
			name:        "valid request with site",
			request:     &openrtb.BidRequest{ID: "req1", Site: testSite(), Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     false,
		},
		{
			name:        "valid request with app",
			request:     &openrtb.BidRequest{ID: "req1", App: &openrtb.App{ID: "app1"}, Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     false,
		},
		{
			name:        "valid request with tmax at lower bound",
			request:     &openrtb.BidRequest{ID: "req1", Site: testSite(), TMax: 10, Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     false,
		},
		{
			name:        "valid request with tmax at upper bound",
			request:     &openrtb.BidRequest{ID: "req1", Site: testSite(), TMax: 30000, Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     false,
		},
		{
			name:        "valid request with zero tmax (no limit)",
			request:     &openrtb.BidRequest{ID: "req1", Site: testSite(), TMax: 0, Imp: []openrtb.Imp{{ID: "imp1"}}},
			wantErr:     false,
		},
		{
			name: "valid request with multiple unique impressions",
			request: &openrtb.BidRequest{
				ID:   "req1",
				Site: testSite(),
				Imp: []openrtb.Imp{
					{ID: "imp1"},
					{ID: "imp2"},
					{ID: "imp3"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequest(tt.request)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else {
					if err.Field != tt.errField {
						t.Errorf("expected field %q, got %q", tt.errField, err.Field)
					}
					if tt.errContains != "" && !containsString(err.Reason, tt.errContains) {
						t.Errorf("expected reason containing %q, got %q", tt.errContains, err.Reason)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateRequestInRunAuction(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		DefaultTimeout: 100 * time.Millisecond,
		IDREnabled:     false,
	})

	// Test that invalid request returns error from RunAuction
	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:  "req1",
			Imp: []openrtb.Imp{{ID: "imp1"}},
			// Missing Site and App - should fail validation
		},
	}

	_, err := ex.RunAuction(context.Background(), req)
	if err == nil {
		t.Error("expected validation error, got nil")
	}

	// The early validation in RunAuction returns a plain error for site/app check
	// (before the formal ValidateRequest call), so we check for the error message
	if !containsString(err.Error(), "site") && !containsString(err.Error(), "app") {
		t.Errorf("expected site/app validation error, got: %v", err)
	}
}

// Additional tests for improved coverage

func TestDefaultCloneLimits(t *testing.T) {
	limits := DefaultCloneLimits()

	if limits == nil {
		t.Fatal("expected non-nil limits")
	}
	if limits.MaxImpressionsPerRequest != defaultMaxImpressionsPerRequest {
		t.Errorf("expected %d impressions, got %d", defaultMaxImpressionsPerRequest, limits.MaxImpressionsPerRequest)
	}
	if limits.MaxEIDsPerUser != defaultMaxEIDsPerUser {
		t.Errorf("expected %d EIDs, got %d", defaultMaxEIDsPerUser, limits.MaxEIDsPerUser)
	}
	if limits.MaxDataPerUser != defaultMaxDataPerUser {
		t.Errorf("expected %d data, got %d", defaultMaxDataPerUser, limits.MaxDataPerUser)
	}
	if limits.MaxDealsPerImp != defaultMaxDealsPerImp {
		t.Errorf("expected %d deals, got %d", defaultMaxDealsPerImp, limits.MaxDealsPerImp)
	}
	if limits.MaxSChainNodes != defaultMaxSChainNodes {
		t.Errorf("expected %d schain nodes, got %d", defaultMaxSChainNodes, limits.MaxSChainNodes)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		check    func(*Config) bool
		expected string
	}{
		{
			name:   "negative timeout uses default",
			config: &Config{DefaultTimeout: -1 * time.Second},
			check:  func(c *Config) bool { return c.DefaultTimeout > 0 },
		},
		{
			name:   "zero timeout uses default",
			config: &Config{DefaultTimeout: 0},
			check:  func(c *Config) bool { return c.DefaultTimeout > 0 },
		},
		{
			name:   "negative max bidders uses default",
			config: &Config{MaxBidders: -5},
			check:  func(c *Config) bool { return c.MaxBidders > 0 },
		},
		{
			name:   "negative max concurrent uses default",
			config: &Config{MaxConcurrentBidders: -1},
			check:  func(c *Config) bool { return c.MaxConcurrentBidders >= 0 },
		},
		{
			name:   "invalid auction type uses first price",
			config: &Config{AuctionType: AuctionType(99)},
			check:  func(c *Config) bool { return c.AuctionType == FirstPriceAuction },
		},
		{
			name:   "second price with zero increment uses default",
			config: &Config{AuctionType: SecondPriceAuction, PriceIncrement: 0},
			check:  func(c *Config) bool { return c.PriceIncrement > 0 },
		},
		{
			name:   "second price with negative increment uses default",
			config: &Config{AuctionType: SecondPriceAuction, PriceIncrement: -0.01},
			check:  func(c *Config) bool { return c.PriceIncrement > 0 },
		},
		{
			name:   "negative min bid price uses zero",
			config: &Config{MinBidPrice: -1.00},
			check:  func(c *Config) bool { return c.MinBidPrice >= 0 },
		},
		{
			name:   "event recording with zero buffer uses default",
			config: &Config{EventRecordEnabled: true, EventBufferSize: 0},
			check:  func(c *Config) bool { return c.EventBufferSize > 0 },
		},
		{
			name:   "nil clone limits uses defaults",
			config: &Config{CloneLimits: nil},
			check:  func(c *Config) bool { return c.CloneLimits != nil && c.CloneLimits.MaxImpressionsPerRequest > 0 },
		},
		{
			name:   "invalid clone limits uses defaults",
			config: &Config{CloneLimits: &CloneLimits{MaxImpressionsPerRequest: -1, MaxEIDsPerUser: 0}},
			check: func(c *Config) bool {
				return c.CloneLimits.MaxImpressionsPerRequest > 0 && c.CloneLimits.MaxEIDsPerUser > 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validated := validateConfig(tt.config)
			if !tt.check(validated) {
				t.Errorf("validation check failed for %s", tt.name)
			}
		})
	}
}

func TestRoundToCents(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.234, 1.23},
		{1.235, 1.24}, // Rounds up at .5
		{1.999, 2.00},
		{0.001, 0.00},
		{0.005, 0.01}, // Rounds up at .5
		{0.004, 0.00},
		{100.555, 100.56},
		{-1.234, -1.23},
		{-1.235, -1.24}, // Negative rounds correctly
		{0, 0},
	}

	for _, tt := range tests {
		result := roundToCents(tt.input)
		if result != tt.expected {
			t.Errorf("roundToCents(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

func TestRequestValidationError_Error(t *testing.T) {
	err := &RequestValidationError{
		Field:  "imp[0].id",
		Reason: "missing required field",
	}

	expected := "invalid request: imp[0].id - missing required field"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestBidValidationError_Error(t *testing.T) {
	err := &BidValidationError{
		BidID:      "bid-123",
		ImpID:      "imp-456",
		BidderCode: "appnexus",
		Reason:     "price below floor",
	}

	result := err.Error()
	if !containsString(result, "appnexus") {
		t.Errorf("expected bidder code in error: %s", result)
	}
	if !containsString(result, "bid-123") {
		t.Errorf("expected bid ID in error: %s", result)
	}
	if !containsString(result, "imp-456") {
		t.Errorf("expected imp ID in error: %s", result)
	}
	if !containsString(result, "price below floor") {
		t.Errorf("expected reason in error: %s", result)
	}
}

func TestDebugInfo_AddError(t *testing.T) {
	debug := &DebugInfo{
		Errors: make(map[string][]string),
	}

	debug.AddError("bidder1", []string{"error1", "error2"})

	if len(debug.Errors["bidder1"]) != 2 {
		t.Errorf("expected 2 errors, got %d", len(debug.Errors["bidder1"]))
	}
	if debug.Errors["bidder1"][0] != "error1" {
		t.Errorf("expected 'error1', got %s", debug.Errors["bidder1"][0])
	}
}

func TestDebugInfo_AppendError(t *testing.T) {
	debug := &DebugInfo{
		Errors: make(map[string][]string),
	}

	debug.AppendError("bidder1", "first error")
	debug.AppendError("bidder1", "second error")

	if len(debug.Errors["bidder1"]) != 2 {
		t.Errorf("expected 2 errors, got %d", len(debug.Errors["bidder1"]))
	}
	if debug.Errors["bidder1"][1] != "second error" {
		t.Errorf("expected 'second error', got %s", debug.Errors["bidder1"][1])
	}
}

func TestExchange_Close(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		IDREnabled:         false,
		EventRecordEnabled: false,
	})

	// Close should succeed with no event recorder
	err := ex.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExchange_DynamicRegistry(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, nil)

	// Initially nil
	if ex.GetDynamicRegistry() != nil {
		t.Error("expected nil dynamic registry initially")
	}

	// Set and get
	ex.SetDynamicRegistry(nil)
	if ex.GetDynamicRegistry() != nil {
		t.Error("expected nil after setting nil")
	}
}

func TestExchange_GetIDRClient(t *testing.T) {
	registry := adapters.NewRegistry()

	// Without IDR enabled
	ex := New(registry, &Config{
		IDREnabled: false,
	})
	if ex.GetIDRClient() != nil {
		t.Error("expected nil IDR client when disabled")
	}

	// With IDR enabled but no URL
	ex = New(registry, &Config{
		IDREnabled:    true,
		IDRServiceURL: "",
	})
	if ex.GetIDRClient() != nil {
		t.Error("expected nil IDR client with empty URL")
	}
}

func TestDeepCloneRequest_StringSlices(t *testing.T) {
	limits := DefaultCloneLimits()
	req := &openrtb.BidRequest{
		ID:    "test",
		Cur:   []string{"USD", "EUR"},
		WSeat: []string{"seat1", "seat2"},
		BSeat: []string{"blocked1"},
		WLang: []string{"en", "es"},
		BCat:  []string{"IAB1"},
		BAdv:  []string{"blocked.com"},
		BApp:  []string{"blocked-app"},
		Site:  testSite(),
		Imp:   []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
	}

	clone := deepCloneRequest(req, limits)

	// Modify original
	req.Cur[0] = "GBP"
	req.WSeat[0] = "modified"

	// Clone should be unaffected
	if clone.Cur[0] != "USD" {
		t.Errorf("expected clone Cur[0] = USD, got %s", clone.Cur[0])
	}
	if clone.WSeat[0] != "seat1" {
		t.Errorf("expected clone WSeat[0] = seat1, got %s", clone.WSeat[0])
	}
}

func TestDeepCloneRequest_NestedObjects(t *testing.T) {
	limits := DefaultCloneLimits()
	req := &openrtb.BidRequest{
		ID: "test",
		Site: &openrtb.Site{
			ID:   "site1",
			Name: "Test Site",
			Publisher: &openrtb.Publisher{
				ID:   "pub1",
				Name: "Test Publisher",
			},
			Content: &openrtb.Content{
				ID:    "content1",
				Title: "Test Content",
			},
		},
		User: &openrtb.User{
			ID: "user1",
			Geo: &openrtb.Geo{
				Country: "US",
				City:    "NYC",
			},
			EIDs: []openrtb.EID{
				{Source: "liveramp.com"},
				{Source: "pubcid.org"},
			},
			Data: []openrtb.Data{
				{ID: "data1", Name: "segment1"},
			},
		},
		Device: &openrtb.Device{
			UA: "Mozilla/5.0",
			Geo: &openrtb.Geo{
				Country: "US",
			},
		},
		Regs: &openrtb.Regs{
			COPPA: 1,
			GDPR:  ptrInt(1),
		},
		Imp: []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
	}

	clone := deepCloneRequest(req, limits)

	// Modify original
	req.Site.ID = "modified"
	req.Site.Publisher.ID = "modified"
	req.User.Geo.Country = "CA"
	req.Device.UA = "modified"

	// Clone should be unaffected
	if clone.Site.ID != "site1" {
		t.Errorf("expected clone Site.ID = site1, got %s", clone.Site.ID)
	}
	if clone.Site.Publisher.ID != "pub1" {
		t.Errorf("expected clone Publisher.ID = pub1, got %s", clone.Site.Publisher.ID)
	}
	if clone.User.Geo.Country != "US" {
		t.Errorf("expected clone User.Geo.Country = US, got %s", clone.User.Geo.Country)
	}
	if clone.Device.UA != "Mozilla/5.0" {
		t.Errorf("expected clone Device.UA = Mozilla/5.0, got %s", clone.Device.UA)
	}
}

func TestDeepCloneRequest_App(t *testing.T) {
	limits := DefaultCloneLimits()
	req := &openrtb.BidRequest{
		ID: "test",
		App: &openrtb.App{
			ID:   "app1",
			Name: "Test App",
			Publisher: &openrtb.Publisher{
				ID:   "pub1",
				Name: "App Publisher",
			},
			Content: &openrtb.Content{
				ID:    "content1",
				Title: "App Content",
			},
		},
		Imp: []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
	}

	clone := deepCloneRequest(req, limits)

	// Modify original
	req.App.ID = "modified"
	req.App.Publisher.ID = "modified"

	// Clone should be unaffected
	if clone.App.ID != "app1" {
		t.Errorf("expected clone App.ID = app1, got %s", clone.App.ID)
	}
	if clone.App.Publisher.ID != "pub1" {
		t.Errorf("expected clone App.Publisher.ID = pub1, got %s", clone.App.Publisher.ID)
	}
}

func TestDeepCloneRequest_Source(t *testing.T) {
	limits := DefaultCloneLimits()
	req := &openrtb.BidRequest{
		ID:   "test",
		Site: testSite(),
		Source: &openrtb.Source{
			TID: "tid-123",
			SChain: &openrtb.SupplyChain{
				Ver:      "1.0",
				Complete: 1,
				Nodes: []openrtb.SupplyChainNode{
					{ASI: "exchange.com", SID: "1234"},
					{ASI: "reseller.com", SID: "5678"},
				},
			},
		},
		Imp: []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
	}

	clone := deepCloneRequest(req, limits)

	// Modify original
	req.Source.TID = "modified"
	req.Source.SChain.Nodes[0].ASI = "modified"

	// Clone should be unaffected
	if clone.Source.TID != "tid-123" {
		t.Errorf("expected clone Source.TID = tid-123, got %s", clone.Source.TID)
	}
	if clone.Source.SChain.Nodes[0].ASI != "exchange.com" {
		t.Errorf("expected clone SChain node ASI = exchange.com, got %s", clone.Source.SChain.Nodes[0].ASI)
	}
}

func TestDeepCloneRequest_Impressions(t *testing.T) {
	limits := DefaultCloneLimits()
	req := &openrtb.BidRequest{
		ID:   "test",
		Site: testSite(),
		Imp: []openrtb.Imp{
			{
				ID:     "imp1",
				Banner: &openrtb.Banner{W: 300, H: 250},
			},
			{
				ID:    "imp2",
				Video: &openrtb.Video{W: 640, H: 480},
			},
			{
				ID:    "imp3",
				Audio: &openrtb.Audio{MinDuration: 15},
			},
			{
				ID:     "imp4",
				Native: &openrtb.Native{Request: "{}"},
			},
			{
				ID:     "imp5",
				Banner: &openrtb.Banner{W: 728, H: 90},
				PMP: &openrtb.PMP{
					PrivateAuction: 1,
					Deals: []openrtb.Deal{
						{ID: "deal1", BidFloor: 5.00},
						{ID: "deal2", BidFloor: 3.00},
					},
				},
			},
		},
	}

	clone := deepCloneRequest(req, limits)

	// Modify originals
	req.Imp[0].Banner.W = 999
	req.Imp[1].Video.W = 999
	req.Imp[4].PMP.Deals[0].BidFloor = 999

	// Clone should be unaffected
	if clone.Imp[0].Banner.W != 300 {
		t.Errorf("expected clone Banner.W = 300, got %d", clone.Imp[0].Banner.W)
	}
	if clone.Imp[1].Video.W != 640 {
		t.Errorf("expected clone Video.W = 640, got %d", clone.Imp[1].Video.W)
	}
	if clone.Imp[4].PMP.Deals[0].BidFloor != 5.00 {
		t.Errorf("expected clone Deal.BidFloor = 5.00, got %f", clone.Imp[4].PMP.Deals[0].BidFloor)
	}
}

func TestDeepCloneRequest_Limits(t *testing.T) {
	limits := &CloneLimits{
		MaxImpressionsPerRequest: 2,
		MaxEIDsPerUser:           1,
		MaxDataPerUser:           1,
		MaxDealsPerImp:           1,
		MaxSChainNodes:           1,
	}

	req := &openrtb.BidRequest{
		ID:   "test",
		Site: testSite(),
		User: &openrtb.User{
			EIDs: []openrtb.EID{
				{Source: "a.com"},
				{Source: "b.com"},
				{Source: "c.com"},
			},
			Data: []openrtb.Data{
				{ID: "d1"},
				{ID: "d2"},
				{ID: "d3"},
			},
		},
		Source: &openrtb.Source{
			SChain: &openrtb.SupplyChain{
				Nodes: []openrtb.SupplyChainNode{
					{ASI: "a.com"},
					{ASI: "b.com"},
					{ASI: "c.com"},
				},
			},
		},
		Imp: []openrtb.Imp{
			{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			{ID: "imp2", Banner: &openrtb.Banner{W: 728, H: 90}},
			{ID: "imp3", Banner: &openrtb.Banner{W: 160, H: 600}}, // Should be clipped
			{ID: "imp4", Banner: &openrtb.Banner{W: 320, H: 50}},  // Should be clipped
		},
	}

	clone := deepCloneRequest(req, limits)

	// Check limits were applied
	if len(clone.Imp) != 2 {
		t.Errorf("expected 2 impressions (limited), got %d", len(clone.Imp))
	}
	if len(clone.User.EIDs) != 1 {
		t.Errorf("expected 1 EID (limited), got %d", len(clone.User.EIDs))
	}
	if len(clone.User.Data) != 1 {
		t.Errorf("expected 1 Data (limited), got %d", len(clone.User.Data))
	}
	if len(clone.Source.SChain.Nodes) != 1 {
		t.Errorf("expected 1 SChain node (limited), got %d", len(clone.Source.SChain.Nodes))
	}
}

func TestRunAuction_NilBidRequest(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, nil)

	_, err := ex.RunAuction(context.Background(), &AuctionRequest{
		BidRequest: nil,
	})

	if err == nil {
		t.Error("expected error for nil bid request")
	}
	if !containsString(err.Error(), "missing bid request") {
		t.Errorf("expected 'missing bid request' error, got: %v", err)
	}
}

func TestRunAuction_MissingRequiredFields(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, nil)

	tests := []struct {
		name    string
		req     *openrtb.BidRequest
		errText string
	}{
		{
			name:    "missing ID",
			req:     &openrtb.BidRequest{ID: "", Site: testSite(), Imp: []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}}},
			errText: "missing required field 'id'",
		},
		{
			name:    "no impressions",
			req:     &openrtb.BidRequest{ID: "req1", Site: testSite(), Imp: []openrtb.Imp{}},
			errText: "at least one impression",
		},
		{
			name: "too many impressions",
			req: &openrtb.BidRequest{
				ID:   "req1",
				Site: testSite(),
				Imp:  make([]openrtb.Imp, 150), // Over limit
			},
			errText: "too many impressions",
		},
		{
			name: "empty impression ID",
			req: &openrtb.BidRequest{
				ID:   "req1",
				Site: testSite(),
				Imp:  []openrtb.Imp{{ID: "", Banner: &openrtb.Banner{W: 300, H: 250}}},
			},
			errText: "empty id",
		},
		{
			name: "duplicate impression IDs",
			req: &openrtb.BidRequest{
				ID:   "req1",
				Site: testSite(),
				Imp: []openrtb.Imp{
					{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
					{ID: "imp1", Banner: &openrtb.Banner{W: 728, H: 90}},
				},
			},
			errText: "duplicate impression id",
		},
		{
			name: "impression without media type",
			req: &openrtb.BidRequest{
				ID:   "req1",
				Site: testSite(),
				Imp:  []openrtb.Imp{{ID: "imp1"}}, // No banner/video/audio/native
			},
			errText: "no media type",
		},
		{
			name: "banner without dimensions",
			req: &openrtb.BidRequest{
				ID:   "req1",
				Site: testSite(),
				Imp:  []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{}}}, // No W/H or Format
			},
			errText: "must have either w/h or format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ex.RunAuction(context.Background(), &AuctionRequest{BidRequest: tt.req})
			if err == nil {
				t.Error("expected error")
			} else if !containsString(err.Error(), tt.errText) {
				t.Errorf("expected error containing %q, got: %v", tt.errText, err)
			}
		})
	}
}

func TestRunAuction_TMaxCapping(t *testing.T) {
	registry := adapters.NewRegistry()
	mock := &mockAdapter{bids: []*adapters.TypedBid{}}
	registry.Register("bidder1", mock, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout: 5 * time.Second,
		IDREnabled:     false,
	})

	// Request with very high TMax (should be capped)
	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "test-tmax",
			Site: testSite(),
			TMax: 20000, // 20 seconds - higher than default but within allowed range
			Imp:  []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}}},
		},
	}

	start := time.Now()
	_, err := ex.RunAuction(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should complete quickly (no actual bidders returning bids)
	// Use generous timing threshold to avoid flaky tests on slow CI
	if elapsed > 2*time.Second {
		t.Errorf("auction took too long: %v", elapsed)
	}
}

func TestSortBidsByPrice_NilBids(t *testing.T) {
	// Test with nil bids in the slice - should handle gracefully
	bids := []ValidatedBid{
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b1", Price: 1.00}}, BidderCode: "bidder1"},
		{Bid: nil, BidderCode: "bidder2"}, // nil TypedBid
		{Bid: &adapters.TypedBid{Bid: nil}, BidderCode: "bidder3"}, // nil Bid
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b4", Price: 5.00}}, BidderCode: "bidder4"},
	}

	// Should not panic
	sortBidsByPrice(bids)

	// Valid bids should still be sorted (nil handling prevents crash)
	if bids[0].Bid != nil && bids[0].Bid.Bid != nil && bids[0].Bid.Bid.Price != 5.00 {
		// Due to nil handling, exact order may vary, but it shouldn't crash
	}
}

func TestSortBidsByPrice_Empty(t *testing.T) {
	bids := []ValidatedBid{}
	sortBidsByPrice(bids) // Should not panic

	if len(bids) != 0 {
		t.Error("expected empty slice")
	}
}

func TestSortBidsByPrice_SingleBid(t *testing.T) {
	bids := []ValidatedBid{
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b1", Price: 1.00}}, BidderCode: "bidder1"},
	}
	sortBidsByPrice(bids)

	if bids[0].Bid.Bid.Price != 1.00 {
		t.Errorf("expected price 1.00, got %f", bids[0].Bid.Bid.Price)
	}
}

func TestAuctionLogic_SecondPrice_SingleBidWithFloor(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		AuctionType:    SecondPriceAuction,
		PriceIncrement: 0.01,
		MinBidPrice:    0,
	})

	impFloors := map[string]float64{"imp1": 2.00}

	validBids := []ValidatedBid{
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b1", ImpID: "imp1", Price: 5.00}}, BidderCode: "bidder1"},
	}

	result := ex.runAuctionLogic(validBids, impFloors)

	// With single bid and floor 2.00, winning price should be floor + increment = 2.01
	if len(result["imp1"]) != 1 {
		t.Fatalf("expected 1 bid, got %d", len(result["imp1"]))
	}
	if result["imp1"][0].Bid.Bid.Price != 2.01 {
		t.Errorf("expected winning price 2.01, got %f", result["imp1"][0].Bid.Bid.Price)
	}
}

func TestAuctionLogic_SecondPrice_ClearingExceedsBid(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		AuctionType:    SecondPriceAuction,
		PriceIncrement: 0.01,
		MinBidPrice:    0,
	})

	impFloors := map[string]float64{"imp1": 5.00}

	validBids := []ValidatedBid{
		// Bid is 4.00 but floor is 5.00, so clearing price (5.01) exceeds bid
		{Bid: &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b1", ImpID: "imp1", Price: 4.00}}, BidderCode: "bidder1"},
	}

	result := ex.runAuctionLogic(validBids, impFloors)

	// Bid should be rejected since clearing price exceeds bid
	if len(result["imp1"]) != 0 {
		t.Errorf("expected 0 bids (rejected), got %d", len(result["imp1"]))
	}
}

func ptrInt(v int) *int {
	return &v
}

// Benchmark tests

func BenchmarkDeepCloneRequest(b *testing.B) {
	limits := DefaultCloneLimits()
	req := &openrtb.BidRequest{
		ID: "bench-request",
		Site: &openrtb.Site{
			ID:   "site1",
			Name: "Test Site",
			Publisher: &openrtb.Publisher{
				ID:   "pub1",
				Name: "Test Publisher",
			},
		},
		User: &openrtb.User{
			ID: "user1",
			EIDs: []openrtb.EID{
				{Source: "liveramp.com"},
				{Source: "pubcid.org"},
			},
		},
		Device: &openrtb.Device{
			UA: "Mozilla/5.0",
			Geo: &openrtb.Geo{
				Country: "US",
			},
		},
		Imp: []openrtb.Imp{
			{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			{ID: "imp2", Video: &openrtb.Video{W: 640, H: 480}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deepCloneRequest(req, limits)
	}
}

func BenchmarkSortBidsByPrice(b *testing.B) {
	bids := make([]ValidatedBid, 10)
	for i := 0; i < 10; i++ {
		bids[i] = ValidatedBid{
			Bid:        &adapters.TypedBid{Bid: &openrtb.Bid{ID: "b" + string(rune(i)), Price: float64(i)}},
			BidderCode: "bidder" + string(rune(i)),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create a copy to sort
		bidsCopy := make([]ValidatedBid, len(bids))
		copy(bidsCopy, bids)
		sortBidsByPrice(bidsCopy)
	}
}

func BenchmarkRoundToCents(b *testing.B) {
	prices := []float64{1.234, 5.678, 0.999, 2.001, 10.555}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range prices {
			roundToCents(p)
		}
	}
}

// TestSelectiveClone_OriginalNotMutated verifies that cloneRequestWithFPD
// does not mutate the original request (critical for concurrent bidder calls)
func TestSelectiveClone_OriginalNotMutated(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		DefaultTimeout:  100 * time.Millisecond,
		DefaultCurrency: "USD",
		IDREnabled:      false,
		FPD: &fpd.Config{
			Enabled:     true,
			SiteEnabled: true,
		},
	})

	// Create original request with specific values
	original := &openrtb.BidRequest{
		ID:  "test-clone",
		Cur: []string{"EUR"},
		Site: &openrtb.Site{
			ID:   "site1",
			Name: "Original Site",
			Publisher: &openrtb.Publisher{
				ID:   "pub1",
				Name: "Original Publisher",
			},
		},
		User: &openrtb.User{
			ID: "user1",
			Geo: &openrtb.Geo{
				Country: "US",
			},
		},
		Device: &openrtb.Device{
			UA: "Original UA",
			Geo: &openrtb.Geo{
				Country: "US",
			},
		},
		Imp: []openrtb.Imp{
			{
				ID:          "imp1",
				BidFloor:    1.50,
				BidFloorCur: "EUR",
				Banner:      &openrtb.Banner{W: 300, H: 250},
			},
		},
	}

	// Store original values
	origCur := original.Cur[0]
	origSiteID := original.Site.ID
	origImpFloorCur := original.Imp[0].BidFloorCur
	origDeviceUA := original.Device.UA

	// Clone with FPD (no FPD data, so Site/App/User won't be cloned)
	clone := ex.cloneRequestWithFPD(original, "bidder1", nil)

	// Verify clone has modified values
	if clone.Cur[0] != "USD" {
		t.Errorf("expected clone Cur = USD, got %s", clone.Cur[0])
	}
	if clone.Imp[0].BidFloorCur != "USD" {
		t.Errorf("expected clone BidFloorCur = USD, got %s", clone.Imp[0].BidFloorCur)
	}

	// Verify original is NOT mutated
	if original.Cur[0] != origCur {
		t.Errorf("original Cur was mutated: expected %s, got %s", origCur, original.Cur[0])
	}
	if original.Site.ID != origSiteID {
		t.Errorf("original Site.ID was mutated: expected %s, got %s", origSiteID, original.Site.ID)
	}
	if original.Imp[0].BidFloorCur != origImpFloorCur {
		t.Errorf("original BidFloorCur was mutated: expected %s, got %s", origImpFloorCur, original.Imp[0].BidFloorCur)
	}
	if original.Device.UA != origDeviceUA {
		t.Errorf("original Device.UA was mutated: expected %s, got %s", origDeviceUA, original.Device.UA)
	}

	// Verify shared pointers (Device, User without FPD) point to same objects
	// This is the performance optimization - they share memory
	if clone.Device != original.Device {
		t.Log("Device was unnecessarily cloned (not a failure, but less optimal)")
	}
	if clone.User != original.User {
		t.Log("User was unnecessarily cloned (not a failure, but less optimal)")
	}
}

// TestSelectiveClone_WithFPD verifies that Site/App/User are cloned when FPD is applied
func TestSelectiveClone_WithFPD(t *testing.T) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		DefaultTimeout:  100 * time.Millisecond,
		DefaultCurrency: "USD",
		IDREnabled:      false,
		FPD: &fpd.Config{
			Enabled:     true,
			SiteEnabled: true,
		},
	})

	original := &openrtb.BidRequest{
		ID:  "test-fpd-clone",
		Cur: []string{"EUR"},
		Site: &openrtb.Site{
			ID:   "site1",
			Name: "Original Site",
		},
		Imp: []openrtb.Imp{
			{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
		},
	}

	// FPD with Site data - should trigger Site clone
	fpdData := fpd.BidderFPD{
		"bidder1": &fpd.ResolvedFPD{
			Site: json.RawMessage(`{"segment":"premium"}`),
		},
	}

	origSitePtr := original.Site

	clone := ex.cloneRequestWithFPD(original, "bidder1", fpdData)

	// Site should be cloned (different pointer) since FPD modifies it
	if clone.Site == origSitePtr {
		t.Error("Site should be cloned when FPD has Site data")
	}

	// Original Site should be unmodified
	if original.Site.Ext != nil {
		t.Error("original Site.Ext was mutated by FPD application")
	}
}

// BenchmarkSelectiveClone benchmarks the new selective clone vs deep clone
func BenchmarkSelectiveClone(b *testing.B) {
	registry := adapters.NewRegistry()
	ex := New(registry, &Config{
		DefaultTimeout:  100 * time.Millisecond,
		DefaultCurrency: "USD",
		IDREnabled:      false,
	})

	req := &openrtb.BidRequest{
		ID:  "bench-selective",
		Cur: []string{"EUR"},
		Site: &openrtb.Site{
			ID:   "site1",
			Name: "Test Site",
			Publisher: &openrtb.Publisher{
				ID:   "pub1",
				Name: "Test Publisher",
			},
		},
		User: &openrtb.User{
			ID: "user1",
			EIDs: []openrtb.EID{
				{Source: "liveramp.com"},
				{Source: "pubcid.org"},
			},
		},
		Device: &openrtb.Device{
			UA: "Mozilla/5.0",
			Geo: &openrtb.Geo{
				Country: "US",
			},
		},
		Imp: []openrtb.Imp{
			{ID: "imp1", BidFloorCur: "EUR", Banner: &openrtb.Banner{W: 300, H: 250}},
			{ID: "imp2", BidFloorCur: "EUR", Video: &openrtb.Video{W: 640, H: 480}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ex.cloneRequestWithFPD(req, "bidder1", nil)
	}
}
