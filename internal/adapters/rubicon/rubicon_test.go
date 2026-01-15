package rubicon

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

func TestNew(t *testing.T) {
	adapter := New("")
	if adapter.endpoint != defaultEndpoint {
		t.Errorf("Expected default endpoint, got %s", adapter.endpoint)
	}

	customEndpoint := "https://custom.rubicon.com"
	adapter = New(customEndpoint)
	if adapter.endpoint != customEndpoint {
		t.Errorf("Expected custom endpoint, got %s", adapter.endpoint)
	}
}

func TestMakeRequests_OnePerImpression(t *testing.T) {
	adapter := New("")

	// Create request with multiple impressions
	request := &openrtb.BidRequest{
		ID: "test-request-1",
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{W: 300, H: 250}},
			{ID: "imp-2", Banner: &openrtb.Banner{W: 728, H: 90}},
			{ID: "imp-3", Video: &openrtb.Video{W: 640, H: 480}},
		},
		Site: &openrtb.Site{Domain: "example.com"},
	}

	requests, errs := adapter.MakeRequests(request, nil)

	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	// Rubicon creates one request per impression
	if len(requests) != 3 {
		t.Fatalf("Expected 3 requests (one per impression), got %d", len(requests))
	}

	// Verify each request has only one impression
	for i, req := range requests {
		var parsed openrtb.BidRequest
		if err := json.Unmarshal(req.Body, &parsed); err != nil {
			t.Errorf("Request %d: failed to parse body: %v", i, err)
			continue
		}

		if len(parsed.Imp) != 1 {
			t.Errorf("Request %d: expected 1 impression, got %d", i, len(parsed.Imp))
		}
	}
}

func TestMakeBids_Success(t *testing.T) {
	adapter := New("")

	request := &openrtb.BidRequest{
		ID: "test-request-1",
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{W: 300, H: 250}},
		},
	}

	responseBody := `{
		"id": "response-1",
		"cur": "USD",
		"seatbid": [{
			"bid": [{
				"id": "bid-1",
				"impid": "imp-1",
				"price": 2.50,
				"adm": "<div>Ad</div>"
			}]
		}]
	}`

	response := &adapters.ResponseData{
		StatusCode: http.StatusOK,
		Body:       []byte(responseBody),
	}

	bidderResponse, errs := adapter.MakeBids(request, response)

	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	if bidderResponse == nil {
		t.Fatal("Expected bidder response, got nil")
	}

	if len(bidderResponse.Bids) != 1 {
		t.Fatalf("Expected 1 bid, got %d", len(bidderResponse.Bids))
	}

	if bidderResponse.Bids[0].Bid.Price != 2.50 {
		t.Errorf("Expected price 2.50, got %f", bidderResponse.Bids[0].Bid.Price)
	}
}

func TestMakeBids_NoContent(t *testing.T) {
	adapter := New("")

	response := &adapters.ResponseData{
		StatusCode: http.StatusNoContent,
	}

	bidderResponse, errs := adapter.MakeBids(&openrtb.BidRequest{}, response)

	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	if bidderResponse != nil {
		t.Error("Expected nil response for NoContent")
	}
}

func TestInfo(t *testing.T) {
	info := Info()

	if !info.Enabled {
		t.Error("Expected adapter to be enabled")
	}

	if info.GVLVendorID != 52 {
		t.Errorf("Expected GVL vendor ID 52, got %d", info.GVLVendorID)
	}

	if info.Capabilities == nil || info.Capabilities.Site == nil {
		t.Fatal("Expected capabilities to be set")
	}
}
