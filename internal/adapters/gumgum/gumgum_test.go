package gumgum

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
		t.Errorf("Expected default endpoint %s, got %s", defaultEndpoint, adapter.endpoint)
	}

	customEndpoint := "https://custom.endpoint.com"
	adapter = New(customEndpoint)
	if adapter.endpoint != customEndpoint {
		t.Errorf("Expected custom endpoint %s, got %s", customEndpoint, adapter.endpoint)
	}
}

func TestMakeRequests(t *testing.T) {
	adapter := New("")

	request := &openrtb.BidRequest{
		ID: "test-request-1",
		Imp: []openrtb.Imp{
			{
				ID: "imp-1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
		Site: &openrtb.Site{
			Domain: "example.com",
		},
	}

	requests, errs := adapter.MakeRequests(request, nil)

	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	if len(requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(requests))
	}

	req := requests[0]
	if req.Method != "POST" {
		t.Errorf("Expected POST method, got %s", req.Method)
	}

	var parsed openrtb.BidRequest
	if err := json.Unmarshal(req.Body, &parsed); err != nil {
		t.Errorf("Request body is not valid JSON: %v", err)
	}
}

func TestMakeBids_Success(t *testing.T) {
	adapter := New("")

	request := &openrtb.BidRequest{
		ID: "test-request-1",
		Imp: []openrtb.Imp{
			{
				ID: "imp-1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
	}

	responseBody := `{
		"id": "response-1",
		"cur": "USD",
		"seatbid": [{
			"bid": [{
				"id": "bid-1",
				"impid": "imp-1",
				"price": 1.50,
				"adm": "<div>Ad</div>",
				"w": 300,
				"h": 250
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
}

func TestMakeBids_NoContent(t *testing.T) {
	adapter := New("")

	response := &adapters.ResponseData{
		StatusCode: http.StatusNoContent,
		Body:       nil,
	}

	bidderResponse, errs := adapter.MakeBids(&openrtb.BidRequest{}, response)

	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	if bidderResponse != nil {
		t.Error("Expected nil response for NoContent")
	}
}

func TestMakeBids_BadStatus(t *testing.T) {
	adapter := New("")

	response := &adapters.ResponseData{
		StatusCode: http.StatusBadRequest,
		Body:       []byte("invalid"),
	}

	bidderResponse, errs := adapter.MakeBids(&openrtb.BidRequest{}, response)

	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}

	if bidderResponse != nil {
		t.Error("Expected nil response for bad status")
	}
}

func TestMakeBids_InvalidJSON(t *testing.T) {
	adapter := New("")

	response := &adapters.ResponseData{
		StatusCode: http.StatusOK,
		Body:       []byte("not json"),
	}

	_, errs := adapter.MakeBids(&openrtb.BidRequest{}, response)

	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(errs))
	}
}

func TestInfo(t *testing.T) {
	info := Info()

	if !info.Enabled {
		t.Error("Expected adapter to be enabled")
	}

	if info.Capabilities == nil {
		t.Fatal("Expected capabilities to be set")
	}
}
