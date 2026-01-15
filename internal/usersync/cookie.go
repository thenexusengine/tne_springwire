// Package usersync provides user ID synchronization for bidders
package usersync

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// UID represents a single user ID for a bidder
type UID struct {
	UID     string    `json:"uid"`
	Expires time.Time `json:"expires"`
}

// Cookie holds all bidder user IDs
type Cookie struct {
	UIDs    map[string]UID `json:"uids"`
	OptOut  bool           `json:"optout,omitempty"`
	Created time.Time      `json:"created"`
	mu      sync.RWMutex
}

const (
	// CookieName is the name of the user sync cookie
	CookieName = "uids"
	// DefaultTTL is the default cookie TTL (90 days)
	DefaultTTL = 90 * 24 * time.Hour
	// MaxCookieSize is the maximum cookie size in bytes
	MaxCookieSize = 4000
)

// NewCookie creates a new empty cookie
func NewCookie() *Cookie {
	return &Cookie{
		UIDs:    make(map[string]UID),
		Created: time.Now().UTC(),
	}
}

// ParseCookie parses a cookie from an HTTP request
func ParseCookie(r *http.Request) *Cookie {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return NewCookie()
	}

	// Decode base64
	decoded, err := base64.URLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return NewCookie()
	}

	var c Cookie
	if err := json.Unmarshal(decoded, &c); err != nil {
		return NewCookie()
	}

	if c.UIDs == nil {
		c.UIDs = make(map[string]UID)
	}

	// Clean expired UIDs
	c.cleanExpired()

	return &c
}

// GetUID returns the UID for a bidder, or empty string if not found/expired
func (c *Cookie) GetUID(bidderCode string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	uid, ok := c.UIDs[bidderCode]
	if !ok {
		return ""
	}

	if time.Now().After(uid.Expires) {
		return ""
	}

	return uid.UID
}

// SetUID sets the UID for a bidder
func (c *Cookie) SetUID(bidderCode, uid string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.UIDs[bidderCode] = UID{
		UID:     uid,
		Expires: time.Now().Add(DefaultTTL),
	}
}

// DeleteUID removes the UID for a bidder
func (c *Cookie) DeleteUID(bidderCode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.UIDs, bidderCode)
}

// HasUID returns true if a valid UID exists for the bidder
func (c *Cookie) HasUID(bidderCode string) bool {
	return c.GetUID(bidderCode) != ""
}

// SyncCount returns the number of synced bidders
func (c *Cookie) SyncCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	now := time.Now()
	for _, uid := range c.UIDs {
		if now.Before(uid.Expires) {
			count++
		}
	}
	return count
}

// SetOptOut marks the user as opted out
func (c *Cookie) SetOptOut(optOut bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.OptOut = optOut
	if optOut {
		c.UIDs = make(map[string]UID)
	}
}

// IsOptOut returns true if user has opted out
func (c *Cookie) IsOptOut() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.OptOut
}

// cleanExpired removes expired UIDs
func (c *Cookie) cleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for bidder, uid := range c.UIDs {
		if now.After(uid.Expires) {
			delete(c.UIDs, bidder)
		}
	}
}

// ToHTTPCookie converts to an http.Cookie for setting in response
// Note: Uses Lock() instead of RLock() because trimToFit() may modify c.UIDs
func (c *Cookie) ToHTTPCookie(domain string) (*http.Cookie, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	encoded := base64.URLEncoding.EncodeToString(data)

	// Check size limit
	if len(encoded) > MaxCookieSize {
		// Trim oldest UIDs to fit
		c.trimToFit()
		if data, err := json.Marshal(c); err == nil {
			encoded = base64.URLEncoding.EncodeToString(data)
		}
	}

	return &http.Cookie{
		Name:     CookieName,
		Value:    encoded,
		Path:     "/",
		Domain:   domain,
		Expires:  time.Now().Add(DefaultTTL),
		MaxAge:   int(DefaultTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
	}, nil
}

// trimToFit removes oldest UIDs to fit within cookie size limit
func (c *Cookie) trimToFit() {
	// Simple approach: remove UIDs with earliest expiry until we fit
	for len(c.UIDs) > 0 {
		data, err := json.Marshal(c)
		if err != nil {
			break // Can't check size if marshal fails
		}
		encoded := base64.URLEncoding.EncodeToString(data)
		if len(encoded) <= MaxCookieSize {
			break
		}

		// Find and remove oldest
		var oldestBidder string
		var oldestTime time.Time
		for bidder, uid := range c.UIDs {
			if oldestBidder == "" || uid.Expires.Before(oldestTime) {
				oldestBidder = bidder
				oldestTime = uid.Expires
			}
		}
		if oldestBidder != "" {
			delete(c.UIDs, oldestBidder)
		}
	}
}

// GetAllUIDs returns a copy of all valid UIDs
func (c *Cookie) GetAllUIDs() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]string)
	now := time.Now()
	for bidder, uid := range c.UIDs {
		if now.Before(uid.Expires) {
			result[bidder] = uid.UID
		}
	}
	return result
}
