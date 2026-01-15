package kargo

import (
	"encoding/json"
	"net/http"
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
		customEndpoint := "https://custom.kargo.com/bid"
		adapter := New(customEndpoint)
		if adapter.endpoint != customEndpoint {
			t.Errorf("expected custom endpoint %s, got %s", customEndpoint, adapter.endpoint)
		}
	})
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
		t.Fatalf("unexpected errors: %v", errs)
	}

	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}

	req := requests[0]
	if req.Method != "POST" {
		t.Errorf("expected POST method, got %s", req.Method)
	}

	if req.URI != defaultEndpoint {
		t.Errorf("expected URI %s, got %s", defaultEndpoint, req.URI)
	}

	if req.Headers.Get("Content-Type") != "application/json;charset=utf-8" {
		t.Errorf("expected Content-Type header, got %s", req.Headers.Get("Content-Type"))
	}
}

func TestMakeBids(t *testing.T) {
	adapter := New("")

	request := &openrtb.BidRequest{
		ID: "test-request-1",
		Imp: []openrtb.Imp{
			{ID: "imp-1"},
		},
	}

	t.Run("successful response with banner", func(t *testing.T) {
		bidResponse := openrtb.BidResponse{
			ID:  "response-1",
			Cur: "USD",
			SeatBid: []openrtb.SeatBid{
				{
					Seat: "kargo",
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

	t.Run("successful response with video", func(t *testing.T) {
		videoExt, _ := json.Marshal(kargoExt{MediaType: "video"})
		bidResponse := openrtb.BidResponse{
			ID:  "response-1",
			Cur: "USD",
			SeatBid: []openrtb.SeatBid{
				{
					Seat: "kargo",
					Bid: []openrtb.Bid{
						{ID: "bid-1", ImpID: "imp-1", Price: 2.0, Ext: videoExt},
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

	t.Run("successful response with native", func(t *testing.T) {
		nativeExt, _ := json.Marshal(kargoExt{MediaType: "native"})
		bidResponse := openrtb.BidResponse{
			ID:  "response-1",
			Cur: "USD",
			SeatBid: []openrtb.SeatBid{
				{
					Seat: "kargo",
					Bid: []openrtb.Bid{
						{ID: "bid-1", ImpID: "imp-1", Price: 1.8, Ext: nativeExt},
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

		if response.Bids[0].BidType != adapters.BidTypeNative {
			t.Errorf("expected native bid type, got %v", response.Bids[0].BidType)
		}
	})

	t.Run("no content response", func(t *testing.T) {
		responseData := &adapters.ResponseData{
			StatusCode: http.StatusNoContent,
			Body:       nil,
		}

		response, errs := adapter.MakeBids(request, responseData)

		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		if response != nil {
			t.Error("expected nil response for no content")
		}
	})

	t.Run("error response", func(t *testing.T) {
		responseData := &adapters.ResponseData{
			StatusCode: http.StatusBadRequest,
			Body:       []byte("bad request"),
		}

		_, errs := adapter.MakeBids(request, responseData)

		if len(errs) == 0 {
			t.Error("expected error for bad request")
		}
	})

	t.Run("invalid json response", func(t *testing.T) {
		responseData := &adapters.ResponseData{
			StatusCode: http.StatusOK,
			Body:       []byte("invalid json"),
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

	if info.GVLVendorID != 972 {
		t.Errorf("expected GVL vendor ID 972, got %d", info.GVLVendorID)
	}

	if info.Endpoint != defaultEndpoint {
		t.Errorf("expected endpoint %s, got %s", defaultEndpoint, info.Endpoint)
	}

	if info.Capabilities == nil || info.Capabilities.Site == nil {
		t.Fatal("expected site capabilities")
	}

	mediaTypes := info.Capabilities.Site.MediaTypes
	if len(mediaTypes) != 3 {
		t.Errorf("expected 3 media types, got %d", len(mediaTypes))
	}
}
