// Package ix implements the Index Exchange bidder adapter
package ix

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

const defaultEndpoint = "https://htlb.casalemedia.com/openrtb/pbjs"

// Adapter implements the Index Exchange bidder
type Adapter struct {
	endpoint string
}

// New creates a new Index Exchange adapter
func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{endpoint: endpoint}
}

// MakeRequests builds HTTP requests for Index Exchange
func (a *Adapter) MakeRequests(request *openrtb.BidRequest, extraInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to marshal request: %w", err)}
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json;charset=utf-8")
	headers.Set("Accept", "application/json")

	return []*adapters.RequestData{
		{Method: "POST", URI: a.endpoint, Body: requestBody, Headers: headers},
	}, nil
}

// MakeBids parses Index Exchange responses into bids
func (a *Adapter) MakeBids(request *openrtb.BidRequest, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if responseData.StatusCode != http.StatusOK {
		return nil, []error{fmt.Errorf("unexpected status: %d", responseData.StatusCode)}
	}

	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(responseData.Body, &bidResp); err != nil {
		return nil, []error{fmt.Errorf("failed to parse response: %w", err)}
	}

	response := &adapters.BidderResponse{Currency: bidResp.Cur, ResponseID: bidResp.ID, Bids: make([]*adapters.TypedBid, 0)}

	// P3-2: Use shared helper for O(1) bid type lookup
	impMap := adapters.BuildImpMap(request.Imp)
	for _, seatBid := range bidResp.SeatBid {
		for i := range seatBid.Bid {
			bid := &seatBid.Bid[i]
			response.Bids = append(response.Bids, &adapters.TypedBid{
				Bid:     bid,
				BidType: adapters.GetBidTypeFromMap(bid, impMap),
			})
		}
	}
	return response, nil
}

// Info returns bidder information
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled:     true,
		GVLVendorID: 10,
		Endpoint:    defaultEndpoint,
		Maintainer:  &adapters.MaintainerInfo{Email: "prebid.support@indexexchange.com"},
		Capabilities: &adapters.CapabilitiesInfo{
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner, adapters.BidTypeVideo, adapters.BidTypeNative}},
			App:  &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner, adapters.BidTypeVideo, adapters.BidTypeNative}},
		},
	}
}

func init() {
	if err := adapters.RegisterAdapter("ix", New(""), Info()); err != nil {
		panic(fmt.Sprintf("failed to register ix adapter: %v", err))
	}
}
