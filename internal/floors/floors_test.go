package floors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

func TestEnforcer_EnrichRequest(t *testing.T) {
	// Create static provider with rules
	provider := NewStaticProvider()
	provider.SetRules("pub123", "example.com", []FloorRule{
		{MediaType: "banner", Size: "300x250", Floor: 0.50},
		{MediaType: "banner", Floor: 0.30},
		{MediaType: "video", Floor: 1.00},
	}, 0.10, "USD")

	enforcer := NewEnforcer(DefaultConfig(), provider)

	tests := []struct {
		name          string
		req           *openrtb.BidRequest
		expectedFloor float64
	}{
		{
			name: "banner 300x250 matches specific rule",
			req: &openrtb.BidRequest{
				Site: &openrtb.Site{
					Domain:    "example.com",
					Publisher: &openrtb.Publisher{ID: "pub123"},
				},
				Imp: []openrtb.Imp{
					{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
				},
			},
			expectedFloor: 0.50,
		},
		{
			name: "banner other size matches generic banner rule",
			req: &openrtb.BidRequest{
				Site: &openrtb.Site{
					Domain:    "example.com",
					Publisher: &openrtb.Publisher{ID: "pub123"},
				},
				Imp: []openrtb.Imp{
					{ID: "imp1", Banner: &openrtb.Banner{W: 728, H: 90}},
				},
			},
			expectedFloor: 0.30,
		},
		{
			name: "video matches video rule",
			req: &openrtb.BidRequest{
				Site: &openrtb.Site{
					Domain:    "example.com",
					Publisher: &openrtb.Publisher{ID: "pub123"},
				},
				Imp: []openrtb.Imp{
					{ID: "imp1", Video: &openrtb.Video{W: 640, H: 480}},
				},
			},
			expectedFloor: 1.00,
		},
		{
			name: "native matches default floor",
			req: &openrtb.BidRequest{
				Site: &openrtb.Site{
					Domain:    "example.com",
					Publisher: &openrtb.Publisher{ID: "pub123"},
				},
				Imp: []openrtb.Imp{
					{ID: "imp1", Native: &openrtb.Native{}},
				},
			},
			expectedFloor: 0.10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := enforcer.EnrichRequest(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("EnrichRequest failed: %v", err)
			}

			if tt.req.Imp[0].BidFloor != tt.expectedFloor {
				t.Errorf("expected floor %f, got %f", tt.expectedFloor, tt.req.Imp[0].BidFloor)
			}
		})
	}
}

func TestEnforcer_ValidateBid(t *testing.T) {
	enforcer := NewEnforcer(DefaultConfig())

	tests := []struct {
		name        string
		bid         *openrtb.Bid
		imp         *openrtb.Imp
		bidCurrency string
		wantValid   bool
	}{
		{
			name:        "bid above floor is valid",
			bid:         &openrtb.Bid{Price: 1.50},
			imp:         &openrtb.Imp{BidFloor: 1.00, BidFloorCur: "USD"},
			bidCurrency: "USD",
			wantValid:   true,
		},
		{
			name:        "bid at floor is valid",
			bid:         &openrtb.Bid{Price: 1.00},
			imp:         &openrtb.Imp{BidFloor: 1.00, BidFloorCur: "USD"},
			bidCurrency: "USD",
			wantValid:   true,
		},
		{
			name:        "bid below floor is invalid",
			bid:         &openrtb.Bid{Price: 0.50},
			imp:         &openrtb.Imp{BidFloor: 1.00, BidFloorCur: "USD"},
			bidCurrency: "USD",
			wantValid:   false,
		},
		{
			name:        "no floor means bid is valid",
			bid:         &openrtb.Bid{Price: 0.01},
			imp:         &openrtb.Imp{},
			bidCurrency: "USD",
			wantValid:   true,
		},
		{
			name:        "zero floor means bid is valid",
			bid:         &openrtb.Bid{Price: 0.01},
			imp:         &openrtb.Imp{BidFloor: 0},
			bidCurrency: "USD",
			wantValid:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := enforcer.ValidateBid(tt.bid, tt.imp, tt.bidCurrency)
			if valid != tt.wantValid {
				t.Errorf("expected valid=%v, got valid=%v", tt.wantValid, valid)
			}
		})
	}
}

func TestEnforcer_SoftFloorMode(t *testing.T) {
	config := DefaultConfig()
	config.EnforceFloors = false // Soft floor mode

	enforcer := NewEnforcer(config)

	bid := &openrtb.Bid{Price: 0.50}
	imp := &openrtb.Imp{BidFloor: 1.00, BidFloorCur: "USD"}

	// In soft mode, bid below floor should still be valid
	valid, _ := enforcer.ValidateBid(bid, imp, "USD")
	if !valid {
		t.Error("expected bid to be valid in soft floor mode")
	}
}

func TestEnforcer_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false

	enforcer := NewEnforcer(config)

	// Should not enrich when disabled
	req := &openrtb.BidRequest{
		Site: &openrtb.Site{Domain: "example.com"},
		Imp:  []openrtb.Imp{{ID: "imp1", Banner: &openrtb.Banner{}}},
	}

	err := enforcer.EnrichRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("EnrichRequest failed: %v", err)
	}

	if req.Imp[0].BidFloor != 0 {
		t.Error("expected no floor when enforcer is disabled")
	}

	// Should accept any bid when disabled
	bid := &openrtb.Bid{Price: 0.001}
	imp := &openrtb.Imp{BidFloor: 100.00}
	valid, _ := enforcer.ValidateBid(bid, imp, "USD")
	if !valid {
		t.Error("expected bid to be valid when enforcer is disabled")
	}
}

func TestStaticProvider(t *testing.T) {
	provider := NewStaticProvider()

	// Set rules for specific publisher/domain
	provider.SetRules("pub1", "site1.com", []FloorRule{
		{Floor: 0.50},
	}, 0.10, "USD")

	// Set rules for publisher-wide
	provider.SetRules("pub1", "", []FloorRule{
		{Floor: 0.30},
	}, 0.05, "USD")

	// Set global default
	provider.SetRules("", "", []FloorRule{
		{Floor: 0.01},
	}, 0.00, "USD")

	tests := []struct {
		name         string
		req          *openrtb.BidRequest
		expectFloor  float64
		expectNil    bool
	}{
		{
			name: "exact match",
			req: &openrtb.BidRequest{
				Site: &openrtb.Site{
					Domain:    "site1.com",
					Publisher: &openrtb.Publisher{ID: "pub1"},
				},
			},
			expectFloor: 0.50,
		},
		{
			name: "publisher-wide match",
			req: &openrtb.BidRequest{
				Site: &openrtb.Site{
					Domain:    "other.com",
					Publisher: &openrtb.Publisher{ID: "pub1"},
				},
			},
			expectFloor: 0.30,
		},
		{
			name: "global default",
			req: &openrtb.BidRequest{
				Site: &openrtb.Site{
					Domain:    "unknown.com",
					Publisher: &openrtb.Publisher{ID: "unknown"},
				},
			},
			expectFloor: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := provider.GetFloors(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("GetFloors failed: %v", err)
			}

			if tt.expectNil {
				if data != nil {
					t.Error("expected nil floor data")
				}
				return
			}

			if data == nil {
				t.Fatal("expected floor data, got nil")
			}

			if len(data.Rules) == 0 {
				t.Fatal("expected rules")
			}

			if data.Rules[0].Floor != tt.expectFloor {
				t.Errorf("expected floor %f, got %f", tt.expectFloor, data.Rules[0].Floor)
			}
		})
	}
}

func TestAPIProvider(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("expected API key header")
		}

		response := FloorData{
			Rules: []FloorRule{
				{Floor: 0.75, MediaType: "banner"},
			},
			DefaultFloor: 0.25,
			Currency:     "USD",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewAPIProvider(&APIProviderConfig{
		Endpoint: server.URL + "/floors/{{publisher_id}}/{{domain}}",
		APIKey:   "test-key",
		Timeout:  1 * time.Second,
		CacheTTL: 1 * time.Minute,
	})

	req := &openrtb.BidRequest{
		Site: &openrtb.Site{
			Domain:    "example.com",
			Publisher: &openrtb.Publisher{ID: "pub123"},
		},
	}

	data, err := provider.GetFloors(context.Background(), req)
	if err != nil {
		t.Fatalf("GetFloors failed: %v", err)
	}

	if data == nil {
		t.Fatal("expected floor data")
	}

	if len(data.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(data.Rules))
	}

	if data.Rules[0].Floor != 0.75 {
		t.Errorf("expected floor 0.75, got %f", data.Rules[0].Floor)
	}

	if data.DefaultFloor != 0.25 {
		t.Errorf("expected default floor 0.25, got %f", data.DefaultFloor)
	}
}

func TestAPIProvider_Caching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		response := FloorData{
			Rules: []FloorRule{{Floor: 1.00}},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewAPIProvider(&APIProviderConfig{
		Endpoint: server.URL,
		CacheTTL: 1 * time.Hour, // Long TTL for test
	})

	req := &openrtb.BidRequest{
		Site: &openrtb.Site{Domain: "test.com"},
	}

	// First call should hit the server
	_, err := provider.GetFloors(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call should use cache
	_, err = provider.GetFloors(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 call (cached), got %d", callCount)
	}

	// Clear cache and call again
	provider.ClearCache()
	_, err = provider.GetFloors(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls after cache clear, got %d", callCount)
	}
}

func TestFloorRuleMatching(t *testing.T) {
	provider := NewStaticProvider()
	provider.SetFloors("pub1", "example.com", &FloorData{
		Rules: []FloorRule{
			// Most specific - publisher + domain + media + size
			{PublisherID: "pub1", Domain: "example.com", MediaType: "banner", Size: "300x250", Floor: 2.00},
			// Publisher + domain + media
			{PublisherID: "pub1", Domain: "example.com", MediaType: "banner", Floor: 1.00},
			// Publisher + domain only
			{PublisherID: "pub1", Domain: "example.com", Floor: 0.50},
		},
		Currency: "USD",
	})

	enforcer := NewEnforcer(DefaultConfig(), provider)

	tests := []struct {
		name          string
		imp           openrtb.Imp
		expectedFloor float64
	}{
		{
			name:          "matches most specific rule",
			imp:           openrtb.Imp{ID: "1", Banner: &openrtb.Banner{W: 300, H: 250}},
			expectedFloor: 2.00,
		},
		{
			name:          "matches media type rule",
			imp:           openrtb.Imp{ID: "2", Banner: &openrtb.Banner{W: 728, H: 90}},
			expectedFloor: 1.00,
		},
		{
			name:          "matches generic rule",
			imp:           openrtb.Imp{ID: "3", Video: &openrtb.Video{W: 640, H: 480}},
			expectedFloor: 0.50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &openrtb.BidRequest{
				Site: &openrtb.Site{
					Domain:    "example.com",
					Publisher: &openrtb.Publisher{ID: "pub1"},
				},
				Imp: []openrtb.Imp{tt.imp},
			}

			err := enforcer.EnrichRequest(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}

			if req.Imp[0].BidFloor != tt.expectedFloor {
				t.Errorf("expected floor %f, got %f", tt.expectedFloor, req.Imp[0].BidFloor)
			}
		})
	}
}

func TestEnforcer_ExistingFloorNotOverwritten(t *testing.T) {
	provider := NewStaticProvider()
	provider.SetRules("pub1", "", []FloorRule{{Floor: 0.50}}, 0, "USD")

	enforcer := NewEnforcer(DefaultConfig(), provider)

	req := &openrtb.BidRequest{
		Site: &openrtb.Site{Publisher: &openrtb.Publisher{ID: "pub1"}},
		Imp: []openrtb.Imp{
			{ID: "1", BidFloor: 1.00, BidFloorCur: "USD", Banner: &openrtb.Banner{}},
		},
	}

	err := enforcer.EnrichRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	// Existing floor should NOT be overwritten
	if req.Imp[0].BidFloor != 1.00 {
		t.Errorf("expected existing floor 1.00 to be preserved, got %f", req.Imp[0].BidFloor)
	}
}

func TestPubXProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := FloorData{
			Rules: []FloorRule{{Floor: 0.80}},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewPubXProvider(&PubXConfig{
		AccountID: "acc123",
		APIKey:    "key123",
		BaseURL:   server.URL,
	})

	if provider.Name() != "pubx" {
		t.Errorf("expected name 'pubx', got '%s'", provider.Name())
	}

	req := &openrtb.BidRequest{
		Site: &openrtb.Site{Domain: "test.com"},
	}

	data, err := provider.GetFloors(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if data == nil || len(data.Rules) == 0 {
		t.Fatal("expected floor data")
	}

	if data.Rules[0].Floor != 0.80 {
		t.Errorf("expected floor 0.80, got %f", data.Rules[0].Floor)
	}
}

func TestDeviceTypeMapping(t *testing.T) {
	tests := []struct {
		deviceType int
		expected   string
	}{
		{1, "mobile"},
		{2, "desktop"},
		{3, "ctv"},
		{4, "phone"},
		{5, "tablet"},
		{6, "connected_device"},
		{7, "set_top_box"},
		{0, ""},
		{99, ""},
	}

	for _, tt := range tests {
		result := mapDeviceType(tt.deviceType)
		if result != tt.expected {
			t.Errorf("mapDeviceType(%d) = %s, expected %s", tt.deviceType, result, tt.expected)
		}
	}
}

func TestFloorCache(t *testing.T) {
	cache := newFloorCache(100 * time.Millisecond)

	data := &FloorData{
		Rules: []FloorRule{{Floor: 1.00}},
	}

	// Set and get
	cache.set("key1", data, 0)
	got := cache.get("key1")
	if got == nil {
		t.Fatal("expected cached data")
	}

	// Get non-existent
	got = cache.get("nonexistent")
	if got != nil {
		t.Error("expected nil for non-existent key")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)
	got = cache.get("key1")
	if got != nil {
		t.Error("expected nil after expiration")
	}

	// Clear cache
	cache.set("key2", data, 1*time.Hour)
	cache.clear()
	got = cache.get("key2")
	if got != nil {
		t.Error("expected nil after clear")
	}
}
