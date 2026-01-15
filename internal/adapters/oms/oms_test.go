package oms

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

func TestNew(t *testing.T) {
	t.Run("default endpoint", func(t *testing.T) {
		adapter := New("")
		if adapter.endpoint != defaultEndpoint {
			t.Errorf("expected default endpoint %s, got %s", defaultEndpoint, adapter.endpoint)
		}
	})

	t.Run("custom endpoint", func(t *testing.T) {
		customEndpoint := "https://custom.oms.com/bid"
		adapter := New(customEndpoint)
		if adapter.endpoint != customEndpoint {
			t.Errorf("expected custom endpoint %s, got %s", customEndpoint, adapter.endpoint)
		}
	})
}

func TestMakeRequests(t *testing.T) {
	adapter := New("")

	t.Run("with publisher ID", func(t *testing.T) {
		impExt, _ := json.Marshal(map[string]interface{}{
			"bidder": ExtImpOms{PublisherID: "test-pub-123"},
		})

		request := &openrtb.BidRequest{
			ID: "test-request-1",
			Imp: []openrtb.Imp{
				{ID: "imp-1", Banner: &openrtb.Banner{W: 300, H: 250}, Ext: impExt},
			},
			Site: &openrtb.Site{Domain: "example.com"},
		}

		requests, errs := adapter.MakeRequests(request, nil)

		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		if len(requests) != 1 {
			t.Fatalf("expected 1 request, got %d", len(requests))
		}

		req := requests[0]
		if !strings.Contains(req.URI, "publisherId=test-pub-123") {
			t.Errorf("expected publisher ID in URI, got %s", req.URI)
		}
	})

	t.Run("without publisher ID", func(t *testing.T) {
		request := &openrtb.BidRequest{
			ID: "test-request-1",
			Imp: []openrtb.Imp{
				{ID: "imp-1", Banner: &openrtb.Banner{W: 300, H: 250}},
			},
		}

		requests, errs := adapter.MakeRequests(request, nil)

		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		if len(requests) != 1 {
			t.Fatalf("expected 1 request, got %d", len(requests))
		}

		req := requests[0]
		if req.URI != defaultEndpoint {
			t.Errorf("expected default endpoint, got %s", req.URI)
		}
	})

	t.Run("request headers", func(t *testing.T) {
		request := &openrtb.BidRequest{
			ID:  "test-request-1",
			Imp: []openrtb.Imp{{ID: "imp-1"}},
		}

		requests, _ := adapter.MakeRequests(request, nil)
		req := requests[0]

		if req.Method != "POST" {
			t.Errorf("expected POST method, got %s", req.Method)
		}

		if req.Headers.Get("Content-Type") != "application/json;charset=utf-8" {
			t.Errorf("expected Content-Type header, got %s", req.Headers.Get("Content-Type"))
		}

		if req.Headers.Get("Accept") != "application/json" {
			t.Errorf("expected Accept header, got %s", req.Headers.Get("Accept"))
		}
	})
}

func TestMakeBids(t *testing.T) {
	adapter := New("")

	t.Run("successful banner response", func(t *testing.T) {
		request := &openrtb.BidRequest{
			ID: "test-request-1",
			Imp: []openrtb.Imp{
				{ID: "imp-1", Banner: &openrtb.Banner{}},
			},
		}

		bidResponse := openrtb.BidResponse{
			ID:  "response-1",
			Cur: "USD",
			SeatBid: []openrtb.SeatBid{
				{
					Seat: "oms",
					Bid: []openrtb.Bid{
						{ID: "bid-1", ImpID: "imp-1", Price: 1.5},
					},
				},
			},
		}
		body, _ := json.Marshal(bidResponse)

		responseData := &adapters.ResponseData{
			StatusCode: http.StatusOK,
			Body:       body,
		}

		response, errs := adapter.MakeBids(request, responseData)

		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		if response == nil {
			t.Fatal("expected response, got nil")
		}

		if len(response.Bids) != 1 {
			t.Fatalf("expected 1 bid, got %d", len(response.Bids))
		}

		if response.Bids[0].BidType != adapters.BidTypeBanner {
			t.Errorf("expected banner bid type, got %v", response.Bids[0].BidType)
		}
	})

	t.Run("successful video response", func(t *testing.T) {
		request := &openrtb.BidRequest{
			ID: "test-request-1",
			Imp: []openrtb.Imp{
				{ID: "imp-1", Video: &openrtb.Video{W: 640, H: 480}},
			},
		}

		bidResponse := openrtb.BidResponse{
			ID:  "response-1",
			Cur: "USD",
			SeatBid: []openrtb.SeatBid{
				{
					Seat: "oms",
					Bid: []openrtb.Bid{
						{ID: "bid-1", ImpID: "imp-1", Price: 2.5},
					},
				},
			},
		}
		body, _ := json.Marshal(bidResponse)

		responseData := &adapters.ResponseData{
			StatusCode: http.StatusOK,
			Body:       body,
		}

		response, errs := adapter.MakeBids(request, responseData)

		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		if response.Bids[0].BidType != adapters.BidTypeVideo {
			t.Errorf("expected video bid type, got %v", response.Bids[0].BidType)
		}
	})

	t.Run("no content response", func(t *testing.T) {
		request := &openrtb.BidRequest{ID: "test-request-1"}
		responseData := &adapters.ResponseData{
			StatusCode: http.StatusNoContent,
		}

		response, errs := adapter.MakeBids(request, responseData)

		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		if response != nil {
			t.Error("expected nil response for no content")
		}
	})

	t.Run("bad request response", func(t *testing.T) {
		request := &openrtb.BidRequest{ID: "test-request-1"}
		responseData := &adapters.ResponseData{
			StatusCode: http.StatusBadRequest,
			Body:       []byte("invalid request"),
		}

		_, errs := adapter.MakeBids(request, responseData)

		if len(errs) == 0 {
			t.Error("expected error for bad request")
		}
	})

	t.Run("server error response", func(t *testing.T) {
		request := &openrtb.BidRequest{ID: "test-request-1"}
		responseData := &adapters.ResponseData{
			StatusCode: http.StatusInternalServerError,
		}

		_, errs := adapter.MakeBids(request, responseData)

		if len(errs) == 0 {
			t.Error("expected error for server error")
		}
	})

	t.Run("invalid json response", func(t *testing.T) {
		request := &openrtb.BidRequest{ID: "test-request-1"}
		responseData := &adapters.ResponseData{
			StatusCode: http.StatusOK,
			Body:       []byte("not valid json"),
		}

		_, errs := adapter.MakeBids(request, responseData)

		if len(errs) == 0 {
			t.Error("expected error for invalid json")
		}
	})
}

func TestInfo(t *testing.T) {
	info := Info()

	if !info.Enabled {
		t.Error("expected adapter to be enabled")
	}

	if info.Endpoint != defaultEndpoint {
		t.Errorf("expected endpoint %s, got %s", defaultEndpoint, info.Endpoint)
	}

	if info.Capabilities == nil {
		t.Fatal("expected capabilities")
	}

	if info.Capabilities.Site == nil {
		t.Fatal("expected site capabilities")
	}

	if info.Capabilities.App == nil {
		t.Fatal("expected app capabilities")
	}

	siteMediaTypes := info.Capabilities.Site.MediaTypes
	if len(siteMediaTypes) != 2 {
		t.Errorf("expected 2 site media types, got %d", len(siteMediaTypes))
	}

	appMediaTypes := info.Capabilities.App.MediaTypes
	if len(appMediaTypes) != 2 {
		t.Errorf("expected 2 app media types, got %d", len(appMediaTypes))
	}
}
