// Package usersync provides user ID synchronization for bidders
package usersync

import (
	"fmt"
	"net/url"
	"strings"
)

// SyncType represents the type of user sync
type SyncType string

const (
	// SyncTypeIframe uses an iframe to sync
	SyncTypeIframe SyncType = "iframe"
	// SyncTypeRedirect uses a redirect/pixel to sync
	SyncTypeRedirect SyncType = "redirect"
)

// SyncerConfig holds the sync configuration for a bidder
type SyncerConfig struct {
	// BidderCode is the bidder identifier
	BidderCode string
	// IframeSyncURL is the URL template for iframe syncs
	// Use {{gdpr}}, {{gdpr_consent}}, {{us_privacy}}, {{redirect_url}} as placeholders
	IframeSyncURL string
	// RedirectSyncURL is the URL template for redirect syncs
	RedirectSyncURL string
	// SupportCORS indicates if the bidder supports CORS for the sync
	SupportCORS bool
	// Enabled indicates if syncing is enabled for this bidder
	Enabled bool
}

// Syncer handles user sync URL generation for a bidder
type Syncer struct {
	config  SyncerConfig
	hostURL string // The PBS host URL for callbacks
}

// NewSyncer creates a new syncer for a bidder
func NewSyncer(config SyncerConfig, hostURL string) *Syncer {
	return &Syncer{
		config:  config,
		hostURL: strings.TrimSuffix(hostURL, "/"),
	}
}

// SyncInfo contains the sync URL and type for a bidder
type SyncInfo struct {
	URL    string   `json:"url"`
	Type   SyncType `json:"type"`
	Bidder string   `json:"bidder"`
}

// GetSync returns the sync info for this bidder
func (s *Syncer) GetSync(syncType SyncType, gdpr string, consent string, usPrivacy string) (*SyncInfo, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("syncing disabled for %s", s.config.BidderCode)
	}

	var urlTemplate string
	switch syncType {
	case SyncTypeIframe:
		urlTemplate = s.config.IframeSyncURL
	case SyncTypeRedirect:
		urlTemplate = s.config.RedirectSyncURL
	default:
		// Prefer redirect, fall back to iframe
		if s.config.RedirectSyncURL != "" {
			urlTemplate = s.config.RedirectSyncURL
			syncType = SyncTypeRedirect
		} else if s.config.IframeSyncURL != "" {
			urlTemplate = s.config.IframeSyncURL
			syncType = SyncTypeIframe
		} else {
			return nil, fmt.Errorf("no sync URL configured for %s", s.config.BidderCode)
		}
	}

	if urlTemplate == "" {
		return nil, fmt.Errorf("no %s sync URL for %s", syncType, s.config.BidderCode)
	}

	// Build the redirect URL (where bidder will send the UID)
	redirectURL := fmt.Sprintf("%s/setuid?bidder=%s&uid=$UID", s.hostURL, url.QueryEscape(s.config.BidderCode))

	// Replace placeholders
	syncURL := urlTemplate
	syncURL = strings.ReplaceAll(syncURL, "{{gdpr}}", gdpr)
	syncURL = strings.ReplaceAll(syncURL, "{{gdpr_consent}}", url.QueryEscape(consent))
	syncURL = strings.ReplaceAll(syncURL, "{{us_privacy}}", url.QueryEscape(usPrivacy))
	syncURL = strings.ReplaceAll(syncURL, "{{redirect_url}}", url.QueryEscape(redirectURL))

	return &SyncInfo{
		URL:    syncURL,
		Type:   syncType,
		Bidder: s.config.BidderCode,
	}, nil
}

// BidderCode returns the bidder code
func (s *Syncer) BidderCode() string {
	return s.config.BidderCode
}

// IsEnabled returns true if syncing is enabled
func (s *Syncer) IsEnabled() bool {
	return s.config.Enabled
}

// DefaultSyncerConfigs returns the default syncer configurations for common bidders
// These URLs are from the official Prebid documentation
func DefaultSyncerConfigs() map[string]SyncerConfig {
	return map[string]SyncerConfig{
		"appnexus": {
			BidderCode:      "appnexus",
			RedirectSyncURL: "https://ib.adnxs.com/getuid?{{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"rubicon": {
			BidderCode:      "rubicon",
			RedirectSyncURL: "https://pixel.rubiconproject.com/exchange/sync.php?p=prebid&gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}&redir={{redirect_url}}",
			IframeSyncURL:   "https://eus.rubiconproject.com/usync.html?p=prebid&gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"pubmatic": {
			BidderCode:      "pubmatic",
			RedirectSyncURL: "https://ads.pubmatic.com/AdServer/js/user_sync.html?gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}&predirect={{redirect_url}}",
			IframeSyncURL:   "https://ads.pubmatic.com/AdServer/js/user_sync.html?gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}&predirect={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"openx": {
			BidderCode:      "openx",
			RedirectSyncURL: "https://rtb.openx.net/sync/prebid?gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&r={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"triplelift": {
			BidderCode:      "triplelift",
			RedirectSyncURL: "https://eb2.3lift.com/sync?gdpr={{gdpr}}&cmp_cs={{gdpr_consent}}&us_privacy={{us_privacy}}&redir={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"ix": {
			BidderCode:      "ix",
			RedirectSyncURL: "https://ssum.casalemedia.com/usermatchredir?s=194962&gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}&cb={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"criteo": {
			BidderCode:      "criteo",
			RedirectSyncURL: "https://gum.criteo.com/syncframe?origin=prebidserver&gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}#{{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"sharethrough": {
			BidderCode:      "sharethrough",
			RedirectSyncURL: "https://match.sharethrough.com/FGMrCMMc/v1?redirectUri={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"sovrn": {
			BidderCode:      "sovrn",
			RedirectSyncURL: "https://ap.lijit.com/pixel?redir={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"33across": {
			BidderCode:      "33across",
			RedirectSyncURL: "https://ssc.33across.com/ps/?gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}&redir={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"gumgum": {
			BidderCode:      "gumgum",
			RedirectSyncURL: "https://rtb.gumgum.com/usync/prbds2s?gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}&r={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
		"medianet": {
			BidderCode:      "medianet",
			RedirectSyncURL: "https://csync.media.net/csync.php?gdpr={{gdpr}}&gdpr_consent={{gdpr_consent}}&us_privacy={{us_privacy}}&rurl={{redirect_url}}",
			SupportCORS:     true,
			Enabled:         true,
		},
	}
}
