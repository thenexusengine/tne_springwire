//go:build integration
// +build integration

package endpoints

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/thenexusengine/tne_springwire/pkg/redis"
)

// These tests require a running Redis instance
// Run with: go test -tags=integration ./internal/endpoints/...
//
// Start the test Redis with:
//   docker-compose -f docker-compose.test.yml up -d redis
//
// Default connection: localhost:6399

func getTestRedisAddr() string {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6399"
	}
	return addr
}

func TestPublisherAdminHandler_Integration(t *testing.T) {
	redisClient, err := redis.NewClient(redis.Config{
		Addr: getTestRedisAddr(),
	})
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}
	defer redisClient.Close()

	handler := NewPublisherAdminHandler(redisClient)

	// Clean up any existing test data
	defer func() {
		redisClient.HDel("tne_catalyst:publishers", "integration-pub-1", "integration-pub-2")
	}()

	t.Run("CreatePublisher", func(t *testing.T) {
		body := PublisherRequest{
			ID:             "integration-pub-1",
			AllowedDomains: "example.com|*.test.com",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Errorf("Expected status 200/201, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("ListPublishers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/publishers", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		var resp PublisherListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if resp.Count == 0 {
			t.Error("Expected at least one publisher")
		}
	})

	t.Run("GetPublisher", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/publishers/integration-pub-1", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var pub Publisher
		if err := json.Unmarshal(rec.Body.Bytes(), &pub); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if pub.ID != "integration-pub-1" {
			t.Errorf("Expected ID integration-pub-1, got %s", pub.ID)
		}
	})

	t.Run("UpdatePublisher", func(t *testing.T) {
		body := PublisherRequest{
			ID:             "integration-pub-1",
			AllowedDomains: "updated.com|*.new.com",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPut, "/admin/publishers/integration-pub-1", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GetNonExistentPublisher", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/publishers/does-not-exist", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", rec.Code)
		}
	})

	t.Run("DeletePublisher", func(t *testing.T) {
		// First create one to delete
		body := PublisherRequest{
			ID:             "integration-pub-2",
			AllowedDomains: "delete-me.com",
		}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader(bodyBytes))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Now delete it
		req = httptest.NewRequest(http.MethodDelete, "/admin/publishers/integration-pub-2", nil)
		rec = httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
			t.Errorf("Expected status 200/204, got %d", rec.Code)
		}

		// Verify it's gone
		req = httptest.NewRequest(http.MethodGet, "/admin/publishers/integration-pub-2", nil)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 after delete, got %d", rec.Code)
		}
	})

	t.Run("UnsupportedMethod", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/admin/publishers", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d", rec.Code)
		}
	})

	t.Run("CreatePublisherInvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader([]byte(`{invalid`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rec.Code)
		}
	})

	t.Run("CreatePublisherMissingID", func(t *testing.T) {
		body := PublisherRequest{
			AllowedDomains: "example.com",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for missing ID, got %d", rec.Code)
		}
	})
}
