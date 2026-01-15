// Package demo implements a demo/test bidder that returns mock bids
// This is useful for testing the auction flow without real SSP credentials
package demo

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// Adapter implements a demo bidder that returns mock bids
type Adapter struct {
	// minCPM and maxCPM define the bid range
	minCPM float64
	maxCPM float64
	// bidRate is the probability of returning a bid (0.0-1.0)
	bidRate float64
}

// New creates a new demo adapter with default settings
func New(_ string) *Adapter {
	return &Adapter{
		minCPM:  0.50, // $0.50 CPM minimum
		maxCPM:  5.00, // $5.00 CPM maximum
		bidRate: 0.80, // 80% bid rate
	}
}

// MakeRequests simulates building HTTP requests (but doesn't actually call an endpoint)
func (a *Adapter) MakeRequests(request *openrtb.BidRequest, _ *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	// For demo purposes, we generate a mock response locally
	// In a real adapter, this would create HTTP requests to the SSP

	// Build a mock response
	response := a.generateMockResponse(request)

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to marshal mock response: %w", err)}
	}

	// Return a "request" that contains the mock response
	// The exchange will call MakeBids with this data
	return []*adapters.RequestData{
		{
			Method: "MOCK", // Special method indicating this is a mock
			URI:    "demo://mock-response",
			Body:   responseBytes,
			Headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
	}, nil
}

// MakeBids parses the mock response into bids
func (a *Adapter) MakeBids(request *openrtb.BidRequest, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	// For demo adapter, the "response" is our mock data
	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(responseData.Body, &bidResp); err != nil {
		return nil, []error{fmt.Errorf("failed to parse mock response: %w", err)}
	}

	response := &adapters.BidderResponse{
		Currency:   bidResp.Cur,
		ResponseID: bidResp.ID,
		Bids:       make([]*adapters.TypedBid, 0),
	}

	impMap := adapters.BuildImpMap(request.Imp)

	for _, seatBid := range bidResp.SeatBid {
		for i := range seatBid.Bid {
			bid := &seatBid.Bid[i]
			bidType := adapters.GetBidTypeFromMap(bid, impMap)

			response.Bids = append(response.Bids, &adapters.TypedBid{
				Bid:     bid,
				BidType: bidType,
			})
		}
	}

	return response, nil
}

// generateMockResponse creates a mock bid response
func (a *Adapter) generateMockResponse(request *openrtb.BidRequest) *openrtb.BidResponse {
	// #nosec G404 -- math/rand is acceptable for demo adapter mock data generation (not security-sensitive)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	response := &openrtb.BidResponse{
		ID:      request.ID,
		Cur:     "USD",
		SeatBid: []openrtb.SeatBid{},
	}

	// Generate bids for each impression
	bids := make([]openrtb.Bid, 0, len(request.Imp))
	for _, imp := range request.Imp {
		// Randomly decide whether to bid
		if rng.Float64() > a.bidRate {
			continue
		}

		// Generate random CPM within range
		cpm := a.minCPM + rng.Float64()*(a.maxCPM-a.minCPM)

		// Determine ad size
		var width, height int64
		if imp.Banner != nil {
			width = int64(imp.Banner.W)
			height = int64(imp.Banner.H)
			if width == 0 && len(imp.Banner.Format) > 0 {
				width = int64(imp.Banner.Format[0].W)
				height = int64(imp.Banner.Format[0].H)
			}
		}
		if width == 0 {
			width = 300
		}
		if height == 0 {
			height = 250
		}

		bid := openrtb.Bid{
			ID:      fmt.Sprintf("demo-bid-%s-%d", imp.ID, time.Now().UnixNano()),
			ImpID:   imp.ID,
			Price:   cpm,
			W:       int(width),
			H:       int(height),
			AdM:     a.generateMockCreative(int(width), int(height), cpm),
			CRID:    fmt.Sprintf("demo-creative-%d", rng.Intn(1000)),
			ADomain: []string{"demo-advertiser.example.com"},
		}

		bids = append(bids, bid)
	}

	if len(bids) > 0 {
		response.SeatBid = []openrtb.SeatBid{
			{
				Bid:  bids,
				Seat: "demo-dsp",
			},
		}
	}

	return response
}

// generateMockCreative creates a simple HTML creative for demo purposes
func (a *Adapter) generateMockCreative(width, height int, cpm float64) string {
	return fmt.Sprintf(`<div style="width:%dpx;height:%dpx;background:linear-gradient(135deg,#667eea 0%%,#764ba2 100%%);display:flex;align-items:center;justify-content:center;font-family:system-ui;color:white;text-align:center;border-radius:8px;box-shadow:0 4px 6px rgba(0,0,0,0.1);">
<div>
<div style="font-size:24px;font-weight:bold;">Demo Ad</div>
<div style="font-size:14px;opacity:0.8;">$%.2f CPM</div>
<div style="font-size:12px;margin-top:8px;">%dx%d</div>
</div>
</div>`, width, height, cpm, width, height)
}

// Info returns bidder information (instance method)
func (a *Adapter) Info() adapters.BidderInfo {
	return Info()
}

// Info returns bidder information (package function for registration)
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled:                 true,
		ModifyingVastXmlAllowed: false,
		GVLVendorID:             0, // No GDPR vendor ID for demo adapter
		Capabilities: &adapters.CapabilitiesInfo{
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner, adapters.BidTypeVideo}},
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner, adapters.BidTypeVideo}},
		},
		DemandType: adapters.DemandTypePlatform, // Platform demand (obfuscated as "thenexusengine")
	}
}

func init() {
	if err := adapters.RegisterAdapter("demo", New(""), Info()); err != nil {
		panic(fmt.Sprintf("failed to register demo adapter: %v", err))
	}
}
