// Package fpd provides First Party Data (FPD) processing for Prebid Server
package fpd

import "encoding/json"

// Config holds FPD processing configuration
type Config struct {
	Enabled             bool     `json:"enabled" yaml:"enabled"`
	SiteEnabled         bool     `json:"site_enabled" yaml:"site_enabled"`
	UserEnabled         bool     `json:"user_enabled" yaml:"user_enabled"`
	ImpEnabled          bool     `json:"imp_enabled" yaml:"imp_enabled"`
	GlobalEnabled       bool     `json:"global_enabled" yaml:"global_enabled"`
	BidderConfigEnabled bool     `json:"bidderconfig_enabled" yaml:"bidderconfig_enabled"`
	ContentEnabled      bool     `json:"content_enabled" yaml:"content_enabled"`
	EIDsEnabled         bool     `json:"eids_enabled" yaml:"eids_enabled"`
	EIDSources          []string `json:"eid_sources" yaml:"eid_sources"`
}

// DefaultConfig returns the default FPD configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:             true,
		SiteEnabled:         true,
		UserEnabled:         true,
		ImpEnabled:          true,
		GlobalEnabled:       false,
		BidderConfigEnabled: false,
		ContentEnabled:      true,
		EIDsEnabled:         true,
		EIDSources:          []string{"liveramp.com", "uidapi.com", "id5-sync.com", "criteo.com"},
	}
}

// PrebidExt represents the ext.prebid object in an OpenRTB request
type PrebidExt struct {
	Data          *PrebidData     `json:"data,omitempty"`
	BidderConfig  []BidderConfig  `json:"bidderconfig,omitempty"`
	BidderParams  json.RawMessage `json:"bidderparams,omitempty"`
	Channel       *Channel        `json:"channel,omitempty"`
	Debug         bool            `json:"debug,omitempty"`
	Targeting     json.RawMessage `json:"targeting,omitempty"`
	Cache         json.RawMessage `json:"cache,omitempty"`
	StoredRequest json.RawMessage `json:"storedrequest,omitempty"`
}

// PrebidData represents ext.prebid.data - global FPD to apply to all bidders
type PrebidData struct {
	Site json.RawMessage `json:"site,omitempty"`
	App  json.RawMessage `json:"app,omitempty"`
	User json.RawMessage `json:"user,omitempty"`
}

// BidderConfig represents an entry in ext.prebid.bidderconfig
type BidderConfig struct {
	Bidders []string   `json:"bidders"`
	Config  *FPDConfig `json:"config,omitempty"`
}

// FPDConfig represents the config object within bidderconfig
type FPDConfig struct {
	ORTB2 *ORTB2Config `json:"ortb2,omitempty"`
}

// ORTB2Config represents OpenRTB 2.x FPD data
type ORTB2Config struct {
	Site *SiteFPD `json:"site,omitempty"`
	App  *AppFPD  `json:"app,omitempty"`
	User *UserFPD `json:"user,omitempty"`
}

// SiteFPD represents site-level first party data
type SiteFPD struct {
	Name       string      `json:"name,omitempty"`
	Domain     string      `json:"domain,omitempty"`
	Cat        []string    `json:"cat,omitempty"`
	SectionCat []string    `json:"sectioncat,omitempty"`
	PageCat    []string    `json:"pagecat,omitempty"`
	Page       string      `json:"page,omitempty"`
	Ref        string      `json:"ref,omitempty"`
	Search     string      `json:"search,omitempty"`
	Keywords   string      `json:"keywords,omitempty"`
	Content    *ContentFPD `json:"content,omitempty"`
	Ext        *ExtData    `json:"ext,omitempty"`
}

// AppFPD represents app-level first party data
type AppFPD struct {
	Name     string      `json:"name,omitempty"`
	Bundle   string      `json:"bundle,omitempty"`
	Domain   string      `json:"domain,omitempty"`
	StoreURL string      `json:"storeurl,omitempty"`
	Cat      []string    `json:"cat,omitempty"`
	Keywords string      `json:"keywords,omitempty"`
	Content  *ContentFPD `json:"content,omitempty"`
	Ext      *ExtData    `json:"ext,omitempty"`
}

// UserFPD represents user-level first party data
type UserFPD struct {
	YOB      int           `json:"yob,omitempty"`
	Gender   string        `json:"gender,omitempty"`
	Keywords string        `json:"keywords,omitempty"`
	Data     []DataSegment `json:"data,omitempty"`
	Ext      *ExtData      `json:"ext,omitempty"`
}

// ContentFPD represents content first party data
type ContentFPD struct {
	ID       string   `json:"id,omitempty"`
	Title    string   `json:"title,omitempty"`
	Series   string   `json:"series,omitempty"`
	Season   string   `json:"season,omitempty"`
	Episode  int      `json:"episode,omitempty"`
	Cat      []string `json:"cat,omitempty"`
	Genre    string   `json:"genre,omitempty"`
	Keywords string   `json:"keywords,omitempty"`
	Language string   `json:"language,omitempty"`
}

// ExtData represents the ext.data object for FPD
type ExtData struct {
	Data json.RawMessage `json:"data,omitempty"`
}

// DataSegment represents a data segment in user.data
type DataSegment struct {
	ID      string    `json:"id,omitempty"`
	Name    string    `json:"name,omitempty"`
	Segment []Segment `json:"segment,omitempty"`
}

// Segment represents an individual segment
type Segment struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// Channel represents ext.prebid.channel
type Channel struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// ResolvedFPD contains the final FPD to apply for a specific bidder
type ResolvedFPD struct {
	Site json.RawMessage
	App  json.RawMessage
	User json.RawMessage
	Imp  map[string]json.RawMessage // keyed by imp ID
}

// BidderFPD maps bidder codes to their resolved FPD
type BidderFPD map[string]*ResolvedFPD

// ConfigFromIDR creates an FPD Config from IDR service response fields
func ConfigFromIDR(enabled, siteEnabled, userEnabled, impEnabled, globalEnabled, bidderConfigEnabled, contentEnabled, eidsEnabled bool, eidSources string) *Config {
	sources := ParseEIDSources(eidSources)
	return &Config{
		Enabled:             enabled,
		SiteEnabled:         siteEnabled,
		UserEnabled:         userEnabled,
		ImpEnabled:          impEnabled,
		GlobalEnabled:       globalEnabled,
		BidderConfigEnabled: bidderConfigEnabled,
		ContentEnabled:      contentEnabled,
		EIDsEnabled:         eidsEnabled,
		EIDSources:          sources,
	}
}

// ParseEIDSources parses a comma-separated string of EID sources
func ParseEIDSources(sources string) []string {
	if sources == "" {
		return nil
	}

	var result []string
	for _, s := range splitAndTrim(sources, ",") {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// splitAndTrim splits a string and trims whitespace from each part
func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}

	parts := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			part := trimSpace(s[start:i])
			if part != "" {
				parts = append(parts, part)
			}
			start = i + 1
		}
	}
	// Don't forget the last part
	part := trimSpace(s[start:])
	if part != "" {
		parts = append(parts, part)
	}
	return parts
}

// trimSpace removes leading and trailing whitespace
func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
