package endpoints

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/thenexusengine/tne_springwire/internal/usersync"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// CookieSyncRequest is the request body for /cookie_sync
type CookieSyncRequest struct {
	// Bidders is the list of bidders to sync (empty = all configured bidders)
	Bidders []string `json:"bidders,omitempty"`
	// GDPR indicates if GDPR applies (0 = no, 1 = yes)
	GDPR int `json:"gdpr,omitempty"`
	// GDPRConsent is the TCF consent string
	GDPRConsent string `json:"gdpr_consent,omitempty"`
	// USPrivacy is the CCPA/US Privacy string
	USPrivacy string `json:"us_privacy,omitempty"`
	// Limit is the max number of syncs to return (default 8)
	Limit int `json:"limit,omitempty"`
	// CooperativeSync enables syncing for bidders not in the request
	CooperativeSync bool `json:"coopSync,omitempty"`
	// FilterSettings controls which sync types to use
	FilterSettings *FilterSettings `json:"filterSettings,omitempty"`
}

// FilterSettings controls sync type filtering
type FilterSettings struct {
	Iframe   *FilterConfig `json:"iframe,omitempty"`
	Redirect *FilterConfig `json:"image,omitempty"` // Called "image" in Prebid spec
}

// FilterConfig is a filter for a sync type
type FilterConfig struct {
	Bidders string   `json:"bidders,omitempty"` // "include" or "exclude"
	Filter  []string `json:"filter,omitempty"`  // List of bidder codes
}

// CookieSyncResponse is the response body for /cookie_sync
type CookieSyncResponse struct {
	Status       string             `json:"status"`
	BidderStatus []BidderSyncStatus `json:"bidder_status,omitempty"`
}

// BidderSyncStatus is the sync status for a single bidder
type BidderSyncStatus struct {
	Bidder   string             `json:"bidder"`
	NoCookie bool               `json:"no_cookie,omitempty"`
	UserSync *usersync.SyncInfo `json:"usersync,omitempty"`
	Error    string             `json:"error,omitempty"`
}

// CookieSyncHandler handles cookie sync requests
type CookieSyncHandler struct {
	syncers  map[string]*usersync.Syncer
	hostURL  string
	maxSyncs int
}

// CookieSyncConfig holds configuration for the cookie sync handler
type CookieSyncConfig struct {
	HostURL     string
	MaxSyncs    int
	SyncConfigs map[string]usersync.SyncerConfig
}

// DefaultCookieSyncConfig returns default configuration
func DefaultCookieSyncConfig(hostURL string) *CookieSyncConfig {
	return &CookieSyncConfig{
		HostURL:     hostURL,
		MaxSyncs:    8,
		SyncConfigs: usersync.DefaultSyncerConfigs(),
	}
}

// NewCookieSyncHandler creates a new cookie sync handler
func NewCookieSyncHandler(config *CookieSyncConfig) *CookieSyncHandler {
	syncers := make(map[string]*usersync.Syncer)

	for code, syncConfig := range config.SyncConfigs {
		syncers[code] = usersync.NewSyncer(syncConfig, config.HostURL)
	}

	return &CookieSyncHandler{
		syncers:  syncers,
		hostURL:  config.HostURL,
		maxSyncs: config.MaxSyncs,
	}
}

// ServeHTTP handles the /cookie_sync endpoint
func (h *CookieSyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only POST is allowed
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req CookieSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Empty body is OK - use defaults
		req = CookieSyncRequest{}
	}

	// Set defaults
	if req.Limit <= 0 || req.Limit > h.maxSyncs {
		req.Limit = h.maxSyncs
	}

	// Parse existing cookie to see what's already synced
	cookie := usersync.ParseCookie(r)

	// Check for opt-out
	if cookie.IsOptOut() {
		h.respondJSON(w, CookieSyncResponse{Status: "ok"})
		return
	}

	// Determine which bidders to sync
	biddersToSync := h.getBiddersToSync(req, cookie)

	// Build response
	response := CookieSyncResponse{
		Status:       "ok",
		BidderStatus: make([]BidderSyncStatus, 0, len(biddersToSync)),
	}

	// GDPR string for sync URLs
	gdprStr := "0"
	if req.GDPR == 1 {
		gdprStr = "1"
	}

	syncCount := 0
	for _, bidderCode := range biddersToSync {
		if syncCount >= req.Limit {
			break
		}

		syncer, ok := h.syncers[strings.ToLower(bidderCode)]
		if !ok {
			response.BidderStatus = append(response.BidderStatus, BidderSyncStatus{
				Bidder: bidderCode,
				Error:  "unsupported bidder",
			})
			continue
		}

		if !syncer.IsEnabled() {
			continue
		}

		// Check if already synced
		if cookie.HasUID(bidderCode) {
			continue
		}

		// Determine sync type based on filterSettings
		syncType := h.getSyncTypeForBidder(bidderCode, req.FilterSettings)
		if syncType == usersync.SyncType("") {
			// Bidder filtered out by filterSettings
			continue
		}

		// Get sync URL
		syncInfo, err := syncer.GetSync(syncType, gdprStr, req.GDPRConsent, req.USPrivacy)
		if err != nil {
			logger.Log.Debug().Err(err).Str("bidder", bidderCode).Msg("Failed to get sync URL")
			response.BidderStatus = append(response.BidderStatus, BidderSyncStatus{
				Bidder: bidderCode,
				Error:  err.Error(),
			})
			continue
		}

		response.BidderStatus = append(response.BidderStatus, BidderSyncStatus{
			Bidder:   bidderCode,
			NoCookie: true,
			UserSync: syncInfo,
		})
		syncCount++
	}

	// Set cookie
	if httpCookie, err := cookie.ToHTTPCookie(h.getCookieDomain(r)); err == nil {
		http.SetCookie(w, httpCookie)
	}

	h.respondJSON(w, response)
}

// getSyncTypeForBidder determines the sync type for a bidder based on filterSettings
// Returns empty string if the bidder should be filtered out
func (h *CookieSyncHandler) getSyncTypeForBidder(bidderCode string, filterSettings *FilterSettings) usersync.SyncType {
	if filterSettings == nil {
		// No filter settings - default to redirect
		return usersync.SyncTypeRedirect
	}

	// Try iframe first (preferred for better sync rates)
	if filterSettings.Iframe != nil {
		if h.shouldIncludeBidder(bidderCode, filterSettings.Iframe) {
			return usersync.SyncTypeIframe
		}
	}

	// Try redirect as fallback
	if filterSettings.Redirect != nil {
		if h.shouldIncludeBidder(bidderCode, filterSettings.Redirect) {
			return usersync.SyncTypeRedirect
		}
	}

	// If filterSettings is provided but bidder doesn't match any filter, default to redirect
	// This matches Prebid.js behavior where filterSettings is advisory, not restrictive
	return usersync.SyncTypeRedirect
}

// shouldIncludeBidder checks if a bidder passes the filter configuration
func (h *CookieSyncHandler) shouldIncludeBidder(bidderCode string, config *FilterConfig) bool {
	if config == nil || len(config.Filter) == 0 {
		return true // No filter = include all
	}

	bidderInList := h.containsBidder(config.Filter, bidderCode)

	if config.Bidders == "include" {
		return bidderInList // Include only if in list
	} else if config.Bidders == "exclude" {
		return !bidderInList // Exclude if in list
	}

	// Unknown mode - default to include
	return true
}

// containsBidder checks if a bidder code is in a list (case-insensitive)
func (h *CookieSyncHandler) containsBidder(list []string, bidder string) bool {
	bidderLower := strings.ToLower(bidder)
	for _, b := range list {
		if strings.ToLower(b) == bidderLower {
			return true
		}
	}
	return false
}

// getBiddersToSync determines which bidders need syncing
func (h *CookieSyncHandler) getBiddersToSync(req CookieSyncRequest, cookie *usersync.Cookie) []string {
	var bidders []string

	if len(req.Bidders) > 0 {
		// Use requested bidders
		bidders = req.Bidders
	} else if req.CooperativeSync {
		// Sync all configured bidders
		for code := range h.syncers {
			bidders = append(bidders, code)
		}
	} else {
		// No bidders specified and no coop sync - return common bidders
		bidders = []string{"appnexus", "rubicon", "pubmatic", "openx", "triplelift"}
	}

	// Filter out bidders that already have UIDs (optimization to avoid redundant syncs)
	if cookie != nil {
		needsSync := make([]string, 0, len(bidders))
		for _, bidder := range bidders {
			if !cookie.HasUID(bidder) {
				needsSync = append(needsSync, bidder)
			}
		}
		return needsSync
	}

	return bidders
}

// getCookieDomain extracts the domain for cookies
func (h *CookieSyncHandler) getCookieDomain(r *http.Request) string {
	host := r.Host
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

// respondJSON writes a JSON response
func (h *CookieSyncHandler) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Log.Error().Err(err).Msg("Failed to encode cookie sync response")
	}
}

// AddSyncer adds a syncer for a bidder
func (h *CookieSyncHandler) AddSyncer(config usersync.SyncerConfig) {
	h.syncers[strings.ToLower(config.BidderCode)] = usersync.NewSyncer(config, h.hostURL)
}

// ListBidders returns all configured bidder codes
func (h *CookieSyncHandler) ListBidders() []string {
	bidders := make([]string, 0, len(h.syncers))
	for code := range h.syncers {
		bidders = append(bidders, code)
	}
	return bidders
}
