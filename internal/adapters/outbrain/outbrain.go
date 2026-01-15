// Package outbrain implements the Outbrain bidder adapter (native specialist)
package outbrain

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

const defaultEndpoint = "https://prebid-server.outbrain.com/openrtb/2.5"

type Adapter struct{ endpoint string }

func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{endpoint: endpoint}
}

func (a *Adapter) MakeRequests(request *openrtb.BidRequest, extraInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, []error{err}
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	return []*adapters.RequestData{{Method: "POST", URI: a.endpoint, Body: body, Headers: headers}}, nil
}

func (a *Adapter) MakeBids(request *openrtb.BidRequest, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode != http.StatusOK {
		return nil, nil
	}
	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(responseData.Body, &bidResp); err != nil {
		return nil, []error{err}
	}
	response := &adapters.BidderResponse{Currency: bidResp.Cur, ResponseID: bidResp.ID, Bids: make([]*adapters.TypedBid, 0)}
	for _, sb := range bidResp.SeatBid {
		for i := range sb.Bid {
			response.Bids = append(response.Bids, &adapters.TypedBid{Bid: &sb.Bid[i], BidType: adapters.BidTypeNative})
		}
	}
	return response, nil
}

func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled: true, GVLVendorID: 164, Endpoint: defaultEndpoint,
		Maintainer: &adapters.MaintainerInfo{Email: "prebid@outbrain.com"},
		Capabilities: &adapters.CapabilitiesInfo{
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner, adapters.BidTypeNative}},
		},
	}
}

func init() {
	if err := adapters.RegisterAdapter("outbrain", New(""), Info()); err != nil {
		panic(fmt.Sprintf("failed to register outbrain adapter: %v", err))
	}
}
