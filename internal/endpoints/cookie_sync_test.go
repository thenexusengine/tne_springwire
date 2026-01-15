package endpoints

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/usersync"
)

func TestDefaultCookieSyncConfig(t *testing.T) {
	hostURL := "https://example.com"
	config := DefaultCookieSyncConfig(hostURL)

	if config.HostURL != hostURL {
		t.Errorf("expected HostURL %s, got %s", hostURL, config.HostURL)
	}

	if config.MaxSyncs != 8 {
		t.Errorf("expected MaxSyncs 8, got %d", config.MaxSyncs)
	}

	if len(config.SyncConfigs) == 0 {
		t.Error("expected default sync configs to be populated")
	}
}

func TestNewCookieSyncHandler(t *testing.T) {
	config := &CookieSyncConfig{
		HostURL:  "https://example.com",
		MaxSyncs: 10,
		SyncConfigs: map[string]usersync.SyncerConfig{
			"appnexus": {
				BidderCode:      "appnexus",
				RedirectSyncURL: "https://ib.adnxs.com/getuid?{{redirect_url}}",
				Enabled:         true,
			},
			"rubicon": {
				BidderCode:      "rubicon",
				RedirectSyncURL: "https://pixel.rubiconproject.com/sync?{{redirect_url}}",
				Enabled:         true,
			},
		},
	}

	handler := NewCookieSyncHandler(config)

	if handler.hostURL != config.HostURL {
		t.Errorf("expected hostURL %s, got %s", config.HostURL, handler.hostURL)
	}

	if handler.maxSyncs != config.MaxSyncs {
		t.Errorf("expected maxSyncs %d, got %d", config.MaxSyncs, handler.maxSyncs)
	}

	if len(handler.syncers) != 2 {
		t.Errorf("expected 2 syncers, got %d", len(handler.syncers))
	}

	if handler.syncers["appnexus"] == nil {
		t.Error("expected appnexus syncer to be created")
	}

	if handler.syncers["rubicon"] == nil {
		t.Error("expected rubicon syncer to be created")
	}
}

func TestCookieSyncHandler_NonPOST(t *testing.T) {
	handler := createTestHandler()

	req := httptest.NewRequest("GET", "/cookie_sync", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestCookieSyncHandler_EmptyBody(t *testing.T) {
	handler := createTestHandler()

	req := httptest.NewRequest("POST", "/cookie_sync", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}

	// With empty body, should get default bidders
	if len(resp.BidderStatus) == 0 {
		t.Error("expected bidder status for default bidders")
	}
}

func TestCookieSyncHandler_WithOptOut(t *testing.T) {
	handler := createTestHandler()

	// Create a request with opt-out cookie
	req := httptest.NewRequest("POST", "/cookie_sync", nil)

	cookie := usersync.NewCookie()
	cookie.SetOptOut(true)
	httpCookie, _ := cookie.ToHTTPCookie("example.com")
	req.AddCookie(httpCookie)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}

	// With opt-out, should not return any bidder status
	if len(resp.BidderStatus) != 0 {
		t.Errorf("expected no bidder status with opt-out, got %d", len(resp.BidderStatus))
	}
}

func TestCookieSyncHandler_SpecificBidders(t *testing.T) {
	handler := createTestHandler()

	reqBody := CookieSyncRequest{
		Bidders: []string{"appnexus", "rubicon"},
		Limit:   10,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/cookie_sync", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}

	// Should get syncs for both requested bidders
	if len(resp.BidderStatus) != 2 {
		t.Errorf("expected 2 bidder statuses, got %d", len(resp.BidderStatus))
	}

	// Verify bidders are correct
	bidders := make(map[string]bool)
	for _, bs := range resp.BidderStatus {
		bidders[bs.Bidder] = true
	}

	if !bidders["appnexus"] {
		t.Error("expected appnexus in response")
	}
	if !bidders["rubicon"] {
		t.Error("expected rubicon in response")
	}
}

func TestCookieSyncHandler_CooperativeSync(t *testing.T) {
	handler := createTestHandler()

	reqBody := CookieSyncRequest{
		CooperativeSync: true,
		Limit:           10,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/cookie_sync", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// With coop sync, should get all configured bidders
	if len(resp.BidderStatus) == 0 {
		t.Error("expected bidder statuses with cooperative sync")
	}
}

func TestCookieSyncHandler_Limit(t *testing.T) {
	handler := createTestHandler()

	reqBody := CookieSyncRequest{
		CooperativeSync: true,
		Limit:           2, // Limit to 2 syncs
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/cookie_sync", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should respect limit
	if len(resp.BidderStatus) > 2 {
		t.Errorf("expected max 2 bidder statuses, got %d", len(resp.BidderStatus))
	}
}

func TestCookieSyncHandler_AlreadySynced(t *testing.T) {
	handler := createTestHandler()

	// Create cookie with already synced bidder
	cookie := usersync.NewCookie()
	cookie.SetUID("appnexus", "existing-uid")

	httpCookie, _ := cookie.ToHTTPCookie("example.com")

	reqBody := CookieSyncRequest{
		Bidders: []string{"appnexus", "rubicon"},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/cookie_sync", bytes.NewReader(body))
	req.AddCookie(httpCookie)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should only get sync for rubicon (appnexus already synced)
	if len(resp.BidderStatus) != 1 {
		t.Errorf("expected 1 bidder status, got %d", len(resp.BidderStatus))
	}

	if len(resp.BidderStatus) > 0 && resp.BidderStatus[0].Bidder != "rubicon" {
		t.Errorf("expected rubicon, got %s", resp.BidderStatus[0].Bidder)
	}
}

func TestCookieSyncHandler_UnsupportedBidder(t *testing.T) {
	handler := createTestHandler()

	reqBody := CookieSyncRequest{
		Bidders: []string{"unsupported"},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/cookie_sync", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.BidderStatus) != 1 {
		t.Fatalf("expected 1 bidder status, got %d", len(resp.BidderStatus))
	}

	if resp.BidderStatus[0].Error != "unsupported bidder" {
		t.Errorf("expected 'unsupported bidder' error, got %s", resp.BidderStatus[0].Error)
	}
}

func TestCookieSyncHandler_GDPR(t *testing.T) {
	handler := createTestHandler()

	reqBody := CookieSyncRequest{
		Bidders:     []string{"appnexus"},
		GDPR:        1,
		GDPRConsent: "consent-string",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/cookie_sync", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.BidderStatus) == 0 {
		t.Fatal("expected at least one bidder status")
	}

	// Check that sync URL includes GDPR parameters would require checking the actual URL
	if resp.BidderStatus[0].UserSync == nil {
		t.Error("expected user sync info")
	}
}

func TestGetBiddersToSync_SpecificBidders(t *testing.T) {
	handler := createTestHandler()
	cookie := usersync.NewCookie()

	req := CookieSyncRequest{
		Bidders: []string{"appnexus", "rubicon"},
	}

	bidders := handler.getBiddersToSync(req, cookie)

	if len(bidders) != 2 {
		t.Errorf("expected 2 bidders, got %d", len(bidders))
	}

	if bidders[0] != "appnexus" || bidders[1] != "rubicon" {
		t.Errorf("expected [appnexus, rubicon], got %v", bidders)
	}
}

func TestGetBiddersToSync_CooperativeSync(t *testing.T) {
	handler := createTestHandler()
	cookie := usersync.NewCookie()

	req := CookieSyncRequest{
		CooperativeSync: true,
	}

	bidders := handler.getBiddersToSync(req, cookie)

	// Should return all configured bidders
	if len(bidders) == 0 {
		t.Error("expected bidders with cooperative sync")
	}
}

func TestGetBiddersToSync_DefaultBidders(t *testing.T) {
	handler := createTestHandler()
	cookie := usersync.NewCookie()

	req := CookieSyncRequest{}

	bidders := handler.getBiddersToSync(req, cookie)

	// Should return default common bidders
	expectedDefaults := []string{"appnexus", "rubicon", "pubmatic", "openx", "triplelift"}
	if len(bidders) != len(expectedDefaults) {
		t.Errorf("expected %d default bidders, got %d", len(expectedDefaults), len(bidders))
	}
}

func TestGetCookieDomain(t *testing.T) {
	handler := createTestHandler()

	tests := []struct {
		host     string
		expected string
	}{
		{"example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"sub.example.com", "sub.example.com"},
		{"sub.example.com:443", "sub.example.com"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = tt.host

		domain := handler.getCookieDomain(req)
		if domain != tt.expected {
			t.Errorf("for host %s, expected domain %s, got %s", tt.host, tt.expected, domain)
		}
	}
}

func TestAddSyncer(t *testing.T) {
	handler := createTestHandler()

	initialCount := len(handler.syncers)

	newConfig := usersync.SyncerConfig{
		BidderCode:      "newssp",
		RedirectSyncURL: "https://newssp.com/sync?{{redirect_url}}",
		Enabled:         true,
	}

	handler.AddSyncer(newConfig)

	if len(handler.syncers) != initialCount+1 {
		t.Errorf("expected %d syncers after adding, got %d", initialCount+1, len(handler.syncers))
	}

	if handler.syncers["newssp"] == nil {
		t.Error("expected newssp syncer to be added")
	}
}

func TestAddSyncer_CaseInsensitive(t *testing.T) {
	handler := createTestHandler()

	newConfig := usersync.SyncerConfig{
		BidderCode:      "NewSSP", // Mixed case
		RedirectSyncURL: "https://newssp.com/sync?{{redirect_url}}",
		Enabled:         true,
	}

	handler.AddSyncer(newConfig)

	// Should be stored as lowercase
	if handler.syncers["newssp"] == nil {
		t.Error("expected newssp syncer to be stored as lowercase")
	}
}

func TestListBidders(t *testing.T) {
	handler := createTestHandler()

	bidders := handler.ListBidders()

	if len(bidders) == 0 {
		t.Error("expected bidders to be returned")
	}

	// Verify known bidders are in the list
	hasBidder := func(name string) bool {
		for _, b := range bidders {
			if b == name {
				return true
			}
		}
		return false
	}

	if !hasBidder("appnexus") {
		t.Error("expected appnexus in bidder list")
	}

	if !hasBidder("rubicon") {
		t.Error("expected rubicon in bidder list")
	}
}

func TestCookieSyncHandler_ResponseContentType(t *testing.T) {
	handler := createTestHandler()

	req := httptest.NewRequest("POST", "/cookie_sync", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected Content-Type to contain application/json, got %s", contentType)
	}
}

func TestCookieSyncHandler_SetsCookie(t *testing.T) {
	handler := createTestHandler()

	reqBody := CookieSyncRequest{
		Bidders: []string{"appnexus"},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/cookie_sync", bytes.NewReader(body))
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	result := w.Result()
	defer result.Body.Close()
	cookies := result.Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == usersync.CookieName {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected uids cookie to be set in response")
	}
}

func TestCookieSyncHandler_MaxSyncsExceedsConfig(t *testing.T) {
	handler := createTestHandler()

	reqBody := CookieSyncRequest{
		CooperativeSync: true,
		Limit:           100, // Exceeds max
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/cookie_sync", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp CookieSyncResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should be capped at handler.maxSyncs
	if len(resp.BidderStatus) > handler.maxSyncs {
		t.Errorf("expected max %d bidder statuses, got %d", handler.maxSyncs, len(resp.BidderStatus))
	}
}

// createTestHandler creates a handler with test configuration
func createTestHandler() *CookieSyncHandler {
	config := &CookieSyncConfig{
		HostURL:  "https://test.example.com",
		MaxSyncs: 8,
		SyncConfigs: map[string]usersync.SyncerConfig{
			"appnexus": {
				BidderCode:      "appnexus",
				RedirectSyncURL: "https://ib.adnxs.com/getuid?{{redirect_url}}",
				Enabled:         true,
			},
			"rubicon": {
				BidderCode:      "rubicon",
				RedirectSyncURL: "https://pixel.rubiconproject.com/sync?{{redirect_url}}",
				Enabled:         true,
			},
			"pubmatic": {
				BidderCode:      "pubmatic",
				RedirectSyncURL: "https://ads.pubmatic.com/sync?{{redirect_url}}",
				Enabled:         true,
			},
			"openx": {
				BidderCode:      "openx",
				RedirectSyncURL: "https://rtb.openx.net/sync?{{redirect_url}}",
				Enabled:         true,
			},
			"triplelift": {
				BidderCode:      "triplelift",
				RedirectSyncURL: "https://eb2.3lift.com/sync?{{redirect_url}}",
				Enabled:         true,
			},
		},
	}
	return NewCookieSyncHandler(config)
}
