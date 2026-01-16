package ortb

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// Helper to create a basic config
func basicConfig() *BidderConfig {
	return &BidderConfig{
		BidderCode:  "testbidder",
		Name:        "Test Bidder",
		Description: "A test bidder",
		Endpoint: EndpointConfig{
			URL:             "https://bidder.example.com/bid",
			Method:          "POST",
			TimeoutMS:       500,
			ProtocolVersion: "2.5",
		},
		Capabilities: CapabilitiesConfig{
			MediaTypes:  []string{"banner", "video"},
			SiteEnabled: true,
			AppEnabled:  true,
		},
		Status: "active",
	}
}

// Helper to create a bid request
func testBidRequest() *openrtb.BidRequest {
	return &openrtb.BidRequest{
		ID: "test-req-1",
		Imp: []openrtb.Imp{
			{
				ID:     "imp-1",
				Banner: &openrtb.Banner{W: 300, H: 250},
			},
		},
		Site: &openrtb.Site{
			ID:     "site-1",
			Domain: "example.com",
		},
	}
}

func TestNew(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.config != config {
		t.Error("expected config to be set")
	}
}

func TestGenericAdapter_UpdateConfig(t *testing.T) {
	config1 := basicConfig()
	adapter := New(config1)

	config2 := basicConfig()
	config2.BidderCode = "updated"
	adapter.UpdateConfig(config2)

	got := adapter.GetConfig()
	if got.BidderCode != "updated" {
		t.Errorf("expected updated bidder code, got %s", got.BidderCode)
	}
}

func TestGenericAdapter_GetConfig(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	got := adapter.GetConfig()
	if got.BidderCode != config.BidderCode {
		t.Error("GetConfig should return the config")
	}
}

func TestGenericAdapter_MakeRequests(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := testBidRequest()
	requests, errs := adapter.MakeRequests(request, nil)

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}

	req := requests[0]
	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URI != config.Endpoint.URL {
		t.Errorf("expected %s, got %s", config.Endpoint.URL, req.URI)
	}
	if len(req.Body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestGenericAdapter_MakeRequests_Headers(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := testBidRequest()
	requests, _ := adapter.MakeRequests(request, nil)

	headers := requests[0].Headers
	if headers.Get("Content-Type") != "application/json;charset=utf-8" {
		t.Error("expected Content-Type header")
	}
	if headers.Get("Accept") != "application/json" {
		t.Error("expected Accept header")
	}
	if headers.Get("X-OpenRTB-Version") != "2.5" {
		t.Error("expected X-OpenRTB-Version header")
	}
}

func TestGenericAdapter_MakeBids_Success(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := testBidRequest()

	bidResp := openrtb.BidResponse{
		ID:  "resp-1",
		Cur: "USD",
		SeatBid: []openrtb.SeatBid{
			{
				Seat: "seat1",
				Bid: []openrtb.Bid{
					{
						ID:    "bid-1",
						ImpID: "imp-1",
						Price: 2.50,
						AdM:   "<html>ad</html>",
					},
				},
			},
		},
	}
	respBody, _ := json.Marshal(bidResp)

	responseData := &adapters.ResponseData{
		StatusCode: http.StatusOK,
		Body:       respBody,
	}

	response, errs := adapter.MakeBids(request, responseData)

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Currency != "USD" {
		t.Errorf("expected USD, got %s", response.Currency)
	}
	if response.ResponseID != "resp-1" {
		t.Errorf("expected resp-1, got %s", response.ResponseID)
	}
	if len(response.Bids) != 1 {
		t.Fatalf("expected 1 bid, got %d", len(response.Bids))
	}
	if response.Bids[0].Bid.ID != "bid-1" {
		t.Error("expected bid ID to be bid-1")
	}
}

func TestGenericAdapter_MakeBids_NoContent(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := testBidRequest()
	responseData := &adapters.ResponseData{
		StatusCode: http.StatusNoContent,
	}

	response, errs := adapter.MakeBids(request, responseData)

	if response != nil {
		t.Error("expected nil response for 204")
	}
	if len(errs) > 0 {
		t.Error("expected no errors for 204")
	}
}

func TestGenericAdapter_MakeBids_BadRequest(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := testBidRequest()
	responseData := &adapters.ResponseData{
		StatusCode: http.StatusBadRequest,
		Body:       []byte("invalid request"),
	}

	response, errs := adapter.MakeBids(request, responseData)

	if response != nil {
		t.Error("expected nil response for 400")
	}
	if len(errs) != 1 {
		t.Fatal("expected 1 error")
	}
	if !strings.Contains(errs[0].Error(), "bad request") {
		t.Errorf("expected 'bad request' in error: %v", errs[0])
	}
}

func TestGenericAdapter_MakeBids_ServerError(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := testBidRequest()
	responseData := &adapters.ResponseData{
		StatusCode: http.StatusInternalServerError,
	}

	response, errs := adapter.MakeBids(request, responseData)

	if response != nil {
		t.Error("expected nil response for 500")
	}
	if len(errs) != 1 {
		t.Fatal("expected 1 error")
	}
	if !strings.Contains(errs[0].Error(), "unexpected status") {
		t.Errorf("expected 'unexpected status' in error: %v", errs[0])
	}
}

func TestGenericAdapter_MakeBids_InvalidJSON(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := testBidRequest()
	responseData := &adapters.ResponseData{
		StatusCode: http.StatusOK,
		Body:       []byte("not json"),
	}

	response, errs := adapter.MakeBids(request, responseData)

	if response != nil {
		t.Error("expected nil response for invalid JSON")
	}
	if len(errs) != 1 {
		t.Fatal("expected 1 error")
	}
	if !strings.Contains(errs[0].Error(), "failed to parse") {
		t.Errorf("expected 'failed to parse' in error: %v", errs[0])
	}
}

func TestGenericAdapter_MakeBids_MultipleSeatBids(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := testBidRequest()
	request.Imp = append(request.Imp, openrtb.Imp{
		ID:    "imp-2",
		Video: &openrtb.Video{},
	})

	bidResp := openrtb.BidResponse{
		ID: "resp-1",
		SeatBid: []openrtb.SeatBid{
			{
				Seat: "seat1",
				Bid: []openrtb.Bid{
					{ID: "bid-1", ImpID: "imp-1", Price: 2.00},
				},
			},
			{
				Seat: "seat2",
				Bid: []openrtb.Bid{
					{ID: "bid-2", ImpID: "imp-2", Price: 3.00},
				},
			},
		},
	}
	respBody, _ := json.Marshal(bidResp)

	responseData := &adapters.ResponseData{
		StatusCode: http.StatusOK,
		Body:       respBody,
	}

	response, _ := adapter.MakeBids(request, responseData)

	if len(response.Bids) != 2 {
		t.Errorf("expected 2 bids, got %d", len(response.Bids))
	}
}

func TestGenericAdapter_MakeBids_BidType(t *testing.T) {
	config := basicConfig()
	adapter := New(config)

	request := &openrtb.BidRequest{
		ID: "test-1",
		Imp: []openrtb.Imp{
			{ID: "imp-banner", Banner: &openrtb.Banner{}},
			{ID: "imp-video", Video: &openrtb.Video{}},
			{ID: "imp-native", Native: &openrtb.Native{}},
			{ID: "imp-audio", Audio: &openrtb.Audio{}},
		},
	}

	bidResp := openrtb.BidResponse{
		SeatBid: []openrtb.SeatBid{
			{
				Bid: []openrtb.Bid{
					{ID: "b1", ImpID: "imp-banner", Price: 1.0},
					{ID: "b2", ImpID: "imp-video", Price: 2.0},
					{ID: "b3", ImpID: "imp-native", Price: 3.0},
					{ID: "b4", ImpID: "imp-audio", Price: 4.0},
				},
			},
		},
	}
	respBody, _ := json.Marshal(bidResp)

	responseData := &adapters.ResponseData{
		StatusCode: http.StatusOK,
		Body:       respBody,
	}

	response, _ := adapter.MakeBids(request, responseData)

	expectedTypes := map[string]adapters.BidType{
		"b1": adapters.BidTypeBanner,
		"b2": adapters.BidTypeVideo,
		"b3": adapters.BidTypeNative,
		"b4": adapters.BidTypeAudio,
	}

	for _, bid := range response.Bids {
		expected := expectedTypes[bid.Bid.ID]
		if bid.BidType != expected {
			t.Errorf("bid %s: expected type %s, got %s", bid.Bid.ID, expected, bid.BidType)
		}
	}
}

func TestGenericAdapter_TransformBid_PriceAdjustment(t *testing.T) {
	config := basicConfig()
	config.ResponseTransform.PriceAdjustment = 0.9 // 10% discount
	adapter := New(config)

	request := testBidRequest()
	bidResp := openrtb.BidResponse{
		SeatBid: []openrtb.SeatBid{
			{
				Bid: []openrtb.Bid{
					{ID: "bid-1", ImpID: "imp-1", Price: 10.00},
				},
			},
		},
	}
	respBody, _ := json.Marshal(bidResp)

	responseData := &adapters.ResponseData{
		StatusCode: http.StatusOK,
		Body:       respBody,
	}

	response, _ := adapter.MakeBids(request, responseData)

	if response.Bids[0].Bid.Price != 9.00 {
		t.Errorf("expected price 9.00 after adjustment, got %.2f", response.Bids[0].Bid.Price)
	}
}

func TestGenericAdapter_BuildHeaders_BasicAuth(t *testing.T) {
	config := basicConfig()
	config.Endpoint.AuthType = "basic"
	config.Endpoint.AuthUsername = "user"
	config.Endpoint.AuthPassword = "pass"
	adapter := New(config)

	headers := adapter.buildHeaders(config)

	authHeader := headers.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Basic ") {
		t.Error("expected Basic auth header")
	}

	expected := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if authHeader != "Basic "+expected {
		t.Errorf("unexpected auth header: %s", authHeader)
	}
}

func TestGenericAdapter_BuildHeaders_BearerAuth(t *testing.T) {
	config := basicConfig()
	config.Endpoint.AuthType = "bearer"
	config.Endpoint.AuthToken = "my-token-123"
	adapter := New(config)

	headers := adapter.buildHeaders(config)

	if headers.Get("Authorization") != "Bearer my-token-123" {
		t.Error("expected Bearer auth header")
	}
}

func TestGenericAdapter_BuildHeaders_CustomHeaderAuth(t *testing.T) {
	config := basicConfig()
	config.Endpoint.AuthType = "header"
	config.Endpoint.AuthHeaderName = "X-API-Key"
	config.Endpoint.AuthHeaderValue = "secret-key"
	adapter := New(config)

	headers := adapter.buildHeaders(config)

	if headers.Get("X-API-Key") != "secret-key" {
		t.Error("expected custom auth header")
	}
}

func TestGenericAdapter_BuildHeaders_CustomHeaders(t *testing.T) {
	config := basicConfig()
	config.Endpoint.CustomHeaders = map[string]string{
		"X-Custom-1": "value1",
		"X-Custom-2": "value2",
	}
	adapter := New(config)

	headers := adapter.buildHeaders(config)

	if headers.Get("X-Custom-1") != "value1" {
		t.Error("expected X-Custom-1 header")
	}
	if headers.Get("X-Custom-2") != "value2" {
		t.Error("expected X-Custom-2 header")
	}
}

func TestGenericAdapter_TransformRequest_RequestExtTemplate(t *testing.T) {
	config := basicConfig()
	config.RequestTransform.RequestExtTemplate = map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}
	adapter := New(config)

	request := testBidRequest()
	transformed := adapter.transformRequest(request, config)

	var ext map[string]interface{}
	json.Unmarshal(transformed.Ext, &ext)

	if ext["key1"] != "value1" {
		t.Error("expected key1 in request ext")
	}
}

func TestGenericAdapter_TransformRequest_ImpExtTemplate(t *testing.T) {
	config := basicConfig()
	config.RequestTransform.ImpExtTemplate = map[string]interface{}{
		"bidder_data": "test",
	}
	adapter := New(config)

	request := testBidRequest()
	transformed := adapter.transformRequest(request, config)

	var ext map[string]interface{}
	json.Unmarshal(transformed.Imp[0].Ext, &ext)

	if ext["bidder_data"] != "test" {
		t.Error("expected bidder_data in imp ext")
	}
}

func TestGenericAdapter_TransformRequest_SiteExtTemplate(t *testing.T) {
	config := basicConfig()
	config.RequestTransform.SiteExtTemplate = map[string]interface{}{
		"site_data": "test",
	}
	adapter := New(config)

	request := testBidRequest()
	transformed := adapter.transformRequest(request, config)

	var ext map[string]interface{}
	json.Unmarshal(transformed.Site.Ext, &ext)

	if ext["site_data"] != "test" {
		t.Error("expected site_data in site ext")
	}
}

func TestGenericAdapter_TransformRequest_UserExtTemplate(t *testing.T) {
	config := basicConfig()
	config.RequestTransform.UserExtTemplate = map[string]interface{}{
		"user_data": "test",
	}
	adapter := New(config)

	request := testBidRequest()
	request.User = &openrtb.User{ID: "user-1"}
	transformed := adapter.transformRequest(request, config)

	var ext map[string]interface{}
	json.Unmarshal(transformed.User.Ext, &ext)

	if ext["user_data"] != "test" {
		t.Error("expected user_data in user ext")
	}
}

func TestGenericAdapter_Info(t *testing.T) {
	gvlID := 123
	config := basicConfig()
	config.GVLVendorID = &gvlID
	config.MaintainerEmail = "test@example.com"
	adapter := New(config)

	info := adapter.Info()

	if !info.Enabled {
		t.Error("expected enabled")
	}
	if info.GVLVendorID != 123 {
		t.Error("expected GVL ID 123")
	}
	if info.Maintainer.Email != "test@example.com" {
		t.Error("expected maintainer email")
	}
	if info.Endpoint != config.Endpoint.URL {
		t.Error("expected endpoint URL")
	}
}

func TestGenericAdapter_Info_Capabilities(t *testing.T) {
	config := basicConfig()
	config.Capabilities.MediaTypes = []string{"banner", "video", "native", "audio"}
	config.Capabilities.SiteEnabled = true
	config.Capabilities.AppEnabled = true
	adapter := New(config)

	info := adapter.Info()

	if info.Capabilities == nil {
		t.Fatal("expected capabilities")
	}
	if info.Capabilities.Site == nil {
		t.Fatal("expected site capabilities")
	}
	if len(info.Capabilities.Site.MediaTypes) != 4 {
		t.Errorf("expected 4 media types, got %d", len(info.Capabilities.Site.MediaTypes))
	}
	if info.Capabilities.App == nil {
		t.Fatal("expected app capabilities")
	}
}

func TestGenericAdapter_IsEnabled(t *testing.T) {
	tests := []struct {
		status   string
		expected bool
	}{
		{"active", true},
		{"testing", true},
		{"inactive", false},
		{"disabled", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			config := basicConfig()
			config.Status = tt.status
			adapter := New(config)

			if adapter.IsEnabled() != tt.expected {
				t.Errorf("status %s: expected %v", tt.status, tt.expected)
			}
		})
	}
}

func TestGenericAdapter_GetTimeout(t *testing.T) {
	config := basicConfig()
	config.Endpoint.TimeoutMS = 750
	adapter := New(config)

	timeout := adapter.GetTimeout()

	if timeout != 750*time.Millisecond {
		t.Errorf("expected 750ms, got %v", timeout)
	}
}

func TestGenericAdapter_CanBidForPublisher(t *testing.T) {
	tests := []struct {
		name      string
		allowed   []string
		blocked   []string
		publisher string
		expected  bool
	}{
		{"no restrictions", nil, nil, "pub-1", true},
		{"in allowed list", []string{"pub-1", "pub-2"}, nil, "pub-1", true},
		{"not in allowed list", []string{"pub-1", "pub-2"}, nil, "pub-3", false},
		{"in blocked list", nil, []string{"pub-1"}, "pub-1", false},
		{"not in blocked list", nil, []string{"pub-1"}, "pub-2", true},
		{"blocked takes priority", []string{"pub-1"}, []string{"pub-1"}, "pub-1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := basicConfig()
			config.AllowedPublishers = tt.allowed
			config.BlockedPublishers = tt.blocked
			adapter := New(config)

			result := adapter.CanBidForPublisher(tt.publisher)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGenericAdapter_CanBidForCountry(t *testing.T) {
	tests := []struct {
		name     string
		allowed  []string
		blocked  []string
		country  string
		expected bool
	}{
		{"no restrictions", nil, nil, "US", true},
		{"in allowed list", []string{"US", "CA"}, nil, "US", true},
		{"not in allowed list", []string{"US", "CA"}, nil, "GB", false},
		{"in blocked list", nil, []string{"CN"}, "CN", false},
		{"not in blocked list", nil, []string{"CN"}, "US", true},
		{"case insensitive", []string{"us"}, nil, "US", true},
		{"blocked case insensitive", nil, []string{"cn"}, "CN", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := basicConfig()
			config.AllowedCountries = tt.allowed
			config.BlockedCountries = tt.blocked
			adapter := New(config)

			result := adapter.CanBidForCountry(tt.country)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMergeJSONExt_Empty(t *testing.T) {
	result := mergeJSONExt(nil, nil)
	if result != nil {
		t.Error("expected nil for empty inputs")
	}
}

func TestMergeJSONExt_AddToEmpty(t *testing.T) {
	additions := map[string]interface{}{"key": "value"}
	result := mergeJSONExt(nil, additions)

	var data map[string]interface{}
	json.Unmarshal(result, &data)

	if data["key"] != "value" {
		t.Error("expected key in result")
	}
}

func TestMergeJSONExt_MergeExisting(t *testing.T) {
	existing := json.RawMessage(`{"existing": "data"}`)
	additions := map[string]interface{}{"new": "value"}

	result := mergeJSONExt(existing, additions)

	var data map[string]interface{}
	json.Unmarshal(result, &data)

	if data["existing"] != "data" {
		t.Error("expected existing data preserved")
	}
	if data["new"] != "value" {
		t.Error("expected new data added")
	}
}

func TestMergeJSONExt_OverwriteExisting(t *testing.T) {
	existing := json.RawMessage(`{"key": "old"}`)
	additions := map[string]interface{}{"key": "new"}

	result := mergeJSONExt(existing, additions)

	var data map[string]interface{}
	json.Unmarshal(result, &data)

	if data["key"] != "new" {
		t.Error("expected key to be overwritten")
	}
}

// Benchmark tests
func BenchmarkMakeRequests(b *testing.B) {
	config := basicConfig()
	adapter := New(config)
	request := testBidRequest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adapter.MakeRequests(request, nil)
	}
}

func BenchmarkMakeBids(b *testing.B) {
	config := basicConfig()
	adapter := New(config)
	request := testBidRequest()

	bidResp := openrtb.BidResponse{
		SeatBid: []openrtb.SeatBid{
			{
				Bid: []openrtb.Bid{
					{ID: "bid-1", ImpID: "imp-1", Price: 2.50},
				},
			},
		},
	}
	respBody, _ := json.Marshal(bidResp)
	responseData := &adapters.ResponseData{
		StatusCode: http.StatusOK,
		Body:       respBody,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adapter.MakeBids(request, responseData)
	}
}

func BenchmarkBuildHeaders(b *testing.B) {
	config := basicConfig()
	config.Endpoint.AuthType = "basic"
	config.Endpoint.AuthUsername = "user"
	config.Endpoint.AuthPassword = "pass"
	config.Endpoint.CustomHeaders = map[string]string{
		"X-Custom-1": "value1",
		"X-Custom-2": "value2",
	}
	adapter := New(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adapter.buildHeaders(config)
	}
}

func BenchmarkTransformRequest(b *testing.B) {
	config := basicConfig()
	config.RequestTransform.RequestExtTemplate = map[string]interface{}{"key": "value"}
	config.RequestTransform.ImpExtTemplate = map[string]interface{}{"bidder": "data"}
	adapter := New(config)
	request := testBidRequest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adapter.transformRequest(request, config)
	}
}

// ============================================================================
// SChain Augmentation Tests
// ============================================================================

func TestAugmentSChain_NilSource(t *testing.T) {
	adapter := &GenericAdapter{}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "nexusengine.com", SID: "nexus-001", HP: 1},
		},
		Version: "1.0",
	}

	result := adapter.augmentSChain(nil, augment)

	if result == nil {
		t.Fatal("expected non-nil result when source is nil")
	}
	if result.SChain == nil {
		t.Fatal("expected non-nil schain")
	}
	if len(result.SChain.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(result.SChain.Nodes))
	}
	if result.SChain.Nodes[0].ASI != "nexusengine.com" {
		t.Errorf("expected ASI 'nexusengine.com', got '%s'", result.SChain.Nodes[0].ASI)
	}
	if result.SChain.Ver != "1.0" {
		t.Errorf("expected version '1.0', got '%s'", result.SChain.Ver)
	}
	if result.SChain.Complete != 1 {
		t.Errorf("expected complete=1 (default), got %d", result.SChain.Complete)
	}
}

func TestAugmentSChain_EmptySource(t *testing.T) {
	adapter := &GenericAdapter{}
	source := &openrtb.Source{}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "test.com", SID: "test-001", HP: 1, Name: "Test Entity"},
		},
		Version: "1.0",
	}

	result := adapter.augmentSChain(source, augment)

	if result.SChain == nil {
		t.Fatal("expected schain to be created")
	}
	if len(result.SChain.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(result.SChain.Nodes))
	}
	if result.SChain.Nodes[0].Name != "Test Entity" {
		t.Errorf("expected Name 'Test Entity', got '%s'", result.SChain.Nodes[0].Name)
	}
}

func TestAugmentSChain_AppendToExisting(t *testing.T) {
	adapter := &GenericAdapter{}
	source := &openrtb.Source{
		SChain: &openrtb.SupplyChain{
			Ver:      "1.0",
			Complete: 1,
			Nodes: []openrtb.SupplyChainNode{
				{ASI: "publisher.com", SID: "pub-001", HP: 1},
			},
		},
	}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "nexusengine.com", SID: "nexus-001", HP: 1},
		},
	}

	result := adapter.augmentSChain(source, augment)

	if len(result.SChain.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result.SChain.Nodes))
	}
	// First node should be the original
	if result.SChain.Nodes[0].ASI != "publisher.com" {
		t.Errorf("first node should be original, got ASI '%s'", result.SChain.Nodes[0].ASI)
	}
	// Second node should be the appended one
	if result.SChain.Nodes[1].ASI != "nexusengine.com" {
		t.Errorf("second node should be appended, got ASI '%s'", result.SChain.Nodes[1].ASI)
	}
}

func TestAugmentSChain_MultipleNodes(t *testing.T) {
	adapter := &GenericAdapter{}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "exchange.com", SID: "ex-001", HP: 1},
			{ASI: "reseller.com", SID: "resell-001", HP: 0},
			{ASI: "ssp.com", SID: "ssp-001", HP: 1},
		},
		Version: "1.0",
	}

	result := adapter.augmentSChain(nil, augment)

	if len(result.SChain.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(result.SChain.Nodes))
	}
	// Verify order is preserved
	if result.SChain.Nodes[0].ASI != "exchange.com" {
		t.Errorf("expected first node ASI 'exchange.com', got '%s'", result.SChain.Nodes[0].ASI)
	}
	if result.SChain.Nodes[1].HP != 0 {
		t.Errorf("expected second node HP=0 (indirect), got %d", result.SChain.Nodes[1].HP)
	}
	if result.SChain.Nodes[2].ASI != "ssp.com" {
		t.Errorf("expected third node ASI 'ssp.com', got '%s'", result.SChain.Nodes[2].ASI)
	}
}

func TestAugmentSChain_OverrideComplete(t *testing.T) {
	adapter := &GenericAdapter{}

	// Test override to 0 (incomplete)
	complete0 := 0
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "test.com", SID: "test-001", HP: 1},
		},
		Complete: &complete0,
	}

	result := adapter.augmentSChain(nil, augment)

	if result.SChain.Complete != 0 {
		t.Errorf("expected complete=0, got %d", result.SChain.Complete)
	}

	// Test override to 1 (complete)
	complete1 := 1
	augment.Complete = &complete1

	result = adapter.augmentSChain(nil, augment)

	if result.SChain.Complete != 1 {
		t.Errorf("expected complete=1, got %d", result.SChain.Complete)
	}
}

func TestAugmentSChain_PreserveOriginalComplete(t *testing.T) {
	adapter := &GenericAdapter{}
	source := &openrtb.Source{
		SChain: &openrtb.SupplyChain{
			Ver:      "1.0",
			Complete: 0, // Original is incomplete
			Nodes: []openrtb.SupplyChainNode{
				{ASI: "original.com", SID: "orig-001", HP: 1},
			},
		},
	}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "appended.com", SID: "app-001", HP: 1},
		},
		Complete: nil, // Don't override
	}

	result := adapter.augmentSChain(source, augment)

	if result.SChain.Complete != 0 {
		t.Errorf("expected complete=0 (preserved), got %d", result.SChain.Complete)
	}
}

func TestAugmentSChain_VersionOverride(t *testing.T) {
	adapter := &GenericAdapter{}
	source := &openrtb.Source{
		SChain: &openrtb.SupplyChain{
			Ver:   "1.0",
			Nodes: []openrtb.SupplyChainNode{},
		},
	}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "test.com", SID: "test-001", HP: 1},
		},
		Version: "2.0",
	}

	result := adapter.augmentSChain(source, augment)

	if result.SChain.Ver != "2.0" {
		t.Errorf("expected version '2.0', got '%s'", result.SChain.Ver)
	}
}

func TestAugmentSChain_DefaultVersion(t *testing.T) {
	adapter := &GenericAdapter{}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "test.com", SID: "test-001", HP: 1},
		},
		Version: "", // Empty version should default to "1.0"
	}

	result := adapter.augmentSChain(nil, augment)

	if result.SChain.Ver != "1.0" {
		t.Errorf("expected default version '1.0', got '%s'", result.SChain.Ver)
	}
}

func TestAugmentSChain_NodeWithAllFields(t *testing.T) {
	adapter := &GenericAdapter{}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{
				ASI:    "nexusengine.com",
				SID:    "nexus-seat-001",
				HP:     1,
				RID:    "request-12345",
				Name:   "The Nexus Engine",
				Domain: "nexusengine.com",
			},
		},
	}

	result := adapter.augmentSChain(nil, augment)

	node := result.SChain.Nodes[0]
	if node.ASI != "nexusengine.com" {
		t.Errorf("expected ASI 'nexusengine.com', got '%s'", node.ASI)
	}
	if node.SID != "nexus-seat-001" {
		t.Errorf("expected SID 'nexus-seat-001', got '%s'", node.SID)
	}
	if node.HP != 1 {
		t.Errorf("expected HP 1, got %d", node.HP)
	}
	if node.RID != "request-12345" {
		t.Errorf("expected RID 'request-12345', got '%s'", node.RID)
	}
	if node.Name != "The Nexus Engine" {
		t.Errorf("expected Name 'The Nexus Engine', got '%s'", node.Name)
	}
	if node.Domain != "nexusengine.com" {
		t.Errorf("expected Domain 'nexusengine.com', got '%s'", node.Domain)
	}
}

func TestAugmentSChain_NodeWithExt(t *testing.T) {
	adapter := &GenericAdapter{}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{
				ASI: "test.com",
				SID: "test-001",
				HP:  1,
				Ext: map[string]interface{}{
					"custom_field": "value",
					"partner_id":   12345,
				},
			},
		},
	}

	result := adapter.augmentSChain(nil, augment)

	node := result.SChain.Nodes[0]
	if node.Ext == nil {
		t.Fatal("expected Ext to be set")
	}

	var extData map[string]interface{}
	if err := json.Unmarshal(node.Ext, &extData); err != nil {
		t.Fatalf("failed to unmarshal ext: %v", err)
	}

	if extData["custom_field"] != "value" {
		t.Errorf("expected custom_field='value', got '%v'", extData["custom_field"])
	}
	// JSON unmarshals numbers as float64
	if extData["partner_id"] != float64(12345) {
		t.Errorf("expected partner_id=12345, got '%v'", extData["partner_id"])
	}
}

func TestAugmentSChain_DoesNotModifyOriginal(t *testing.T) {
	adapter := &GenericAdapter{}
	originalNodes := []openrtb.SupplyChainNode{
		{ASI: "original.com", SID: "orig-001", HP: 1},
	}
	source := &openrtb.Source{
		SChain: &openrtb.SupplyChain{
			Ver:      "1.0",
			Complete: 1,
			Nodes:    originalNodes,
		},
	}
	augment := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "appended.com", SID: "app-001", HP: 1},
		},
	}

	result := adapter.augmentSChain(source, augment)

	// Verify original is not modified
	if len(source.SChain.Nodes) != 1 {
		t.Errorf("original schain should still have 1 node, got %d", len(source.SChain.Nodes))
	}
	if source.SChain.Nodes[0].ASI != "original.com" {
		t.Errorf("original node should be unchanged, got ASI '%s'", source.SChain.Nodes[0].ASI)
	}

	// Verify result has both nodes
	if len(result.SChain.Nodes) != 2 {
		t.Errorf("result should have 2 nodes, got %d", len(result.SChain.Nodes))
	}
}

func TestAugmentSChain_HP_DirectAndIndirect(t *testing.T) {
	adapter := &GenericAdapter{}

	// Test HP=1 (direct/payment)
	augmentDirect := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "direct.com", SID: "dir-001", HP: 1},
		},
	}

	result := adapter.augmentSChain(nil, augmentDirect)
	if result.SChain.Nodes[0].HP != 1 {
		t.Errorf("expected HP=1 for direct, got %d", result.SChain.Nodes[0].HP)
	}

	// Test HP=0 (indirect)
	augmentIndirect := &SChainAugmentConfig{
		Enabled: true,
		Nodes: []SChainNodeConfig{
			{ASI: "indirect.com", SID: "ind-001", HP: 0},
		},
	}

	result = adapter.augmentSChain(nil, augmentIndirect)
	if result.SChain.Nodes[0].HP != 0 {
		t.Errorf("expected HP=0 for indirect, got %d", result.SChain.Nodes[0].HP)
	}
}

// Test the transform request function integrates schain correctly
func TestTransformRequest_WithSChainAugment(t *testing.T) {
	config := &BidderConfig{
		BidderCode: "test-bidder",
		Name:       "Test Bidder",
		Endpoint: EndpointConfig{
			URL: "https://test.com/bid",
		},
		RequestTransform: RequestTransformConfig{
			SChainAugment: SChainAugmentConfig{
				Enabled: true,
				Nodes: []SChainNodeConfig{
					{ASI: "nexusengine.com", SID: "nexus-001", HP: 1},
				},
				Version: "1.0",
			},
		},
	}

	adapter := New(config)
	request := &openrtb.BidRequest{
		ID: "test-request",
	}

	result := adapter.transformRequest(request, config)

	if result.Source == nil {
		t.Fatal("expected Source to be set")
	}
	if result.Source.SChain == nil {
		t.Fatal("expected SChain to be set")
	}
	if len(result.Source.SChain.Nodes) != 1 {
		t.Errorf("expected 1 schain node, got %d", len(result.Source.SChain.Nodes))
	}
	if result.Source.SChain.Nodes[0].ASI != "nexusengine.com" {
		t.Errorf("expected ASI 'nexusengine.com', got '%s'", result.Source.SChain.Nodes[0].ASI)
	}
}

func TestTransformRequest_SChainDisabled(t *testing.T) {
	config := &BidderConfig{
		BidderCode: "test-bidder",
		Name:       "Test Bidder",
		Endpoint: EndpointConfig{
			URL: "https://test.com/bid",
		},
		RequestTransform: RequestTransformConfig{
			SChainAugment: SChainAugmentConfig{
				Enabled: false, // Disabled
				Nodes: []SChainNodeConfig{
					{ASI: "nexusengine.com", SID: "nexus-001", HP: 1},
				},
			},
		},
	}

	adapter := New(config)
	request := &openrtb.BidRequest{
		ID: "test-request",
	}

	result := adapter.transformRequest(request, config)

	// SChain should not be added when disabled
	if result.Source != nil && result.Source.SChain != nil {
		t.Error("expected no SChain when augmentation is disabled")
	}
}

func TestTransformRequest_SChainEmptyNodes(t *testing.T) {
	config := &BidderConfig{
		BidderCode: "test-bidder",
		Name:       "Test Bidder",
		Endpoint: EndpointConfig{
			URL: "https://test.com/bid",
		},
		RequestTransform: RequestTransformConfig{
			SChainAugment: SChainAugmentConfig{
				Enabled: true,
				Nodes:   []SChainNodeConfig{}, // Empty nodes
			},
		},
	}

	adapter := New(config)
	request := &openrtb.BidRequest{
		ID: "test-request",
	}

	result := adapter.transformRequest(request, config)

	// SChain should not be added when nodes are empty
	if result.Source != nil && result.Source.SChain != nil {
		t.Error("expected no SChain when nodes are empty")
	}
}
