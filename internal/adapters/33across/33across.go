// Package thirtythreeacross implements the 33Across bidder adapter
// P2-5: Refactored to use SimpleAdapter base to reduce duplication
package thirtythreeacross

import (
	"fmt"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
)

const (
	bidderCode      = "33across"
	defaultEndpoint = "https://ssc.33across.com/api/v1/s2s"
	gvlVendorID     = 58
)

// Adapter wraps SimpleAdapter for 33Across
type Adapter struct {
	*adapters.SimpleAdapter
}

// New creates a new 33Across adapter
func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{
		// Empty DefaultBidType means it will auto-detect from impression
		SimpleAdapter: adapters.NewSimpleAdapter(bidderCode, endpoint, ""),
	}
}

// Info returns 33Across bidder information
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled:     true,
		GVLVendorID: gvlVendorID,
		Endpoint:    defaultEndpoint,
		Maintainer:  &adapters.MaintainerInfo{Email: "headerbidding@33across.com"},
		Capabilities: &adapters.CapabilitiesInfo{
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner, adapters.BidTypeVideo}},
		},
	}
}

func init() {
	if err := adapters.RegisterAdapter(bidderCode, New(""), Info()); err != nil {
		panic(fmt.Sprintf("failed to register %s adapter: %v", bidderCode, err))
	}
}
