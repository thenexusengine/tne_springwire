// Package oms implements the OMS (OneMobile) bidder adapter
package oms

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

const defaultEndpoint = "https://rt.marphezis.com/prebid"

// Adapter implements the OMS bidder
type Adapter struct {
	endpoint string
}

// ExtImpOms defines the OMS-specific impression extension
type ExtImpOms struct {
	PublisherID string `json:"publisherId"`
}

// New creates a new OMS adapter
func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{endpoint: endpoint}
}

// MakeRequests builds the HTTP requests for the OMS bidder
func (a *Adapter) MakeRequests(request *openrtb.BidRequest, extraInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	var errs []error

	// Extract publisher ID from first impression's ext
	publisherID := ""
	if len(request.Imp) > 0 && request.Imp[0].Ext != nil {
		var bidderExt struct {
			Bidder ExtImpOms `json:"bidder"`
		}
		if err := json.Unmarshal(request.Imp[0].Ext, &bidderExt); err == nil {
			publisherID = bidderExt.Bidder.PublisherID
		}
	}

	// Build endpoint URL with publisher ID
	endpointURL := a.endpoint
	if publisherID != "" {
		u, err := url.Parse(a.endpoint)
		if err == nil {
			q := u.Query()
			q.Set("publisherId", publisherID)
			u.RawQuery = q.Encode()
			endpointURL = u.String()
		}
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to marshal request: %w", err)}
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json;charset=utf-8")
	headers.Set("Accept", "application/json")

	return []*adapters.RequestData{
		{
			Method:  "POST",
			URI:     endpointURL,
			Body:    body,
			Headers: headers,
		},
	}, errs
}

// MakeBids parses the response from the OMS bidder
func (a *Adapter) MakeBids(request *openrtb.BidRequest, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if responseData.StatusCode == http.StatusBadRequest {
		return nil, []error{fmt.Errorf("bad request: %s", string(responseData.Body))}
	}

	if responseData.StatusCode != http.StatusOK {
		return nil, []error{fmt.Errorf("unexpected status code: %d", responseData.StatusCode)}
	}

	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(responseData.Body, &bidResp); err != nil {
		return nil, []error{fmt.Errorf("failed to unmarshal response: %w", err)}
	}

	response := &adapters.BidderResponse{
		Currency:   bidResp.Cur,
		ResponseID: bidResp.ID,
		Bids:       make([]*adapters.TypedBid, 0, len(bidResp.SeatBid)),
	}

	// Build impression ID to type map
	impTypes := make(map[string]adapters.BidType)
	for _, imp := range request.Imp {
		if imp.Video != nil {
			impTypes[imp.ID] = adapters.BidTypeVideo
		} else {
			impTypes[imp.ID] = adapters.BidTypeBanner
		}
	}

	for _, seatBid := range bidResp.SeatBid {
		for i := range seatBid.Bid {
			bid := &seatBid.Bid[i]
			bidType := adapters.BidTypeBanner
			if t, ok := impTypes[bid.ImpID]; ok {
				bidType = t
			}
			response.Bids = append(response.Bids, &adapters.TypedBid{
				Bid:     bid,
				BidType: bidType,
			})
		}
	}

	return response, nil
}

// Info returns the bidder information for OMS
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled:     true,
		GVLVendorID: 0, // OMS doesn't have an IAB GVL Vendor ID
		Endpoint:    defaultEndpoint,
		Maintainer:  &adapters.MaintainerInfo{Email: "prebid@onemobile.com"},
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
	}
}

func init() {
	adapters.RegisterAdapter("oms", New(""), Info())
}
