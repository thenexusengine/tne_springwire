package fpd

import (
	"strings"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// EIDFilter handles filtering of Extended IDs (EIDs) based on configuration
type EIDFilter struct {
	enabled        bool
	allowedSources map[string]bool
	allowAll       bool
}

// NewEIDFilter creates a new EID filter with the given configuration
func NewEIDFilter(config *Config) *EIDFilter {
	if config == nil {
		config = DefaultConfig()
	}

	filter := &EIDFilter{
		enabled:        config.EIDsEnabled,
		allowedSources: make(map[string]bool),
		allowAll:       len(config.EIDSources) == 0,
	}

	// Build allowed sources map for O(1) lookup
	for _, source := range config.EIDSources {
		source = strings.TrimSpace(strings.ToLower(source))
		if source != "" {
			filter.allowedSources[source] = true
		}
	}

	// If no sources configured, allow all
	if len(filter.allowedSources) == 0 {
		filter.allowAll = true
	}

	return filter
}

// FilterEIDs filters the user's EIDs based on allowed sources
// Returns a new slice containing only allowed EIDs
func (f *EIDFilter) FilterEIDs(eids []openrtb.EID) []openrtb.EID {
	if !f.enabled {
		return nil // EIDs disabled, return nothing
	}

	if f.allowAll {
		return eids // All sources allowed, return as-is
	}

	if len(eids) == 0 {
		return eids
	}

	filtered := make([]openrtb.EID, 0, len(eids))
	for _, eid := range eids {
		if f.isSourceAllowed(eid.Source) {
			filtered = append(filtered, eid)
		}
	}

	return filtered
}

// isSourceAllowed checks if an EID source is in the allowed list
func (f *EIDFilter) isSourceAllowed(source string) bool {
	if f.allowAll {
		return true
	}
	source = strings.TrimSpace(strings.ToLower(source))
	return f.allowedSources[source]
}

// FilterUserEIDs filters EIDs in a User object, modifying it in place
func (f *EIDFilter) FilterUserEIDs(user *openrtb.User) {
	if user == nil || !f.enabled {
		return
	}

	user.EIDs = f.FilterEIDs(user.EIDs)
}

// ProcessRequestEIDs filters EIDs in a bid request, returning a modified copy
// This is the main entry point for EID filtering
func (f *EIDFilter) ProcessRequestEIDs(req *openrtb.BidRequest) *openrtb.BidRequest {
	if req == nil || req.User == nil {
		return req
	}

	// Filter EIDs
	filteredEIDs := f.FilterEIDs(req.User.EIDs)
	req.User.EIDs = filteredEIDs

	return req
}

// GetAllowedSources returns the list of allowed EID sources
func (f *EIDFilter) GetAllowedSources() []string {
	if f.allowAll {
		return nil // nil indicates all sources allowed
	}

	sources := make([]string, 0, len(f.allowedSources))
	for source := range f.allowedSources {
		sources = append(sources, source)
	}
	return sources
}

// IsEnabled returns whether EID filtering is enabled
func (f *EIDFilter) IsEnabled() bool {
	return f.enabled
}

// AllowsAllSources returns whether all EID sources are allowed
func (f *EIDFilter) AllowsAllSources() bool {
	return f.allowAll
}

// CommonEIDSources contains well-known EID source domains
var CommonEIDSources = map[string]string{
	"liveramp.com":          "LiveRamp IdentityLink",
	"uidapi.com":            "Unified ID 2.0",
	"id5-sync.com":          "ID5",
	"criteo.com":            "Criteo",
	"pubcid.org":            "Publisher Common ID",
	"adserver.org":          "Advertising ID Consortium",
	"sharedid.org":          "SharedID",
	"intentiq.com":          "Intent IQ",
	"quantcast.com":         "Quantcast",
	"tapad.com":             "Tapad",
	"zeotap.com":            "Zeotap",
	"netid.de":              "NetID",
	"parrable.com":          "Parrable",
	"britepool.com":         "BritePool",
	"liveintent.com":        "LiveIntent",
	"admixer.net":           "Admixer",
	"amxrtb.com":            "AMX RTB",
	"deepintent.com":        "Deep Intent",
	"mediawallahscript.com": "MediaWallah",
}

// GetEIDSourceDescription returns a human-readable description for an EID source
func GetEIDSourceDescription(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	if desc, ok := CommonEIDSources[source]; ok {
		return desc
	}
	return source
}

// EIDStats tracks statistics about EID filtering
type EIDStats struct {
	TotalEIDs    int            `json:"total_eids"`
	FilteredEIDs int            `json:"filtered_eids"`
	AllowedEIDs  int            `json:"allowed_eids"`
	BySource     map[string]int `json:"by_source"`
}

// CollectEIDStats collects statistics about EIDs in a request
func (f *EIDFilter) CollectEIDStats(eids []openrtb.EID) *EIDStats {
	stats := &EIDStats{
		TotalEIDs: len(eids),
		BySource:  make(map[string]int),
	}

	for _, eid := range eids {
		source := strings.ToLower(eid.Source)
		stats.BySource[source]++

		if f.isSourceAllowed(eid.Source) {
			stats.AllowedEIDs++
		} else {
			stats.FilteredEIDs++
		}
	}

	return stats
}
