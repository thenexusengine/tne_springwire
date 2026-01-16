package exchange

import (
	"context"
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/fpd"
	"github.com/thenexusengine/tne_springwire/internal/middleware"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
	"github.com/thenexusengine/tne_springwire/pkg/idr"
)

// Mock publisher for testing bid multiplier extraction
type mockPublisherWithMultiplier struct {
	PublisherID   string
	BidMultiplier float64
}

func (m *mockPublisherWithMultiplier) GetPublisherID() string {
	return m.PublisherID
}

func (m *mockPublisherWithMultiplier) GetBidMultiplier() float64 {
	return m.BidMultiplier
}

func (m *mockPublisherWithMultiplier) GetAllowedDomains() string {
	return "example.com"
}

// TestGetDemandType_NotFound tests demand type for unknown bidders
func TestGetDemandType_NotFound(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	// Unknown bidder should default to platform
	demandType := exchange.getDemandType("unknown-bidder")
	if demandType != adapters.DemandTypePlatform {
		t.Errorf("Expected DemandTypePlatform for unknown bidder, got %v", demandType)
	}
}

// TestBuildImpFloorMap_NoPublisher tests floor map building without publisher
func TestBuildImpFloorMap_NoPublisher(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	req := &openrtb.BidRequest{
		ID: "test-request",
		Imp: []openrtb.Imp{
			{ID: "imp1", BidFloor: 1.0},
			{ID: "imp2", BidFloor: 2.5},
			{ID: "imp3", BidFloor: 0.0},
		},
	}

	ctx := context.Background()
	floorMap := exchange.buildImpFloorMap(ctx, req)

	if floorMap["imp1"] != 1.0 {
		t.Errorf("Expected floor 1.0 for imp1, got %f", floorMap["imp1"])
	}
	if floorMap["imp2"] != 2.5 {
		t.Errorf("Expected floor 2.5 for imp2, got %f", floorMap["imp2"])
	}
	if floorMap["imp3"] != 0.0 {
		t.Errorf("Expected floor 0.0 for imp3, got %f", floorMap["imp3"])
	}
}

// TestExtractBidMultiplier_Interface tests multiplier extraction via interface
func TestExtractBidMultiplier_Interface(t *testing.T) {
	pub := &mockPublisherWithMultiplier{
		BidMultiplier: 1.05,
	}

	multiplier, ok := extractBidMultiplier(pub)
	if !ok {
		t.Error("Expected to extract bid multiplier")
	}
	if multiplier != 1.05 {
		t.Errorf("Expected 1.05, got %f", multiplier)
	}
}

// TestExtractBidMultiplier_NotFound tests multiplier extraction when not present
func TestExtractBidMultiplier_NotFound(t *testing.T) {
	type noBidMultiplier struct {
		SomeField string
	}
	obj := &noBidMultiplier{SomeField: "value"}

	_, ok := extractBidMultiplier(obj)
	if ok {
		t.Error("Expected not to extract bid multiplier from object without field")
	}
}

// TestExtractPublisherID_Interface tests publisher ID extraction
func TestExtractPublisherID_Interface(t *testing.T) {
	pub := &mockPublisherWithMultiplier{
		PublisherID: "pub-123",
	}

	id, ok := extractPublisherID(pub)
	if !ok {
		t.Error("Expected to extract publisher ID")
	}
	if id != "pub-123" {
		t.Errorf("Expected 'pub-123', got '%s'", id)
	}
}

// TestExtractPublisherID_EmptyID tests publisher ID extraction with empty ID
func TestExtractPublisherID_EmptyID(t *testing.T) {
	pub := &mockPublisherWithMultiplier{
		PublisherID: "",
	}

	_, ok := extractPublisherID(pub)
	if ok {
		t.Error("Expected not to extract empty publisher ID")
	}
}

// TestExtractPublisherID_NotFound tests publisher ID extraction when not present
func TestExtractPublisherID_NotFound(t *testing.T) {
	type noPublisherID struct {
		SomeField string
	}
	obj := &noPublisherID{SomeField: "value"}

	_, ok := extractPublisherID(obj)
	if ok {
		t.Error("Expected not to extract publisher ID from object without field")
	}
}

// TestBuildBidExtension_PlatformDemand tests bid extension for platform demand
func TestBuildBidExtension_PlatformDemand(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	vb := ValidatedBid{
		Bid: &adapters.TypedBid{
			Bid: &openrtb.Bid{
				ID:     "bid1",
				ImpID:  "imp1",
				Price:  2.5,
				W:      300,
				H:      250,
				DealID: "deal123",
			},
			BidType: adapters.BidTypeBanner,
		},
		BidderCode: "appnexus",
		DemandType: adapters.DemandTypePlatform,
	}

	ext := exchange.buildBidExtension(vb)

	if ext.Prebid == nil {
		t.Fatal("Expected non-nil Prebid extension")
	}

	// Should use "thenexusengine" for platform demand
	if ext.Prebid.Targeting["hb_bidder"] != "thenexusengine" {
		t.Errorf("Expected hb_bidder 'thenexusengine', got '%s'", ext.Prebid.Targeting["hb_bidder"])
	}

	// Should include deal ID
	if ext.Prebid.Targeting["hb_deal"] != "deal123" {
		t.Errorf("Expected hb_deal 'deal123', got '%s'", ext.Prebid.Targeting["hb_deal"])
	}

	// Should include size
	if ext.Prebid.Targeting["hb_size"] != "300x250" {
		t.Errorf("Expected hb_size '300x250', got '%s'", ext.Prebid.Targeting["hb_size"])
	}
}

// TestBuildBidExtension_PublisherDemand tests bid extension for publisher demand
func TestBuildBidExtension_PublisherDemand(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	vb := ValidatedBid{
		Bid: &adapters.TypedBid{
			Bid: &openrtb.Bid{
				ID:    "bid1",
				ImpID: "imp1",
				Price: 2.5,
				W:     728,
				H:     90,
			},
			BidType: adapters.BidTypeBanner,
		},
		BidderCode: "rubicon",
		DemandType: adapters.DemandTypePublisher,
	}

	ext := exchange.buildBidExtension(vb)

	if ext.Prebid == nil {
		t.Fatal("Expected non-nil Prebid extension")
	}

	// Should use original bidder code for publisher demand
	if ext.Prebid.Targeting["hb_bidder"] != "rubicon" {
		t.Errorf("Expected hb_bidder 'rubicon', got '%s'", ext.Prebid.Targeting["hb_bidder"])
	}

	// Should include size
	if ext.Prebid.Targeting["hb_size"] != "728x90" {
		t.Errorf("Expected hb_size '728x90', got '%s'", ext.Prebid.Targeting["hb_size"])
	}
}

// TestBuildBidExtension_VideoType tests bid extension for video
func TestBuildBidExtension_VideoType(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	vb := ValidatedBid{
		Bid: &adapters.TypedBid{
			Bid: &openrtb.Bid{
				ID:    "bid1",
				ImpID: "imp1",
				Price: 5.0,
				W:     640,
				H:     480,
			},
			BidType: adapters.BidTypeVideo,
		},
		BidderCode: "appnexus",
		DemandType: adapters.DemandTypePlatform,
	}

	ext := exchange.buildBidExtension(vb)

	if ext.Prebid == nil {
		t.Fatal("Expected non-nil Prebid extension")
	}

	if ext.Prebid.Type != "video" {
		t.Errorf("Expected type 'video', got '%s'", ext.Prebid.Type)
	}

	if ext.Prebid.Meta.MediaType != "video" {
		t.Errorf("Expected media_type 'video', got '%s'", ext.Prebid.Meta.MediaType)
	}
}

// TestSetMetrics tests setting metrics recorder
func TestSetMetrics(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	// Create mock metrics recorder
	metrics := &mockMetricsRecorder{}

	exchange.SetMetrics(metrics)

	// Verify it was set by checking it's not nil (we can't access private field directly)
	if exchange.metrics == nil {
		t.Error("Expected metrics to be set")
	}
}

// TestClose_WithEventRecorder tests Close with event recorder
func TestClose_WithEventRecorder(t *testing.T) {
	registry := adapters.NewRegistry()

	// Create real event recorder with empty URL (will work for Close)
	eventRecorder := idr.NewEventRecorder("", 10)

	exchange := New(registry, nil)
	exchange.eventRecorder = eventRecorder

	err := exchange.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// TestClose_NoEventRecorder tests Close without event recorder
func TestClose_NoEventRecorder(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)
	exchange.eventRecorder = nil

	err := exchange.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

// TestBuildImpFloorMap_MultipleImpressions tests floor map with multiple impressions
func TestBuildImpFloorMap_MultipleImpressions(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	req := &openrtb.BidRequest{
		ID: "test-request",
		Imp: []openrtb.Imp{
			{ID: "imp1", BidFloor: 1.0},
			{ID: "imp2", BidFloor: 2.5},
			{ID: "imp3", BidFloor: 0.5},
		},
	}

	ctx := context.Background()
	floorMap := exchange.buildImpFloorMap(ctx, req)

	// Should have all impression IDs in the map
	if len(floorMap) != 3 {
		t.Errorf("Expected 3 floors in map, got %d", len(floorMap))
	}

	// Should preserve original floors when no multiplier
	if floorMap["imp1"] != 1.0 {
		t.Errorf("Expected floor 1.0 for imp1, got %f", floorMap["imp1"])
	}
	if floorMap["imp2"] != 2.5 {
		t.Errorf("Expected floor 2.5 for imp2, got %f", floorMap["imp2"])
	}
	if floorMap["imp3"] != 0.5 {
		t.Errorf("Expected floor 0.5 for imp3, got %f", floorMap["imp3"])
	}
}

// Mock implementations for testing

// TestFormatPriceBucket tests price bucket formatting
func TestFormatPriceBucket(t *testing.T) {
	tests := []struct {
		price    float64
		expected string
	}{
		{0.0, "0.00"},
		{-1.0, "0.00"},
		{0.55, "0.55"},
		{1.23, "1.23"},
		{4.99, "4.99"},
		{5.00, "5.00"},
		{5.10, "5.10"},
		{5.67, "5.65"},
		{7.89, "7.85"},
		{10.00, "10.00"},
		{10.25, "10.00"},
		{12.75, "12.50"},
		{15.99, "15.50"},
		{20.00, "20.00"},
		{25.00, "20.00"},
		{100.00, "20.00"},
	}

	for _, tt := range tests {
		result := formatPriceBucket(tt.price)
		if result != tt.expected {
			t.Errorf("formatPriceBucket(%f) = %s, expected %s", tt.price, result, tt.expected)
		}
	}
}

// TestBuildMinimalIDRRequest_Site tests minimal request building from site
func TestBuildMinimalIDRRequest_Site(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	req := &openrtb.BidRequest{
		ID: "test-request-123",
		Site: &openrtb.Site{
			Domain: "example.com",
			Cat:    []string{"IAB1", "IAB2"},
			Publisher: &openrtb.Publisher{
				ID: "pub-123",
			},
		},
		Device: &openrtb.Device{
			DeviceType: 1, // mobile
			Geo: &openrtb.Geo{
				Country: "US",
				Region:  "CA",
			},
		},
		Imp: []openrtb.Imp{
			{
				ID: "imp1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
					Format: []openrtb.Format{
						{W: 728, H: 90},
					},
				},
			},
		},
	}

	minimalReq := exchange.buildMinimalIDRRequest(req)

	if minimalReq == nil {
		t.Fatal("Expected non-nil minimal request")
	}

	// Verify basic fields are extracted
	if minimalReq.ID != "test-request-123" {
		t.Errorf("Expected request ID 'test-request-123', got '%s'", minimalReq.ID)
	}

	if minimalReq.Site == nil {
		t.Fatal("Expected non-nil Site")
	}

	if minimalReq.Site.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com', got '%s'", minimalReq.Site.Domain)
	}

	if minimalReq.Site.Publisher != "pub-123" {
		t.Errorf("Expected publisher 'pub-123', got '%s'", minimalReq.Site.Publisher)
	}

	if len(minimalReq.Site.Categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(minimalReq.Site.Categories))
	}

	if minimalReq.Geo == nil {
		t.Fatal("Expected non-nil Geo")
	}

	if minimalReq.Geo.Country != "US" {
		t.Errorf("Expected country 'US', got '%s'", minimalReq.Geo.Country)
	}

	if minimalReq.DeviceType != "mobile" {
		t.Errorf("Expected device type 'mobile', got '%s'", minimalReq.DeviceType)
	}

	if len(minimalReq.Imp) != 1 {
		t.Fatalf("Expected 1 impression, got %d", len(minimalReq.Imp))
	}

	if minimalReq.Imp[0].ID != "imp1" {
		t.Errorf("Expected impression ID 'imp1', got '%s'", minimalReq.Imp[0].ID)
	}
}

// TestBuildMinimalIDRRequest_App tests minimal request building from app
func TestBuildMinimalIDRRequest_App(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	req := &openrtb.BidRequest{
		ID: "app-request",
		App: &openrtb.App{
			Bundle: "com.example.app",
			Cat:    []string{"IAB1-1"},
			Publisher: &openrtb.Publisher{
				ID: "app-pub-456",
			},
		},
		User: &openrtb.User{
			Geo: &openrtb.Geo{
				Country: "GB",
				Region:  "LND",
			},
		},
		Device: &openrtb.Device{
			DeviceType: 4, // phone
		},
		Imp: []openrtb.Imp{
			{
				ID: "imp-video",
				Video: &openrtb.Video{
					W: 640,
					H: 480,
				},
			},
		},
	}

	minimalReq := exchange.buildMinimalIDRRequest(req)

	if minimalReq.App == nil {
		t.Fatal("Expected non-nil App")
	}

	if minimalReq.Site != nil {
		t.Error("Expected nil Site for app request")
	}

	if minimalReq.App.Bundle != "com.example.app" {
		t.Errorf("Expected app bundle 'com.example.app', got '%s'", minimalReq.App.Bundle)
	}

	if minimalReq.App.Publisher != "app-pub-456" {
		t.Errorf("Expected publisher 'app-pub-456', got '%s'", minimalReq.App.Publisher)
	}

	if minimalReq.Geo == nil {
		t.Fatal("Expected non-nil Geo")
	}

	if minimalReq.Geo.Country != "GB" {
		t.Errorf("Expected country 'GB', got '%s'", minimalReq.Geo.Country)
	}

	if minimalReq.DeviceType != "phone" {
		t.Errorf("Expected device type 'phone', got '%s'", minimalReq.DeviceType)
	}
}

// TestBuildMinimalIDRRequest_MultipleMediaTypes tests multiple media types
func TestBuildMinimalIDRRequest_MultipleMediaTypes(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	req := &openrtb.BidRequest{
		ID: "multi-media",
		Site: &openrtb.Site{
			Domain: "test.com",
		},
		Imp: []openrtb.Imp{
			{
				ID: "imp-all",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
				Video: &openrtb.Video{
					W: 640,
					H: 480,
				},
				Native: &openrtb.Native{},
				Audio:  &openrtb.Audio{},
			},
		},
	}

	minimalReq := exchange.buildMinimalIDRRequest(req)

	if len(minimalReq.Imp) != 1 {
		t.Fatalf("Expected 1 impression, got %d", len(minimalReq.Imp))
	}

	imp := minimalReq.Imp[0]
	if len(imp.MediaTypes) != 4 {
		t.Errorf("Expected 4 media types, got %d", len(imp.MediaTypes))
	}
}

// TestBuildMinimalIDRRequest_DeviceTypes tests all device type mappings
func TestBuildMinimalIDRRequest_DeviceTypes(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	deviceTests := []struct {
		deviceType int
		expected   string
	}{
		{1, "mobile"},
		{2, "pc"},
		{3, "ctv"},
		{4, "phone"},
		{5, "tablet"},
		{6, "connected_device"},
		{7, "set_top_box"},
		{0, ""},
		{99, ""},
	}

	for _, tt := range deviceTests {
		req := &openrtb.BidRequest{
			ID: "device-test",
			Site: &openrtb.Site{
				Domain: "test.com",
			},
			Device: &openrtb.Device{
				DeviceType: tt.deviceType,
			},
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			},
		}

		minimalReq := exchange.buildMinimalIDRRequest(req)

		if minimalReq.DeviceType != tt.expected {
			t.Errorf("Device type %d: expected '%s', got '%s'", tt.deviceType, tt.expected, minimalReq.DeviceType)
		}
	}
}

// TestBuildMinimalIDRRequest_NoGeo tests request without geo information
func TestBuildMinimalIDRRequest_NoGeo(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	req := &openrtb.BidRequest{
		ID: "no-geo",
		Site: &openrtb.Site{
			Domain: "test.com",
		},
		Imp: []openrtb.Imp{
			{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
		},
	}

	minimalReq := exchange.buildMinimalIDRRequest(req)

	if minimalReq.Geo != nil {
		if minimalReq.Geo.Country != "" {
			t.Errorf("Expected empty country, got '%s'", minimalReq.Geo.Country)
		}
		if minimalReq.Geo.Region != "" {
			t.Errorf("Expected empty region, got '%s'", minimalReq.Geo.Region)
		}
	}
}

// TestGetDemandType_DefaultBehavior tests getDemandType basic behavior
func TestGetDemandType_DefaultBehavior(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	// Unknown bidder should default to platform
	demandType := exchange.getDemandType("unknown-bidder-xyz")
	if demandType != adapters.DemandTypePlatform {
		t.Errorf("Expected DemandTypePlatform for unknown bidder, got %v", demandType)
	}
}

// TestGetFPDConfig_NoConfig tests getting FPD config when config is nil
func TestGetFPDConfig_NoConfig(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)
	exchange.config = nil

	config := exchange.GetFPDConfig()
	if config != nil {
		t.Error("Expected nil config")
	}
}

// TestGetFPDConfig_WithConfig tests getting FPD config
func TestGetFPDConfig_WithConfig(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	// Set a config with FPD
	fpdConfig := &fpd.Config{
		Enabled: true,
	}
	exchange.config.FPD = fpdConfig

	retrieved := exchange.GetFPDConfig()
	if retrieved == nil {
		t.Fatal("Expected non-nil config")
	}

	if !retrieved.Enabled {
		t.Error("Expected enabled to be true")
	}
}

// Mock implementations for testing

// TestUpdateFPDConfig tests updating FPD configuration
func TestUpdateFPDConfig(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	newConfig := &fpd.Config{
		Enabled: true,
	}

	exchange.UpdateFPDConfig(newConfig)

	retrieved := exchange.GetFPDConfig()
	if retrieved == nil {
		t.Fatal("Expected non-nil config after update")
	}

	if !retrieved.Enabled {
		t.Error("Expected enabled to be true")
	}
}

// TestUpdateFPDConfig_NilConfig tests updating with nil config
func TestUpdateFPDConfig_NilConfig(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	// Store original config
	originalConfig := exchange.GetFPDConfig()

	// Update with nil should be no-op
	exchange.UpdateFPDConfig(nil)

	// Config should remain unchanged
	currentConfig := exchange.GetFPDConfig()
	if originalConfig != currentConfig {
		t.Error("Config should not change when updating with nil")
	}
}

// TestApplyBidMultiplier_NoPublisher tests multiplier with no publisher
func TestApplyBidMultiplier_NoPublisher(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	bidsByImp := map[string][]ValidatedBid{
		"imp1": {
			{
				Bid: &adapters.TypedBid{
					Bid: &openrtb.Bid{
						ID:    "bid1",
						ImpID: "imp1",
						Price: 2.50,
					},
					BidType: adapters.BidTypeBanner,
				},
				BidderCode: "appnexus",
			},
		},
	}

	ctx := context.Background()
	result := exchange.applyBidMultiplier(ctx, bidsByImp)

	// Should return unchanged when no publisher
	if result["imp1"][0].Bid.Bid.Price != 2.50 {
		t.Errorf("Expected price 2.50, got %f", result["imp1"][0].Bid.Bid.Price)
	}
}

// TestApplyBidMultiplier_MultiplierOne tests multiplier of 1.0
func TestApplyBidMultiplier_MultiplierOne(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	pub := &mockPublisherWithMultiplier{
		PublisherID:   "pub-123",
		BidMultiplier: 1.0,
	}

	bidsByImp := map[string][]ValidatedBid{
		"imp1": {
			{
				Bid: &adapters.TypedBid{
					Bid: &openrtb.Bid{
						ID:    "bid1",
						ImpID: "imp1",
						Price: 2.50,
					},
					BidType: adapters.BidTypeBanner,
				},
				BidderCode: "appnexus",
			},
		},
	}

	ctx := middleware.NewContextWithPublisher(context.Background(), pub)
	result := exchange.applyBidMultiplier(ctx, bidsByImp)

	// Should return unchanged with multiplier 1.0
	if result["imp1"][0].Bid.Bid.Price != 2.50 {
		t.Errorf("Expected price 2.50, got %f", result["imp1"][0].Bid.Bid.Price)
	}
}

// TestApplyBidMultiplier_InvalidMultiplier tests out-of-range multiplier
func TestApplyBidMultiplier_InvalidMultiplier(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	tests := []struct {
		name       string
		multiplier float64
	}{
		{"Too low", 0.5},
		{"Too high", 15.0},
		{"Negative", -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &mockPublisherWithMultiplier{
				PublisherID:   "pub-123",
				BidMultiplier: tt.multiplier,
			}

			bidsByImp := map[string][]ValidatedBid{
				"imp1": {
					{
						Bid: &adapters.TypedBid{
							Bid: &openrtb.Bid{
								ID:    "bid1",
								ImpID: "imp1",
								Price: 2.50,
							},
							BidType: adapters.BidTypeBanner,
						},
						BidderCode: "appnexus",
					},
				},
			}

			ctx := middleware.NewContextWithPublisher(context.Background(), pub)
			result := exchange.applyBidMultiplier(ctx, bidsByImp)

			// Should return unchanged with invalid multiplier
			if result["imp1"][0].Bid.Bid.Price != 2.50 {
				t.Errorf("Expected price 2.50 with invalid multiplier %f, got %f", tt.multiplier, result["imp1"][0].Bid.Bid.Price)
			}
		})
	}
}

// TestApplyBidMultiplier_ValidMultiplier tests valid multiplier application
func TestApplyBidMultiplier_ValidMultiplier(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)
	exchange.SetMetrics(&mockMetricsRecorder{})

	pub := &mockPublisherWithMultiplier{
		PublisherID:   "pub-123",
		BidMultiplier: 2.0,
	}

	bidsByImp := map[string][]ValidatedBid{
		"imp1": {
			{
				Bid: &adapters.TypedBid{
					Bid: &openrtb.Bid{
						ID:    "bid1",
						ImpID: "imp1",
						Price: 2.00,
					},
					BidType: adapters.BidTypeBanner,
				},
				BidderCode: "appnexus",
			},
		},
	}

	ctx := middleware.NewContextWithPublisher(context.Background(), pub)
	result := exchange.applyBidMultiplier(ctx, bidsByImp)

	// With multiplier 2.0, price 2.00 becomes 1.00 (divided by 2)
	if result["imp1"][0].Bid.Bid.Price != 1.00 {
		t.Errorf("Expected price 1.00, got %f", result["imp1"][0].Bid.Bid.Price)
	}
}

// TestApplyBidMultiplier_MultipleMediaTypes tests different media types
func TestApplyBidMultiplier_MultipleMediaTypes(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)
	exchange.SetMetrics(&mockMetricsRecorder{})

	pub := &mockPublisherWithMultiplier{
		PublisherID:   "pub-123",
		BidMultiplier: 1.5,
	}

	bidsByImp := map[string][]ValidatedBid{
		"imp-banner": {
			{
				Bid: &adapters.TypedBid{
					Bid:     &openrtb.Bid{ID: "bid1", ImpID: "imp-banner", Price: 3.00},
					BidType: adapters.BidTypeBanner,
				},
				BidderCode: "appnexus",
			},
		},
		"imp-video": {
			{
				Bid: &adapters.TypedBid{
					Bid:     &openrtb.Bid{ID: "bid2", ImpID: "imp-video", Price: 6.00},
					BidType: adapters.BidTypeVideo,
				},
				BidderCode: "rubicon",
			},
		},
		"imp-native": {
			{
				Bid: &adapters.TypedBid{
					Bid:     &openrtb.Bid{ID: "bid3", ImpID: "imp-native", Price: 4.50},
					BidType: adapters.BidTypeNative,
				},
				BidderCode: "pubmatic",
			},
		},
		"imp-audio": {
			{
				Bid: &adapters.TypedBid{
					Bid:     &openrtb.Bid{ID: "bid4", ImpID: "imp-audio", Price: 1.50},
					BidType: adapters.BidTypeAudio,
				},
				BidderCode: "ix",
			},
		},
	}

	ctx := middleware.NewContextWithPublisher(context.Background(), pub)
	result := exchange.applyBidMultiplier(ctx, bidsByImp)

	// Check each media type (divided by 1.5)
	if result["imp-banner"][0].Bid.Bid.Price != 2.00 {
		t.Errorf("Expected banner price 2.00, got %f", result["imp-banner"][0].Bid.Bid.Price)
	}
	if result["imp-video"][0].Bid.Bid.Price != 4.00 {
		t.Errorf("Expected video price 4.00, got %f", result["imp-video"][0].Bid.Bid.Price)
	}
	if result["imp-native"][0].Bid.Bid.Price != 3.00 {
		t.Errorf("Expected native price 3.00, got %f", result["imp-native"][0].Bid.Bid.Price)
	}
	if result["imp-audio"][0].Bid.Bid.Price != 1.00 {
		t.Errorf("Expected audio price 1.00, got %f", result["imp-audio"][0].Bid.Bid.Price)
	}
}

// TestApplyBidMultiplier_NilBid tests handling of nil bids
func TestApplyBidMultiplier_NilBid(t *testing.T) {
	registry := adapters.NewRegistry()
	exchange := New(registry, nil)

	pub := &mockPublisherWithMultiplier{
		PublisherID:   "pub-123",
		BidMultiplier: 2.0,
	}

	bidsByImp := map[string][]ValidatedBid{
		"imp1": {
			{
				Bid:        nil, // Nil bid
				BidderCode: "appnexus",
			},
		},
	}

	ctx := middleware.NewContextWithPublisher(context.Background(), pub)

	// Should not panic with nil bid
	result := exchange.applyBidMultiplier(ctx, bidsByImp)

	if len(result["imp1"]) != 1 {
		t.Error("Expected 1 bid in result")
	}
}

// Mock implementations for testing

type mockMetricsRecorder struct{}

func (m *mockMetricsRecorder) RecordMargin(publisher, bidder, mediaType string, originalPrice, adjustedPrice, platformCut float64) {
}
func (m *mockMetricsRecorder) RecordFloorAdjustment(publisher string) {}
