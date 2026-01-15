// Package kargo implements the Kargo bidder adapter
package kargo

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

const defaultEndpoint = "https://krk2.kargo.com/api/v1/prebid"

// Adapter implements the Kargo bidder
type Adapter struct {
	endpoint string
}

// New creates a new Kargo adapter
func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{endpoint: endpoint}
}

// MakeRequests builds the HTTP requests for the Kargo bidder
func (a *Adapter) MakeRequests(request *openrtb.BidRequest, extraInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
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
			URI:     a.endpoint,
			Body:    body,
			Headers: headers,
		},
	}, nil
}

// kargoExt represents the Kargo-specific bid extension
type kargoExt struct {
	MediaType string `json:"mediaType,omitempty"`
}

// MakeBids parses the response from the Kargo bidder
func (a *Adapter) MakeBids(request *openrtb.BidRequest, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
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

	for _, seatBid := range bidResp.SeatBid {
		for i := range seatBid.Bid {
			bid := &seatBid.Bid[i]
			bidType := getMediaTypeForBid(bid)
			response.Bids = append(response.Bids, &adapters.TypedBid{
				Bid:     bid,
				BidType: bidType,
			})
		}
	}

	return response, nil
}

// getMediaTypeForBid determines the bid type from the bid's ext field
func getMediaTypeForBid(bid *openrtb.Bid) adapters.BidType {
	if bid.Ext != nil {
		var ext kargoExt
		if err := json.Unmarshal(bid.Ext, &ext); err == nil {
			switch ext.MediaType {
			case "video":
				return adapters.BidTypeVideo
			case "native":
				return adapters.BidTypeNative
			}
		}
	}
	return adapters.BidTypeBanner
}

// Info returns the bidder information for Kargo
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled:     true,
		GVLVendorID: 972, // Kargo's IAB GVL Vendor ID
		Endpoint:    defaultEndpoint,
		Maintainer:  &adapters.MaintainerInfo{Email: "krk@kargo.com"},
		Capabilities: &adapters.CapabilitiesInfo{
			Site: &adapters.PlatformInfo{
				MediaTypes: []adapters.BidType{
					adapters.BidTypeBanner,
					adapters.BidTypeVideo,
					adapters.BidTypeNative,
				},
			},
		},
	}
}

func init() {
	adapters.RegisterAdapter("kargo", New(""), Info())
}
