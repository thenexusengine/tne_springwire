// Package teads implements the Teads bidder adapter (native/video specialist)
// P2-5: Refactored to use SimpleAdapter base to reduce duplication
package teads

import (
	"fmt"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
)

const (
	bidderCode      = "teads"
	defaultEndpoint = "https://a.teads.tv/prebid-server/bid-request"
	gvlVendorID     = 132
)

// Adapter wraps SimpleAdapter for Teads
type Adapter struct {
	*adapters.SimpleAdapter
}

// New creates a new Teads adapter
func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{
		SimpleAdapter: adapters.NewSimpleAdapter(bidderCode, endpoint, adapters.BidTypeVideo),
	}
}

// Info returns Teads bidder information
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled:     true,
		GVLVendorID: gvlVendorID,
		Endpoint:    defaultEndpoint,
		Maintainer:  &adapters.MaintainerInfo{Email: "prebid-support@teads.com"},
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
