// Package taboola implements the Taboola bidder adapter (native specialist)
// P2-5: Refactored to use SimpleAdapter base to reduce duplication
package taboola

import (
	"fmt"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
)

const (
	bidderCode      = "taboola"
	defaultEndpoint = "https://prebid-server.production.taboolasyndication.com/openrtb/2.5"
	gvlVendorID     = 42
)

// Adapter wraps SimpleAdapter for Taboola
type Adapter struct {
	*adapters.SimpleAdapter
}

// New creates a new Taboola adapter
func New(endpoint string) *Adapter {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Adapter{
		SimpleAdapter: adapters.NewSimpleAdapter(bidderCode, endpoint, adapters.BidTypeNative),
	}
}

// Info returns Taboola bidder information
func Info() adapters.BidderInfo {
	return adapters.BidderInfo{
		Enabled:     true,
		GVLVendorID: gvlVendorID,
		Endpoint:    defaultEndpoint,
		Maintainer:  &adapters.MaintainerInfo{Email: "prebid.prebid@taboola.com"},
		Capabilities: &adapters.CapabilitiesInfo{
			Site: &adapters.PlatformInfo{MediaTypes: []adapters.BidType{adapters.BidTypeBanner, adapters.BidTypeNative}},
		},
	}
}

func init() {
	if err := adapters.RegisterAdapter(bidderCode, New(""), Info()); err != nil {
		panic(fmt.Sprintf("failed to register %s adapter: %v", bidderCode, err))
	}
}
