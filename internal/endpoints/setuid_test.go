package endpoints

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/usersync"
)

func TestNewSetUIDHandler(t *testing.T) {
	bidders := []string{"AppNexus", "Rubicon", "PubMatic"}
	handler := NewSetUIDHandler(bidders)

	if handler == nil {
		t.Fatal("Expected handler to be created")
	}

	// Check bidders are stored in lowercase
	if !handler.validBidders["appnexus"] {
		t.Error("Expected appnexus to be valid")
	}
	if !handler.validBidders["rubicon"] {
		t.Error("Expected rubicon to be valid")
	}
	if !handler.validBidders["pubmatic"] {
		t.Error("Expected pubmatic to be valid")
	}
}

func TestSetUIDHandler_MissingBidder(t *testing.T) {
	handler := NewSetUIDHandler([]string{"appnexus"})

	req := httptest.NewRequest("GET", "/setuid?uid=test123", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "missing bidder parameter") {
		t.Error("Expected missing bidder error message")
	}
}

func TestSetUIDHandler_ValidUID(t *testing.T) {
	handler := NewSetUIDHandler([]string{"appnexus"})

	req := httptest.NewRequest("GET", "/setuid?bidder=appnexus&uid=user123", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check GIF response
	if w.Header().Get("Content-Type") != "image/gif" {
		t.Errorf("Expected image/gif content type, got %s", w.Header().Get("Content-Type"))
	}

	// Check cache headers
	if w.Header().Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
		t.Error("Expected no-cache header")
	}

	// Check cookie was set
	result := w.Result()
	defer result.Body.Close()
	cookies := result.Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected cookie to be set")
	}

	cookie := cookies[0]
	if cookie.Name != "uids" {
		t.Errorf("Expected cookie name 'uids', got %s", cookie.Name)
	}

	// Verify UID was stored in cookie
	decoded, err := base64.URLEncoding.DecodeString(cookie.Value)
	if err != nil {
		t.Fatalf("Failed to decode cookie: %v", err)
	}
	if !strings.Contains(string(decoded), "user123") {
		t.Error("Expected UID to be stored in cookie")
	}
}

func TestSetUIDHandler_EmptyUID(t *testing.T) {
	handler := NewSetUIDHandler([]string{"appnexus"})

	// First set a UID
	req1 := httptest.NewRequest("GET", "/setuid?bidder=appnexus&uid=user123", nil)
	req1.Host = "example.com"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// Get the cookie from first request
	result1 := w1.Result()
	defer result1.Body.Close()
	cookie1 := result1.Cookies()[0]

	// Now send empty UID to delete it
	req2 := httptest.NewRequest("GET", "/setuid?bidder=appnexus&uid=", nil)
	req2.Host = "example.com"
	req2.AddCookie(cookie1)
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w2.Code)
	}

	// Verify GIF response
	if w2.Header().Get("Content-Type") != "image/gif" {
		t.Error("Expected GIF response")
	}
}

func TestSetUIDHandler_PlaceholderUID(t *testing.T) {
	handler := NewSetUIDHandler([]string{"appnexus"})

	testCases := []string{"$UID", "0"}

	for _, uid := range testCases {
		t.Run(uid, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/setuid?bidder=appnexus&uid="+uid, nil)
			req.Host = "example.com"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			// Should return pixel but not store the placeholder UID
			if w.Header().Get("Content-Type") != "image/gif" {
				t.Error("Expected GIF response")
			}
		})
	}
}

func TestSetUIDHandler_OptOut(t *testing.T) {
	handler := NewSetUIDHandler([]string{"appnexus"})

	// Create a cookie with opt-out set
	optOutCookie := usersync.NewCookie()
	optOutCookie.SetOptOut(true)
	httpCookie, _ := optOutCookie.ToHTTPCookie("example.com")

	req := httptest.NewRequest("GET", "/setuid?bidder=appnexus&uid=user123", nil)
	req.Host = "example.com"
	req.AddCookie(httpCookie)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Should return pixel without storing UID
	if w.Header().Get("Content-Type") != "image/gif" {
		t.Error("Expected GIF response")
	}
}

func TestSetUIDHandler_UnknownBidder(t *testing.T) {
	handler := NewSetUIDHandler([]string{"appnexus"})

	// Try to set UID for unknown bidder
	req := httptest.NewRequest("GET", "/setuid?bidder=unknown&uid=user123", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should still process (bidder might be dynamically registered)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "image/gif" {
		t.Error("Expected GIF response")
	}
}

func TestSetUIDHandler_GetCookieDomain(t *testing.T) {
	handler := NewSetUIDHandler([]string{})

	testCases := []struct {
		host     string
		expected string
	}{
		{"example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"localhost:3000", "localhost"},
		{"sub.example.com", "sub.example.com"},
	}

	for _, tc := range testCases {
		t.Run(tc.host, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/setuid", nil)
			req.Host = tc.host

			domain := handler.getCookieDomain(req)
			if domain != tc.expected {
				t.Errorf("Expected domain %s, got %s", tc.expected, domain)
			}
		})
	}
}

func TestSetUIDHandler_PixelResponse(t *testing.T) {
	handler := NewSetUIDHandler([]string{"appnexus"})

	req := httptest.NewRequest("GET", "/setuid?bidder=appnexus&uid=user123", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify it's a valid GIF
	body := w.Body.Bytes()
	if len(body) < 10 {
		t.Error("Expected GIF pixel data")
	}

	// Check GIF magic bytes (GIF89a)
	if body[0] != 0x47 || body[1] != 0x49 || body[2] != 0x46 {
		t.Error("Expected GIF magic bytes")
	}

	// Verify headers
	if w.Header().Get("Content-Type") != "image/gif" {
		t.Error("Expected image/gif content type")
	}
	if w.Header().Get("Pragma") != "no-cache" {
		t.Error("Expected Pragma: no-cache")
	}
	if w.Header().Get("Expires") != "0" {
		t.Error("Expected Expires: 0")
	}
}

func TestSetUIDHandler_AddBidder(t *testing.T) {
	handler := NewSetUIDHandler([]string{"appnexus"})

	// Initially unknown bidder
	if handler.validBidders["rubicon"] {
		t.Error("Expected rubicon to be invalid initially")
	}

	// Add bidder
	handler.AddBidder("Rubicon")

	// Now should be valid (lowercase)
	if !handler.validBidders["rubicon"] {
		t.Error("Expected rubicon to be valid after adding")
	}

	// Test with the newly added bidder
	req := httptest.NewRequest("GET", "/setuid?bidder=rubicon&uid=test456", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestSetUIDHandler_CaseInsensitive(t *testing.T) {
	handler := NewSetUIDHandler([]string{"AppNexus"})

	testCases := []string{"appnexus", "AppNexus", "APPNEXUS", "aPpNeXuS"}

	for _, bidder := range testCases {
		t.Run(bidder, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/setuid?bidder="+bidder+"&uid=test123", nil)
			req.Host = "example.com"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200 for bidder %s, got %d", bidder, w.Code)
			}
		})
	}
}

// OptOutHandler tests

func TestNewOptOutHandler(t *testing.T) {
	handler := NewOptOutHandler()

	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
}

func TestOptOutHandler_SetsOptOut(t *testing.T) {
	handler := NewOptOutHandler()

	req := httptest.NewRequest("GET", "/optout", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check HTML response
	if w.Header().Get("Content-Type") != "text/html" {
		t.Errorf("Expected text/html content type, got %s", w.Header().Get("Content-Type"))
	}

	body := w.Body.String()
	if !strings.Contains(body, "You have been opted out") {
		t.Error("Expected opt-out confirmation message")
	}

	// Check cookie was set with opt-out
	result := w.Result()
	defer result.Body.Close()
	cookies := result.Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected cookie to be set")
	}

	cookie := cookies[0]
	if cookie.Name != "uids" {
		t.Errorf("Expected cookie name 'uids', got %s", cookie.Name)
	}

	// Verify opt-out is in the cookie
	decoded, err := base64.URLEncoding.DecodeString(cookie.Value)
	if err != nil {
		t.Fatalf("Failed to decode cookie: %v", err)
	}
	if !strings.Contains(string(decoded), "optout") {
		t.Error("Expected opt-out flag in cookie")
	}
}

func TestOptOutHandler_HTMLContent(t *testing.T) {
	handler := NewOptOutHandler()

	req := httptest.NewRequest("GET", "/optout", nil)
	req.Host = "example.com:8080"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	body := w.Body.String()

	// Verify HTML structure
	expectedStrings := []string{
		"<!DOCTYPE html>",
		"<html>",
		"<title>Opted Out</title>",
		"You have been opted out",
		"personalized ads",
		"opt back in",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(body, expected) {
			t.Errorf("Expected HTML to contain: %s", expected)
		}
	}
}

func TestOptOutHandler_PreservesExistingUIDs(t *testing.T) {
	handler := NewOptOutHandler()

	// Create a cookie with existing UIDs
	cookie := usersync.NewCookie()
	cookie.SetUID("appnexus", "user123")
	cookie.SetUID("rubicon", "user456")
	httpCookie, _ := cookie.ToHTTPCookie("example.com")

	req := httptest.NewRequest("GET", "/optout", nil)
	req.Host = "example.com"
	req.AddCookie(httpCookie)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Cookie should have opt-out set
	result := w.Result()
	defer result.Body.Close()
	cookies := result.Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected cookie to be set")
	}

	resultCookie := cookies[0]
	decoded, _ := base64.URLEncoding.DecodeString(resultCookie.Value)
	if !strings.Contains(string(decoded), "optout") {
		t.Error("Expected opt-out in cookie")
	}
}

func TestOptOutHandler_DomainWithPort(t *testing.T) {
	handler := NewOptOutHandler()

	req := httptest.NewRequest("GET", "/optout", nil)
	req.Host = "example.com:8080"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	result := w.Result()
	defer result.Body.Close()
	cookies := result.Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected cookie to be set")
	}

	cookie := cookies[0]
	// Domain should be without port
	if cookie.Domain != "" && strings.Contains(cookie.Domain, ":") {
		t.Error("Expected domain without port in cookie")
	}
}
