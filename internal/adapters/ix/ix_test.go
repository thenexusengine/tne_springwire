package ix

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
}

func TestMakeRequests(t *testing.T) {
	adapter := New("")

	request := &openrtb.BidRequest{
		ID: "test-request-1",
		Imp: []openrtb.Imp{
			{ID: "imp-1", Banner: &openrtb.Banner{W: 300, H: 250}},
		},
		Site: &openrtb.Site{Domain: "example.com"},
	}

	requests, errs := adapter.MakeRequests(request, nil)

	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	if len(requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(requests))
	}

	if requests[0].Method != "POST" {
		t.Errorf("Expected POST method, got %s", requests[0].Method)
	}

	var parsed openrtb.BidRequest
	if err := json.Unmarshal(requests[0].Body, &parsed); err != nil {
		t.Errorf("Failed to parse request body: %v", err)
	}
}

func TestMakeBids_Success(t *testing.T) {
	adapter := New("")

	request := &openrtb.BidRequest{
		ID:  "test-request-1",
		Imp: []openrtb.Imp{{ID: "imp-1", Banner: &openrtb.Banner{W: 300, H: 250}}},
	}

	responseBody := `{
		"id": "response-1",
		"cur": "USD",
		"seatbid": [{
			"bid": [{"id": "bid-1", "impid": "imp-1", "price": 2.00}]
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

	if bidderResponse == nil || len(bidderResponse.Bids) != 1 {
		t.Fatal("Expected 1 bid")
	}
}

func TestMakeBids_NoContent(t *testing.T) {
	adapter := New("")
	response := &adapters.ResponseData{StatusCode: http.StatusNoContent}
	result, errs := adapter.MakeBids(&openrtb.BidRequest{}, response)
	if len(errs) > 0 || result != nil {
		t.Error("Expected nil response for NoContent")
	}
}

func TestInfo(t *testing.T) {
	info := Info()
	if !info.Enabled {
		t.Error("Expected adapter to be enabled")
	}
	if info.GVLVendorID != 10 {
		t.Errorf("Expected GVL vendor ID 10, got %d", info.GVLVendorID)
	}
}
