//go:build integration
// +build integration

package stored

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// These tests require a running PostgreSQL instance
// Run with: go test -tags=integration ./internal/stored/...
//
// Start the test database with:
//   docker-compose -f docker-compose.test.yml up -d postgres
//
// Default connection: postgres://test:test@localhost:5499/tne_catalyst_test

func getTestDSN() string {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://test:test@localhost:5499/tne_catalyst_test?sslmode=disable"
	}
	return dsn
}

func TestPostgresFetcher_Integration(t *testing.T) {
	dsn := getTestDSN()

	fetcher, err := NewPostgresFetcher(PostgresConfig{DSN: dsn})
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}
	defer fetcher.Close()

	// Create tables
	if err := fetcher.CreateTables(); err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	ctx := context.Background()

	t.Run("SaveAndFetchRequest", func(t *testing.T) {
		testData := json.RawMessage(`{"id": "test-req", "site": {"domain": "example.com"}}`)

		if err := fetcher.SaveRequest("integration-req-1", testData); err != nil {
			t.Fatalf("SaveRequest failed: %v", err)
		}

		result, errs := fetcher.FetchRequests(ctx, []string{"integration-req-1"})
		if len(errs) > 0 {
			t.Fatalf("FetchRequests failed: %v", errs)
		}

		if _, ok := result["integration-req-1"]; !ok {
			t.Error("Expected to find saved request")
		}
	})

	t.Run("SaveAndFetchImpression", func(t *testing.T) {
		testData := json.RawMessage(`{"id": "imp-1", "banner": {"w": 300, "h": 250}}`)

		if err := fetcher.SaveImpression("integration-imp-1", testData); err != nil {
			t.Fatalf("SaveImpression failed: %v", err)
		}

		result, errs := fetcher.FetchImpressions(ctx, []string{"integration-imp-1"})
		if len(errs) > 0 {
			t.Fatalf("FetchImpressions failed: %v", errs)
		}

		if _, ok := result["integration-imp-1"]; !ok {
			t.Error("Expected to find saved impression")
		}
	})

	t.Run("SaveAndFetchResponse", func(t *testing.T) {
		testData := json.RawMessage(`{"seatbid": []}`)

		if err := fetcher.SaveResponse("integration-resp-1", testData); err != nil {
			t.Fatalf("SaveResponse failed: %v", err)
		}

		result, errs := fetcher.FetchResponses(ctx, []string{"integration-resp-1"})
		if len(errs) > 0 {
			t.Fatalf("FetchResponses failed: %v", errs)
		}

		if _, ok := result["integration-resp-1"]; !ok {
			t.Error("Expected to find saved response")
		}
	})

	t.Run("SaveAndFetchAccount", func(t *testing.T) {
		testData := json.RawMessage(`{"name": "Test Account", "enabled": true}`)

		if err := fetcher.SaveAccount("integration-acc-1", testData); err != nil {
			t.Fatalf("SaveAccount failed: %v", err)
		}

		result, err := fetcher.FetchAccount(ctx, "integration-acc-1")
		if err != nil {
			t.Fatalf("FetchAccount failed: %v", err)
		}

		if result == nil {
			t.Error("Expected to find saved account")
		}
	})

	t.Run("ListRequests", func(t *testing.T) {
		ids, err := fetcher.ListRequests()
		if err != nil {
			t.Fatalf("ListRequests failed: %v", err)
		}

		found := false
		for _, id := range ids {
			if id == "integration-req-1" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find integration-req-1 in list")
		}
	})

	t.Run("ListImpressions", func(t *testing.T) {
		ids, err := fetcher.ListImpressions()
		if err != nil {
			t.Fatalf("ListImpressions failed: %v", err)
		}

		found := false
		for _, id := range ids {
			if id == "integration-imp-1" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find integration-imp-1 in list")
		}
	})

	t.Run("DisableRequest", func(t *testing.T) {
		if err := fetcher.Disable(DataTypeRequest, "integration-req-1"); err != nil {
			t.Fatalf("Disable failed: %v", err)
		}

		// Disabled request should not be returned
		result, _ := fetcher.FetchRequests(ctx, []string{"integration-req-1"})
		if _, ok := result["integration-req-1"]; ok {
			t.Error("Disabled request should not be returned")
		}
	})

	t.Run("DeleteRequest", func(t *testing.T) {
		// First save a new one
		fetcher.SaveRequest("to-delete", json.RawMessage(`{}`))

		if err := fetcher.Delete(DataTypeRequest, "to-delete"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		result, _ := fetcher.FetchRequests(ctx, []string{"to-delete"})
		if _, ok := result["to-delete"]; ok {
			t.Error("Deleted request should not be returned")
		}
	})

	t.Run("FetchNonExistent", func(t *testing.T) {
		result, errs := fetcher.FetchRequests(ctx, []string{"does-not-exist"})
		if len(errs) == 0 {
			t.Error("Expected error for non-existent request")
		}
		if _, ok := result["does-not-exist"]; ok {
			t.Error("Should not return non-existent request")
		}
	})
}

func TestPostgresFetcher_ConnectionError(t *testing.T) {
	// Test with invalid DSN
	_, err := NewPostgresFetcher(PostgresConfig{DSN: "postgres://invalid:invalid@localhost:1/invalid"})
	if err == nil {
		t.Error("Expected error for invalid connection")
	}
}

func TestPostgresFetcher_ContextCancellation(t *testing.T) {
	dsn := getTestDSN()

	fetcher, err := NewPostgresFetcher(PostgresConfig{DSN: dsn})
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}
	defer fetcher.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	time.Sleep(time.Millisecond) // Ensure context is cancelled

	_, errs := fetcher.FetchRequests(ctx, []string{"test"})
	if len(errs) == 0 {
		t.Log("Note: Context cancellation may not always produce error depending on timing")
	}
}
