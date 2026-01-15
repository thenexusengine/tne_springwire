// Package rubicon implements the Rubicon/Magnite bidder adapter
package rubicon

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

const (
	defaultEndpoint = "https://prebid-server.rubiconproject.com/openrtb2/auction"
)

// Adapter implements the Rubicon bidder
type Adapter struct {
	endpoint string
}

// New creates a new Rubicon adapter
func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{endpoint: endpoint}
}

// MakeRequests builds HTTP requests for Rubicon
func (a *Adapter) MakeRequests(request *openrtb.BidRequest, extraInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	var errors []error
	requests := make([]*adapters.RequestData, 0, len(request.Imp))

	// Rubicon requires one request per impression
	for _, imp := range request.Imp {
		reqCopy := *request
		reqCopy.Imp = []openrtb.Imp{imp}

		requestBody, err := json.Marshal(reqCopy)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to marshal request for imp %s: %w", imp.ID, err))
			continue
		}

		headers := http.Header{}
		headers.Set("Content-Type", "application/json;charset=utf-8")
		headers.Set("Accept", "application/json")

		requests = append(requests, &adapters.RequestData{
			Method:  "POST",
			URI:     a.endpoint,
			Body:    requestBody,
			Headers: headers,
		})
	}

	return requests, errors
}

// MakeBids parses Rubicon responses into bids
func (a *Adapter) MakeBids(request *openrtb.BidRequest, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if responseData.StatusCode == http.StatusBadRequest {
		return nil, []error{fmt.Errorf("bad request: %s", string(responseData.Body))}
	}

	if responseData.StatusCode != http.StatusOK {
		return nil, []error{fmt.Errorf("unexpected status: %d", responseData.StatusCode)}
	}

	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(responseData.Body, &bidResp); err != nil {
		return nil, []error{fmt.Errorf("failed to parse response: %w", err)}
	}

	response := &adapters.BidderResponse{
		Currency:   bidResp.Cur,
		ResponseID: bidResp.ID, // P1-1: Include ResponseID for validation
		Bids:       make([]*adapters.TypedBid, 0),
	}

	// P2-3: Build impression map once for O(1) lookups instead of O(n) per bid
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

// Info returns bidder information
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled: true,
		Maintainer: &adapters.MaintainerInfo{
			Email: "header-bidding@rubiconproject.com",
		},
		Capabilities: &adapters.CapabilitiesInfo{
			Site: &adapters.PlatformInfo{
				MediaTypes: []adapters.BidType{
					adapters.BidTypeBanner,
					adapters.BidTypeVideo,
				},
			},
			App: &adapters.PlatformInfo{
				MediaTypes: []adapters.BidType{
					adapters.BidTypeBanner,
					adapters.BidTypeVideo,
				},
			},
		},
		GVLVendorID: 52,
		Endpoint:    defaultEndpoint,
		DemandType:  adapters.DemandTypePlatform, // Platform demand (obfuscated as "thenexusengine")
	}
}

func init() {
	if err := adapters.RegisterAdapter("rubicon", New(""), Info()); err != nil {
		panic(fmt.Sprintf("failed to register rubicon adapter: %v", err))
	}
}
