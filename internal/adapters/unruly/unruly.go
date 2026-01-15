// Package unruly implements the Unruly bidder adapter (video specialist)
// P2-5: Refactored to use SimpleAdapter base to reduce duplication
package unruly

import (
	"fmt"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
)

const (
	bidderCode      = "unruly"
	defaultEndpoint = "https://targeting.unrulymedia.com/openrtb/2.2"
	gvlVendorID     = 36
)

// Adapter wraps SimpleAdapter for Unruly
type Adapter struct {
	*adapters.SimpleAdapter
}

// New creates a new Unruly adapter
func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{
		SimpleAdapter: adapters.NewSimpleAdapter(bidderCode, endpoint, adapters.BidTypeVideo),
	}
}

// Info returns Unruly bidder information
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled:     true,
		GVLVendorID: gvlVendorID,
		Endpoint:    defaultEndpoint,
		Maintainer:  &adapters.MaintainerInfo{Email: "prebid@unruly.co"},
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
