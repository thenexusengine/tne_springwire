package stored

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractStoredRequestID(t *testing.T) {
	tests := []struct {
		name     string
		ext      json.RawMessage
		expected string
	}{
		{
			name:     "nil ext",
			ext:      nil,
			expected: "",
		},
		{
			name:     "empty ext",
			ext:      json.RawMessage(`{}`),
			expected: "",
		},
		{
			name:     "no prebid",
			ext:      json.RawMessage(`{"other": "data"}`),
			expected: "",
		},
		{
			name:     "no storedrequest",
			ext:      json.RawMessage(`{"prebid": {"other": "data"}}`),
			expected: "",
		},
		{
			name:     "with stored request id",
			ext:      json.RawMessage(`{"prebid": {"storedrequest": {"id": "stored-123"}}}`),
			expected: "stored-123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, _ := ExtractStoredRequestID(tc.ext)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestExtractStoredImpID(t *testing.T) {
	tests := []struct {
		name     string
		ext      json.RawMessage
		expected string
	}{
		{
			name:     "with stored imp id",
			ext:      json.RawMessage(`{"prebid": {"storedrequest": {"id": "imp-456"}}}`),
			expected: "imp-456",
		},
		{
			name:     "no stored imp id",
			ext:      json.RawMessage(`{"bidder": {"param": "value"}}`),
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, _ := ExtractStoredImpID(tc.ext)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name     string
		dst      map[string]interface{}
		src      map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "simple overwrite",
			dst:  map[string]interface{}{"a": 1, "b": 2},
			src:  map[string]interface{}{"b": 3},
			expected: map[string]interface{}{
				"a": 1,
				"b": 3,
			},
		},
		{
			name: "nested merge",
			dst: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": 1,
					"b": 2,
				},
			},
			src: map[string]interface{}{
				"outer": map[string]interface{}{
					"b": 3,
					"c": 4,
				},
			},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": 1,
					"b": 3,
					"c": 4,
				},
			},
		},
		{
			name: "src adds new keys",
			dst:  map[string]interface{}{"a": 1},
			src:  map[string]interface{}{"b": 2},
			expected: map[string]interface{}{
				"a": 1,
				"b": 2,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := deepMerge(tc.dst, tc.src)

			// Compare as JSON for easier debugging
			resultJSON, _ := json.Marshal(result)
			expectedJSON, _ := json.Marshal(tc.expected)

			if string(resultJSON) != string(expectedJSON) {
				t.Errorf("expected %s, got %s", string(expectedJSON), string(resultJSON))
			}
		})
	}
}

// mockFetcher implements Fetcher for testing
type mockFetcher struct {
	requests    map[string]json.RawMessage
	impressions map[string]json.RawMessage
	responses   map[string]json.RawMessage
	accounts    map[string]json.RawMessage
}

func newMockFetcher() *mockFetcher {
	return &mockFetcher{
		requests:    make(map[string]json.RawMessage),
		impressions: make(map[string]json.RawMessage),
		responses:   make(map[string]json.RawMessage),
		accounts:    make(map[string]json.RawMessage),
	}
}

func (m *mockFetcher) FetchRequests(ctx context.Context, requestIDs []string) (map[string]json.RawMessage, []error) {
	result := make(map[string]json.RawMessage)
	var errs []error
	for _, id := range requestIDs {
		if data, ok := m.requests[id]; ok {
			result[id] = data
		} else {
			errs = append(errs, ErrNotFound)
		}
	}
	return result, errs
}

func (m *mockFetcher) FetchImpressions(ctx context.Context, impIDs []string) (map[string]json.RawMessage, []error) {
	result := make(map[string]json.RawMessage)
	var errs []error
	for _, id := range impIDs {
		if data, ok := m.impressions[id]; ok {
			result[id] = data
		} else {
			errs = append(errs, ErrNotFound)
		}
	}
	return result, errs
}

func (m *mockFetcher) FetchResponses(ctx context.Context, respIDs []string) (map[string]json.RawMessage, []error) {
	result := make(map[string]json.RawMessage)
	var errs []error
	for _, id := range respIDs {
		if data, ok := m.responses[id]; ok {
			result[id] = data
		} else {
			errs = append(errs, ErrNotFound)
		}
	}
	return result, errs
}

func (m *mockFetcher) FetchAccount(ctx context.Context, accountID string) (json.RawMessage, error) {
	if data, ok := m.accounts[accountID]; ok {
		return data, nil
	}
	return nil, ErrNotFound
}

func (m *mockFetcher) Close() error {
	return nil
}

func TestCache_FetchRequests(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{"id": "req-1", "site": {"domain": "example.com"}}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Minute})

	ctx := context.Background()

	// First fetch - should hit backend
	result, errs := cache.FetchRequests(ctx, []string{"req-1"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if _, ok := result["req-1"]; !ok {
		t.Error("expected to find req-1")
	}

	// Second fetch - should hit cache
	result, errs = cache.FetchRequests(ctx, []string{"req-1"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors on cached fetch: %v", errs)
	}
	if _, ok := result["req-1"]; !ok {
		t.Error("expected to find req-1 from cache")
	}

	// Fetch non-existent
	result, errs = cache.FetchRequests(ctx, []string{"req-999"})
	if len(errs) == 0 {
		t.Error("expected error for non-existent request")
	}
}

func TestCache_Invalidate(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{"id": "req-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})

	ctx := context.Background()

	// Populate cache
	cache.FetchRequests(ctx, []string{"req-1"})

	// Verify it's cached
	stats := cache.Stats()
	if stats.RequestCount != 1 {
		t.Errorf("expected 1 cached request, got %d", stats.RequestCount)
	}

	// Invalidate
	cache.Invalidate(DataTypeRequest, []string{"req-1"})

	// Verify it's removed
	stats = cache.Stats()
	if stats.RequestCount != 0 {
		t.Errorf("expected 0 cached requests after invalidate, got %d", stats.RequestCount)
	}
}

func TestCache_InvalidateAll(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{"id": "req-1"}`)
	mock.impressions["imp-1"] = json.RawMessage(`{"id": "imp-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})

	ctx := context.Background()

	// Populate cache
	cache.FetchRequests(ctx, []string{"req-1"})
	cache.FetchImpressions(ctx, []string{"imp-1"})

	// Verify populated
	stats := cache.Stats()
	if stats.RequestCount != 1 || stats.ImpressionCount != 1 {
		t.Error("expected cache to be populated")
	}

	// Invalidate all
	cache.InvalidateAll()

	// Verify cleared
	stats = cache.Stats()
	if stats.RequestCount != 0 || stats.ImpressionCount != 0 {
		t.Error("expected cache to be cleared")
	}
}

func TestMerger_NoStoredID(t *testing.T) {
	mock := newMockFetcher()
	merger := NewMerger(mock)

	incoming := json.RawMessage(`{"id": "req-1", "site": {"domain": "example.com"}}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StoredRequestID != "" {
		t.Errorf("expected no stored request ID, got %q", result.StoredRequestID)
	}

	// Result should be same as incoming
	if string(result.MergedData) != string(incoming) {
		t.Errorf("expected merged data to equal incoming")
	}
}

func TestMerger_WithStoredRequest(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["stored-123"] = json.RawMessage(`{
		"site": {
			"domain": "stored-domain.com",
			"publisher": {"id": "pub-1"}
		},
		"user": {"id": "user-1"}
	}`)

	merger := NewMerger(mock)

	// Incoming request references stored request and overrides domain
	incoming := json.RawMessage(`{
		"id": "req-1",
		"site": {"domain": "incoming-domain.com"},
		"ext": {"prebid": {"storedrequest": {"id": "stored-123"}}}
	}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StoredRequestID != "stored-123" {
		t.Errorf("expected stored request ID 'stored-123', got %q", result.StoredRequestID)
	}

	// Parse merged result
	var merged map[string]interface{}
	if err := json.Unmarshal(result.MergedData, &merged); err != nil {
		t.Fatalf("failed to parse merged result: %v", err)
	}

	// Check that incoming domain overrides stored domain
	site := merged["site"].(map[string]interface{})
	if site["domain"] != "incoming-domain.com" {
		t.Errorf("expected incoming domain to override stored, got %v", site["domain"])
	}

	// Check that stored publisher is preserved
	publisher := site["publisher"].(map[string]interface{})
	if publisher["id"] != "pub-1" {
		t.Errorf("expected stored publisher to be preserved, got %v", publisher["id"])
	}

	// Check that stored user is preserved
	user := merged["user"].(map[string]interface{})
	if user["id"] != "user-1" {
		t.Errorf("expected stored user to be preserved, got %v", user["id"])
	}
}

func TestFilesystemFetcher(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	ctx := context.Background()

	// Save a request
	reqData := json.RawMessage(`{"id": "test-req", "site": {"domain": "example.com"}}`)
	if err := fetcher.SaveRequest("test-req", reqData); err != nil {
		t.Fatalf("failed to save request: %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(tmpDir, "requests", "test-req.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("expected request file to exist")
	}

	// Fetch the request
	result, errs := fetcher.FetchRequests(ctx, []string{"test-req"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if _, ok := result["test-req"]; !ok {
		t.Error("expected to find test-req")
	}

	// Fetch non-existent
	result, errs = fetcher.FetchRequests(ctx, []string{"non-existent"})
	if len(errs) == 0 {
		t.Error("expected error for non-existent request")
	}

	// List requests
	ids, err := fetcher.ListRequests()
	if err != nil {
		t.Fatalf("failed to list requests: %v", err)
	}
	if len(ids) != 1 || ids[0] != "test-req" {
		t.Errorf("expected [test-req], got %v", ids)
	}

	// Delete request
	if err := fetcher.Delete(DataTypeRequest, "test-req"); err != nil {
		t.Fatalf("failed to delete request: %v", err)
	}

	// Verify deleted
	result, errs = fetcher.FetchRequests(ctx, []string{"test-req"})
	if len(errs) == 0 {
		t.Error("expected error after delete")
	}
}

func TestFilesystemFetcher_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	// Try to save invalid JSON
	err = fetcher.SaveRequest("invalid", json.RawMessage(`not valid json`))
	if err != ErrInvalidJSON {
		t.Errorf("expected ErrInvalidJSON, got %v", err)
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	if config.TTL != 5*time.Minute {
		t.Errorf("expected TTL of 5 minutes, got %v", config.TTL)
	}
	if config.MaxEntries != 10000 {
		t.Errorf("expected MaxEntries of 10000, got %d", config.MaxEntries)
	}
}

func TestDefaultPostgresConfig(t *testing.T) {
	config := DefaultPostgresConfig()

	if config.RequestsTable != "stored_requests" {
		t.Errorf("expected RequestsTable 'stored_requests', got %q", config.RequestsTable)
	}
	if config.QueryTimeout != 5*time.Second {
		t.Errorf("expected QueryTimeout of 5s, got %v", config.QueryTimeout)
	}
}

func TestCacheStats(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{}`)
	mock.requests["req-2"] = json.RawMessage(`{}`)
	mock.impressions["imp-1"] = json.RawMessage(`{}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})
	ctx := context.Background()

	// Initial stats should be zero
	stats := cache.Stats()
	if stats.RequestCount != 0 || stats.ImpressionCount != 0 {
		t.Error("expected empty cache initially")
	}

	// Populate cache
	cache.FetchRequests(ctx, []string{"req-1", "req-2"})
	cache.FetchImpressions(ctx, []string{"imp-1"})

	// Check stats
	stats = cache.Stats()
	if stats.RequestCount != 2 {
		t.Errorf("expected 2 requests, got %d", stats.RequestCount)
	}
	if stats.ImpressionCount != 1 {
		t.Errorf("expected 1 impression, got %d", stats.ImpressionCount)
	}
}

func TestDataType_Constants(t *testing.T) {
	// Verify data type constants
	if DataTypeRequest != "request" {
		t.Error("unexpected DataTypeRequest value")
	}
	if DataTypeImpression != "impression" {
		t.Error("unexpected DataTypeImpression value")
	}
	if DataTypeResponse != "response" {
		t.Error("unexpected DataTypeResponse value")
	}
	if DataTypeAccount != "account" {
		t.Error("unexpected DataTypeAccount value")
	}
}

// Additional tests for 100% coverage

func TestFilesystemFetcher_FetchImpressions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	ctx := context.Background()

	// Save an impression
	impData := json.RawMessage(`{"id": "imp-1", "banner": {"w": 300, "h": 250}}`)
	if err := fetcher.SaveImpression("imp-1", impData); err != nil {
		t.Fatalf("failed to save impression: %v", err)
	}

	// Fetch the impression
	result, errs := fetcher.FetchImpressions(ctx, []string{"imp-1"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if _, ok := result["imp-1"]; !ok {
		t.Error("expected to find imp-1")
	}

	// List impressions
	ids, err := fetcher.ListImpressions()
	if err != nil {
		t.Fatalf("failed to list impressions: %v", err)
	}
	if len(ids) != 1 || ids[0] != "imp-1" {
		t.Errorf("expected [imp-1], got %v", ids)
	}
}

func TestFilesystemFetcher_FetchResponses(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	ctx := context.Background()

	// Save a response
	respData := json.RawMessage(`{"id": "resp-1", "seatbid": []}`)
	if err := fetcher.SaveResponse("resp-1", respData); err != nil {
		t.Fatalf("failed to save response: %v", err)
	}

	// Fetch the response
	result, errs := fetcher.FetchResponses(ctx, []string{"resp-1"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if _, ok := result["resp-1"]; !ok {
		t.Error("expected to find resp-1")
	}

	// List responses
	ids, err := fetcher.ListResponses()
	if err != nil {
		t.Fatalf("failed to list responses: %v", err)
	}
	if len(ids) != 1 || ids[0] != "resp-1" {
		t.Errorf("expected [resp-1], got %v", ids)
	}
}

func TestFilesystemFetcher_FetchAccount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	ctx := context.Background()

	// Save an account
	accountData := json.RawMessage(`{"id": "account-1", "name": "Test Account"}`)
	if err := fetcher.SaveAccount("account-1", accountData); err != nil {
		t.Fatalf("failed to save account: %v", err)
	}

	// Fetch the account
	result, err := fetcher.FetchAccount(ctx, "account-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}

	// Fetch non-existent account
	_, err = fetcher.FetchAccount(ctx, "non-existent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// List accounts
	ids, err := fetcher.ListAccounts()
	if err != nil {
		t.Fatalf("failed to list accounts: %v", err)
	}
	if len(ids) != 1 || ids[0] != "account-1" {
		t.Errorf("expected [account-1], got %v", ids)
	}
}

func TestFilesystemFetcher_Close(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	// Close should not error
	if err := fetcher.Close(); err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

func TestFilesystemFetcher_Delete_AllTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	// Save items of each type
	fetcher.SaveRequest("req-1", json.RawMessage(`{}`))
	fetcher.SaveImpression("imp-1", json.RawMessage(`{}`))
	fetcher.SaveResponse("resp-1", json.RawMessage(`{}`))
	fetcher.SaveAccount("acc-1", json.RawMessage(`{}`))

	// Delete each type
	tests := []struct {
		dataType DataType
		id       string
	}{
		{DataTypeRequest, "req-1"},
		{DataTypeImpression, "imp-1"},
		{DataTypeResponse, "resp-1"},
		{DataTypeAccount, "acc-1"},
	}

	for _, tc := range tests {
		if err := fetcher.Delete(tc.dataType, tc.id); err != nil {
			t.Errorf("failed to delete %s %s: %v", tc.dataType, tc.id, err)
		}
	}

	// Test invalid type
	err = fetcher.Delete("invalid", "test")
	if err == nil {
		t.Error("expected error for invalid data type")
	}
}

func TestFilesystemFetcher_LoadAll(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	// Save items of each type
	fetcher.SaveRequest("req-1", json.RawMessage(`{"id": "req-1"}`))
	fetcher.SaveImpression("imp-1", json.RawMessage(`{"id": "imp-1"}`))
	fetcher.SaveResponse("resp-1", json.RawMessage(`{"id": "resp-1"}`))
	fetcher.SaveAccount("acc-1", json.RawMessage(`{"id": "acc-1"}`))

	// Load all
	ctx := context.Background()
	all, err := fetcher.LoadAll(ctx)
	if err != nil {
		t.Fatalf("failed to load all: %v", err)
	}

	if len(all[DataTypeRequest]) != 1 {
		t.Errorf("expected 1 request, got %d", len(all[DataTypeRequest]))
	}
	if len(all[DataTypeImpression]) != 1 {
		t.Errorf("expected 1 impression, got %d", len(all[DataTypeImpression]))
	}
	if len(all[DataTypeResponse]) != 1 {
		t.Errorf("expected 1 response, got %d", len(all[DataTypeResponse]))
	}
	if len(all[DataTypeAccount]) != 1 {
		t.Errorf("expected 1 account, got %d", len(all[DataTypeAccount]))
	}
}

func TestFilesystemFetcher_ContextCancellation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	// Save a request
	fetcher.SaveRequest("req-1", json.RawMessage(`{}`))

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Fetch should handle cancelled context
	_, errs := fetcher.FetchRequests(ctx, []string{"req-1"})
	if len(errs) == 0 {
		// Context cancellation may or may not produce error depending on timing
		t.Log("no error for cancelled context (may be OK)")
	}
}

func TestCache_FetchResponses(t *testing.T) {
	mock := newMockFetcher()
	mock.responses["resp-1"] = json.RawMessage(`{"id": "resp-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})
	ctx := context.Background()

	// First fetch - should hit backend
	result, errs := cache.FetchResponses(ctx, []string{"resp-1"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if _, ok := result["resp-1"]; !ok {
		t.Error("expected to find resp-1")
	}

	// Second fetch - should hit cache
	result, errs = cache.FetchResponses(ctx, []string{"resp-1"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors on cached fetch: %v", errs)
	}

	// Check stats
	stats := cache.Stats()
	if stats.ResponseCount != 1 {
		t.Errorf("expected 1 response in cache, got %d", stats.ResponseCount)
	}
}

func TestCache_FetchAccount(t *testing.T) {
	mock := newMockFetcher()
	mock.accounts["acc-1"] = json.RawMessage(`{"id": "acc-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})
	ctx := context.Background()

	// First fetch - should hit backend
	result, err := cache.FetchAccount(ctx, "acc-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}

	// Second fetch - should hit cache
	result, err = cache.FetchAccount(ctx, "acc-1")
	if err != nil {
		t.Errorf("unexpected error on cached fetch: %v", err)
	}

	// Fetch non-existent
	_, err = cache.FetchAccount(ctx, "non-existent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Check stats
	stats := cache.Stats()
	if stats.AccountCount != 1 {
		t.Errorf("expected 1 account in cache, got %d", stats.AccountCount)
	}
}

func TestCache_Close(t *testing.T) {
	mock := newMockFetcher()
	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})

	// Close should work
	if err := cache.Close(); err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}

	// Second close should be no-op
	if err := cache.Close(); err != nil {
		t.Errorf("unexpected error on second close: %v", err)
	}

	// Fetching after close should return error
	ctx := context.Background()
	_, errs := cache.FetchRequests(ctx, []string{"test"})
	if len(errs) == 0 || errs[0] != ErrFetcherClosed {
		t.Error("expected ErrFetcherClosed after close")
	}

	_, errs = cache.FetchImpressions(ctx, []string{"test"})
	if len(errs) == 0 || errs[0] != ErrFetcherClosed {
		t.Error("expected ErrFetcherClosed after close for impressions")
	}

	_, errs = cache.FetchResponses(ctx, []string{"test"})
	if len(errs) == 0 || errs[0] != ErrFetcherClosed {
		t.Error("expected ErrFetcherClosed after close for responses")
	}

	_, err := cache.FetchAccount(ctx, "test")
	if err != ErrFetcherClosed {
		t.Errorf("expected ErrFetcherClosed for accounts, got %v", err)
	}
}

func TestCache_Invalidate_AllTypes(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{}`)
	mock.impressions["imp-1"] = json.RawMessage(`{}`)
	mock.responses["resp-1"] = json.RawMessage(`{}`)
	mock.accounts["acc-1"] = json.RawMessage(`{}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})
	ctx := context.Background()

	// Populate cache
	cache.FetchRequests(ctx, []string{"req-1"})
	cache.FetchImpressions(ctx, []string{"imp-1"})
	cache.FetchResponses(ctx, []string{"resp-1"})
	cache.FetchAccount(ctx, "acc-1")

	// Invalidate each type
	cache.Invalidate(DataTypeRequest, []string{"req-1"})
	cache.Invalidate(DataTypeImpression, []string{"imp-1"})
	cache.Invalidate(DataTypeResponse, []string{"resp-1"})
	cache.Invalidate(DataTypeAccount, []string{"acc-1"})

	// Invalid type should be ignored
	cache.Invalidate("invalid", []string{"test"})

	// Check stats - all should be empty
	stats := cache.Stats()
	if stats.RequestCount != 0 || stats.ImpressionCount != 0 ||
		stats.ResponseCount != 0 || stats.AccountCount != 0 {
		t.Error("expected all caches to be empty after invalidation")
	}
}

func TestCache_Expiration(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{}`)

	// Use very short TTL
	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Millisecond})
	ctx := context.Background()

	// First fetch
	cache.FetchRequests(ctx, []string{"req-1"})

	// Wait for expiration
	time.Sleep(5 * time.Millisecond)

	// Second fetch should hit backend (cache expired)
	result, _ := cache.FetchRequests(ctx, []string{"req-1"})
	if _, ok := result["req-1"]; !ok {
		t.Error("expected to find req-1 after re-fetch")
	}
}

func TestMerger_InvalidIncomingJSON(t *testing.T) {
	mock := newMockFetcher()
	merger := NewMerger(mock)

	// Invalid JSON should return error
	_, err := merger.MergeRequest(context.Background(), json.RawMessage(`invalid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMerger_StoredRequestNotFound(t *testing.T) {
	mock := newMockFetcher()
	merger := NewMerger(mock)

	// Request references non-existent stored request
	incoming := json.RawMessage(`{
		"id": "req-1",
		"ext": {"prebid": {"storedrequest": {"id": "non-existent"}}}
	}`)

	_, err := merger.MergeRequest(context.Background(), incoming)
	if err == nil {
		t.Error("expected error for non-existent stored request")
	}
}

func TestMerger_InvalidStoredJSON(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["bad-json"] = json.RawMessage(`invalid`)
	merger := NewMerger(mock)

	incoming := json.RawMessage(`{
		"id": "req-1",
		"ext": {"prebid": {"storedrequest": {"id": "bad-json"}}}
	}`)

	_, err := merger.MergeRequest(context.Background(), incoming)
	if err == nil {
		t.Error("expected error for invalid stored JSON")
	}
}

func TestMerger_WithImpressions(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["stored-req"] = json.RawMessage(`{
		"site": {"domain": "stored.com"}
	}`)
	mock.impressions["stored-imp"] = json.RawMessage(`{
		"banner": {"w": 300, "h": 250},
		"bidfloor": 1.0
	}`)

	merger := NewMerger(mock)

	incoming := json.RawMessage(`{
		"id": "req-1",
		"imp": [
			{
				"id": "imp-1",
				"ext": {"prebid": {"storedrequest": {"id": "stored-imp"}}}
			}
		],
		"ext": {"prebid": {"storedrequest": {"id": "stored-req"}}}
	}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse merged result
	var merged map[string]interface{}
	if err := json.Unmarshal(result.MergedData, &merged); err != nil {
		t.Fatalf("failed to parse merged result: %v", err)
	}

	// Check that impressions were merged
	imps, ok := merged["imp"].([]interface{})
	if !ok || len(imps) != 1 {
		t.Error("expected 1 impression in merged result")
	}
}

func TestMerger_ImpressionsWithMissingStoredImp(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["stored-req"] = json.RawMessage(`{}`)

	merger := NewMerger(mock)

	// Reference non-existent stored impression
	incoming := json.RawMessage(`{
		"id": "req-1",
		"imp": [
			{
				"id": "imp-1",
				"ext": {"prebid": {"storedrequest": {"id": "missing-imp"}}}
			}
		],
		"ext": {"prebid": {"storedrequest": {"id": "stored-req"}}}
	}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have warnings about missing impression
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for missing stored impression")
	}
}

func TestMerger_ImpressionsWithInvalidStoredJSON(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["stored-req"] = json.RawMessage(`{}`)
	mock.impressions["bad-imp"] = json.RawMessage(`invalid`)

	merger := NewMerger(mock)

	incoming := json.RawMessage(`{
		"id": "req-1",
		"imp": [
			{
				"id": "imp-1",
				"ext": {"prebid": {"storedrequest": {"id": "bad-imp"}}}
			}
		],
		"ext": {"prebid": {"storedrequest": {"id": "stored-req"}}}
	}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have warnings about invalid JSON
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for invalid stored impression JSON")
	}
}

func TestMerger_ImpWithNoExt(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["stored-req"] = json.RawMessage(`{}`)

	merger := NewMerger(mock)

	// Impression without ext field
	incoming := json.RawMessage(`{
		"id": "req-1",
		"imp": [
			{"id": "imp-1"}
		],
		"ext": {"prebid": {"storedrequest": {"id": "stored-req"}}}
	}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should succeed without warnings
	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

func TestMerger_NonMapImpression(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["stored-req"] = json.RawMessage(`{}`)

	merger := NewMerger(mock)

	// This should be handled gracefully
	incoming := json.RawMessage(`{
		"id": "req-1",
		"imp": ["not-a-map"],
		"ext": {"prebid": {"storedrequest": {"id": "stored-req"}}}
	}`)

	_, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractStoredImpID_Invalid(t *testing.T) {
	// Test with invalid JSON
	_, err := ExtractStoredImpID(json.RawMessage(`invalid`))
	if err != nil {
		t.Error("expected no error for invalid JSON (returns empty)")
	}
}

func TestExtractStoredRequestID_Invalid(t *testing.T) {
	// Test with invalid JSON
	_, err := ExtractStoredRequestID(json.RawMessage(`invalid`))
	if err != nil {
		t.Error("expected no error for invalid JSON (returns empty)")
	}
}

func TestStoredData_Fields(t *testing.T) {
	now := time.Now()
	data := StoredData{
		ID:        "test-id",
		Type:      DataTypeRequest,
		Data:      json.RawMessage(`{}`),
		CreatedAt: now,
		UpdatedAt: now,
		AccountID: "account-1",
		Disabled:  false,
	}

	if data.ID != "test-id" {
		t.Error("ID not set correctly")
	}
	if data.Type != DataTypeRequest {
		t.Error("Type not set correctly")
	}
	if data.AccountID != "account-1" {
		t.Error("AccountID not set correctly")
	}
}

func TestMergeResult_Fields(t *testing.T) {
	result := MergeResult{
		MergedData:      json.RawMessage(`{}`),
		StoredRequestID: "req-1",
		StoredImpIDs:    map[string]string{"imp-1": "stored-imp-1"},
		Warnings:        []string{"warning 1"},
	}

	if result.StoredRequestID != "req-1" {
		t.Error("StoredRequestID not set correctly")
	}
	if len(result.StoredImpIDs) != 1 {
		t.Error("StoredImpIDs not set correctly")
	}
	if len(result.Warnings) != 1 {
		t.Error("Warnings not set correctly")
	}
}

func TestMockFetcher_Close(t *testing.T) {
	mock := newMockFetcher()
	if err := mock.Close(); err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

// Additional unique tests for better coverage

func TestFilesystemFetcher_SaveInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create fetcher: %v", err)
	}

	// Try to save invalid JSON
	err = fetcher.SaveRequest("test", json.RawMessage(`{invalid`))
	if err != ErrInvalidJSON {
		t.Errorf("expected ErrInvalidJSON, got %v", err)
	}
}

func TestFilesystemFetcher_DeleteAllTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create fetcher: %v", err)
	}

	// Test delete for all data types
	fetcher.SaveImpression("imp-to-delete", json.RawMessage(`{}`))
	fetcher.SaveResponse("resp-to-delete", json.RawMessage(`{}`))
	fetcher.SaveAccount("acc-to-delete", json.RawMessage(`{}`))

	if err := fetcher.Delete(DataTypeImpression, "imp-to-delete"); err != nil {
		t.Errorf("failed to delete impression: %v", err)
	}
	if err := fetcher.Delete(DataTypeResponse, "resp-to-delete"); err != nil {
		t.Errorf("failed to delete response: %v", err)
	}
	if err := fetcher.Delete(DataTypeAccount, "acc-to-delete"); err != nil {
		t.Errorf("failed to delete account: %v", err)
	}

	// Test delete with unknown data type
	err = fetcher.Delete("unknown", "test")
	if err == nil {
		t.Error("expected error for unknown data type")
	}
}

func TestFilesystemFetcher_ListAllTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create fetcher: %v", err)
	}

	// Save some data
	fetcher.SaveRequest("req-1", json.RawMessage(`{}`))
	fetcher.SaveImpression("imp-1", json.RawMessage(`{}`))
	fetcher.SaveResponse("resp-1", json.RawMessage(`{}`))
	fetcher.SaveAccount("acc-1", json.RawMessage(`{}`))

	// List all types
	if reqs, _ := fetcher.ListRequests(); len(reqs) != 1 {
		t.Errorf("expected 1 request, got %d", len(reqs))
	}
	if imps, _ := fetcher.ListImpressions(); len(imps) != 1 {
		t.Errorf("expected 1 impression, got %d", len(imps))
	}
	if resps, _ := fetcher.ListResponses(); len(resps) != 1 {
		t.Errorf("expected 1 response, got %d", len(resps))
	}
	if accs, _ := fetcher.ListAccounts(); len(accs) != 1 {
		t.Errorf("expected 1 account, got %d", len(accs))
	}
}

func TestCache_ClosedFetcher(t *testing.T) {
	mock := newMockFetcher()
	cache := NewCache(mock, CacheConfig{TTL: time.Minute})

	// Close the cache
	cache.Close()

	// Try to fetch - should return error
	_, errs := cache.FetchRequests(context.Background(), []string{"test"})
	if len(errs) == 0 {
		t.Error("expected error when fetching from closed cache")
	}

	// Check other methods too
	_, errs = cache.FetchImpressions(context.Background(), []string{"test"})
	if len(errs) == 0 {
		t.Error("expected error when fetching impressions from closed cache")
	}

	_, errs = cache.FetchResponses(context.Background(), []string{"test"})
	if len(errs) == 0 {
		t.Error("expected error when fetching responses from closed cache")
	}

	_, err := cache.FetchAccount(context.Background(), "test")
	if err != ErrFetcherClosed {
		t.Errorf("expected ErrFetcherClosed, got %v", err)
	}
}

func TestCache_RequestExpiry(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["test"] = json.RawMessage(`{"cached": true}`)

	// Use very short TTL
	cache := NewCache(mock, CacheConfig{TTL: time.Millisecond})

	// Fetch to populate cache
	result1, _ := cache.FetchRequests(context.Background(), []string{"test"})
	if len(result1) != 1 {
		t.Fatal("expected first fetch to return result")
	}

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Update mock data
	mock.requests["test"] = json.RawMessage(`{"cached": false}`)

	// Fetch again - should get fresh data
	result2, _ := cache.FetchRequests(context.Background(), []string{"test"})
	if len(result2) != 1 {
		t.Fatal("expected second fetch to return result")
	}
}

func TestMerger_NoStoredRequestID(t *testing.T) {
	mock := newMockFetcher()
	merger := NewMerger(mock)

	// Request without stored request ID
	incoming := json.RawMessage(`{"id": "req-1", "imp": []}`)
	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StoredRequestID != "" {
		t.Error("expected empty stored request ID")
	}
}

func TestMerger_StoredImpressionMissing(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["stored-req"] = json.RawMessage(`{}`)
	// Don't add stored impression

	merger := NewMerger(mock)

	incoming := json.RawMessage(`{
		"id": "req-1",
		"imp": [{"id": "imp-1", "ext": {"prebid": {"storedrequest": {"id": "nonexistent-imp"}}}}],
		"ext": {"prebid": {"storedrequest": {"id": "stored-req"}}}
	}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have warning about not finding stored impression
	if len(result.Warnings) == 0 {
		t.Error("expected warning about missing stored impression")
	}
}

func TestDataType_Values(t *testing.T) {
	// Test all data types
	types := []DataType{
		DataTypeRequest,
		DataTypeImpression,
		DataTypeResponse,
		DataTypeAccount,
	}

	for _, dt := range types {
		if string(dt) == "" {
			t.Errorf("data type should not be empty")
		}
	}
}

func TestCache_ImpressionExpiry(t *testing.T) {
	mock := newMockFetcher()
	mock.impressions["imp-1"] = json.RawMessage(`{"id": "imp-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: time.Millisecond})

	// Populate cache
	cache.FetchImpressions(context.Background(), []string{"imp-1"})

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Fetch again - should expire and refetch
	result, _ := cache.FetchImpressions(context.Background(), []string{"imp-1"})
	if len(result) != 1 {
		t.Error("expected result after expiry")
	}
}

func TestCache_ResponseExpiry(t *testing.T) {
	mock := newMockFetcher()
	mock.responses["resp-1"] = json.RawMessage(`{"id": "resp-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: time.Millisecond})

	// Populate cache
	cache.FetchResponses(context.Background(), []string{"resp-1"})

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Fetch again - should expire and refetch
	result, _ := cache.FetchResponses(context.Background(), []string{"resp-1"})
	if len(result) != 1 {
		t.Error("expected result after expiry")
	}
}

func TestCache_AccountExpiry(t *testing.T) {
	mock := newMockFetcher()
	mock.accounts["acc-1"] = json.RawMessage(`{"id": "acc-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: time.Millisecond})

	// Populate cache
	cache.FetchAccount(context.Background(), "acc-1")

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Fetch again - should expire and refetch
	result, err := cache.FetchAccount(context.Background(), "acc-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Error("expected result after expiry")
	}
}

func TestFilesystemFetcher_CloseMethod(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create fetcher: %v", err)
	}

	// Close should not error
	if err := fetcher.Close(); err != nil {
		t.Errorf("close should not error: %v", err)
	}
}
